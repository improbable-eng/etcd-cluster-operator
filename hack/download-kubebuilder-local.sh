#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

version="3.0.0"
arch=$(go env GOARCH)
os=$(go env GOOS)

projectdir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )/.."

mkdir -p "$projectdir/bin"

# Download kubebuilder-assets
tmp_dir=$(mktemp -d XXXXX)
curl -fsL "https://storage.googleapis.com/kubebuilder-tools/kubebuilder-tools-1.19.2-${os}-${arch}.tar.gz" -o ${tmp_dir}/kubebuilder-tools
tar -zvxf ${tmp_dir}/kubebuilder-tools -C $projectdir/bin/

# Download kubebuilder
curl -L -o $projectdir/bin/kubebuilder/bin/kubebuilder  https://go.kubebuilder.io/dl/${version}/${os}/${arch}o
chmod +x $projectdir/bin/kubebuilder/bin/kubebuilder
rm -rf ${tmp_dir}
