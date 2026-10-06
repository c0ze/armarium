# Releases

1. Commit and push the tested release source to `main`. Pick an unused semantic
   version, then run `gh workflow run release.yml --ref main -f version=vX.Y.Z`.
   The workflow runs the checks, publishes multi-architecture Docker images to
   GHCR (and Docker Hub when configured), and creates a **draft** GitHub release
   with Linux/Windows archives. It does not publish unsigned macOS builds.
2. On the signing Mac, check out the exact commit used by the workflow. Run:
   ```sh
   scripts/notarize-macos.sh vX.Y.Z /absolute/path/to/release-assets
   ```
   This builds an Intel/Apple Silicon universal CLI, signs it with Developer ID,
   and signs, notarizes and staples its DMG. The default identity is
   `Developer ID Application: Gand (7696W4CMNC)`; the `gand-notary` profile lives
   in the login keychain. `DEVELOPER_ID`, `SIGNING_KEYCHAIN` and `NOTARY_PROFILE`
   can select another existing identity/profile. Never add signing credentials
   to the repository or release assets.
3. Download the draft's Linux/Windows archives into the release-assets directory:
   ```sh
   gh release download vX.Y.Z --dir /absolute/path/to/release-assets \
     --pattern '*.tar.gz' --pattern '*.zip'
   cd /absolute/path/to/release-assets
   shasum -a 256 *.tar.gz *.zip *.dmg > checksums.txt
   gh release upload vX.Y.Z *.dmg checksums.txt --clobber --repo c0ze/armarium
   ```
4. Check all four platform assets, hashes and the DMG's Gatekeeper assessment.
   Add release notes explaining any required configuration migration, then
   publish the draft with `gh release edit vX.Y.Z --draft=false --latest`.

Existing installations retain their configuration. For v0.3.2, release notes
must tell users to merge `tauri://localhost`, `http://tauri.localhost` and
`https://tauri.localhost` into `cors_origins` and restart. New installations
using the shipped example already allow both hosted readers and desktop apps;
authentication remains required and cross-origin cookies remain disabled.
