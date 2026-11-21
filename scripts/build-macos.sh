#!/bin/sh
set -eu
cd "$(dirname "$0")/.."

case "$(uname -m)" in
  arm64) platform=arm64 ;;
  x86_64) platform=amd64 ;;
  *) echo "Unsupported macOS architecture" >&2; exit 1 ;;
esac

./scripts/install-opencode.sh "resources/darwin-$platform/opencode" "$platform"
go tool mygo build -platform "darwin/$platform" "$@"
