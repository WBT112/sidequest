#!/bin/sh
set -eu

REPO="${SIDEQUEST_REPO:-WBT112/sidequest}"
VERSION="${SIDEQUEST_VERSION:-}"
INSTALL_DIR="${SIDEQUEST_INSTALL_DIR:-${HOME:-}/.local/bin}"
DOWNLOAD_BASE_URL="${SIDEQUEST_DOWNLOAD_BASE_URL:-}"
UPDATE_PATH="${SIDEQUEST_UPDATE_PATH:-0}"
INSTALL_TMP_TARGET=""
PROFILE_TMP_TARGET=""

fail() {
	printf 'sidequest install: %s\n' "$*" >&2
	exit 1
}

usage() {
	cat <<'USAGE'
Usage: install.sh [--update-path]

Options:
  --update-path  Add $HOME/.local/bin to the current user's shell startup file.
  -h, --help     Show this help text.

Environment:
  SIDEQUEST_VERSION       Install a specific release tag, for example v0.1.0.
  SIDEQUEST_INSTALL_DIR   Override the installation directory.
  SIDEQUEST_UPDATE_PATH   Set to 1 as an alternative to --update-path.
USAGE
}

case "$UPDATE_PATH" in
	1|true|yes) UPDATE_PATH=1 ;;
	0|false|no|'') UPDATE_PATH=0 ;;
	*) fail "SIDEQUEST_UPDATE_PATH must be 0 or 1" ;;
esac

while [ "$#" -gt 0 ]; do
	case "$1" in
		--update-path)
			UPDATE_PATH=1
			;;
		-h|--help)
			usage
			exit 0
			;;
		*)
			fail "unknown option: $1"
			;;
	esac
	shift
done

need_cmd() {
	command -v "$1" >/dev/null 2>&1 || fail "$1 is required"
}

cleanup() {
	if [ "$INSTALL_TMP_TARGET" ]; then
		rm -f "$INSTALL_TMP_TARGET"
	fi
	if [ "$PROFILE_TMP_TARGET" ]; then
		rm -f "$PROFILE_TMP_TARGET"
	fi
	rm -rf "$TMPDIR"
}

path_parent() {
	case "$1" in
		*/*)
			parent="${1%/*}"
			[ "$parent" ] || parent="/"
			printf '%s\n' "$parent"
			;;
		*)
			printf '.\n'
			;;
	esac
}

existing_parent() {
	existing_path="$1"
	while [ ! -e "$existing_path" ]; do
		parent="$(path_parent "$existing_path")"
		[ "$parent" != "$existing_path" ] || break
		existing_path="$parent"
	done
	printf '%s\n' "$existing_path"
}

reject_symlink_path() {
	symlink_path="$1"
	while [ "$symlink_path" ] && [ "$symlink_path" != "." ] && [ "$symlink_path" != "/" ]; do
		[ ! -L "$symlink_path" ] || fail "$symlink_path is a symlink; refusing to write through symlinked paths"
		case "$symlink_path" in
			*/*) symlink_path="$(path_parent "$symlink_path")" ;;
			*) break ;;
		esac
	done
	[ "$symlink_path" != "/" ] || [ ! -L / ] || fail "/ is a symlink; refusing to write through symlinked paths"
}

current_uid() {
	id -u
}

path_uid() {
	stat -c '%u' "$1" 2>/dev/null || fail "could not stat $1"
}

path_mode() {
	stat -c '%a' "$1" 2>/dev/null || fail "could not stat $1"
}

validate_owned_by_current_user() {
	owned_path="$1"
	uid="$(current_uid)"
	owner="$(path_uid "$owned_path")"
	[ "$owner" = "$uid" ] || fail "$owned_path is owned by uid $owner, not current uid $uid"
}

mode_digit_is_writable() {
	case "$1" in
		2|3|6|7) return 0 ;;
		*) return 1 ;;
	esac
}

validate_not_shared_writable_dir() {
	mode_path="$1"
	mode="$(path_mode "$mode_path")"
	other_digit="${mode#${mode%?}}"
	without_other="${mode%?}"
	group_digit="${without_other#${without_other%?}}"
	if mode_digit_is_writable "$group_digit" || mode_digit_is_writable "$other_digit"; then
		fail "$mode_path is group- or world-writable; refusing unsafe installation directory"
	fi
}

validate_safe_directory() {
	dir_path="$1"
	reject_symlink_path "$dir_path"
	[ -d "$dir_path" ] || fail "$dir_path is not a directory"
	validate_owned_by_current_user "$dir_path"
	validate_not_shared_writable_dir "$dir_path"
}

uname_s() {
	if [ "${SIDEQUEST_TEST_UNAME_S:-}" ]; then
		printf '%s\n' "$SIDEQUEST_TEST_UNAME_S"
	else
		uname -s
	fi
}

