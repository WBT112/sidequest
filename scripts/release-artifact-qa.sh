#!/usr/bin/env sh
set -eu

DIST_DIR="${1:-dist}"

fail() {
	printf 'release artifact QA: %s\n' "$*" >&2
	exit 1
}

asset="$(find "$DIST_DIR" -name 'sidequest_*_linux_amd64.tar.gz' | sort | head -n 1)"
[ "$asset" ] || fail "linux amd64 tarball not found in $DIST_DIR"
[ -f "$DIST_DIR/checksums.txt" ] || fail "checksums.txt not found in $DIST_DIR"
[ -f "$DIST_DIR/install.sh" ] || fail "install.sh not found in $DIST_DIR"

name="$(basename "$asset")"
version="$(printf '%s\n' "$name" | sed 's/^sidequest_//; s/_linux_amd64\.tar\.gz$//')"
[ "$version" ] || fail "could not derive version from $name"

install_dir="$(mktemp -d "${TMPDIR:-/tmp}/sidequest-artifact-qa.XXXXXX")"
trap 'rm -rf "$install_dir"' EXIT INT HUP TERM

SIDEQUEST_VERSION="$version" \
SIDEQUEST_DOWNLOAD_BASE_URL="$(CDPATH= cd -- "$(dirname -- "$asset")" && pwd)" \
SIDEQUEST_INSTALL_DIR="$install_dir/bin" \
SIDEQUEST_TEST_UNAME_S=Linux \
SIDEQUEST_TEST_UNAME_M=x86_64 \
sh "$DIST_DIR/install.sh"

"$install_dir/bin/sidequest" --version | grep "sidequest $version" >/dev/null || fail "installed binary version mismatch"
"$install_dir/bin/sidequest" --help >/dev/null || fail "installed binary help failed"
[ -x "$install_dir/bin/sidequest" ] || fail "installed binary is not executable"

printf 'release artifact QA passed\n'
