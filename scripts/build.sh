#!/bin/bash
set -e

CDIR=$(cd `dirname "$0"`/.. && pwd)
cd "$CDIR"

ORG_PATH="github.com/ovh"
REPO_PATH="${ORG_PATH}/svfs"

export GOPATH="${CDIR}/gopath"

export PATH="${PATH}:${GOPATH}/bin"

eval $(go env)

if [ ! -h gopath/src/${REPO_PATH} ]; then
    mkdir -p gopath/src/${ORG_PATH}
    ln -s ../../../.. gopath/src/${REPO_PATH} || exit 255
fi

if [ -z "$1" ]; then
    OS_PLATFORM_ARG=(-os="darwin linux")
else
    OS_PLATFORM_ARG=($1)
fi

if [ -z "$2" ]; then
    OS_ARCH_ARG=(-arch="386 amd64 arm ppc64le")
else
    OS_ARCH_ARG=($2)
fi

if ! which gox > /dev/null ; then
    go get github.com/mitchellh/gox
fi

cd "$GOPATH/src/${REPO_PATH}"
gox "${OS_PLATFORM_ARG[@]}" "${OS_ARCH_ARG[@]}" -output="dist/{{.OS}}/{{.Arch}}/{{.Dir}}" -ldflags="-w" ${REPO_PATH}