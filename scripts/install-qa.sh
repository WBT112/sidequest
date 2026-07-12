#!/usr/bin/env sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
INSTALLER="$ROOT_DIR/install.sh"

fail() {
	printf 'install QA: %s\n' "$*" >&2
	exit 1
}

need_cmd() {
	command -v "$1" >/dev/null 2>&1 || fail "$1 is required"
}

need_cmd tar
if ! command -v sha256sum >/dev/null 2>&1 && ! command -v shasum >/dev/null 2>&1; then
	fail "sha256sum or shasum is required"
fi

TMPDIR="$(mktemp -d "${TMPDIR:-/tmp}/sidequest-install-qa.XXXXXX")"
trap 'rm -rf "$TMPDIR"' EXIT INT HUP TERM

ASSETS="$TMPDIR/assets"
PAYLOAD="$TMPDIR/payload"
HOME_DIR="$TMPDIR/home"
INSTALL_DIR="$HOME_DIR/.local/bin"
PROFILE="$HOME_DIR/.bashrc"
VERSION="v9.8.7"
VERSION_NO_V="9.8.7"
ASSET="sidequest_${VERSION_NO_V}_linux_amd64.tar.gz"
mkdir -p "$ASSETS" "$PAYLOAD" "$HOME_DIR"

cat >"$PAYLOAD/sidequest" <<'SH'
#!/usr/bin/env sh
case "${1:-}" in
  --version) printf 'sidequest 9.8.7\n' ;;
  --help) printf 'Usage: sidequest\n' ;;
  *) printf 'sidequest test binary\n' ;;
esac
SH
chmod 755 "$PAYLOAD/sidequest"
tar -C "$PAYLOAD" -czf "$ASSETS/$ASSET" sidequest
(
	cd "$ASSETS"
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$ASSET" >checksums.txt
	else
		shasum -a 256 "$ASSET" | awk '{print $1 "  " $2}' >checksums.txt
	fi
)

run_install() {
	HOME="$HOME_DIR" \
	SHELL=/bin/bash \
	SIDEQUEST_VERSION="$VERSION" \
	SIDEQUEST_DOWNLOAD_BASE_URL="$ASSETS" \
	SIDEQUEST_INSTALL_DIR="$INSTALL_DIR" \
	SIDEQUEST_TEST_UNAME_S=Linux \
	SIDEQUEST_TEST_UNAME_M=x86_64 \
	PATH="/usr/bin:/bin" \
	sh "$INSTALLER" "$@"
}

output="$(run_install)"
printf '%s\n' "$output" | grep "Installed sidequest 9.8.7" >/dev/null || fail "install output did not include version"
printf '%s\n' "$output" | grep "Add $INSTALL_DIR to PATH" >/dev/null || fail "install output did not include PATH hint"
printf '%s\n' "$output" | grep "rerun the installer with --update-path" >/dev/null || fail "install output did not mention --update-path"
[ ! -e "$PROFILE" ] || fail "default install modified the shell profile"
[ -x "$INSTALL_DIR/sidequest" ] || fail "installed binary is not executable"
"$INSTALL_DIR/sidequest" --version | grep "sidequest 9.8.7" >/dev/null || fail "installed binary version mismatch"

run_install >/dev/null
[ ! -e "$PROFILE" ] || fail "default reinstall modified the shell profile"

path_output="$(run_install --update-path)"
printf '%s\n' "$path_output" | grep "Added $INSTALL_DIR to PATH in $PROFILE" >/dev/null || fail "PATH update output did not identify the profile"
[ -f "$PROFILE" ] || fail "--update-path did not create the shell profile"
path_line='export PATH="$HOME/.local/bin:$PATH"'
grep -F "$path_line" "$PROFILE" >/dev/null || fail "shell profile is missing PATH line"
[ "$(grep -Fxc "$path_line" "$PROFILE")" -eq 1 ] || fail "shell profile contains duplicate PATH lines"

run_install --update-path >/dev/null
[ "$(grep -Fxc "$path_line" "$PROFILE")" -eq 1 ] || fail "reinstall duplicated the PATH line"

custom_dir="$TMPDIR/custom-bin"
if HOME="$HOME_DIR" SHELL=/bin/bash SIDEQUEST_VERSION="$VERSION" SIDEQUEST_DOWNLOAD_BASE_URL="$ASSETS" SIDEQUEST_INSTALL_DIR="$custom_dir" SIDEQUEST_TEST_UNAME_S=Linux SIDEQUEST_TEST_UNAME_M=amd64 sh "$INSTALLER" --update-path >/dev/null 2>&1; then
	fail "installer accepted --update-path with a custom installation directory"
fi
[ ! -e "$custom_dir/sidequest" ] || fail "installer wrote the binary before rejecting custom PATH setup"

bad_assets="$TMPDIR/bad-assets"
mkdir "$bad_assets"
cp "$ASSETS/checksums.txt" "$bad_assets/checksums.txt"
printf 'broken\n' >"$bad_assets/$ASSET"
if SIDEQUEST_VERSION="$VERSION" SIDEQUEST_DOWNLOAD_BASE_URL="$bad_assets" SIDEQUEST_INSTALL_DIR="$TMPDIR/bad-bin" SIDEQUEST_TEST_UNAME_S=Linux SIDEQUEST_TEST_UNAME_M=amd64 sh "$INSTALLER" >/dev/null 2>&1; then
	fail "installer accepted corrupted archive"
fi

