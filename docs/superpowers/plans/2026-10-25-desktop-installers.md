# Desktop installers and guided setup: implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for
> tracking.

**Goal:** A non-technical owner installs Armarium on a Mac or a Windows PC with an installer, finishes setup
in the browser, and pairs a phone, without editing TOML or using a terminal.

**Architecture:**
- The server's open/serve lifecycle moves out of `cmd/armarium` into `internal/server`, so a second binary,
  `cmd/armarium-desktop`, can run it in-process behind a tray or menu-bar icon.
- `internal/config` learns a managed (`[desktop]`) config that it writes itself; listen address, hosts and
  public URL are worked out from it at load.
- `internal/httpapi` gains a setup flow (one-time code), a password stored in the database, and a Settings
  API that calls back into the desktop app through an interface.
- Packaging: Inno Setup on Windows; an `.app` in a DMG built, signed and notarized by
  `scripts/notarize-macos.sh` on the Mac.

**Tech stack:** Go 1.26 (module `github.com/c0ze/armarium`), Svelte 5 + Vite + vitest, `fyne.io/systray`,
`golang.org/x/sys` (registry, file locks), Inno Setup 6, `go-winres`, codesign/notarytool/stapler, GitHub
Actions.

**Spec:** `docs/superpowers/specs/2026-10-25-desktop-installers-design.md`. Read it first; this plan does not
repeat its reasoning.

## Global constraints

- `cmd/armarium` stays `CGO_ENABLED=0`, static, and imports no GUI package. Only `cmd/armarium-desktop` may
  use cgo, and only on macOS.
- No route weakens the Host allowlist, the CSRF rule (`writeAllowed`) or CORS. Setup and Settings routes go
  through `guard` like every other route.
- Installers need no administrator rights.
- macOS 13 or later; Windows 10 1809 or later, x64.
- Signing and notarization:
  - identity `Developer ID Application: Gand (7696W4CMNC)`;
  - `--keychain ~/Library/Keychains/login.keychain-db`;
  - notary profile `gand-notary`;
  - nothing from `~/apple-signing` in the repo.
- Asset names, with no leading `v` in the version: `Armarium-<X.Y.Z>-macOS.dmg`,
  `Armarium-Setup-<X.Y.Z>-x64.exe`.
- Managed config header text, verbatim:
  `# Written by Armarium. Quit Armarium before editing this file by hand; Armarium rewrites it when you change Settings.`
- Bundle id `com.gand.armarium`. Inno `AppId` `{{8B4E1C52-6F3A-4D0B-9E7A-2C5F1A9D3E61}`. Never change either
  after the first release.
- The `cors_origins` of a managed config are always exactly the five in `deploy/armarium.example.toml`.
- **Library names:**
  - Windows is case-insensitive, so the desktop exe is `Armarium.exe` and the CLI goes in `cli\armarium.exe`.
  - macOS volumes are case-insensitive too, so the CLI goes in `Armarium.app/Contents/Helpers/armarium`. The
    spec said `Contents/MacOS/armarium`, which would collide with `Contents/MacOS/Armarium`.
- Run heavy suites through `~/.local/bin/heavy-slot` (`make check`, full `go test -race ./...`, release
  builds). Targeted `go test -run` does not need it.
- UI and guide copy: plain language, one action per sentence, no jargon a non-technical owner wouldn't know
  (apply the `ux-design` and `unslop` skills).

## Review focus

1. **Tailscale-proxied requests arrive from `127.0.0.1`.** They must never count as "on this computer" for
   the native folder dialog. Owned by Task 5 (`TestFolderDialogRefusesProxiedLoopback`).
2. **A stale setup link opened after the password exists** shows "Armarium is already set up" with a link to
   log in, not an error page. Owned by Task 3 (API 410) and Task 7 (UI).
3. **Saving Settings with a port that's taken, or while a scan runs,** must leave the server running on the
   old config with a clear message. Owned by Task 9 (`TestApplyRollsBackWhenListenFails`) and Task 5 (409
   while scanning).
4. **A library folder that is unplugged, unreadable or refused by macOS privacy** gets a check-folder error
   that names the cause. Armarium still starts with that library configured. Owned by Task 5
   (`TestCheckFolder`).
5. **The machine's LAN address changes between starts** (DHCP). `public_url` and the allowed hosts follow the
   new address with no stale value in the file. Owned by Task 4 (`TestDeriveHomeFollowsAddressChange`).

---

## File structure

| File | Responsibility |
|---|---|
| `internal/server/server.go` (new) | `App`: open config + store + scanner, `Serve(ctx)`, `Close()`, scheduled scans (moved from `cmd/armarium/main.go`). |
| `internal/config/desktop.go` (new) | `Desktop` table, `Settings`, `Probe`, derivation into `Config`. |
| `internal/config/managed.go` (new) | `WriteManaged`: atomic write of a managed file. |
| `internal/config/hosts.go` | `HostAllowed` unchanged; new `LocalAddress(host string) bool` for the interface-address rule. |
| `internal/store/owner.go` (new) | Owner password hash in `meta`. |
| `internal/httpapi/setup.go` (new) | Setup code, `/api/setup*` routes. |
| `internal/httpapi/settings.go` (new) | `Host` interface, `/api/settings*`, `/api/password`, check-folder. |
| `internal/httpapi/pair.go` | Native Books (and Comics if supported) target allowed over HTTP. |
| `internal/desktop/` (new) | Paths, single-instance lock, log rotation, supervisor/reload, `Host` implementation, Tailscale CLI, folder dialog, open browser (per-OS files with build tags). |
| `cmd/armarium-desktop/` (new) | `main`, tray menu, login item (`login_darwin.go` with cgo `SMAppService`, `login_windows.go` with the Run key), embedded icons, `winres/`. |
| `packaging/icons/` (new) | `armarium.svg` source plus generated `Armarium.icns`, `armarium.ico`, tray PNGs; `scripts/make-icons.sh` regenerates them. |
| `packaging/macos/Info.plist` (new) | App bundle plist template (`__VERSION__` placeholder). |
| `packaging/windows/armarium.iss` (new) | Inno Setup script. |
| `web/src/views/Setup.svelte`, `web/src/views/Settings.svelte`, `web/src/lib/setup.ts` (new) | Setup wizard, Settings page, their pure logic. |
| `docs/install-mac.md`, `docs/install-windows.md` (new) | Plain-language guides. |

