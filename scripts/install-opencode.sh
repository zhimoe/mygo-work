#!/bin/sh
set -eu

version=2.3.2
destination=${1:-"${MYGO_WORK_DATA_DIR:-"$HOME/Library/Application Support/MyGo Work"}/bin/opencode"}
architecture=${2:-$(uname -m)}
case "$architecture" in
  arm64)
    npm_arch=arm64
    integrity='cYrIzW8CvYc52INFA/cXmw7U2mk+JNWaV+olH7AflYqa/5srSrA/hbLwF7G1N4QTRMtOFrmeTM0LVN81q396sg=='
    ;;
  x86_64|amd64)
    npm_arch=x64
    integrity='vNLWgHc2I/hAbW37OOANepj6VNS8mbbLp9TbWgEKY6MMvzZlx0ifsrcwUuLI0Qoso82UEj6RV20xcBWyZdF84w=='
    ;;
  *) echo "Unsupported macOS architecture: $architecture" >&2; exit 1 ;;
esac

if [ -x "$destination" ] && [ "$("$destination" --version 2>/dev/null || true)" = "opencode v$version" ]; then
  echo "$destination"
  exit 0
fi

folder=$(dirname "$destination")
mkdir -p "$folder"
temp=$(mktemp -d "$folder/.opencode.XXXXXXXX")
trap 'rm -rf "$temp"' EXIT
archive="$temp/opencode.tgz"
url="https://registry.npmjs.org/@opencode/cli-darwin-$npm_arch/-/cli-darwin-$npm_arch-$version.tgz"
curl --fail --silent --show-error --location --retry 2 --output "$archive" "$url"
actual=$(openssl dgst -sha512 -binary "$archive" | openssl base64 -A)
if [ "$actual" != "$integrity" ]; then
  echo "OpenCode v$version download failed integrity verification" >&2
  exit 1
fi
tar -xzf "$archive" -C "$temp" package/bin/opencode
chmod 755 "$temp/package/bin/opencode"
mv -f "$temp/package/bin/opencode" "$destination"
echo "$destination"
