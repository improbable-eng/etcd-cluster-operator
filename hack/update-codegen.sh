#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS="crd:trivialVersions=true"

projectdir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )/.."

cd "${projectdir}"

controller-gen object:headerFile=./hack/boilerplate.go.txt paths="./..."
