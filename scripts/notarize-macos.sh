#!/usr/bin/env bash
# Build Armarium.app (universal), sign it with Developer ID, notarize and
# staple it, then do the same for the DMG it ships in. No credentials are
# stored here; notarytool uses an existing keychain profile.
#
# Run on the signing Mac. A local Terminal session signs with the (unlocked)
# login keychain. Over ssh the login keychain is locked, so the default is the
# dedicated gand-signing keychain, unlocked here for this session from its
# password file. Override with MAC_KEYCHAIN, MAC_KEYCHAIN_PASS_FILE,
# MAC_NOTARY_PROFILE and MAC_SIGN_IDENTITY.
#
#   scripts/notarize-macos.sh vX.Y.Z OUTPUT_DIRECTORY
#   scripts/notarize-macos.sh --local vX.Y.Z-dev.N OUTPUT_DIRECTORY
#
# --local is for trying a build: it skips the clean-tree and release-commit
# checks and names the DMG ...-local.dmg, but still signs and notarizes, so
# Gatekeeper sees exactly what a release gets.
set -euo pipefail

local_build=false
if [[ ${1:-} == --local ]]; then
  local_build=true
  shift
fi
usage="Usage: scripts/notarize-macos.sh [--local] vX.Y.Z OUTPUT_DIRECTORY"
version=${1:?$usage}
output_dir=${2:?$usage}
[[ "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?$ ]] || { echo "Invalid version" >&2; exit 1; }
[[ $(uname -s) == Darwin ]] || { echo "Run on the signing Mac" >&2; exit 1; }
cd "$(dirname "$0")/.."
if ! $local_build; then
  [[ -z $(git status --porcelain) ]] || { echo "Commit the release source first" >&2; exit 1; }
fi
mkdir -p "$output_dir"
output_dir=$(cd "$output_dir" && pwd)
name="Armarium-${version#v}-macOS"
if $local_build; then name="$name-local"; fi
dmg="$output_dir/$name.dmg"
[[ ! -e "$dmg" ]] || { echo "Output already exists: $dmg" >&2; exit 1; }

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
if ! $local_build; then
  if command -v gh >/dev/null; then
    target=$(gh release view "$version" --repo c0ze/armarium --json targetCommitish -q .targetCommitish 2>/dev/null || true)
    if [[ -n "$target" && "$target" != "$(git rev-parse HEAD)" ]]; then
      echo "HEAD is not $target, the commit the $version release was built from" >&2; exit 1
    fi
    [[ -n "$target" ]] || echo "warning: could not read the $version release; check that HEAD is its commit" >&2
  else
    echo "warning: gh is not installed; check that HEAD is the commit the $version release was built from" >&2
  fi
fi

# Everything that runs third-party code (npm install scripts, the Go toolchain)
# runs before the signing keychain is unlocked.
(cd web && npm ci --no-audit --no-fund && npm run build)
app=$(scripts/build-mac-app.sh "$version" "$scratch/app" | tail -1)

# notarize submits a file and waits; anything but Accepted stops the script
# with the notary log.
notarize() {
  local out id
  out=$(xcrun notarytool submit "$1" --keychain-profile "$profile" --keychain "$keychain" --wait 2>&1) || true
  echo "$out"
  id=$(awk '/^  id: /{print $2; exit}' <<<"$out")
  if ! grep -q "status: Accepted" <<<"$out"; then
    if [[ -n "$id" ]]; then xcrun notarytool log "$id" --keychain-profile "$profile" --keychain "$keychain" >&2 || true; fi
    echo "Notarization of $(basename "$1") was not accepted" >&2
    exit 1
  fi
}

# Unlocked only to sign; the cleanup trap locks it again. The password is on
# security's argument list for this one call.
if [[ -n "$pass_file" ]]; then security unlock-keychain -p "$(cat "$pass_file")" "$keychain"; fi
sign=(codesign --force --timestamp --options runtime --keychain "$keychain" -s "$identity")
# Inside-out: the command line, then the app around it.
"${sign[@]}" --identifier com.gand.armarium.cli "$app/Contents/Helpers/armarium"
"${sign[@]}" "$app"
codesign --verify --deep --strict --verbose=2 "$app"
"$app/Contents/Helpers/armarium" version

ditto -c -k --keepParent "$app" "$scratch/upload.zip"
notarize "$scratch/upload.zip"
xcrun stapler staple "$app"
spctl -a -t exec -vv "$app" 2>&1 | tee "$scratch/spctl.txt"
grep -q "source=Notarized Developer ID" "$scratch/spctl.txt" || { echo "Gatekeeper does not see a notarized app" >&2; exit 1; }

# The DMG opens to the app and an Applications shortcut to drag it onto.
mkdir "$scratch/dmg"
ditto "$app" "$scratch/dmg/Armarium.app"
ln -s /Applications "$scratch/dmg/Applications"
hdiutil create -volname "Armarium ${version#v}" -srcfolder "$scratch/dmg" -format UDZO "$dmg"
codesign --force --timestamp --keychain "$keychain" -s "$identity" "$dmg"
codesign --verify --strict --verbose=2 "$dmg"
notarize "$dmg"
xcrun stapler staple "$dmg"
xcrun stapler validate "$dmg"
spctl -a -t open --context context:primary-signature -vv "$dmg"
echo "$dmg"
