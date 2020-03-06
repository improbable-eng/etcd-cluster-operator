#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

# If you change this version, update the checksums below!
version="2.3.0"
arch=$(go env GOARCH)
os=$(go env GOOS)

projectdir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )/.."

# Download Kubebuilder archive
wget -O /tmp/kubebuilder_${version}_${os}_${arch}.tar.gz \
     https://github.com/kubernetes-sigs/kubebuilder/releases/download/v${version}/kubebuilder_${version}_${os}_${arch}.tar.gz

# Generate SHA256 Checksum
if [ "$os" == "darwin" ]
then
    checksum=$(shasum -a 256 /tmp/kubebuilder_${version}_${os}_${arch}.tar.gz | awk '{print $1;}')
elif [ "$os" == "linux" ]
then
    checksum=$(sha256sum /tmp/kubebuilder_${version}_${os}_${arch}.tar.gz | awk '{print $1;}')
else
    echo "Unable to verify checksum, no SHA256 checksum generator on this platform"
    exit 1
fi

# Verify Checksum
if [ "$os" == "darwin" ] && [ "$arch" == "amd64" ]
then
    if [ "$checksum" == "b44f6c7ba1edf0046354ff29abffee3410d156fabae6eb3fa62da806988aa8bd" ]
    then
        echo "Checksum verified"
    else
        echo "Checksum mismatch!"
        exit 1
    fi
elif [ "$os" == "linux" ] && [ "$arch" == "amd64" ]
then
    if [ "$checksum" == "a8ffea619f8d6e6c9fab2df8543cf0912420568e3979f44340a7613de5944141" ]
    then
        echo "Checksum verified"
    else
        echo "Checksum mismatch!"
        exit 1
    fi
else
    echo "Unable to verify kubebuilder, I don't know the checksum for kubebuilder_${version}_${os}_${arch}.tar.gz"
    echo "Please install kubebuilder to bin/kubebuilder yourself."
    exit 1
fi

# Extract
tmpdir=$(mktemp -d)
tar -C "$tmpdir" -zxvf /tmp/kubebuilder_${version}_${os}_${arch}.tar.gz

mkdir -p "$projectdir/bin"
mv "$tmpdir/kubebuilder_${version}_${os}_${arch}" "$projectdir/bin/kubebuilder"

# cleanup
rm /tmp/kubebuilder_${version}_${os}_${arch}.tar.gz
rm -r "$tmpdir"