missing_checksum="$TMPDIR/missing-checksum"
mkdir "$missing_checksum"
cp "$ASSETS/$ASSET" "$missing_checksum/$ASSET"
: >"$missing_checksum/checksums.txt"
if SIDEQUEST_VERSION="$VERSION" SIDEQUEST_DOWNLOAD_BASE_URL="$missing_checksum" SIDEQUEST_INSTALL_DIR="$TMPDIR/missing-bin" SIDEQUEST_TEST_UNAME_S=Linux SIDEQUEST_TEST_UNAME_M=amd64 sh "$INSTALLER" >/dev/null 2>&1; then
	fail "installer accepted missing checksum entry"
fi

if SIDEQUEST_VERSION="$VERSION" SIDEQUEST_DOWNLOAD_BASE_URL="$ASSETS" SIDEQUEST_INSTALL_DIR="$TMPDIR/unsupported-bin" SIDEQUEST_TEST_UNAME_S=Linux SIDEQUEST_TEST_UNAME_M=sparc sh "$INSTALLER" >/dev/null 2>&1; then
	fail "installer accepted unsupported architecture"
fi

if SIDEQUEST_VERSION="$VERSION" SIDEQUEST_DOWNLOAD_BASE_URL="$ASSETS" SIDEQUEST_INSTALL_DIR="$TMPDIR/windows-bin" SIDEQUEST_TEST_UNAME_S=MINGW64_NT SIDEQUEST_TEST_UNAME_M=x86_64 sh "$INSTALLER" >/dev/null 2>&1; then
	fail "installer accepted native Windows"
fi

FAKE_BIN="$TMPDIR/fake-bin"
mkdir "$FAKE_BIN"
cat >"$FAKE_BIN/curl" <<'SH'
#!/usr/bin/env sh
set -eu

auth=0
output=
url=
while [ "$#" -gt 0 ]; do
	case "$1" in
		-H)
			shift
			case "${1:-}" in
				Authorization:*) auth=1 ;;
			esac
			;;
		-o)
			shift
			output="${1:-}"
			;;
		-*)
			;;
		*)
			url="$1"
			;;
	esac
	shift
done

printf '%s auth=%s\n' "$url" "$auth" >>"$FAKE_CURL_LOG"
if [ "${FAKE_CURL_MODE:-public}" = require-auth ] && [ "$auth" -eq 0 ]; then
	exit 22
fi

case "$url" in
	*/checksums.txt)
		cp "$FAKE_CURL_ASSETS/checksums.txt" "$output"
		;;
	*)
		cp "$FAKE_CURL_ASSETS/$FAKE_CURL_ASSET" "$output"
		;;
esac
SH
chmod 755 "$FAKE_BIN/curl"

run_http_install() {
	log="$1"
	base_url="$2"
	curl_mode="$3"
	token="$4"
	target_dir="$5"
	HOME="$HOME_DIR" \
	SHELL=/bin/bash \
	SIDEQUEST_VERSION="$VERSION" \
	SIDEQUEST_DOWNLOAD_BASE_URL="$base_url" \
	SIDEQUEST_INSTALL_DIR="$target_dir" \
	SIDEQUEST_TEST_UNAME_S=Linux \
	SIDEQUEST_TEST_UNAME_M=x86_64 \
	GITHUB_TOKEN="$token" \
	FAKE_CURL_ASSETS="$ASSETS" \
	FAKE_CURL_ASSET="$ASSET" \
	FAKE_CURL_LOG="$log" \
	FAKE_CURL_MODE="$curl_mode" \
	PATH="$FAKE_BIN:/usr/bin:/bin" \
	sh "$INSTALLER"
}

github_base="https://github.com/WBT112/sidequest/releases/download/$VERSION"
public_github_log="$TMPDIR/public-github-curl.log"
run_http_install "$public_github_log" "$github_base" public "" "$TMPDIR/public-github-bin" >/dev/null
if grep 'auth=1' "$public_github_log" >/dev/null 2>&1; then
	fail "public GitHub downloads sent an Authorization header"
fi

token_public_github_log="$TMPDIR/token-public-github-curl.log"
run_http_install "$token_public_github_log" "$github_base" public "secret-token" "$TMPDIR/token-public-github-bin" >/dev/null
if grep 'auth=1' "$token_public_github_log" >/dev/null 2>&1; then
	fail "public GitHub downloads used a token before authentication was required"
fi

auth_github_log="$TMPDIR/auth-github-curl.log"
run_http_install "$auth_github_log" "$github_base" require-auth "secret-token" "$TMPDIR/auth-github-bin" >/dev/null
if ! grep 'auth=1' "$auth_github_log" >/dev/null 2>&1; then
	fail "authenticated GitHub fallback did not send an Authorization header"
fi

external_base="https://downloads.example.test/sidequest/$VERSION"
external_log="$TMPDIR/external-curl.log"
run_http_install "$external_log" "$external_base" public "secret-token" "$TMPDIR/external-bin" >/dev/null
if grep 'auth=1' "$external_log" >/dev/null 2>&1; then
	fail "non-GitHub downloads received an Authorization header"
fi

external_auth_log="$TMPDIR/external-auth-curl.log"
if run_http_install "$external_auth_log" "$external_base" require-auth "secret-token" "$TMPDIR/external-auth-bin" >/dev/null 2>&1; then
	fail "installer authenticated to a non-GitHub download host"
fi
if grep 'auth=1' "$external_auth_log" >/dev/null 2>&1; then
	fail "non-GitHub authenticated retry sent an Authorization header"
fi

printf 'install QA passed\n'