uname_m() {
	if [ "${SIDEQUEST_TEST_UNAME_M:-}" ]; then
		printf '%s\n' "$SIDEQUEST_TEST_UNAME_M"
	else
		uname -m
	fi
}

detect_os() {
	case "$(uname_s)" in
		Linux) printf 'linux\n' ;;
		Darwin) fail "native macOS is not supported yet; use a Linux release or follow the tracked macOS support work" ;;
		MINGW*|MSYS*|CYGWIN*|Windows_NT) fail "native Windows shells are not supported; install Sidequest inside WSL 2" ;;
		*) fail "unsupported operating system: $(uname_s)" ;;
	esac
}

detect_arch() {
	case "$(uname_m)" in
		x86_64|amd64) printf 'amd64\n' ;;
		aarch64|arm64) printf 'arm64\n' ;;
		*) fail "unsupported architecture: $(uname_m)" ;;
	esac
}

latest_version() {
	need_cmd curl
	api_url="https://api.github.com/repos/${REPO}/releases/latest"
	tmp_json="$TMPDIR/latest.json"
	token="${GITHUB_TOKEN:-${GH_TOKEN:-}}"
	if ! curl -fsSL "$api_url" -o "$tmp_json"; then
		if [ "$token" ]; then
			curl -fsSL -H "Authorization: Bearer $token" "$api_url" -o "$tmp_json" || fail "could not resolve latest release"
		else
			fail "could not resolve latest public release; set SIDEQUEST_VERSION to select one explicitly"
		fi
	fi
	sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$tmp_json" | head -n 1
}

github_auth_allowed() {
	case "$1" in
		https://api.github.com/repos/"$REPO"/*|https://github.com/"$REPO"/*)
			return 0
			;;
		*)
			return 1
			;;
	esac
}

download_http() {
	source_url="$1"
	destination="$2"
	need_cmd curl
	if curl -fsSL "$source_url" -o "$destination"; then
		return 0
	fi
	token="${GITHUB_TOKEN:-${GH_TOKEN:-}}"
	if [ "$token" ] && github_auth_allowed "$source_url"; then
		curl -fsSL -H "Authorization: Bearer $token" "$source_url" -o "$destination" || fail "could not download $source_url"
		return 0
	fi
	fail "could not download $source_url"
}

download() {
	source_url="$1"
	destination="$2"
	case "$source_url" in
		file://*)
			cp "${source_url#file://}" "$destination"
			;;
		/*|./*|../*)
			cp "$source_url" "$destination"
			;;
		http://*|https://*)
			download_http "$source_url" "$destination"
			;;
		*)
			fail "unsupported download URL: $source_url"
			;;
	esac
}

checksum_file_contains() {
	checksums="$1"
	asset="$2"
	grep -F "  ${asset}" "$checksums" >/dev/null 2>&1 || grep -F " *${asset}" "$checksums" >/dev/null 2>&1
}

verify_checksum() {
	checksums="$1"
	asset="$2"
	need_cmd tar
	checksum_file_contains "$checksums" "$asset" || fail "checksums.txt does not contain $asset"
	if command -v sha256sum >/dev/null 2>&1; then
		(cd "$TMPDIR" && sha256sum -c --ignore-missing checksums.txt) >/dev/null || fail "checksum verification failed"
	elif command -v shasum >/dev/null 2>&1; then
		expected="$(awk -v name="$asset" '$2 == name || $2 == "*" name { print $1; exit }' "$checksums")"
		actual="$(shasum -a 256 "$TMPDIR/$asset" | awk '{print $1}')"
		[ "$expected" = "$actual" ] || fail "checksum verification failed"
	else
		fail "sha256sum or shasum is required to verify downloads"
	fi
}

path_contains_dir() {
	case ":${PATH:-}:" in
		*:"$1":*) return 0 ;;
		*) return 1 ;;
	esac
}

shell_name() {
	shell_path="${SHELL:-}"
	printf '%s\n' "${shell_path##*/}"
}

shell_profile() {
	case "$(shell_name)" in
		bash) printf '%s\n' "$HOME/.bashrc" ;;
		zsh) printf '%s\n' "${ZDOTDIR:-$HOME}/.zshrc" ;;
		fish) printf '%s\n' "$HOME/.config/fish/config.fish" ;;
		*) printf '%s\n' "$HOME/.profile" ;;
	esac
}

validate_path_update() {
	[ -n "${HOME:-}" ] || fail "--update-path requires HOME to be set"
	default_install_dir="$HOME/.local/bin"
	[ "$INSTALL_DIR" = "$default_install_dir" ] || fail "--update-path is supported only for the default $default_install_dir installation directory"
}

