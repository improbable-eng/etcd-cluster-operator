#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

version="3.0.0"
arch=$(go env GOARCH)
os=$(go env GOOS)

projectdir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )/.."

mkdir -p "$projectdir/bin"

# Download kubebuilder
curl -L -o $projectdir/bin/kubebuilder  https://go.kubebuilder.io/dl/${version}/${os}/${arch}o
chmod +x $projectdir/bin/kubebuilder