---

### Task 1: Move the server lifecycle into `internal/server`

**Files:**
- Create: `internal/server/server.go`, `internal/server/server_test.go`
- Modify: `cmd/armarium/main.go` (`open`, `serve`, `scheduledScans` move out; commands keep using `App`)

**Interfaces:**
- Produces:
  ```go
  type App struct { Cfg config.Config; Store *store.Store; Scanner *scan.Scanner; Log *slog.Logger; Version string }
  func Open(cfg config.Config, log *slog.Logger, syncLibraries bool) (*App, error) // MkdirAll data dir, store.Open, SyncLibraries (same error text as today), scanner
  func (a *App) Serve(ctx context.Context, opts ServeOptions) error                 // today's serve(): session reset, page remap, httpapi.New, scheduled scans, srv.Serve
  func (a *App) Close() error
  type ServeOptions struct { Host httpapi.Host; UI fs.FS; Ready func(addr string) } // Host nil for the CLI; Ready is called once listening
  ```
- `cmd/armarium` keeps its config path discovery (`--config`, `ARMARIUM_CONFIG`, `./armarium.toml`) and calls
  `config.Load` then `server.Open`.

- [ ] **Step 1: Write `TestServeAnswersHealthzAndStops`.** Open an `App` on a temp data dir with
  `Listen: "127.0.0.1:0"`. `Serve` in a goroutine; `Ready` reports the real address. `GET /healthz` returns
  `ok\n`. Cancelling ctx makes `Serve` return nil within 10 s.
- [ ] **Step 2:** `go test ./internal/server/ -run TestServe` fails (package missing).
- [ ] **Step 3: Implement** by moving code, not rewriting it. `httpapi.Server.Serve` gains a listener
  parameter, `ServeListener(ctx, ln net.Listener)`, so `Ready` can report `ln.Addr()`. `Serve(ctx)` keeps
  working via `net.Listen`.
- [ ] **Step 4:** `go test ./internal/server/ ./cmd/... ./internal/httpapi/` passes, and `go vet ./...` is
  clean.
- [ ] **Step 5: Commit** `refactor: move the server lifecycle into internal/server`.

### Task 2: Owner password in the database, and `armarium password`

**Files:**
- Create: `internal/store/owner.go`, `internal/store/owner_test.go`
- Modify: `internal/httpapi/session.go` (login and getSession use the effective hash),
  `internal/httpapi/server.go` (route), `internal/server/server.go` (session reset uses the effective hash),
  `cmd/armarium/main.go` + `commands.go` (new `password` command, usage text), `docs/deploy.md` ("Forgot the
  owner password?")
- Test: `internal/httpapi/password_test.go`

**Interfaces:**
- Produces:
  ```go
  // store
  func (s *Store) OwnerPassword(ctx context.Context) (string, error)        // meta key "owner_password_hash"; "" when unset
  func (s *Store) SetOwnerPassword(ctx context.Context, hash string) error  // in one tx: set the hash, DELETE FROM session, set meta "password" to the new fingerprint (same sha256 hex ResetSessionsIfPasswordChanged uses)
  // httpapi
  func (s *Server) passwordHash(ctx context.Context) (hash string, fromConfig bool, err error) // Cfg.PasswordHash wins when non-empty
  // PUT /api/password, Admin: {"current": "...", "new": "..."} → 200 + a fresh session cookie
  ```
- `getSession` adds `"passwordFromConfig": bool` and `"setupOpen": bool` (true while no effective hash
  exists). The web `Session` type gets both fields.
- `armarium password` reads the new password twice from stdin (hidden, like `hash-password`), needs at least
  8 characters, and calls `SetOwnerPassword`. When `password_hash` is set in the config it refuses with:
  `password_hash in the config wins over a stored password; remove it from armarium.toml first`.

- [ ] **Step 1: Write the store tests.**
  - `TestOwnerPasswordRoundTrip`: unset → `""`; set → read back.
  - `TestSetOwnerPasswordEndsSessions`: add 2 sessions, set the password, `SessionValid` is false for both.
- [ ] **Step 2: Write the HTTP tests.**
  - `TestLoginWithStoredPassword`: config hash empty, stored hash set → login 200.
  - `TestConfigPasswordWins`: both set → only the config password logs in.
  - `TestChangePassword`:
    - wrong `current` → 401 (and it counts against the limiter);
    - new password under 8 characters → 400;
    - success → old cookie invalid, new cookie valid;
    - with a config hash → 409 `{"error":"password_hash is set in armarium.toml"}`.
  - `TestChangePasswordCSRF`: a foreign Origin → 403; no session → 401.
- [ ] **Step 3:** The new tests fail.
- [ ] **Step 4: Implement.** Use the existing `loginLimiter` for both login and change-password checks.
- [ ] **Step 5:** `go test ./internal/store/ ./internal/httpapi/ ./cmd/...` passes.
- [ ] **Step 6: Commit** `feat: owner password stored in the database; armarium password`.

### Task 3: Setup mode

**Files:**
- Create: `internal/httpapi/setup.go`, `internal/httpapi/setup_test.go`
- Modify: `internal/httpapi/server.go` (routes, `SetupCode` field), `internal/server/server.go` (generate
  the code and log the link when setup is open)

**Interfaces:**
- Consumes: Task 2 `passwordHash`, `SetOwnerPassword`; Task 5's `Host` (nil-safe: settings are applied only
  when `Host != nil`).
- Produces:
  - `httpapi.Server.setup *setupState`: holds the code, discards it once a password exists, and is safe for
    concurrent use.
  - `func NewSetupCode() string`: 32 bytes from crypto/rand, base64url.
  - The code travels in the header `X-Armarium-Setup` on every `/api/setup*` call. A custom header forces a
    CORS preflight for cross-site callers, on top of the JSON-only and own-Origin rules.
  - Routes, all `Public` access in `guard`, so Host and CSRF checks still apply:
    - `GET /api/setup` → `{"desktop": bool, "managed": bool, "settings": Settings|null,
      "tailscale": {"available": bool, "name": string}}`;
    - `POST /api/setup/finish`, body `{"password": string, "settings": Settings|null, "startAtLogin": bool}`
      → 200 with a session cookie, the same cookie attributes as `login`;
    - `POST /api/setup/check-folder` and `POST /api/setup/folder`: the same handlers as Task 5's settings
      variants, gated by the code instead of the session.
  - Answers:
    - no or wrong code → 401 `{"error":"setup code required"}`, each attempt reserved on `s.logins`
      first;
    - a password already exists → 410 `{"error":"Armarium is already set up"}`.
  - `server.App.Serve` logs `Finish setup: <url>/setup#code=<code>` at Info. `<url>` is
    `http://127.0.0.1:<port>` when listening on loopback or all interfaces, otherwise the listen address.
    `ServeOptions.Ready` receives that URL.