configure_path() {
	default_install_dir="$HOME/.local/bin"
	profile="$(shell_profile)"
	profile_dir="${profile%/*}"
	reject_symlink_path "$profile_dir"
	validate_safe_directory "$(existing_parent "$profile_dir")"
	mkdir -p "$profile_dir"
	validate_safe_directory "$profile_dir"
	[ ! -L "$profile" ] || fail "$profile is a symlink; refusing to update it"
	if [ -e "$profile" ] && [ ! -f "$profile" ]; then
		fail "$profile is not a regular file"
	fi
	if [ -e "$profile" ]; then
		validate_owned_by_current_user "$profile"
	else
		PROFILE_TMP_TARGET="$(mktemp "$profile_dir/.sidequest-profile.XXXXXX")" || fail "could not create temporary shell profile"
		mv "$PROFILE_TMP_TARGET" "$profile"
		PROFILE_TMP_TARGET=""
	fi

	case "$(shell_name)" in
		fish)
			path_line='fish_add_path "$HOME/.local/bin"'
			reload_command="source \"$profile\""
			;;
		*)
			path_line='export PATH="$HOME/.local/bin:$PATH"'
			reload_command=". \"$profile\""
			;;
	esac

	if grep -F "$path_line" "$profile" >/dev/null 2>&1; then
		printf '%s is already configured in %s\n' "$default_install_dir" "$profile"
	else
		PROFILE_TMP_TARGET="$(mktemp "$profile_dir/.sidequest-profile.XXXXXX")" || fail "could not create temporary shell profile"
		cat "$profile" >"$PROFILE_TMP_TARGET"
		printf '\n# Added by Sidequest installer\n%s\n' "$path_line" >>"$PROFILE_TMP_TARGET"
		chmod "$(path_mode "$profile")" "$PROFILE_TMP_TARGET"
		mv "$PROFILE_TMP_TARGET" "$profile"
		PROFILE_TMP_TARGET=""
		printf 'Added %s to PATH in %s\n' "$default_install_dir" "$profile"
	fi
	printf 'Open a new terminal or reload the file with:\n  %s\n' "$reload_command"
}

install_binary() {
	extracted="$1"
	[ -f "$extracted/sidequest" ] || fail "archive did not contain sidequest binary"
	reject_symlink_path "$INSTALL_DIR"
	validate_safe_directory "$(existing_parent "$INSTALL_DIR")"
	mkdir -p "$INSTALL_DIR"
	validate_safe_directory "$INSTALL_DIR"
	target="$INSTALL_DIR/sidequest"
	[ ! -L "$target" ] || fail "$target is a symlink; refusing to replace it"
	if [ -e "$target" ] && [ ! -f "$target" ]; then
		fail "$target is not a regular file"
	fi
	chmod 755 "$extracted/sidequest"
	INSTALL_TMP_TARGET="$(mktemp "$INSTALL_DIR/.sidequest.XXXXXX")" || fail "could not create temporary binary in $INSTALL_DIR"
	cp "$extracted/sidequest" "$INSTALL_TMP_TARGET"
	chmod 755 "$INSTALL_TMP_TARGET"
	mv "$INSTALL_TMP_TARGET" "$target"
	INSTALL_TMP_TARGET=""
}

TMPDIR="$(mktemp -d "${TMPDIR:-/tmp}/sidequest-install.XXXXXX")"
trap cleanup EXIT INT HUP TERM

OS="$(detect_os)"
ARCH="$(detect_arch)"

if [ -z "$INSTALL_DIR" ]; then
	fail "SIDEQUEST_INSTALL_DIR is empty and HOME is not set"
fi
if [ "$UPDATE_PATH" -eq 1 ]; then
	validate_path_update
fi

if [ -z "$VERSION" ]; then
	VERSION="$(latest_version)"
	[ "$VERSION" ] || fail "latest release did not include a tag"
fi

VERSION_NO_V="${VERSION#v}"
ASSET="sidequest_${VERSION_NO_V}_${OS}_${ARCH}.tar.gz"

if [ "$DOWNLOAD_BASE_URL" ]; then
	BASE="${DOWNLOAD_BASE_URL%/}"
else
	BASE="https://github.com/${REPO}/releases/download/${VERSION}"
fi

download "$BASE/$ASSET" "$TMPDIR/$ASSET"
download "$BASE/checksums.txt" "$TMPDIR/checksums.txt"
verify_checksum "$TMPDIR/checksums.txt" "$ASSET"

mkdir "$TMPDIR/extract"
tar -xzf "$TMPDIR/$ASSET" -C "$TMPDIR/extract"
install_binary "$TMPDIR/extract"

installed_version="$("$INSTALL_DIR/sidequest" --version 2>/dev/null || true)"
printf 'Installed %s to %s/sidequest\n' "${installed_version:-sidequest}" "$INSTALL_DIR"
if path_contains_dir "$INSTALL_DIR"; then
	printf '%s is already on PATH\n' "$INSTALL_DIR"
elif [ "$UPDATE_PATH" -eq 1 ]; then
	configure_path
else
	printf 'Add %s to PATH, for example:\n' "$INSTALL_DIR"
	printf '  export PATH="%s:$PATH"\n' "$INSTALL_DIR"
	printf 'Or rerun the installer with --update-path.\n'
fi
