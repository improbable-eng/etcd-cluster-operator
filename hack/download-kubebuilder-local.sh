#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

# If you change this version, update the checksums below!
version="2.2.0"
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
    if [ "$checksum" == "5ccb9803d391e819b606b0c702610093619ad08e429ae34401b3e4d448dd2553" ]
    then
        echo "Checksum verified"
    else
        echo "Checksum mismatch!"
        exit 1
    fi
elif [ "$os" == "linux" ] && [ "$arch" == "amd64" ]
then
    if [ "$checksum" == "9ef35a4a4e92408f7606f1dd1e68fe986fa222a88d34e40ecc07b6ffffcc8c12" ]
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
