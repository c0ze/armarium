#!/usr/bin/env bash
# Build a universal CLI in a signed, notarized and stapled DMG. No credentials
# are stored here; notarytool uses an existing keychain profile.
#
# Run on the signing Mac. A local Terminal session signs with the (unlocked)
# login keychain. Over ssh the login keychain is locked, so the default is the
# dedicated gand-signing keychain, unlocked here for this session from its
# password file. Override with MAC_KEYCHAIN, MAC_KEYCHAIN_PASS_FILE,
# MAC_NOTARY_PROFILE and MAC_SIGN_IDENTITY.
set -euo pipefail

version=${1:?Usage: scripts/notarize-macos.sh vX.Y.Z OUTPUT_DIRECTORY}
output_dir=${2:?Usage: scripts/notarize-macos.sh vX.Y.Z OUTPUT_DIRECTORY}
[[ "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?$ ]] || { echo "Invalid version" >&2; exit 1; }
[[ $(uname -s) == Darwin ]] || { echo "Run on the signing Mac" >&2; exit 1; }
cd "$(dirname "$0")/.."
[[ -z $(git status --porcelain) ]] || { echo "Commit the release source first" >&2; exit 1; }
mkdir -p "$output_dir"
output_dir=$(cd "$output_dir" && pwd)
identity=${MAC_SIGN_IDENTITY:-Developer ID Application: Gand (7696W4CMNC)}
if [[ -n ${SSH_CONNECTION:-} ]]; then
  keychain=${MAC_KEYCHAIN:-$HOME/Library/Keychains/gand-signing.keychain-db}
  pass_file=${MAC_KEYCHAIN_PASS_FILE:-$HOME/apple-signing/kcpass}
else
  keychain=${MAC_KEYCHAIN:-$HOME/Library/Keychains/login.keychain-db}
  pass_file=${MAC_KEYCHAIN_PASS_FILE:-}   # the login keychain is already unlocked
fi
profile=${MAC_NOTARY_PROFILE:-gand-notary}
scratch=$(mktemp -d "${TMPDIR:-/tmp}/armarium-macos.XXXXXX")
cleanup() {
  rm -rf "$scratch"
  # Lock again only a keychain this script unlocked (never the login keychain).
  if [[ -n "$pass_file" ]]; then security lock-keychain "$keychain" || true; fi
}
trap cleanup EXIT

# Sign only the commit the release workflow built (its draft's target).
if command -v gh >/dev/null; then
  target=$(gh release view "$version" --repo c0ze/armarium --json targetCommitish -q .targetCommitish 2>/dev/null || true)
  if [[ -n "$target" && "$target" != "$(git rev-parse HEAD)" ]]; then
    echo "HEAD is not $target, the commit the $version release was built from" >&2; exit 1
  fi
  [[ -n "$target" ]] || echo "warning: could not read the $version release; check that HEAD is its commit" >&2
else
  echo "warning: gh is not installed; check that HEAD is the commit the $version release was built from" >&2
fi

# Everything that runs third-party code (npm install scripts, the Go toolchain)
# runs before the signing keychain is unlocked.
(cd web && npm ci --no-audit --no-fund && npm run build)
for arch in amd64 arm64; do
  CGO_ENABLED=0 GOOS=darwin GOARCH=$arch go build -trimpath \
    -ldflags "-s -w -X main.version=$version" -o "$scratch/armarium-$arch" ./cmd/armarium
done
mkdir "$scratch/package"
lipo -create "$scratch/armarium-amd64" "$scratch/armarium-arm64" -output "$scratch/package/armarium"
# Unlocked only to sign; the cleanup trap locks it again. The password is on
# security's argument list for this one call.
if [[ -n "$pass_file" ]]; then security unlock-keychain -p "$(cat "$pass_file")" "$keychain"; fi
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
