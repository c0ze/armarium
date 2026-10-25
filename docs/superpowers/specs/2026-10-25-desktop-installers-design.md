# Desktop installers and guided setup: design

Status: draft for Arda's review (2026-10-25). Branch `feat/desktop-installers`.

## Why

Arda asked for Armarium to be easier to install and configure, with Windows and Mac installers. Today a new
owner has to:

- pick Docker, a NAS or systemd from `docs/deploy.md`;
- hand-edit `armarium.toml`: library paths, `listen`, `allowed_hosts`, `public_url` and `cors_origins`;
- run `armarium hash-password` in a terminal and paste the hash into the file;
- learn about HTTPS before a phone can read anything.

The people Armarium now reaches through Skrivist Books (live on Google Play, in App Store review since
2026-10-24) mostly have a Mac or a Windows PC with their books on it, and no terminal habits. For them,
installing Armarium should feel like installing any other app: download, double-click, answer a few questions
in the browser, scan a QR code with the phone.

## Goals

1. **A real installer per platform.**
   - macOS: `Armarium.app` (Apple silicon and Intel), signed with `Developer ID Application: Gand
     (7696W4CMNC)`, notarized and stapled, in a DMG you drag to Applications.
   - Windows (x64): `Armarium-Setup-<version>-x64.exe`, installed for the current user, with a Start menu entry
     and an uninstaller.
2. **A menu-bar (macOS) or tray (Windows) app** that runs the server while the user is logged in, starts at
   login, and gives a way back into Armarium without remembering an address.
3. **A first-run setup in the browser** that replaces hand-editing TOML and `hash-password`:
   - the owner password;
   - library folders;
   - who can read: this computer, devices at home, or anywhere through Tailscale.
4. **Settings stay editable** in Admin after setup, for desktop installs.
5. **Phones at home without HTTPS:** pair the native Skrivist Books app over plain HTTP on the local network.
   The iOS app allows `NSAllowsLocalNetworking` and the Android app allows cleartext; Armarium's Admin
   currently refuses this.