- [ ] **Step 1: Write `TestSetupGating`**, a table over `env.do`:
  - no header → 401;
  - wrong code → 401, and the 6th wrong try within a minute → 429;
  - a foreign Host → 421;
  - POST with `Origin: https://evil.example` → 403;
  - POST with `Content-Type: text/plain` → 415;
  - the right code with a password configured → 410.
- [ ] **Step 2: Write `TestSetupFinishSetsPasswordAndLogsIn`.**
  - Env with an empty config hash; finish with a valid code and an 11-character password → 200 with a
    cookie.
  - `GET /api/session` with that cookie → admin.
  - A second finish → 410.
  - `/api/login` with the new password → 200.
- [ ] **Step 3: Write `TestSetupFinishRejectsShortPassword`** (400, code still valid) and
  `TestSetupFinishAppliesSettingsThroughHost`, which uses the fake `Host` from Task 5's test helpers and
  asserts `Apply` received the settings.
- [ ] **Step 4:** The tests fail.
- [ ] **Step 5: Implement.** Compare with `subtle.ConstantTimeCompare`, and order the finish handler so a
  failing `Host.Apply` leaves no password set. The `newEnv` helper gains an option for an empty password.
- [ ] **Step 6:** `go test ./internal/httpapi/ ./internal/server/` passes.
- [ ] **Step 7: Commit** `feat: first-run setup with a one-time code`.

### Task 4: Managed config and network derivation

**Files:**
- Create: `internal/config/desktop.go`, `internal/config/managed.go`, `internal/config/desktop_test.go`
- Modify: `internal/config/config.go` (`Desktop *Desktop \`toml:"desktop"\``, `AllowLocalAddresses bool \`toml:"-"\``;
  `Load` calls `deriveDesktop` before `validate` when `Desktop != nil`), `internal/config/hosts.go`,
  `internal/httpapi/middleware.go` (`hosts` also accepts `config.LocalAddress(host)` when
  `Cfg.AllowLocalAddresses`)

**Interfaces:**
- Produces:
  ```go
  type Desktop struct { Network string `toml:"network"`; Tailscale bool `toml:"tailscale"`; Port int `toml:"port"` } // "local" | "home"; Port 0 → 8580
  type Settings struct { // JSON shape shared by setup, Settings and the desktop app
      Libraries  []Library `json:"libraries"`
      Network    string    `json:"network"`
      Tailscale  bool      `json:"tailscale"`
      Port       int       `json:"port"`
      PublicURL  string    `json:"publicUrl"`  // advanced: own HTTPS address, "" for none
      ExtraHosts []string  `json:"extraHosts"` // advanced
  }
  type Probe interface { LANIPv4() (netip.Addr, bool); InterfaceAddrs() []netip.Addr; LocalHostName() string; TailscaleName() string }
  var SystemProbe Probe // net.Dial("udp","192.0.2.1:9") for the primary address (no packet sent), net.InterfaceAddrs, os.Hostname → "<short>.local", `tailscale status --json` Self.DNSName without the trailing dot (see Task 9 for the CLI path)
  var probe Probe = SystemProbe // tests swap it
  func (c Config) Settings() Settings                      // what the managed file holds
  func WriteManaged(path string, s Settings, dataDir string) error // header line, data_dir, [desktop], public_url/allowed_hosts only when advanced fields are set, [[library]] blocks; temp file + rename, 0600
  func LocalAddress(host string) bool                      // host (no port, brackets stripped) is loopback or equals a current interface address; interface list cached 10 s
  ```
