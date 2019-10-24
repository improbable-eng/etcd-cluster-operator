#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

projectdir="$( cd "$( dirname "${BASH_SOURCE[0]}" )/.." && pwd )"

cd "${projectdir}"

tmp="$(mktemp --directory --tmpdir verify.sh.XXXXXXXX)"

cleanup() {
    rm -rf "${tmp}"
}
trap "cleanup" EXIT SIGINT

cp -a "${projectdir}/." "${tmp}"
pushd "${tmp}" >/dev/null

"$@"

popd >/dev/null

if ! diff --new-file --text --unified --show-c-function --recursive "${tmp}" "${projectdir}"
then
    echo
    echo "Project '${projectdir}' is out of date."
    echo "Please run '${*}'"
    exit 1
fi
