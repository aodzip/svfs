package cmd

import (
	"fmt"
	"net/http"
	_ "net/http/pprof" // profiling server
	"os"
	"os/user"
	"runtime/pprof"
	"strconv"
	"time"

	fuse "bazil.org/fuse"
	fusefs "bazil.org/fuse/fs"
	"github.com/Sirupsen/logrus"
	"github.com/fatih/color"
	"github.com/ovh/svfs/config"
	"github.com/ovh/svfs/svfs"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/xlucas/swift"
)

var (
	configError error
	debug       bool
	gid         uint64
	uid         uint64
	fs          svfs.SVFS
	srv         *fusefs.Server
	profAddr    string
	cpuProf     string
	memProf     string
	cfgFile     string
	device      string
	mountpoint  string
)

func init() {
	configError = config.LoadConfig()

	// Logger
	formatter := new(logrus.TextFormatter)
	formatter.TimestampFormat = time.RFC3339
	formatter.FullTimestamp = true
	logrus.SetFormatter(formatter)
	logrus.SetOutput(os.Stdout)

	// Uid & Gid
	currentUser, err := user.Current()
	if err != nil {
		gid = 0
		uid = 0
	} else {
		// Non-parsable uid & gid should never be seen
		gid, _ = strconv.ParseUint(currentUser.Gid, 10, 64)
		uid, _ = strconv.ParseUint(currentUser.Uid, 10, 64)
	}

	setFlags()
	RootCmd.AddCommand(mountCmd)
}

// mountCmd represents the base command when called without any subcommands
var mountCmd = &cobra.Command{
	Use:   "mount --device name --mountpoint path",
	Short: "Mount object storage as a device",
	Long: "Mount object storage either from HubiC or a vanilla Swift access\n" +
		"as a device at the given mountpoint.",
	Run: func(cmd *cobra.Command, args []string) {

		// Debug
		if debug {
			setDebug()
		}

		// Mount time
		svfs.MountTime = time.Now()

		// Config validation
		if configError != nil {
			yellow := color.New(color.FgYellow).SprintFunc()
			cyan := color.New(color.FgCyan).SprintFunc()
			logrus.WithField("source", yellow("svfs")).Debugln(cyan("Skipping configuration : ", configError))
		}

		//Mandatory flags
		cmd.MarkPersistentFlagRequired("device")
		cmd.MarkPersistentFlagRequired("mountpoint")

		// Use config file or ENV var if set
		useConfiguration()

		// Live profiling
		if profAddr != "" {
			go func() {
				if err := http.ListenAndServe(profAddr, nil); err != nil {
					logrus.Fatal(err)
				}
			}()
		}

		// CPU profiling
		if cpuProf != "" {
			createCPUProf(cpuProf)
			defer pprof.StopCPUProfile()
		}

		// Check segment size
		if err := checkOptions(); err != nil {
			logrus.Fatal(err)
		}

		// Mount SVFS
		c, err := fuse.Mount(mountpoint, mountOptions(device)...)
		if err != nil {
			goto Err
		}

		defer c.Close()

		// Initialize SVFS
		if err = fs.Init(); err != nil {
			goto Err
		}

		// Serve SVFS
		srv = fusefs.New(c, nil)
		if err = srv.Serve(&fs); err != nil {
			goto Err
		}

		// Check for mount errors
		<-c.Ready

		// Memory profiling
		if memProf != "" {
			createMemProf(memProf)
		}

		if err = c.MountError; err != nil {
			goto Err
		}

		return

	Err:
		fuse.Unmount(mountpoint)
		logrus.Fatal(err)
	},
}