- **Derivation** (`deriveDesktop`), exactly the spec's table:

  | | local | home | tailscale = true (adds to either) |
  |---|---|---|---|
  | `Listen` | `127.0.0.1:<port>` | `:<port>` | nothing |
  | `AllowedHosts` | `localhost`, `127.0.0.1`, `::1` | also `LocalHostName()` and `AllowLocalAddresses = true` | also `TailscaleName()` |
  | `PublicURL` | none | `http://<LANIPv4>:<port>` | `https://<TailscaleName>` (replaces home's) |
  | `TrustedProxies` | none | none | `127.0.0.1`, `::1` |

  - `CORSOrigins` is always the five example origins.
  - Explicit `PublicURL` and `ExtraHosts` from the file are appended; an explicit `PublicURL` wins over the
    derived one.
  - When the Tailscale name is empty (Tailscale stopped), it is skipped silently and Admin's existing
    `public_url` warning covers it.

- [ ] **Step 1: Write `TestDeriveLocal`, `TestDeriveHome` and `TestDeriveTailscale`** with a fake probe
  (LAN `192.168.1.20`, name `studio.local`, tailnet `studio.tail1234.ts.net`). Assert every row above.
- [ ] **Step 2: Write `TestDeriveHomeFollowsAddressChange`** (Review focus 5): the same file loaded with the
  probe returning `.20`, then `.37`. `PublicURL` changes, and the file on disk is unchanged byte for byte.
- [ ] **Step 3: Write `TestWriteManagedRoundTrip`.**
  - Write the settings with 2 libraries (one Calibre), then `Load` → `Settings()` equals the input.
  - The first line is the header text.
  - On Unix the file mode is 0600.
  - A crash mid-write never leaves a half file: write to a read-only directory → error, and the old file is
    intact.
- [ ] **Step 4: Write `TestLocalAddress`** (loopback v4/v6, an interface address from the fake list, a
  public IP, a hostname → false) and the middleware test `TestHostsAcceptsOwnAddressOnlyWhenManaged`.
- [ ] **Step 5:** The tests fail.
- [ ] **Step 6: Implement.** `WriteManaged` uses `toml.NewEncoder` for values. The header and section order
  are written by hand, so the file reads well.
- [ ] **Step 7:** `go test ./internal/config/ ./internal/httpapi/` passes.
- [ ] **Step 8: Commit** `feat(config): managed desktop config with derived network settings`.

### Task 5: Settings API, check-folder, folder dialog

**Files:**
- Create: `internal/httpapi/settings.go`, `internal/httpapi/settings_test.go`, `internal/httpapi/fakehost_test.go`
- Modify: `internal/httpapi/server.go` (`Host Host` field, routes)

**Interfaces:**
- Produces:
  ```go
  type Host interface {
      Apply(ctx context.Context, ch Change) error       // validate, write the managed config, apply store renames/removals, schedule a reload; must not return before validation is done
      ChooseFolder(ctx context.Context) (string, error) // native dialog on this computer; ErrCancelled when dismissed
      Tailscale(ctx context.Context) TailscaleStatus
  }
  type Change struct { Settings config.Settings; Renames map[string]string; Removals []string; StartAtLogin *bool }
  type TailscaleStatus struct { Available bool `json:"available"`; Name string `json:"name"`; Serving bool `json:"serving"`; Conflict string `json:"conflict"` }
  var ErrCancelled = errors.New("cancelled")
  ```
- Routes:
  - `GET /api/settings` (Admin) → `{"editable": bool, "settings": Settings, "derived": {"listen", "allowedHosts",
    "publicUrl"}, "passwordFromConfig": bool, "snippet": string}`.
    - `editable` = `Host != nil && Cfg.Desktop != nil`.
    - `snippet` is the TOML an unmanaged owner would paste for the current values.
  - `PUT /api/settings` (Admin): body `Change`.
    - 409 `{"error":"Wait for the scan to finish"}` while `Scanner.Status().Running`;
    - 403 when not editable;
    - 400 with the config's validation message;
    - 200 `{"reloadUrl": "http://127.0.0.1:<port>/settings"}`.
  - `POST /api/settings/check-folder` (Admin): `{"path"}` → `{"ok": bool, "kind": "books"|"comics"|"",
    "calibre": bool, "error": string}`.
    - The guess reads at most 200 directory entries, breadth-first, at most 3 levels deep.
    - Kind is `comics` when `.cbz` and `.cbr` outnumber `.epub`, `.pdf`, `.mobi` and `.azw3`.
    - `metadata.db` at the root means Calibre, kind books.
    - Errors, in this order: `That folder doesn't exist.`, `That's a file, not a folder.`,
      `Armarium isn't allowed to read that folder.` (EACCES/EPERM; macOS privacy refusals land here),
      `That folder can't be read right now.` (anything else).
  - `POST /api/settings/folder` (Admin) → `{"path"}`, or 204 when cancelled.
    - Allowed only when `Host != nil` and the request is local: peer is loopback, the Host is
      `localhost`/`127.0.0.1`/`[::1]` (any port), and there is no `X-Forwarded-For`. Otherwise 403
      `{"error":"Choose a folder on the computer running Armarium"}`.

- [ ] **Step 1: Write the fake host.** `fakeHost` records the `Change`, returns a configurable error, and
  returns a fixed folder path.
- [ ] **Step 2: Write `TestSettingsReadOnlyWithoutHost`** (editable false, snippet contains `allowed_hosts`)
  and `TestSettingsPutAppliesChange`.
- [ ] **Step 3: Write `TestSettingsPutRefusedWhileScanning`** and `TestSettingsNeedsOwner` (token → 403,
  anonymous → 401, foreign Origin → 403).
- [ ] **Step 4: Write `TestCheckFolder`** (Review focus 4):
  - a missing path, a file, an unreadable dir (chmod 000, skipped as root), a comics dir, an EPUB dir and a
    Calibre dir each give the expected `ok`, `kind`, `calibre` and `error`;
  - a 5,000-entry dir returns in under a second.
- [ ] **Step 5: Write `TestFolderDialogLocalOnly` and `TestFolderDialogRefusesProxiedLoopback`** (Review
  focus 1):
  - `remote 127.0.0.1:5000` with Host `localhost:8580` → 200;
  - the same with `X-Forwarded-For: 100.64.0.7` → 403;
  - the same with Host `studio.tail1234.ts.net` → 403;
  - remote `192.168.1.5` → 403.
- [ ] **Step 6:** The tests fail.
- [ ] **Step 7: Implement.** Store renames and removals are applied by the `Host`, not by the handler; the
  handler only validates shape (renames refer to existing names, removals are not also renamed).
- [ ] **Step 8:** `go test ./internal/httpapi/` passes.
- [ ] **Step 9: Commit** `feat: settings API, folder checks and the native folder dialog hook`.

### Task 6: Pair the native Books app over plain HTTP

**Files:**
- Modify: `internal/httpapi/pair.go`, `internal/httpapi/pair_test.go`, `web/src/lib/pair.ts`,
  `web/src/lib/pair.test.ts`, `web/src/views/PairPhone.svelte`

**Interfaces:**
- `pairReaders` gains `{"books-app", "Skrivist Books app (iPhone, iPad, Android)", "<link base>", Native: true}`.
  - `pairReader` gains `Native bool \`json:"native"\``.
  - Native targets need no CORS entry. `canPair(reader, plainHttp)` becomes
    `reader.desktop || reader.native || !plainHttp`.
- **First action of this task:** read `skrivist-native` `ios/SkrivistBooks/Sources/AddSourceFlow.swift` and
  `android/books/.../AddSourceFlow.kt`. Record in the commit message the exact link form the in-app "Scan QR
  code" accepts.
  - Use the narrowest form it accepts that does not open the hosted web reader when scanned by the phone's
    camera app, for example a `skrivist-books://pair#opds=` link if the apps register that scheme.
  - If the only accepted form is `https://books.skriv.ist/#opds=…`, the QR for this target is shown only
    with the note "Scan this from the Books app: Add library → Scan QR code".
  - Add a Comics app target only if the Comics app accepts the same; otherwise leave a one-line note in the
    commit message.

- [ ] **Step 1: Write the tests.**
  - `TestPairNativeTargetOverHTTP`: `/api/pair` lists `books-app` with `native: true` and empty `missing`.
  - In vitest, `canPair({native: true}, true) === true` and `canPair({}, true) === false`.
- [ ] **Step 2:** The tests fail; implement; the tests pass (`go test ./internal/httpapi/ -run Pair`,
  `cd web && npx vitest run src/lib/pair.test.ts`).
- [ ] **Step 3: Commit** `feat(pair): pair the Skrivist Books app over plain HTTP at home`.

### Task 7: Web setup wizard

**Files:**
- Create: `web/src/lib/setup.ts`, `web/src/lib/setup.test.ts`, `web/src/views/Setup.svelte`
- Modify: `web/src/lib/api.ts` (setup and settings calls, `Settings` type mirroring Go), `web/src/lib/router.svelte.ts`
  (`'setup'` and `'settings'` routes), `web/src/App.svelte` (when `session.setupOpen`, every route shows
  `Setup`; `/setup` while set up shows the "already set up" panel)

**Interfaces:**
- `setup.ts`:
  ```ts
  export function readCode(hash: string): string | null           // "#code=…" → code
  export function passwordProblem(a: string, b: string): string | null // "Use at least 8 characters." | "The two passwords don't match." | null
  export function defaultName(path: string): string               // last path segment, works for / and \ paths
  export function settingsFor(libs: LibraryDraft[], network: 'local'|'home', tailscale: boolean, port: number): Settings
  ```
- On load, `Setup.svelte` takes the code from `location.hash`, keeps it in `sessionStorage`
  (`armarium.setup`), and removes it from the URL with `history.replaceState`.
- **No code:** the page says
  `Open Armarium from its menu-bar icon (Mac) or tray icon (Windows) to finish setting it up. On a server, the setup link is in Armarium's log.`
- **Finish** has the "Start Armarium when you log in" checkbox (desktop only, default on; sent as
  `startAtLogin`). After a 200 it starts a scan (`POST /api/scan`) and goes to the wall.
- **Steps** as in the spec: Password → Libraries → Who can read → Finish. Unmanaged servers (`managed:
  false`) see only Password, then the TOML guidance.
- The Libraries step:
  - shows "Choose folder…" only when `desktop: true`, and always shows a path field;
  - runs check-folder on each path and shows its `error` text under the row;
  - lets the owner override the kind with a Books/Comics segmented control.
- **Tailscale:** the option is disabled with "Tailscale isn't installed on this computer." when
  `available: false`. When `conflict` is set, it shows the conflict text and the manual command.
- **Already set up** (410 from any setup call): "Armarium is already set up." plus a "Log in" button.
- Follow `DESIGN.md`: one amber button only, for the final "Start reading"; every other button is a neutral
  outline.

- [ ] **Step 1: Write the vitest cases** for all four functions, including Windows `C:\Books\Comics\` →
  `Comics` and `/` → `Library`.
- [ ] **Step 2:** The tests fail; implement `setup.ts`; the tests pass.
- [ ] **Step 3: Build `Setup.svelte`** and the router and App wiring. `cd web && npm run check` is clean.
- [ ] **Step 4: Manual check.**
  - Run `bin/armarium serve` against a config with no password and open the logged link. Walk all steps at
    375 px and desktop width.
  - Take a screenshot of each step into the scratchpad; these become the guide's images in Task 12.
- [ ] **Step 5: Commit** `feat(web): first-run setup wizard`.

### Task 8: Web Settings and change password

**Files:**
- Create: `web/src/views/Settings.svelte`
- Modify: `web/src/views/Sidebar.svelte` (Settings entry for admin), `web/src/views/Admin.svelte` (a link
  near the top), `web/src/lib/api.ts`

**Interfaces:**
- Consumes: Task 5 routes, Task 2 `PUT /api/password`, the Task 7 library row and network components. Extract
  the shared parts into `web/src/views/LibraryRows.svelte` and `web/src/views/NetworkChoice.svelte` rather
  than duplicating them.
- **Not editable:** show the derived values read-only and the `snippet` in a copyable block.
- **Removing a library:** the confirm step reads `Delete "<name>" and its <n> items from Armarium? Your files
  stay where they are. Reading progress for these items is lost.` `<n>` comes from `/api/libraries` counts;
  add `items` to that response if it isn't there.
- **After a 200:** show "Restarting Armarium…" and poll `GET <reloadUrl origin>/healthz` (no-cors, every
  500 ms, up to 15 s), then `location.assign(reloadUrl)`.
- **Change password:** hidden behind "Change password" when `passwordFromConfig`. In that case the page says
  `The password is set in armarium.toml (password_hash).`

- [ ] **Step 1:** `npm run check` and `npm test` pass after building the page. Re-run the Task 7 tests after
  the shared components are extracted.
- [ ] **Step 2: Manual check** against `armarium-desktop` once Task 9 lands: rename a library, change the
  port, remove a library.
- [ ] **Step 3: Commit** `feat(web): settings page and password change`.

### Task 9: Desktop core (`internal/desktop`)

**Files:**
- Create:
  - `internal/desktop/paths.go`, `paths_darwin.go`, `paths_windows.go`
  - `lock.go`, `lock_unix.go`, `lock_windows.go`
  - `logfile.go`
  - `supervisor.go` (also implements `httpapi.Host`)
  - `tailscale.go`
  - `dialog_darwin.go`, `dialog_windows.go`
  - `open_darwin.go`, `open_windows.go`
  - `desktop_test.go`
  - `//go:build !darwin && !windows` stubs, so `go vet ./...` and tests work on Linux CI

**Interfaces:**
- Produces:
  ```go
  type Paths struct { Config, Data, Logs string }
  func DefaultPaths() (Paths, error)               // spec table: ~/Library/Application Support/Armarium{,/data}, ~/Library/Logs/Armarium; %LOCALAPPDATA%\Armarium{,\data,\logs}
  func Lock(dir string) (release func(), err error) // ErrRunning when held by another process
  var ErrRunning = errors.New("armarium is already running")
  func NewLogFile(dir string, maxBytes int64, keep int) (io.WriteCloser, error) // armarium.log, rotated to armarium.1.log … keep; 5 MiB, keep 3
  type Supervisor struct {
      Paths   Paths
      Log     *slog.Logger
      UI      fs.FS
      Version string
      OnState func(State)              // tray status updates
      LoginItem interface{ Enabled() bool; SetEnabled(bool) error }
  }
  type State struct { Running bool; URL string; SetupURL string; Err string; Scanning bool }
  func (s *Supervisor) Run(ctx context.Context) error // first run writes a default managed config (network "local", no libraries); then loop: config.Load → server.Open → Serve(ServeOptions{Host: s}) until ctx done or a reload is requested
  func (s *Supervisor) Apply(ctx context.Context, ch httpapi.Change) error // the httpapi.Host method
  func (s *Supervisor) ChooseFolder(ctx context.Context) (string, error)
  func (s *Supervisor) Tailscale(ctx context.Context) httpapi.TailscaleStatus
  func OpenBrowser(url string) error // `open` on macOS; `rundll32 url.dll,FileProtocolHandler` on Windows
  ```
- **`Apply` order:**
  1. validate the new settings with `config.Validate`, exported if needed;
  2. if the port changes, test it with `net.Listen` and close it again;
  3. run store renames, then removals (`RemoveLibrary`);
  4. `WriteManaged`;
  5. set the login item when `StartAtLogin != nil`;
  6. Tailscale serve on or off when that changes;
  7. request a reload 300 ms later, so the HTTP response flushes first.

  When the reload fails to listen, the supervisor writes the previous file back, starts on it, and puts the
  failure into `State.Err`.
- **Tailscale CLI path:**
  - `tailscale` on PATH;
  - else `/Applications/Tailscale.app/Contents/MacOS/Tailscale` on macOS;
  - else `%ProgramFiles%\Tailscale\tailscale.exe` on Windows.
- **Tailscale commands:**
  - Name: `status --json`, field `Self.DNSName`.
  - Serve on: `serve --bg --https=443 http://127.0.0.1:<port>`.
  - Ownership: `serve status --json`; Armarium owns the entry when the only https:443 handler is `/` proxying
    to `http://127.0.0.1:<port>`.
  - Off: `serve --https=443 off`, only when Armarium owns it.
  - Anything else on https:443 → `TailscaleStatus.Conflict`:
    `Tailscale already serves something else on this computer. Armarium left it alone.`
- **X-Forwarded-Proto check:** verify on this Mac whether `tailscale serve` sends it. If not, `isHTTPS` also
  treats a request as HTTPS when its Host equals the managed Tailscale name and the peer is loopback. Note
  the result in the commit message.
- **Folder dialog:**
  - macOS: `osascript -e 'tell me to activate' -e 'POSIX path of (choose folder with prompt "Choose a folder for Armarium")'`.
    Exit status 1 with "User canceled" → `ErrCancelled`.
  - Windows: `powershell -NoProfile -STA -WindowStyle Hidden` running a `FolderBrowserDialog` with a topmost
    owner form, launched with `CREATE_NO_WINDOW`. An empty result → `ErrCancelled`.

- [ ] **Step 1: Write `TestDefaultPathsEnv`** (Windows paths from `LOCALAPPDATA`, macOS from `HOME`, with
  per-OS tests skipped elsewhere) and `TestLockExclusive` (a second `Lock` in the same dir →
  `ErrRunning`; after release it succeeds).
- [ ] **Step 2: Write `TestLogFileRotates`.** Write 3× maxBytes and check the file count and names.
- [ ] **Step 3: Write `TestSupervisorFirstRunWritesManagedConfig`.** Temp paths; `Run` with `Listen` forced
  to port 0 through `Desktop.Port` = a free port; `OnState` reports `SetupURL` containing `/setup#code=`; the
  config file exists with network `local`.
- [ ] **Step 4: Write `TestApplyAddsLibraryAndReloads`.**
  - Apply settings with one comics folder.
  - The state goes Running → Running on the same URL.
  - `/api/libraries`, called with the session from setup finish, lists it.
- [ ] **Step 5: Write `TestApplyRenameKeepsProgress` and `TestApplyRemovalDeletes`.** Use store fixtures
  with progress rows.
- [ ] **Step 6: Write `TestApplyRollsBackWhenListenFails`** (Review focus 3): occupy a port, apply a change
  to it → error before writing. Then simulate a reload failure by occupying the port after the check (test
  hook) → the old file is restored, `State.Err` is set, and the server is running.
- [ ] **Step 7: Write `TestTailscaleOwnership`.** Parse captured `serve status --json` fixtures (empty, ours,
  someone else's on `/`, ours plus an extra path) into owned/conflict. Capture the real JSON shape from
  `tailscale serve status --json` on this Mac if Tailscale is installed; otherwise from the Tailscale docs.
- [ ] **Step 8:** The tests fail; implement until `go test ./internal/desktop/` passes on macOS, and
  `GOOS=windows go vet ./internal/desktop/` and `GOOS=linux go vet ./...` are clean.
- [ ] **Step 9: Commit** `feat(desktop): supervisor, managed config apply, paths, lock, logs, Tailscale`.

### Task 10: `cmd/armarium-desktop`, tray menu, login item, icons

**Files:**
- Create:
  - `cmd/armarium-desktop/main.go`, `tray.go`
  - `login_darwin.go` + `login_darwin.m` (cgo `SMAppService.mainAppService` register/unregister/status),
    `login_windows.go` (`HKCU\Software\Microsoft\Windows\CurrentVersion\Run`, value `Armarium` =
    `"<exe path>"`), `login_other.go`
  - `icons.go` (`//go:embed`)
  - `winres/winres.json`
  - `packaging/icons/armarium.svg` (from `web/public/favicon.svg`, on a 1024 canvas with 10% padding)
  - `packaging/icons/tray-template.svg` (a monochrome two-book glyph)
  - `scripts/make-icons.sh`, plus the generated `packaging/icons/{Armarium.icns, armarium.ico, tray-mac.png,
    tray-mac@2x.png, tray-win.ico}`
- Modify: `Makefile` (`desktop` target: native `go build ./cmd/armarium-desktop`), `go.mod`
  (`fyne.io/systray`, `golang.org/x/sys` direct)

**Interfaces:**
- Consumes: Task 9 `Supervisor`, `Lock`, `DefaultPaths`, `NewLogFile`, `OpenBrowser`.
- **`main` flow:**
  1. `DefaultPaths` and `Lock(Paths.Data)`. On `ErrRunning`, read the config's port, `OpenBrowser`, exit 0.
  2. Logs go to `NewLogFile`.
  3. `systray.Run(onReady, onExit)`.
  4. `onReady` builds the menu and starts `Supervisor.Run` in a goroutine.
  5. The first `State` with a `SetupURL` opens the browser on it, once per process.
- **Menu,** with fixed copy:
  - `Open Armarium`
  - `Pair a phone…` (opens `<URL>/admin#pair`; make Admin scroll to the pair section on `#pair`)
  - `Settings…` (`<URL>/settings`)
  - a disabled status item: `Armarium is running` / `Scanning your library…` / `Finish setup in your
    browser` / `Stopped: <Err>`
  - a separator
  - `Start at login` (checkbox)
  - `Show log folder`
  - a separator
  - `Quit Armarium`
- **Icons:** macOS uses `systray.SetTemplateIcon(trayMac2x, trayMac2x)`; Windows uses
  `systray.SetIcon(trayWinIco)`; the tooltip is `Armarium`.
- Version via `-ldflags "-X main.version=…"`, like the CLI.
- **Windows build:** `-H windowsgui`, and `go run github.com/tc-hib/go-winres@v0.3.3 make --in
  cmd/armarium-desktop/winres/winres.json --out cmd/armarium-desktop/rsrc` before the build. The generated
  `.syso` files are git-ignored.

- [ ] **Step 1:** `scripts/make-icons.sh` produces the icon files (`rsvg-convert`, `iconutil`, `magick`).
  Commit the outputs, so CI never needs these tools.
- [ ] **Step 2: Build on this Mac:** `CGO_ENABLED=1 go build -o bin/armarium-desktop ./cmd/armarium-desktop`.
  Run it from a terminal and check:
  - the menu-bar icon appears;
  - the browser opens setup;
  - Quit stops the server, so `curl 127.0.0.1:8580/healthz` fails afterwards;
  - a second launch opens the browser and exits.
- [ ] **Step 3: Cross-check Windows:**
  `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "-H windowsgui" ./cmd/armarium-desktop`
  succeeds.
- [ ] **Step 4: Commit** `feat: Armarium desktop app with tray and menu-bar icon`.

### Task 11: macOS app bundle, DMG, signing and notarization

**Files:**
- Create: `packaging/macos/Info.plist`
- Modify: `scripts/notarize-macos.sh`, `docs/releasing.md`, `Makefile` (`mac-app` target: an unsigned local
  bundle for testing)

**Interfaces:**
- **`Info.plist`:**
  - `CFBundleIdentifier com.gand.armarium`, `CFBundleName Armarium`, `CFBundleExecutable Armarium`;
  - `CFBundleShortVersionString`/`CFBundleVersion` = `__VERSION__`, without the `v`;
  - `LSUIElement true`, `LSMinimumSystemVersion 13.0`, `CFBundleIconFile Armarium`;
  - `NSDocumentsFolderUsageDescription`, `NSDownloadsFolderUsageDescription`,
    `NSDesktopFolderUsageDescription`, `NSRemovableVolumesUsageDescription`,
    `NSNetworkVolumesUsageDescription`, each reading: `Armarium reads the books and comics in the folders
    you choose.`
- **Bundle layout:** `Contents/MacOS/Armarium` (desktop, universal), `Contents/Helpers/armarium` (CLI,
  universal, `CGO_ENABLED=0`), `Contents/Resources/Armarium.icns`.
- **Script steps:**
  1. Keep the existing guards (clean tree, the release target commit, the ssh keychain switch, unlocking only
     after builds).
  2. Build the desktop binary per architecture with `CGO_ENABLED=1 GOARCH=<arch>
     CC="clang -arch <x86_64|arm64>" MACOSX_DEPLOYMENT_TARGET=13.0`, then `lipo`.
  3. Sign, inside-out:
     `codesign --force --timestamp --options runtime --keychain "$keychain" -s "$identity"` with
     `--identifier com.gand.armarium.cli` on `Contents/Helpers/armarium`, then with no identifier on the app.
  4. Verify: `codesign --verify --deep --strict --verbose=2`.
  5. Notarize the app: `ditto -c -k --keepParent` → `notarytool submit --keychain-profile "$profile"
     --keychain "$keychain" --wait`. Then `stapler staple` on the app, and `spctl -a -t exec -vv` must show
     `source=Notarized Developer ID`.
  6. Build the DMG (`hdiutil create -volname "Armarium <X.Y.Z>"` from a folder holding the app and an
     `Applications` symlink). Sign it, notarize it, staple it, then
     `spctl -a -t open --context context:primary-signature -vv`.
  7. Output `Armarium-<X.Y.Z>-macOS.dmg`. The CLI-only DMG and `START-HERE.txt` are gone.
- Add `--local` for testing on a dirty tree. It builds and signs but skips the release-commit check, and
  writes the DMG under the given output dir with a `-local` suffix. It still notarizes, so the real
  Gatekeeper path gets tested.

- [ ] **Step 1:** Write the plist and the `make mac-app` target. Open the unsigned app locally and check it
  launches as a menu-bar-only app.
- [ ] **Step 2: Run** `heavy-slot scripts/notarize-macos.sh v0.6.0-dev.1 <scratchpad>/mac --local`
  (escalated: keychain and network). Expected: notarization `Accepted`, both `spctl` checks pass.
- [ ] **Step 3: Real install test.** Mount the DMG, drag the app to `/Applications`, and launch it from
  Finder. Gatekeeper must not complain.
  - Go through setup with a test library under `~/Documents`; the macOS privacy prompt names Armarium.
  - Scan, then read a book.
  - Start at login: log out and in, or check the `SMAppService` status in the menu.
  - Quit and relaunch.
  - Install a newer `-local` build over it; settings are kept.
  - Delete the app; the login item disappears from System Settings → General → Login Items.
  - Record each result in the PR description.
- [ ] **Step 4:** Update `docs/releasing.md` step 2 to the new output.
- [ ] **Step 5: Commit** `feat(release): signed, notarized Armarium.app DMG`.

### Task 12: Windows installer and release workflow

**Files:**
- Create: `packaging/windows/armarium.iss`
- Modify: `.github/workflows/release.yml`, `.github/workflows/ci.yml` (build the desktop binary for
  windows/amd64 and run `go vet` for windows and darwin, so it can't rot), `docs/releasing.md`, `.gitignore`

**Interfaces:**
- **`armarium.iss`:**
  - `AppId={{8B4E1C52-6F3A-4D0B-9E7A-2C5F1A9D3E61}`, `AppName=Armarium`, `AppPublisher=Gand`,
    `AppVersion` from `/DMyAppVersion=X.Y.Z`;
  - `DefaultDirName={localappdata}\Programs\Armarium`, `PrivilegesRequired=lowest`,
    `DisableProgramGroupPage=yes`, `ArchitecturesAllowed=x64compatible`,
    `ArchitecturesInstallIn64BitMode=x64compatible`, `MinVersion=10.0.17763`;
  - `AppMutex=ArmariumDesktopRunning`. The desktop app creates this named mutex on Windows, in
    `lock_windows.go`, next to the file lock;
  - `CloseApplications=yes`, `SetupIconFile=..\icons\armarium.ico`,
    `UninstallDisplayIcon={app}\Armarium.exe`, `OutputBaseFilename=Armarium-Setup-{#MyAppVersion}-x64`;
  - Files: `Armarium.exe` → `{app}`, `armarium.exe` → `{app}\cli`, plus `LICENSE` and `README.md`;
  - an `[Icons]` Start menu entry; `[Run]` "Launch Armarium" (`postinstall nowait skipifsilent`).
- **Uninstall:**
  - `[UninstallRun]` closes the app via the mutex check.
  - `[Code]` removes the Run value, then asks `Also delete your Armarium settings and reading progress?`
    (default No). On Yes it deletes `{localappdata}\Armarium`.
- **release.yml:** a new `windows-installer` job on `windows-latest`:
  1. checkout, setup-go, setup-node;
  2. web build;
  3. go-winres;
  4. build `Armarium.exe` (`-H windowsgui`) and the CLI;
  5. `iscc` (install with `choco install innosetup -y --no-progress` when `iscc` is missing);
  6. a signing step guarded by `if: vars.WINDOWS_SIGNING == 'azure'`, using
     `azure/artifact-signing-action` on both exes before packaging and on the setup exe after. Add a
     commented-out `permissions: id-token: write` with a note;
  7. upload the artifact.

  The `release` job gets `needs: windows-installer`, downloads it into `dist/` before `checksums.txt`, and
  attaches the setup exe to the draft.

- [ ] **Step 1:** Write the `.iss` and the workflow changes. Validate with `actionlint` if it is installed;
  otherwise run a `workflow_dispatch` on the branch: add a temporary
  `on: workflow_dispatch: inputs: dry_run` path that skips Docker push and the release, or run the
  `windows-installer` job alone via a `workflow_call` split. The goal is to see `iscc` succeed in CI before
  merge.
- [ ] **Step 2:** Download the CI-built installer artifact into the scratchpad. Record its SHA-256 for the
  Windows test.
- [ ] **Step 3: Real Windows test,** on the machine Arda provides:
  - SmartScreen screen → More info → Run anyway;
  - install with no UAC prompt;
  - first launch opens setup; add a library;
  - "Devices at home": the firewall prompt, then pair the Books app from a phone;
  - reboot → Armarium starts at login;
  - upgrade with a newer build; settings are kept;
  - uninstall with the "keep" answer, then reinstall and the settings are still there;
  - uninstall with the "delete" answer.
  - Record the results in the PR description.
- [ ] **Step 4: Commit** `feat(release): Windows installer (Inno Setup), signing step ready but off`.

### Task 13: Guides, README, deploy docs

**Files:**
- Create: `docs/install-mac.md`, `docs/install-windows.md`, `docs/images/install/*.png` (the screenshots
  from Tasks 7, 11 and 12)
- Modify: `README.md` (quick start first: Mac/Windows download links to the latest release, then "Docker, NAS
  and Linux" → `docs/deploy.md`), `docs/deploy.md` (setup link via `docker logs armarium`,
  `armarium password`, managed vs hand-written config), `docs/clients.md` (pairing the Books app at home over
  HTTP)

**Content (each guide):**
- what you need;
- download;
- install (Mac: drag to Applications, first-open; Windows: SmartScreen with a screenshot);
- the setup page, step by step;
- adding more folders later;
- reading on your phone (home Wi-Fi; anywhere with Tailscale);
- updating;
- uninstalling, and what stays behind (the settings folder path, the Tailscale serve removal command);
- forgot the password;
- where Armarium keeps its files;
- for Terminal users (the CLI location, with a symlink example).

- [ ] **Step 1:** Write both guides with the `unslop` skill applied. Every step is one action. Check every
  path and menu name against the built app.
- [ ] **Step 2:** Update the README and the deploy and clients docs. Every link resolves (check relative
  links with a short script or `lychee` if it is installed).
- [ ] **Step 3: Commit** `docs: install guides for Mac and Windows`.

### Task 14: Site (PR #2)

**Files:**
- Worktree `/Users/arda/projects/armarium/armarium-site` (branch `site/companion-apps`): `site/index.html`,
  `site/styles.css` if needed, `.github/workflows/pages.yml`, `docs/github-pages.md`

**Changes:**
- The Download block lists "Mac (Apple silicon and Intel)" → `Armarium-<X.Y.Z>-macOS.dmg`, then "Windows" →
  `Armarium-Setup-<X.Y.Z>-x64.exe`, with a one-line note: "Windows may warn that the app is unrecognised:
  choose More info, then Run anyway." Then Linux and Docker as today.
- `pages.yml` `data-asset` rewrite: the new names. The minimum link count matches the new block.
- Skrivist Books copy: "on Google Play, and coming to the App Store (in review)", with the Play listing link.
  Get the URL from `skrivist-native` docs or the store handoff; ask the parent session if it isn't
  recorded.
- An "Install in three steps" section: download, open, finish setup in the browser. It links to the two
  guides on GitHub.

- [ ] **Step 1:** Make the edits. Preview per `docs/github-pages.md`, at 375 px and desktop width, with no
  horizontal scroll.
- [ ] **Step 2:** Commit on `site/companion-apps` and push. Update PR #2's description to cover the new
  links and copy, and say it must merge after the first release that carries the installers.

### Task 15: Whole-branch verification, review, PR

- [ ] **Step 1:** `heavy-slot make check` and `heavy-slot go test -race ./...` both pass.
- [ ] **Step 2:** Run `ai-reviewer`, then the review it names. Codex: `/codex:adversarial-review` on
  `origin/main...feat/desktop-installers`, focused on the setup and settings security, the folder dialog's
  local check, `Apply` rollback and the installers. Fix the findings, re-run step 1, and re-review if the
  fixes were large.
- [ ] **Step 3:** Push `feat/desktop-installers` and open the PR against `main`. The body:
  - what changed;
  - the macOS real-install results;
  - the Windows results, or "pending Windows machine";
  - the reviewer's verdict;
  - "merge and release are Arda's call".

  Bind the PR with the ccd_pr tools.
