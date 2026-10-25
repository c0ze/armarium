#!/usr/bin/env bash
# Build an unsigned universal Armarium.app into OUTPUT_DIRECTORY: the desktop
# app (Contents/MacOS/Armarium, cgo for the menu bar and login item) and the
# command line (Contents/Helpers/armarium). The web UI must be built first
# (make web). scripts/notarize-macos.sh signs and notarizes the result.
set -euo pipefail

version=${1:?Usage: scripts/build-mac-app.sh vX.Y.Z OUTPUT_DIRECTORY}
out=${2:?Usage: scripts/build-mac-app.sh vX.Y.Z OUTPUT_DIRECTORY}
[[ $(uname -s) == Darwin ]] || { echo "Build the Mac app on a Mac" >&2; exit 1; }
cd "$(dirname "$0")/.."
mkdir -p "$out"
out=$(cd "$out" && pwd)
app="$out/Armarium.app"
[[ ! -e "$app" ]] || { echo "Output already exists: $app" >&2; exit 1; }
work=$(mktemp -d "${TMPDIR:-/tmp}/armarium-app.XXXXXX")
trap 'rm -rf "$work"' EXIT

ldflags="-s -w -X main.version=$version"
# Names differ by more than case: macOS volumes are usually case-insensitive.
for arch in amd64 arm64; do
  carch=$arch; [[ $arch == amd64 ]] && carch=x86_64
  CGO_ENABLED=0 GOOS=darwin GOARCH=$arch go build -trimpath -ldflags "$ldflags" \
    -o "$work/cli-$arch" ./cmd/armarium
  CGO_ENABLED=1 GOOS=darwin GOARCH=$arch CC="clang -arch $carch" MACOSX_DEPLOYMENT_TARGET=13.0 \
    go build -trimpath -ldflags "$ldflags" -o "$work/desktop-$arch" ./cmd/armarium-desktop
done

mkdir -p "$app/Contents/MacOS" "$app/Contents/Helpers" "$app/Contents/Resources"
lipo -create "$work/desktop-amd64" "$work/desktop-arm64" -output "$app/Contents/MacOS/Armarium"
lipo -create "$work/cli-amd64" "$work/cli-arm64" -output "$app/Contents/Helpers/armarium"
sed "s/__VERSION__/${version#v}/g" packaging/macos/Info.plist > "$app/Contents/Info.plist"
printf 'APPL????' > "$app/Contents/PkgInfo"
cp packaging/icons/Armarium.icns "$app/Contents/Resources/Armarium.icns"
plutil -lint "$app/Contents/Info.plist" >/dev/null
echo "$app"
