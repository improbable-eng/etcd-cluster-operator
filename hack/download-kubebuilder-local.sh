#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

# If you change this version, update the checksums below!
version="2.0.1"
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
    if [ "$checksum" == "a2cd518da553584aee2e8a74818da1521f5dd4a9a4a97c8e18b2634e8a8266ca" ]
    then
        echo "Checksum verified"
    else
        echo "Checksum mismatch!"
        exit 1
    fi
elif [ "$os" == "linux" ] && [ "$arch" == "amd64" ]
then
    if [ "$checksum" == "e8d287535c79013bfebcee22f748153686128926ae5992f023725e7b17996a04" ]
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
mkdir -p "$projectdir/bin"
tar -C /tmp -zxvf /tmp/kubebuilder_${version}_${os}_${arch}.tar.gz
mv /tmp/kubebuilder_${version}_${os}_${arch} "$projectdir/bin/kubebuilder"

# cleanup
rm /tmp/kubebuilder_${version}_${os}_${arch}.tar.gz
