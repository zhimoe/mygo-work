#!/bin/sh
set -eu
cd "$(dirname "$0")/.."

version=2.3.2
target=resources/windows-arm64/opencode.exe
binary_hash=cde2ef5e052143ecca725af3ab330e00b7b7ab6bab1b971bb1a9467fc2c15063
if [ ! -f "$target" ] || [ "$(shasum -a 256 "$target" | cut -d ' ' -f 1)" != "$binary_hash" ]; then
  mkdir -p "$(dirname "$target")"
  temp=$(mktemp -d "$(dirname "$target")/.opencode.XXXXXXXX")
  trap 'rm -rf "$temp"' EXIT
  archive="$temp/opencode.tgz"
  url="https://registry.npmjs.org/@opencode/cli-windows-arm64/-/cli-windows-arm64-$version.tgz"
  curl --fail --silent --show-error --location --retry 2 --output "$archive" "$url"
  integrity='bo9F9LOV26aJVhSBYcXgcNS1jlUksfOgVQPf0nF8CFYQcVAmEJeg5qHAQb8B/x49FzvVSw2i27O12bTW7dTS1g=='
  if [ "$(openssl dgst -sha512 -binary "$archive" | openssl base64 -A)" != "$integrity" ]; then
    echo "OpenCode v$version download failed integrity verification" >&2
    exit 1
  fi
  tar -xzf "$archive" -C "$temp" package/bin/opencode.exe
  mv -f "$temp/package/bin/opencode.exe" "$target"
fi

go tool mygo build -platform windows/arm64 "$@"