func setFlags() {
	flags := mountCmd.PersistentFlags()

	//Swift options
	flags.StringVar(&svfs.SwiftConnection.AuthUrl, "os-auth-url", "https://auth.cloud.ovh.net/v2.0", "Authentification URL")
	flags.StringVar(&svfs.TargetContainer, "os-container-name", "", "Container name")
	flags.StringVar(&svfs.SwiftConnection.AuthToken, "os-auth-token", "", "Authentification token")
	flags.StringVar(&svfs.SwiftConnection.UserName, "os-username", "", "Username")
	flags.StringVar(&svfs.SwiftConnection.ApiKey, "os-password", "", "User password")
	flags.StringVar(&svfs.SwiftConnection.Domain, "os-domain-name", "", "Domain name")
	flags.StringVar(&svfs.SwiftConnection.Region, "os-region-name", "", "Region name")
	flags.StringVar(&svfs.SwiftConnection.StorageUrl, "os-storage-url", "", "Storage URL")
	flags.BoolVar(&svfs.SwiftConnection.Internal, "os-internal-endpoint", false, "Use internal storage URL")
	flags.StringVar(&svfs.SwiftConnection.Tenant, "os-tenant-name", "", "Tenant name")
	flags.IntVar(&svfs.SwiftConnection.AuthVersion, "os-auth-version", 0, "Authentification version, 0 = auto")
	flags.DurationVar(&svfs.SwiftConnection.ConnectTimeout, "os-connect-timeout", 15*time.Second, "Swift connection timeout")
	flags.DurationVar(&svfs.SwiftConnection.Timeout, "os-request-timeout", 5*time.Minute, "Swift operation timeout")
	flags.Uint64Var(&svfs.SegmentSize, "os-segment-size", 256, "Swift segment size in MiB")
	flags.StringVar(&svfs.StoragePolicy, "os-storage-policy", "", "Only show containers using this storage policy")
	flags.StringVar(&swift.DefaultUserAgent, "user-agent", "svfs/"+svfs.Version, "Default User-Agent")
	flags.StringVar(&swift.ClientIP, "client-ip", "", "Client IP")

	//HubiC options
	flags.StringVar(&svfs.HubicAuthorization, "hubic-authorization", "", "hubiC authorization code")
	flags.StringVar(&svfs.HubicRefreshToken, "hubic-refresh-token", "", "hubiC refresh token")
	flags.BoolVar(&svfs.HubicTimes, "hubic-times", false, "Use file times set by hubiC synchronization clients")

	// Permissions
	flags.Uint64Var(&svfs.DefaultUID, "default-uid", uid, "Default UID")
	flags.Uint64Var(&svfs.DefaultGID, "default-gid", gid, "Default GID")
	flags.Uint64Var(&svfs.DefaultMode, "default-mode", 0700, "Default permissions")
	flags.BoolVar(&svfs.AllowRoot, "allow-root", false, "Fuse allow-root option")
	flags.BoolVar(&svfs.AllowOther, "allow-other", true, "Fuse allow_other option")
	flags.BoolVar(&svfs.DefaultPermissions, "default-permissions", true, "Fuse default_permissions option")
	flags.BoolVar(&svfs.ReadOnly, "read-only", false, "Read only access")

	// Prefetch
	flags.Uint64Var(&svfs.ListerConcurrency, "readdir-concurrency", 20, "Directory listing concurrency")
	flags.BoolVar(&svfs.Attr, "readdir-base-attributes", false, "Fetch base attributes")
	flags.BoolVar(&svfs.Xattr, "readdir-extended-attributes", false, "Fetch extended attributes")
	flags.UintVar(&svfs.BlockSize, "block-size", 4096, "Block size in bytes")
	flags.UintVar(&svfs.ReadAheadSize, "readahead-size", 128, "Per file readhead size in KiB")
	flags.IntVar(&svfs.TransferMode, "transfer-mode", 0, "Transfer optimizations mode")

	// Cache Options
	flags.DurationVar(&svfs.CacheTimeout, "cache-ttl", 1*time.Minute, "Cache timeout")
	flags.Int64Var(&svfs.CacheMaxEntries, "cache-max-entries", -1, "Maximum overall entries allowed in cache")
	flags.Int64Var(&svfs.CacheMaxAccess, "cache-max-access", -1, "Maximum access count to cached entries")

	// Debug and profiling
	flags.BoolVar(&debug, "debug", false, "Enable fuse debug log")
	flags.StringVar(&profAddr, "profile-bind", "", "Profiling information will be served at this address")
	flags.StringVar(&cpuProf, "profile-cpu", "", "Write cpu profile to this file")
	flags.StringVar(&memProf, "profile-ram", "", "Write memory profile to this file")

	// Mandatory flags
	flags.StringVar(&device, "device", "", "Device name")
	flags.StringVar(&mountpoint, "mountpoint", "", "Mountpoint")

	mountCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")

	// Bind cobra flags to viper flags
	viper.BindPFlag("os_auth_url", mountCmd.PersistentFlags().Lookup("os-auth-url"))
	viper.BindPFlag("os_username", mountCmd.PersistentFlags().Lookup("os-username"))
	viper.BindPFlag("os_password", mountCmd.PersistentFlags().Lookup("os-password"))
	viper.BindPFlag("os_tenant_name", mountCmd.PersistentFlags().Lookup("os-tenant-name"))
	viper.BindPFlag("os_domain_name", mountCmd.PersistentFlags().Lookup("os-domain-name"))
	viper.BindPFlag("os_region_name", mountCmd.PersistentFlags().Lookup("os-region-name"))
	viper.BindPFlag("os_auth_token", mountCmd.PersistentFlags().Lookup("os-auth-token"))
	viper.BindPFlag("os_storage_url", mountCmd.PersistentFlags().Lookup("os-storage-url"))
	viper.BindPFlag("hubic_auth", mountCmd.PersistentFlags().Lookup("hubic-authorization"))
	viper.BindPFlag("hubic_token", mountCmd.PersistentFlags().Lookup("hubic-refresh-token"))
}

