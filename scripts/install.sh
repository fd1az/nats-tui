#!/bin/sh
set -eu

repository=${NATS_TUI_REPO:-fd1az/nats-tui}
version=${NATS_TUI_VERSION:-}
install_dir=${NATS_TUI_INSTALL_DIR:-"${HOME}/.local/bin"}
release_url=${NATS_TUI_RELEASE_URL:-"https://github.com/${repository}/releases/download"}

fail() {
  printf 'nats-tui installer: %s\n' "$1" >&2
  exit 1
}

need() {
  command -v "$1" >/dev/null 2>&1 || fail "required command not found: $1"
}

need curl
need tar

os=${NATS_TUI_OS:-$(uname -s)}
case "$os" in
  Darwin|darwin) os=darwin ;;
  Linux|linux) os=linux ;;
  *) fail "unsupported operating system: $os" ;;
esac

arch=${NATS_TUI_ARCH:-$(uname -m)}
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) fail "unsupported architecture: $arch" ;;
esac

if [ -z "$version" ]; then
  version=$(curl -fsSL "https://api.github.com/repos/${repository}/releases/latest" |
    sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' |
    sed -n '1p')
  [ -n "$version" ] || fail "could not resolve the latest release"
fi

tag=$version
case "$tag" in
  v*) ;;
  *) tag="v${tag}" ;;
esac
version=${tag#v}
asset="nats-tui_${version}_${os}_${arch}.tar.gz"

tmp_dir=$(mktemp -d 2>/dev/null || mktemp -d -t nats-tui)
trap 'rm -rf "$tmp_dir"' EXIT HUP INT TERM

printf 'Downloading nats-tui %s for %s/%s...\n' "$version" "$os" "$arch"
curl -fsSL "${release_url}/${tag}/${asset}" -o "${tmp_dir}/${asset}"
curl -fsSL "${release_url}/${tag}/checksums.txt" -o "${tmp_dir}/checksums.txt"

expected=$(awk -v asset="$asset" '$2 == asset { print $1; exit }' "${tmp_dir}/checksums.txt")
[ -n "$expected" ] || fail "release checksum not found for ${asset}"

if command -v sha256sum >/dev/null 2>&1; then
  actual=$(sha256sum "${tmp_dir}/${asset}" | awk '{ print $1 }')
elif command -v shasum >/dev/null 2>&1; then
  actual=$(shasum -a 256 "${tmp_dir}/${asset}" | awk '{ print $1 }')
else
  fail "sha256sum or shasum is required to verify the download"
fi

[ "$actual" = "$expected" ] || fail "checksum verification failed for ${asset}"

tar -xzf "${tmp_dir}/${asset}" -C "$tmp_dir"
[ -f "${tmp_dir}/nats-tui" ] || fail "release archive does not contain nats-tui"
mkdir -p "$install_dir"
install -m 0755 "${tmp_dir}/nats-tui" "${install_dir}/nats-tui"

printf 'Installed nats-tui %s at %s/nats-tui\n' "$version" "$install_dir"
case ":${PATH}:" in
  *":${install_dir}:"*) ;;
  *) printf 'Add %s to PATH, then run: nats-tui\n' "$install_dir" ;;
esac

