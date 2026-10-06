#!/usr/bin/env bash
# Build a universal CLI in a signed, notarized and stapled DMG. No credentials
# are stored here; notarytool uses an existing login-keychain profile.
set -euo pipefail

version=${1:?Usage: scripts/notarize-macos.sh vX.Y.Z OUTPUT_DIRECTORY}
output_dir=${2:?Usage: scripts/notarize-macos.sh vX.Y.Z OUTPUT_DIRECTORY}
[[ "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?$ ]] || { echo "Invalid version" >&2; exit 1; }
[[ $(uname -s) == Darwin ]] || { echo "Run on the signing Mac" >&2; exit 1; }
cd "$(dirname "$0")/.."
[[ -z $(git status --porcelain) ]] || { echo "Commit the release source first" >&2; exit 1; }
mkdir -p "$output_dir"
output_dir=$(cd "$output_dir" && pwd)
identity=${DEVELOPER_ID:-Developer ID Application: Gand (7696W4CMNC)}
keychain=${SIGNING_KEYCHAIN:-$HOME/Library/Keychains/login.keychain-db}
profile=${NOTARY_PROFILE:-gand-notary}
scratch=$(mktemp -d "${TMPDIR:-/tmp}/armarium-macos.XXXXXX")
trap 'rm -rf "$scratch"' EXIT

(cd web && npm ci --no-audit --no-fund && npm run build)
for arch in amd64 arm64; do
  CGO_ENABLED=0 GOOS=darwin GOARCH=$arch go build -trimpath \
    -ldflags "-s -w -X main.version=$version" -o "$scratch/armarium-$arch" ./cmd/armarium
done
mkdir "$scratch/package"
lipo -create "$scratch/armarium-amd64" "$scratch/armarium-arm64" -output "$scratch/package/armarium"
codesign --force --timestamp --options runtime --keychain "$keychain" \
  --identifier com.gand.armarium -s "$identity" "$scratch/package/armarium"
codesign --verify --strict --verbose=2 "$scratch/package/armarium"
"$scratch/package/armarium" version
cp LICENSE README.md deploy/armarium.example.toml "$scratch/package/"
cat > "$scratch/package/START-HERE.txt" <<'EOF'
Armarium for macOS (Apple Silicon and Intel)

Copy armarium and armarium.example.toml to a folder, then use Terminal:
  cd /path/to/your/folder
  cp armarium.example.toml armarium.toml
  ./armarium hash-password
Edit armarium.toml: set your library paths, paste the password hash, and add
your server hostname to allowed_hosts. Then start the server:
  ./armarium serve

The example allows Skrivist Books and Comics in the browser and on macOS and
Windows. Existing installations must add the three desktop cors_origins to
their own config when upgrading. Requests still require an API token.

Setup: https://github.com/c0ze/armarium/blob/main/docs/deploy.md
EOF
dmg="$output_dir/armarium_${version#v}_darwin_universal.dmg"
[[ ! -e "$dmg" ]] || { echo "Output already exists: $dmg" >&2; exit 1; }
hdiutil create -volname "Armarium ${version#v}" -srcfolder "$scratch/package" \
  -format UDZO "$dmg"
codesign --force --timestamp --keychain "$keychain" -s "$identity" "$dmg"
codesign --verify --strict --verbose=2 "$dmg"
xcrun notarytool submit "$dmg" --keychain-profile "$profile" --keychain "$keychain" --wait
xcrun stapler staple "$dmg"
xcrun stapler validate "$dmg"
spctl -a -t open --context context:primary-signature -vv "$dmg"
