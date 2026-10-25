# Releases

A release has these downloads:

| File | What it is | Built by |
|---|---|---|
| `Armarium-<X.Y.Z>-macOS.dmg` | `Armarium.app` (Apple silicon and Intel), signed, notarized and stapled; the command line is inside at `Armarium.app/Contents/Helpers/armarium` | `scripts/notarize-macos.sh` on the signing Mac |
| `Armarium-Setup-<X.Y.Z>-x64.exe` | Windows installer (tray app and command line), per user, no administrator rights; **unsigned** for now | the release workflow (Inno Setup) |
| `armarium_<X.Y.Z>_linux_{amd64,arm64}.tar.gz`, `armarium_<X.Y.Z>_windows_amd64.zip` | the command line alone, for servers | the release workflow |
| `checksums.txt` | SHA-256 of every file | the workflow, then again after the DMG is added |

Docker images go to GHCR (and Docker Hub when configured).

1. Commit and push the tested release source to `main`. Pick an unused semantic
   version, then run `gh workflow run release.yml --ref main -f version=vX.Y.Z`.
   The workflow builds the Windows installer on a Windows runner, runs the
   checks, publishes multi-architecture Docker images, and creates a **draft**
   GitHub release with the installer, the archives and `checksums.txt`. It does
   not publish unsigned macOS builds.
2. On the signing Mac, check out the exact commit used by the workflow. Run:
   ```sh
   scripts/notarize-macos.sh vX.Y.Z /absolute/path/to/release-assets
   ```
   It builds `Armarium.app` (the desktop app and the command line, both
   universal), signs them inside-out with Developer ID, notarizes and staples
   the app, checks that Gatekeeper reports `source=Notarized Developer ID`, then
   builds the DMG (the app plus an Applications shortcut) and signs, notarizes
   and staples that too. The default identity is
   `Developer ID Application: Gand (7696W4CMNC)` with the `gand-notary`
   profile. In a local Terminal session it signs with the login keychain; over
   ssh (where the login keychain is locked) it uses the dedicated
   `gand-signing` keychain, unlocks it from its password file just before
   signing (after the web and Go builds, so no install script runs while it
   is unlocked) and locks it again when the script exits. The password is on
   `security`'s argument list for that one call. With `gh` installed, the
   script refuses to sign unless HEAD is the draft release's target commit.
   `MAC_SIGN_IDENTITY`, `MAC_KEYCHAIN`, `MAC_KEYCHAIN_PASS_FILE` and
   `MAC_NOTARY_PROFILE` select another existing identity, keychain or profile.
   Never add signing credentials to the repository or release assets.

   To try a build before releasing, `scripts/notarize-macos.sh --local
   v0.6.0-dev.1 <dir>` skips the clean-tree and release-commit checks and names
   the DMG `…-local.dmg`; it still signs and notarizes. `make mac-app` builds an
   unsigned `bin/Armarium.app` for quick local runs.
3. Add the DMG to the draft and refresh the checksums:
   ```sh
   gh release download vX.Y.Z --dir /absolute/path/to/release-assets \
     --pattern '*.tar.gz' --pattern '*.zip' --pattern 'Armarium-Setup-*.exe'
   cd /absolute/path/to/release-assets
   shasum -a 256 *.tar.gz *.zip Armarium-Setup-*.exe Armarium-*-macOS.dmg > checksums.txt
   gh release upload vX.Y.Z Armarium-*-macOS.dmg checksums.txt --clobber --repo c0ze/armarium
   ```
4. Check every asset, the hashes and the DMG's Gatekeeper assessment. Install
   the Windows build on a Windows 10 or 11 PC (SmartScreen: More info, Run
   anyway) and the Mac build from the DMG. Add release notes explaining any
   required configuration migration, then publish the draft with
   `gh release edit vX.Y.Z --draft=false --latest`. Publishing also updates the
   download links on the project site.

## Windows code signing

The installer is unsigned, so SmartScreen warns on first run; the install guide
shows the "More info, Run anyway" step. Gand qualifies for Azure Artifact
Signing (a company registered in Japan). Once it is set up:

1. In Azure: an Artifact Signing account (Basic), a completed Organization
   identity validation and a Public Trust certificate profile. Give an Entra app
   registration the *Artifact Signing Certificate Profile Signer* role and a
   federated credential for this repository's GitHub Actions.
2. In the repository: secrets `AZURE_CLIENT_ID`, `AZURE_TENANT_ID`,
   `AZURE_SUBSCRIPTION_ID`; variables `AZURE_SIGNING_ENDPOINT` (for example
   `https://jpe.codesigning.azure.net/`), `AZURE_SIGNING_ACCOUNT`,
   `AZURE_SIGNING_PROFILE`, and `WINDOWS_SIGNING` = `azure`.

The release workflow then signs `Armarium.exe` and `armarium.exe` before
packaging, and the setup program after. No secret is stored in the repository.

## Configuration across upgrades

Existing installations retain their configuration. The desktop apps keep
theirs in `~/Library/Application Support/Armarium` (macOS) and
`%LOCALAPPDATA%\Armarium` (Windows); upgrading or uninstalling keeps it unless
the owner chooses otherwise in the Windows uninstaller. For v0.3.2, release
notes must tell users to merge `tauri://localhost`, `http://tauri.localhost`
and `https://tauri.localhost` into `cors_origins` and restart. New
installations using the shipped example already allow both hosted readers and
desktop apps; authentication remains required and cross-origin cookies remain
disabled.
