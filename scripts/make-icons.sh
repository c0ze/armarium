#!/usr/bin/env bash
# Regenerate the app and tray icons from packaging/icons/*.svg. Needs
# rsvg-convert, iconutil (macOS) and ImageMagick; the outputs are committed so
# release builds don't need these tools.
set -euo pipefail
cd "$(dirname "$0")/../packaging/icons"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

# macOS app icon
mkdir "$tmp/Armarium.iconset"
for s in 16 32 128 256 512; do
  rsvg-convert -w $s -h $s armarium.svg -o "$tmp/Armarium.iconset/icon_${s}x${s}.png"
  rsvg-convert -w $((s * 2)) -h $((s * 2)) armarium.svg -o "$tmp/Armarium.iconset/icon_${s}x${s}@2x.png"
done
iconutil -c icns "$tmp/Armarium.iconset" -o Armarium.icns

# Windows app icon (installer, exe, Start menu)
for s in 16 24 32 48 64 128 256; do rsvg-convert -w $s -h $s armarium.svg -o "$tmp/app-$s.png"; done
magick "$tmp"/app-{16,24,32,48,64,128,256}.png armarium.ico

# Menu bar (template: black with alpha, macOS tints it) and Windows tray
rsvg-convert -w 22 -h 22 tray-template.svg -o tray-mac.png
rsvg-convert -w 44 -h 44 tray-template.svg -o tray-mac@2x.png
for s in 16 20 24 32; do rsvg-convert -w $s -h $s armarium.svg -o "$tmp/tray-$s.png"; done
magick "$tmp"/tray-{16,20,24,32}.png tray-win.ico
cp tray-mac@2x.png tray-win.ico ../../cmd/armarium-desktop/icons/
echo "icons written to packaging/icons and cmd/armarium-desktop/icons"
