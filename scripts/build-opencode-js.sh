#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
root=$(pwd)

version=2.3.2
node_version=26.4.0
work=${OPENCODE_BUILD_DIR:-"$root/.cache/opencode-$version"}
dest="$root/resources/windows-amd64"

if ! command -v bun >/dev/null 2>&1; then
  echo "bun 1.4.4 is required: https://bun.sh" >&2
  exit 1
fi

if [ ! -d "$work/.git" ]; then
  mkdir -p "$(dirname "$work")"
  git clone --depth 1 --branch "v$version" https://github.com/anomalyco/opencode.git "$work"
fi

if [ ! -d "$work/node_modules" ]; then
  (cd "$work" && bun install --os='*' --cpu='*' --libc='*')
fi

(cd "$work" && OPENCODE_TARGET=windows-x64 bun run --cwd packages/cli build:node -- --bundle-only --skip-install)

if [ ! -f "$work/packages/cli/dist-node/opencode.mjs" ] || [ ! -f "$work/packages/cli/dist-node/assets/app-archive.bin" ]; then
  echo "OpenCode bundle build did not produce the expected files" >&2
  exit 1
fi

temp=$(mktemp -d)
trap 'rm -rf "$temp"' EXIT
archive="node-v$node_version-win-x64.zip"
base="https://nodejs.org/dist/v$node_version"
curl --fail --silent --show-error --location --retry 2 --output "$temp/$archive" "$base/$archive"
curl --fail --silent --show-error --location --retry 2 --output "$temp/SHASUMS256.txt" "$base/SHASUMS256.txt"
expected=$(awk -v name="$archive" '$2 == name { print $1 }' "$temp/SHASUMS256.txt")
actual=$(shasum -a 256 "$temp/$archive" | cut -d ' ' -f 1)
if [ -z "$expected" ] || [ "$expected" != "$actual" ]; then
  echo "Node $node_version download failed integrity verification" >&2
  exit 1
fi
unzip -q "$temp/$archive" -d "$temp/node"
node_exe="$temp/node/node-v$node_version-win-x64/node.exe"
if [ ! -f "$node_exe" ]; then
  echo "Node $node_version archive does not contain node.exe" >&2
  exit 1
fi

mkdir -p "$dest"
rm -rf "$dest/assets" "$dest/opencode.mjs" "$dest/node.exe"
cp "$work/packages/cli/dist-node/opencode.mjs" "$dest/opencode.mjs"
cp -R "$work/packages/cli/dist-node/assets" "$dest/assets"
cp "$node_exe" "$dest/node.exe"

# Native opencode.exe remains supported and is kept by default. Set
# DROP_BUN=1 only when you want a smaller package that relies on a separately
# installed opencode.exe.
if [ "${DROP_BUN:-0}" = "1" ]; then
  rm -f "$dest/opencode.exe"
fi

echo "OpenCode $version JavaScript bundle is ready in $dest"
