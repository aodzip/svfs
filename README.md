# The Swift Virtual File System

[![Build Status](https://travis-ci.org/ovh/svfs.svg?branch=master)](https://travis-ci.org/ovh/svfs)
[![GoDoc](https://godoc.org/github.com/ovh/svfs/svfs?status.svg)](https://godoc.org/github.com/ovh/svfs/svfs)

**SVFS** is a Virtual File System over Openstack Swift built upon fuse. It is compatible with [hubiC](https://hubic.com),
[OVH Public Cloud Storage](https://www.ovh.com/fr/cloud/storage/object-storage) and basically every endpoint using a standard Openstack Swift setup. It brings a layer of abstraction over object storage, making it as accessible and convenient as a filesystem, without being intrusive on the way your data is stored.

## Disclaimer
This is not an official project of the Openstack community.

## Installation

Download and install the latest [release](https://github.com/ovh/svfs/releases) packaged for your distribution.

## Usage

You can either use standard mount conventions or use the svfs binary directly.

Using the mount command :
```
mount -t svfs -o username=..,password=..,tenant=..,region=..,container=.. myName /mountpoint
```

Using `/etc/fstab` :
```
myName   /mountpoint   svfs   username=..,password=..,tenant=..,region=..,container=..  0 0
```

Using svfs directly :

```
svfs --os-username=.. --os-password=.. ... myName /mountpoint &
```

## Usage with OVH products

- Usage with OVH Public Cloud Storage is explained [here](docs/PCS.md).
- Usage with hubiC is explained [here](docs/HubiC.md).

## Options

#### Keystone options

* `identity_url`: keystone URL (default is https://auth.cloud.ovh.net/v2.0).
* `username`: your keystone user name.
* `password`: your keystone password.
* `tenant`: your project name.
* `region`: the region where your tenant is.
* `version`: authentication version (0 means auto-discovery which is the default).

In case you already have a token and storage URL (for instance with [hubiC](https://hubic.com)) :
* `storage_url`: the URL to your data.
* `token`: your token.

#### Swift options

* `container`: which container should be selected while mounting the filesystem. If not set,
all containers within the tenant will be available under the chosen mountpoint.
* `segment_size`: large object segments size in MB. When an object has a content larger than
this setting, it will be uploaded in multiple parts of the specified size. Default is 256 MB.
Segment size should not exceed 5 GB.
* `timeout`: connection timeout to the swift storage endpoint. If an operation takes longer
than this timeout and no data has been seen on open sockets, an error is returned. This can
happen when copying non-segmented large files server-side. Default is 5 minutes.

#### Prefetch options

* `readahead_size`: Readahead size in bytes. Default is 128 KB.
* `readdir`: Overall concurrency factor when listing segmented objects in directories (default is 20).

#### Cache options

* `cache_access`: cache entry access count before refresh. Default is -1 (unlimited access).
* `cache_entries`: maximum entry count in cache. Default is -1 (unlimited).
* `cache_ttl`: cache entry timeout before refresh. Default is 1 minute.

#### Ownership options
* `uid` : default files uid (default is 0 i.e. root).
* `gid` : default files gid (default is 0 i.e. root).
* `mode` : default files permissions (default is 0700).

#### Debug options

* `debug`: set it to true to enable debug log.
* `profile_cpu`: Golang CPU profiling information will be stored to this file if set.
* `profile_ram`: Golang RAM profiling information will be stored to this file if set.

## Limitations

**Be aware that SVFS doesn't transform object storage to block storage.**

* SVFS does not support creating, moving or deleting containers.
* SVFS does not support opening a file in append mode.
* SVFS does not support moving directories.
* SVFS does not support SLO (but supports DLO).
* SVFS does not support per-file uid/gid/permissions (but per-mountpoint).

Take a look at the [docs](docs) for further discussions about SVFS approach.

## Hacking

Make sure to use the latest version of go and follow [contribution guidelines](CONTRIBUTING.md) of SVFS.

## License
This work is under the BSD license, see the [LICENSE](LICENSE) file for details.