6. **Release tooling** that builds both installers next to the existing archives and Docker images.
7. **Plain-language install guides** for Mac and Windows, with the Docker/NAS route kept for advanced users.
   The project site (PR c0ze/armarium#2) links to the installers.

## Non-goals

- **Windows code signing, for now.** Arda decided (2026-10-25) to ship unsigned and document SmartScreen's
  "More info → Run anyway". Gand qualifies for Azure Artifact Signing (a company registered in Japan, verified
  in Partner Center). The release job keeps a signing step that is off until it is set up.
- A Windows service, or running with no one logged in.
- Automatic updates or update checks. Armarium makes no network calls the user didn't set up. Release notes
  and the site announce new versions.
- A Linux desktop app. Linux keeps the tar.gz, systemd and Docker.
- The Microsoft Store, the Mac App Store and winget.
- Editing a hand-written `armarium.toml` from the UI. Docker and NAS configs are often mounted read-only and
  carry comments.
- A server-side folder browser.
- Bonjour/mDNS advertising.

## Anti-goals (failure even if it works)

- Any path to the library without the owner password or a token. The setup page is never reachable without
  the one-time setup code, and never after a password exists.
- A weaker Host allowlist, CSRF rule or CORS rule for setup, Settings or the desktop app.
- An installer that asks for administrator rights.
- Writing to library folders, or deleting reading progress without the owner explicitly asking.
- Telemetry or phoning home.

## Constraints

- One Go codebase. The server binary used by Docker and Linux stays `CGO_ENABLED=0`, static, and free of GUI
  dependencies.
- Signing and notarization follow `~/.claude/CLAUDE.md`:
  - identity `Developer ID Application: Gand (7696W4CMNC)`, login keychain, `gand-notary` profile;
  - sign inside-out, verify, notarize, staple, `spctl`;
  - nothing from `~/apple-signing` goes into the repo.

  There is no Developer ID Installer identity, so there is no `.pkg`.
- Heavy builds and test suites go through `~/.local/bin/heavy-slot`.
- Releases stay drafts until Arda publishes them.
- macOS 13 or later (for `SMAppService`). Windows 10 1809 or later.

## Decisions

Arda's decisions are marked **(Arda)**; the rest are my calls, open to override.

### Desktop app

- **(Arda)** Windows ships unsigned for now.
- A new `cmd/armarium-desktop` binary runs the server in-process and owns the tray or menu-bar icon. It runs
  as the logged-in user, so it sees the user's folders and mapped NAS drives; a service would not.
  - It uses `fyne.io/systray` (or an equivalent). That needs cgo on macOS and is pure Go on Windows.
  - On Windows it is built with `-H windowsgui`, so no console window opens.
  - The plain `armarium` CLI binary ships next to it for advanced use and support:
    - macOS: inside the bundle, at `Armarium.app/Contents/MacOS/armarium`;
    - Windows: in the install folder.
- **Single instance.** Launching it again while it runs opens the browser instead of starting a second
  server. Use a lock file in the data folder plus the loopback URL.
- **Menu**, built with the `game-copy`/`ux-design` wording rules:
  - Open Armarium;
  - Pair a phone…;
  - Settings…;
  - a status line (running, scanning, or the error that stopped it);
  - Start at login (a check item);
  - Show log folder;
  - Quit Armarium.
- **Start at login** is on by default and asked in setup.
  - macOS: `SMAppService.mainApp`, which is removed automatically when the app is deleted.
  - Windows: an `HKCU\…\Run` value that the uninstaller removes.
- On first launch, the app opens the browser at the setup page by itself.
- **Port 8580 already taken:** the status line says so, and Settings offers another port. Builder's call how
  far to take this; at least a clear message.

### Where things live

| | Config | Data (database, covers) | Logs | Program |
|---|---|---|---|---|
| macOS | `~/Library/Application Support/Armarium/armarium.toml` | `…/Armarium/data/` | `~/Library/Logs/Armarium/` | `/Applications/Armarium.app` |
| Windows | `%LOCALAPPDATA%\Armarium\armarium.toml` | `%LOCALAPPDATA%\Armarium\data\` | `%LOCALAPPDATA%\Armarium\logs\` | `%LOCALAPPDATA%\Programs\Armarium\` |

Logs rotate at a small size cap.

### Managed configuration

- The desktop app's `armarium.toml` is **managed**: Armarium writes the whole file. A header says so and asks
  the reader to quit Armarium before editing it by hand. The file is still ordinary TOML that `armarium serve
  --config` reads.
- A new `[desktop]` table holds what setup chose; listen address and hosts are worked out from it at each
  start, so a changed home IP never leaves stale values:

  ```toml
  [desktop]
  network = "local"      # "local" = this computer only; "home" = devices on the local network
  tailscale = false      # also serve https://<machine>.<tailnet>.ts.net through `tailscale serve`
  port = 8580
  ```

- **Derived at start** (builder may refine):

  | Setting | local | home | tailscale = true |
  |---|---|---|---|
  | `listen` | `127.0.0.1:<port>` | all interfaces on `<port>` | adds nothing |
  | `allowed_hosts` | `localhost`, `127.0.0.1`, `::1` | also the machine's own interface addresses and its `.local` name | also the `ts.net` name |
  | `public_url` | none | `http://<primary LAN IPv4>:<port>` | `https://<ts.net name>`, preferred over the LAN URL |
  | `trusted_proxies` | none | none | `127.0.0.1` |

  - `cors_origins` is always the five Skrivist origins in `deploy/armarium.example.toml`.
  - An explicit `public_url` or extra `allowed_hosts` in the file (the "own HTTPS address" advanced field in
    Settings) is added to the derived values.
  - **Own interface addresses.** A Host that is an IP literal equal to one of this machine's current interface
    addresses is allowed (a new `config` option that only managed configs turn on). Rebinding attacks need a
    hostname the attacker controls; a literal IP host is its own origin, so this does not weaken the
    DNS-rebinding defence. Tests must cover this.
- **Unmanaged configs** (Docker, NAS, systemd, hand-written) are never written. Settings shows them read-only,
  with copyable TOML snippets for what the owner may want to change.

### Owner password in the database

- The owner password set in the browser is stored as an argon2id hash in the database (a settings row), not
  in TOML. Docker configs are mounted `:ro`.
- `password_hash` in TOML (or `ARMARIUM_PASSWORD_HASH`) keeps working and **wins** when set. Admin's "Change
  password" is then disabled and explains why.
- Changing the stored hash ends owner sessions, the same as `ResetSessionsIfPasswordChanged` does for the TOML
  key.
- A new `armarium password` command sets or replaces the stored password from the terminal, for "forgot my
  password" in Docker as well. `hash-password` stays.

### First-run setup

**When.** Setup is open while no owner password exists in TOML, the environment or the database. At start,
Armarium then:

- generates a random one-time **setup code** (at least 128 bits), kept in memory only;
- logs `Finish setup: http://<host>:<port>/setup#code=<code>`;
- in the desktop app, opens that URL in the browser itself.

Docker users find the link in `docker logs`.

**Security.**

- The `/api/setup/*` routes are public-level, but every call needs the code, sent in the JSON body.
- The usual Host allowlist applies, along with the CSRF rule: `application/json` only, and an `Origin` that is
  one of Armarium's own hosts.
- A wrong code counts against the login limiter.
- Once a password exists, the routes answer 410 and the code is discarded.
- Finishing setup logs the owner in, using the same cookie as `/api/login`.
- In the desktop app the server listens on loopback until setup chooses "home".

**Steps** (desktop; on an unmanaged config only step 1 is offered and the others are shown as guidance):

1. **Owner password**, typed twice, with a short "why" line.
2. **Libraries.**
   - Add one or more folders with "Choose folder…", which opens the operating system's own folder dialog,
     or by typing a path.
   - Each folder gets a name and Books/Comics, guessed from the files.
   - A Calibre library (`metadata.db`) is detected and marked as Calibre.
   - The server checks that the path exists, is a folder and is readable before accepting it.
   - The native dialog endpoint answers only in the desktop app, and only to requests from loopback. It
     returns a path, never a listing.
3. **Who can read.**
   - **This computer only.** The browser, plus the Skrivist Books and Comics desktop apps.
   - **Devices at home.** Phones and tablets on the same Wi-Fi. Explains the Windows firewall prompt, and
     that the hosted web readers need HTTPS.
   - **Anywhere, with Tailscale.**
     - Detects the Tailscale CLI and shows the `ts.net` name.
     - On the owner's click it runs `tailscale serve --bg` for the port. If that fails or HTTPS certificates
       are off in the tailnet, it shows the exact command and the Tailscale page to fix it.
     - It never replaces someone else's Serve setup. If `tailscale serve status --json` shows anything on
       https:443 that Armarium didn't create, setup stops and explains, with the command to run by hand.
     - Turning Tailscale off in Settings, or changing the port, removes only the entry Armarium made.
       Uninstalling does not touch Tailscale. Deleting a Mac app runs no code, so the guides give the one
       `tailscale serve` command that removes it.
     - Can be combined with "Devices at home".
   - **Advanced** (collapsed): my own HTTPS address. That is `public_url` plus its host, with a note about
     `trusted_proxies`.
4. **Start at login** (desktop) and **Scan now.** It ends on the library, with "Pair a phone" one click away.

Saving writes the managed config, then the desktop app reloads the server in-process: it closes the store,
reopens it with the new config and starts listening again. No restart by hand.

### Admin → Settings

- The same screens as setup: libraries, who can read, port, change password.
- Libraries:
  - Changing a library's name or folder keeps its items and reading progress. A rename runs
    `RenameLibrary` and the config change in the same save; a new folder is picked up by the next scan.
  - Removing a library deletes its items and reading progress (`library rm`). It needs a confirmation that
    names the library and how many items go with it. There is no "detached" state: the store refuses a
    library missing from the config, and adding one is not worth a schema change.
- Saving reloads as above.
- Owner session only (`Admin` access), with the usual CSRF rule.

### Pairing the Books app over plain HTTP

- Admin → Pair a phone gets a target for the native **Skrivist Books app (iPhone, iPad, Android)** that is
  allowed over plain HTTP on the local network.
- The hosted readers keep refusing HTTP; they cannot load it.
- The builder confirms the link format the native app's "Scan QR code" accepts, against
  `skrivist-native` `AddSourceFlow` on iOS and Android. It is believed to be any link carrying `opds=` with
  an `http` or `https` catalogue.
- The QR must never put a token into a link that opens the hosted web reader over HTTP.
- If the native Comics app accepts the same, it gets a target too.

### Windows installer

- Built with **Inno Setup** on the `windows-latest` runner:
  - per-user install (`PrivilegesRequired=lowest`), so there is no UAC prompt;
  - a stable `AppId`, so a newer installer upgrades in place;
  - `AppMutex` and closing the running app before replacing files;
  - a Start menu shortcut, and "Launch Armarium" on the last page.
- The uninstaller:
  - removes the program, the shortcut and the Run value;
  - asks "Also delete your Armarium settings and reading progress?" (default **No**);
  - never touches libraries.
- Unsigned. The install guide shows the SmartScreen screen and the "More info → Run anyway" step.
- The release job has a signing step for `armarium-desktop.exe`, `armarium.exe` and the setup `.exe`. It is
  skipped unless a repository variable turns it on, and is written for `azure/artifact-signing-action` with
  GitHub OIDC, so no secret is stored.

### macOS app

- A universal `Armarium.app`:
  - `LSUIElement`, so it has a menu-bar icon and no Dock icon;
  - bundle id `com.gand.armarium`;
  - Hardened Runtime, plus folder-access usage strings for Documents, Downloads, Desktop, removable volumes
    and network volumes.
- `scripts/notarize-macos.sh`:
  - builds both Go binaries for both architectures (the desktop one with cgo) and `lipo`s them;
  - signs inside-out: the CLI, then the app;
  - verifies, notarizes, staples and runs `spctl`;
  - then builds a DMG with the app and an Applications link, and signs, notarizes and staples the DMG.

  It keeps the existing safety rails (clean tree, release target commit, gand-signing over ssh).
- **The app DMG replaces the CLI-only DMG.** The CLI is inside the bundle, and the guide shows the one-line
  symlink for Terminal users.

### Existing installs

- People already running the CLI from an archive or the old DMG keep their own config and data. The desktop
  app does not import them, and nothing moves.
- They can keep running the CLI (inside the app bundle on macOS, in the zip and install folder on Windows), or
  start fresh with the app. The guides say so, and say how to point the CLI at an existing config.
- If port 8580 is already in use when the app starts, its status line says so. It does not try to stop the
  other copy.

### Release

- `release.yml`:
  - unchanged for Docker and Linux;
  - the Windows zip stays, with the CLI;
  - a new Windows job builds the desktop binary and the installer and attaches
    `Armarium-Setup-<version>-x64.exe` to the draft;
  - `checksums.txt` covers every asset.
- macOS is still built on the signing Mac (`notarize-macos.sh`), and the script's output is
  `Armarium-<version>-macOS.dmg`.
- In asset names, `<version>` has no leading `v`, matching the existing archives:
  `Armarium-0.6.0-macOS.dmg`, `Armarium-Setup-0.6.0-x64.exe`.
- `docs/releasing.md` is updated to match.
- The site's `pages.yml` link rewrite (PR #2) learns the new asset names.

### Documentation

- `docs/install-mac.md` and `docs/install-windows.md`:
  - plain language, one action per step, screenshots;
  - download, install, the setup page, adding libraries, pairing a phone, updating, uninstalling, forgotten
    password, where files live.

  Written with the `unslop` and `ux-design` skills in mind.
- `README.md` quick start leads with "Download for Mac / Windows", then "Docker, NAS and Linux" pointing to
  `docs/deploy.md`.
- `docs/deploy.md` gains the setup link from `docker logs` and `armarium password`.
- Site changes go onto PR #2's branch (`site/companion-apps`), which is still open, in its own worktree:
  `pages.yml` `data-asset` names, the Download block and the copy.
- PR #2 site copy:
  - Skrivist Books is on Google Play and in App Store review. Change "coming soon" to say so and link the
    Play listing.
  - The Download block points to the two installers, then the archives.
  - A three-step "Install" section.

## Testing

- **Go**, test-first:
  - setup gating: no code, wrong code, a code after the password is set, foreign Host, foreign Origin,
    non-JSON, limiter;
  - password precedence (TOML or env over database) and session reset;
  - `armarium password`;
  - the managed config writer round trip;
  - network derivation for each mode;
  - the interface-address Host rule;
  - folder validation;
  - the loopback-only folder dialog;
  - Settings library removal semantics;
  - pairing targets over HTTP.

  The existing `security_test.go` cases keep passing.
- **Web:** `svelte-check` and vitest for the setup and settings logic.
- **Real installs on this Mac:**
  - DMG → Applications → first launch → setup → scan → open a book;
  - "devices at home" from another device or the iOS simulator through `sim-lock`, pairing the Books app;
  - quit and relaunch, login item, upgrade over an older build, delete the app;
  - `spctl` reports "Notarized Developer ID".
- **Windows:** install, setup, firewall prompt, pairing, upgrade and uninstall on a real Windows 10/11
  machine. Arda provides the machine or VM. Until then, CI only proves it builds.
- **Reviews:** `ai-reviewer` order (Codex first), findings fixed before the PR is handed over.

## Left to the builder

- The exact tray library, folder-dialog approach (Cocoa `NSOpenPanel` or `osascript` on macOS;
  `IFileOpenDialog` on Windows) and log rotation.
- Setup page layout, within `DESIGN.md`'s system and the existing Svelte app.
- How far the port-conflict handling goes beyond a clear message.
- Whether `tailscale serve` sets `X-Forwarded-Proto` (verify; if not, `secure_cookie` handling for the
  `ts.net` origin needs another route).
- Splitting this into reviewable PRs. Suggested order:
  1. server: password in the database, setup mode, managed config, Settings API;
  2. web: setup and Settings;
  3. desktop app;
  4. installers and release;
  5. docs and site.
