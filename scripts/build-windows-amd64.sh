#!/bin/sh
set -eu
cd "$(dirname "$0")/.."

# A JavaScript bundle staged by scripts/build-opencode-js.sh takes precedence.
# A separately installed opencode.exe is still detected at runtime through PATH
# or MYGO_WORK_OPENCODE.
if [ -f resources/windows-amd64/opencode.mjs ] &&
   [ -f resources/windows-amd64/node.exe ] &&
   [ -f resources/windows-amd64/assets/app-archive.bin ]; then
  go tool mygo build -platform windows/amd64 "$@"
  exit 0
fi

version=2.3.2
target=resources/windows-amd64/opencode.exe
binary_hash=5d52d86e2c3fd2c4d51cefc893f64c9ee66bc2e9535332195802f7c646b2a3a3
if [ ! -f "$target" ] || [ "$(shasum -a 256 "$target" | cut -d ' ' -f 1)" != "$binary_hash" ]; then
  mkdir -p "$(dirname "$target")"
  temp=$(mktemp -d "$(dirname "$target")/.opencode.XXXXXXXX")
  trap 'rm -rf "$temp"' EXIT
  archive="$temp/opencode.tgz"
  url="https://registry.npmjs.org/@opencode/cli-windows-x64/-/cli-windows-x64-$version.tgz"
  curl --fail --silent --show-error --location --retry 2 --output "$archive" "$url"
  integrity='CtyKq9He8g3WJ4zdj0+bHO1cXXn+K918GgpCEFnXZCwVfa9YghEulv5SH3pMHc9ZxyZIU9d9h3IhIi97+LR6mg=='
  if [ "$(openssl dgst -sha512 -binary "$archive" | openssl base64 -A)" != "$integrity" ]; then
    echo "OpenCode v$version download failed integrity verification" >&2
    exit 1
  fi
  tar -xzf "$archive" -C "$temp" package/bin/opencode.exe
  mv -f "$temp/package/bin/opencode.exe" "$target"
fi

go tool mygo build -platform windows/amd64 "$@"