func mountOptions(device string) (options []fuse.MountOption) {
	if svfs.AllowOther {
		options = append(options, fuse.AllowOther())
	}
	if svfs.AllowRoot {
		options = append(options, fuse.AllowRoot())
	}
	if svfs.DefaultPermissions {
		options = append(options, fuse.DefaultPermissions())
	}
	if svfs.ReadOnly {
		options = append(options, fuse.ReadOnly())
	}

	options = append(options, fuse.MaxReadahead(uint32(svfs.ReadAheadSize)))
	options = append(options, fuse.Subtype("svfs"))
	options = append(options, fuse.FSName(device))

	return options
}

func checkOptions() error {
	// Convert to MB
	svfs.SegmentSize *= (1 << 20)
	svfs.ReadAheadSize *= (1 << 10)

	// Should not exceed swift maximum object size.
	if svfs.SegmentSize > 5*(1<<30) {
		return fmt.Errorf("Segment size can't exceed 5 GiB")
	}
	return nil
}

func setDebug() {
	logrus.SetLevel(logrus.DebugLevel)
	yellow := color.New(color.FgYellow).SprintFunc()
	blue := color.New(color.FgBlue).SprintFunc()
	fuse.Debug = func(msg interface{}) {
		logrus.WithField("source", yellow("fuse")).Debugln(blue(msg))
	}
}

func createCPUProf(cpuProf string) {
	f, err := os.Create(cpuProf)
	if err != nil {
		logrus.Fatal(err)
	}
	pprof.StartCPUProfile(f)
}

func createMemProf(memProf string) {
	f, err := os.Create(memProf)
	if err != nil {
		logrus.Fatal(err)
	}
	pprof.WriteHeapProfile(f)

	f.Close()
}

func useConfiguration() {
	svfs.HubicAuthorization = viper.GetString("hubic_auth")
	svfs.HubicRefreshToken = viper.GetString("hubic_token")

	svfs.SwiftConnection.AuthToken = viper.GetString("os_auth_token")
	svfs.SwiftConnection.StorageUrl = viper.GetString("os_storage_url")

	svfs.SwiftConnection.AuthUrl = viper.GetString("os_auth_url")
	svfs.SwiftConnection.Tenant = viper.GetString("os_tenant_name")
	svfs.SwiftConnection.UserName = viper.GetString("os_username")
	svfs.SwiftConnection.ApiKey = viper.GetString("os_password")
	svfs.SwiftConnection.Domain = viper.GetString("os_domain_name")
	svfs.SwiftConnection.Region = viper.GetString("os_region_name")
}
