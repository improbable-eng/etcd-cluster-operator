#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

projectdir="$( cd "$( dirname "${BASH_SOURCE[0]}" )/.." && pwd )"

cd "${projectdir}"

# Use short form arguments here to support BSD/macOS. `-d` instructs
# it to make a directory, `-t` provides a prefix to use for the directory name.
tmp="$(mktemp -d /tmp/verify.sh.XXXXXXXX)"

cleanup() {
    rm -rf "${tmp}"
}
trap "cleanup" EXIT SIGINT

cp -a "${projectdir}/." "${tmp}"
pushd "${tmp}" >/dev/null

"$@"

popd >/dev/null

if ! diff --new-file --exclude=bin --unified --show-c-function --recursive "${projectdir}" "${tmp}"
then
    echo
    echo "Project '${projectdir}' is out of date."
    echo "Please run '${*}'"
    exit 1
fi
