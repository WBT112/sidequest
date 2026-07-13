#!/usr/bin/env sh
set -eu

DIST_DIR="${1:-dist}"

fail() {
	printf 'release asset list: %s\n' "$*" >&2
	exit 1
}

[ -d "$DIST_DIR" ] || fail "$DIST_DIR is not a directory"

find "$DIST_DIR" -maxdepth 1 -type f \( \
	-name 'sidequest_*_linux_*.tar.gz' \
	-o -name 'sidequest_*_linux_*.deb' \
	-o -name 'sidequest_*_linux_*.rpm' \
	-o -name 'checksums.txt' \
	-o -name 'install.sh' \
	\) | LC_ALL=C sort
