#!/bin/sh
set -eu

root_dir=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
test_dir=$(mktemp -d 2>/dev/null || mktemp -d -t nats-tui-installer-test)
trap 'rm -rf "$test_dir"' EXIT HUP INT TERM

version=1.2.3
tag="v${version}"
release_dir="${test_dir}/releases/${tag}"
payload_dir="${test_dir}/payload"
install_dir="${test_dir}/bin"
mkdir -p "$release_dir" "$payload_dir"

printf '#!/bin/sh\nprintf "nats-tui test binary\\n"\n' > "${payload_dir}/nats-tui"
chmod 0755 "${payload_dir}/nats-tui"

for target in linux_amd64 linux_arm64 darwin_amd64 darwin_arm64; do
  asset="nats-tui_${version}_${target}.tar.gz"
  tar -czf "${release_dir}/${asset}" -C "$payload_dir" nats-tui
done

: > "${release_dir}/checksums.txt"
for archive in "${release_dir}"/*.tar.gz; do
  if command -v sha256sum >/dev/null 2>&1; then
    digest=$(sha256sum "$archive" | awk '{ print $1 }')
  else
    digest=$(shasum -a 256 "$archive" | awk '{ print $1 }')
  fi
  printf '%s  %s\n' "$digest" "$(basename "$archive")" >> "${release_dir}/checksums.txt"
done

for target in linux_amd64 linux_arm64 darwin_amd64 darwin_arm64; do
  os=${target%_*}
  arch=${target#*_}
  NATS_TUI_VERSION="$version" \
  NATS_TUI_OS="$os" \
  NATS_TUI_ARCH="$arch" \
  NATS_TUI_INSTALL_DIR="$install_dir" \
  NATS_TUI_RELEASE_URL="file://${test_dir}/releases" \
    sh "${root_dir}/scripts/install.sh" >/dev/null
  output=$("${install_dir}/nats-tui")
  [ "$output" = "nats-tui test binary" ] || exit 1
done

printf 'tampered' >> "${release_dir}/nats-tui_${version}_linux_amd64.tar.gz"
if NATS_TUI_VERSION="$version" \
  NATS_TUI_OS=linux \
  NATS_TUI_ARCH=amd64 \
  NATS_TUI_INSTALL_DIR="$install_dir" \
  NATS_TUI_RELEASE_URL="file://${test_dir}/releases" \
    sh "${root_dir}/scripts/install.sh" >/dev/null 2>&1; then
  printf 'installer accepted a tampered archive\n' >&2
  exit 1
fi

printf 'installer e2e: ok\n'
