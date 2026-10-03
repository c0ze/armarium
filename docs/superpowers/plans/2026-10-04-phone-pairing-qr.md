# Phone Pairing by QR Code Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Armarium's Admin page shows a QR code that opens comics.skriv.ist or books.skriv.ist on a phone with the Armarium OPDS catalogue filled in.

**Architecture:** Armarium gains a canonical `public_url`, a host matcher shared by config and middleware, and an admin-only `GET /api/pair`. The Admin UI mints a named token through the existing `POST /api/tokens` and renders the QR itself as an SVG path. Both readers move `#opds=` out of the fragment with an early classic script, then fill the catalogue form from it.

**Tech Stack:** Go 1.26 (net/url, net/http, stdlib tests); Svelte 5 + Vite + vitest (Armarium web); `qrcode-generator` 2.0.4 (MIT); Svelte 4-style PWA (books.skriv.ist, vitest/jsdom); SvelteKit (comics.skriv.ist, vitest/node).

**Spec:** `docs/superpowers/specs/2026-10-04-phone-pairing-qr-design.md` (read it before starting; the plan argues from it).

**Repositories and branches** (all on branch `phone-pairing`):
- Armarium: this repo (`armarium/`), remotes `cachyos` (source of truth: `cachyos:projects/armarium/armarium`) and `origin` (GitHub). Tasks 1–5.
- books.skriv.ist: `~/projects/gand/skrivist/books.skriv.ist`. Task 6.
- comics.skriv.ist: `~/projects/gand/skrivist/comics.skriv.ist`. Task 7.

## Global Constraints

- Link format: `<reader origin>/#opds=<encodeURIComponent(<base>/opds/t/<secret>/v1.2/catalog)>`, built by string concatenation, one trailing slash stripped from each input.
- Reader list (fixed): `comics` "Skrivist Comics" `https://comics.skriv.ist`; `books` "Skrivist Books" `https://books.skriv.ist`.
- Token name: `<id>-phone-<YYYY-MM-DD-HHMM>`, local time, 24-hour, zero-padded; on HTTP 409 retry once with `-2` appended.
- sessionStorage key used by both readers: `skrivist.opds`. Scrubber file name: `opds-hash.js`.
- QR: `qrcode-generator`, `qrcode(0, 'M')`, byte mode, 4-module white quiet zone, one `<path>` with `fill` attributes, no `style=` attributes, no `{@html}`, nothing loaded from a CDN.
- HTTP base (`http:`) blocks pairing before any token is minted; empty `public_url`, loopback host and `corsAllowed: false` are warnings only.
- Readers never auto-fetch a paired catalogue (comics keeps its existing localhost-only auto-connect).
- Nothing is pushed to GitHub. Commits end with `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.

## Review Focus

1. A second QR scanned into an already-open books tab (fragment-only navigation): the catalogue form must update to the new URL. Pinned in Task 6 (`takeOpdsLink` from fragment + Catalogue reactive copy, checked manually in Task 8).
2. `public_url` written as the owner would type it (`HTTPS://NAS.Example:443/`): must canonicalise to `https://nas.example` and pass `allowed_hosts = ["nas.example"]`. Pinned in Task 2.
3. Two pairings in the same minute for the same reader: second must get `-2`, not an error. Pinned in Task 5 (`createPairToken` test with a fake `createToken`).
4. A reader origin listed in `cors_origins` with a trailing slash (`https://books.skriv.ist/`): `corsAllowed` must stay false and the warning must name the origin without a slash. Pinned in Task 3.
5. A very long host producing a large QR: must still render (auto version), not throw. Pinned in Task 4 (`qrPath` long-host test).

---

### Task 1: Shared host matcher

**Files:**
- Create: `internal/config/hosts.go`, `internal/config/hosts_test.go`
- Modify: `internal/httpapi/middleware.go:57-81` (delete `hostAllowed`; `hosts` calls `config.HostAllowed(s.Cfg.AllowedHosts, r.Host)`)
- Test: `internal/httpapi/security_test.go` (`TestHostAllowlist`, add IPv6 case)

**Interfaces:**
- Produces: `func HostAllowed(allowed []string, hostport string) bool` in package `config`.

- [ ] **Step 1: Write the failing test** `TestHostAllowed` (table-driven) in `hosts_test.go`:

```go
cases := []struct{ allowed []string; host string; want bool }{
	{[]string{"nas.local"}, "nas.local:8580", true},
	{[]string{"nas.local"}, "NAS.local", true},
	{[]string{"nas.local:8580"}, "nas.local:8580", true},
	{[]string{"nas.local:8580"}, "nas.local:9000", false},
	{[]string{"nas.local:8580"}, "nas.local", false},
	{[]string{"[::1]"}, "[::1]:8580", true},
	{[]string{"::1"}, "[::1]:8580", true},
	{[]string{"[::1]:8580"}, "[::1]:8580", true},
	{[]string{"[::1]:8580"}, "[::1]:9000", false},
	{[]string{"localhost"}, "evil.example", false},
}
```

- [ ] **Step 2: Run** `go test ./internal/config -run TestHostAllowed` — Expected: FAIL (undefined: HostAllowed).

- [ ] **Step 3: Implement `HostAllowed`.** For each entry: if `net.SplitHostPort(entry)` succeeds it is port-qualified and must equal `hostport` case-insensitively (compare after `strings.ToLower`). Otherwise the entry is a host on any port: strip `[]` from the entry and compare case-insensitively with the request host (request host = `SplitHostPort(hostport)` host if that succeeds, else `hostport` with `[]` stripped). Move the doc comment ("DNS-rebinding defence…") with it.

- [ ] **Step 4: Switch the middleware** to `config.HostAllowed` and add to `TestHostAllowlist`: with `cfg.AllowedHosts` extended by `"[::1]"` (set on `e.srv.Cfg` before the request, or build a fresh env) a request with `host: "[::1]:8580"` gets 200.

- [ ] **Step 5: Run** `go test ./internal/config ./internal/httpapi` — Expected: PASS.

- [ ] **Step 6: Commit** `fix: share the Host allowlist matcher and let portless IPv6 entries match any port`.

---

### Task 2: `public_url` setting

**Files:**
- Modify: `internal/config/config.go` (field, `applyEnv`, `validate`)
- Create: `internal/config/publicurl.go`
- Test: `internal/config/config_test.go`
- Modify: `deploy/armarium.example.toml` (env list at top; commented `public_url` line with a one-line explanation next to `cors_origins`)

**Interfaces:**
- Consumes: `HostAllowed` (Task 1).
- Produces: `Config.PublicURL string` (toml `public_url`), canonical `scheme://host[:port]` or `""`; `func canonicalPublicURL(raw string) (string, error)`.

- [ ] **Step 1: Write the failing tests.** `TestPublicURL` table, run through `Load` with `allowed_hosts = ["h", "192.168.1.10", "nas.example"]`:

| input | want |
|---|---|
| `""` | `""` |
| `"  "` | `""` |
| `https://h` | `https://h` |
| `https://h/` | `https://h` |
| `https://h:8443` | `https://h:8443` |
| `https://h:443` | `https://h` |
| `http://h:80` | `http://h` |
| `HTTPS://NAS.Example:443/` | `https://nas.example` |
| `http://192.168.1.10:8580` | `http://192.168.1.10:8580` |

Rejected (Load returns an error): `h`, `ftp://h`, `https://u:p@h`, `https://h?`, `https://h?q=1`, `https://h#`, `https://h/armarium`, `https://h:0`, `https://h:70000`, `https://bücher.example`, `https://other.example`. The last error message contains both `public_url` and `allowed_hosts`.

`TestPublicURLEnv`: `t.Setenv("ARMARIUM_PUBLIC_URL", "https://h/")` with `allowed_hosts=["h"]` → `c.PublicURL == "https://h"`.

- [ ] **Step 2: Run** `go test ./internal/config -run PublicURL` — Expected: FAIL.

- [ ] **Step 3: Implement.** `canonicalPublicURL` trims spaces, rejects a raw `?` or `#` anywhere before parsing, then `url.Parse`; checks scheme, `User == nil`, `Opaque == ""`, `Path` in `{"", "/"}`, hostname non-empty and all bytes < 0x80, port (if any) parses as 1–65535; lowercases scheme and host; drops `:443` for https and `:80` for http; returns `scheme + "://" + u.Host` (rebuilt with `net.JoinHostPort` when a port remains). `validate` calls it, stores the result, then checks `HostAllowed(c.AllowedHosts, host)` with `host` being the canonical authority. `applyEnv` copies `ARMARIUM_PUBLIC_URL`.

- [ ] **Step 4: Run** `go test ./internal/config` — Expected: PASS.

- [ ] **Step 5: Update `deploy/armarium.example.toml`** and commit `feat(config): public_url, the address phones and HTTPS readers use`.

---

### Task 3: `GET /api/pair`

**Files:**
- Create: `internal/httpapi/pair.go`, `internal/httpapi/pair_test.go`
- Modify: `internal/httpapi/server.go` (route `GET /api/pair`, Admin, next to the token routes)

**Interfaces:**
- Consumes: `Config.PublicURL`, `Config.CORSOrigins`.
- Produces: JSON `{"publicUrl": string, "readers": [{"id","name","url","origin": string, "corsAllowed": bool}]}`; Go: `var pairReaders = []pairReader{...}` and `func (s *Server) getPair(w, r)`.

- [ ] **Step 1: Write the failing tests** in `pair_test.go` using `newEnv`:
  - anonymous → 401; `token: e.token` → 403; after `e.login()`, `cookie: true` → 200.
  - with the harness config (`cors_origins = ["https://comics.skriv.ist"]`, `PublicURL` empty): `publicUrl == ""`, readers in order `comics`, `books`; comics `corsAllowed == true`, `origin == "https://comics.skriv.ist"`; books `corsAllowed == false`.
  - `e.srv.Cfg.PublicURL = "https://armarium.test"` → echoed.
  - `e.srv.Cfg.CORSOrigins = []string{"https://books.skriv.ist/"}` → books `corsAllowed == false`.

- [ ] **Step 2: Run** `go test ./internal/httpapi -run Pair` — Expected: FAIL (404).

- [ ] **Step 3: Implement.** `pairReader{ID, Name, URL string}` with the two readers from Global Constraints. `origin` comes from `url.Parse(URL)` → `scheme + "://" + host`; `corsAllowed` is `slices.Contains(s.Cfg.CORSOrigins, origin)`. Respond with `writeJSON(w, 200, …)`.

- [ ] **Step 4: Run** `go test ./...` — Expected: PASS.

- [ ] **Step 5: Commit** `feat(api): GET /api/pair lists the readers a phone can pair with`.

---

### Task 4: Pairing helpers and QR path (web)

**Files:**
- Modify: `web/package.json` (dependency `qrcode-generator` `^2.0.4`, via `npm install qrcode-generator@^2.0.4`)
- Create: `web/src/lib/pair.ts`, `web/src/lib/pair.test.ts`

**Interfaces:**
- Produces (all exported from `web/src/lib/pair.ts`):
  - `pairLink(readerUrl: string, base: string, secret: string): string`
  - `pairTokenName(id: string, d: Date): string`
  - `qrPath(text: string): { size: number; d: string }`, where `size` is the module count plus 8 (quiet zone) and `d` draws each dark module as `M{x} {y}h1v1h-1z` offset by 4. Throws whatever the generator throws.
  - `isLoopback(base: string): boolean` (hostname `localhost`, `127.0.0.1`, `::1`, `[::1]`).

- [ ] **Step 1: Write the failing tests:**

```ts
const secret = 'Ab-_' + 'x'.repeat(39);
const link = pairLink('https://books.skriv.ist/', 'https://armarium.example/', secret);
expect(link.startsWith('https://books.skriv.ist/#opds=https%3A%2F%2F')).toBe(true);
expect(link).not.toContain('%253A');
expect(new URLSearchParams(link.split('#')[1]).get('opds'))
  .toBe(`https://armarium.example/opds/t/${secret}/v1.2/catalog`);
expect(pairTokenName('comics', new Date(2026, 0, 5, 9, 7))).toBe('comics-phone-2026-01-05-0907');
expect(pairTokenName('books', new Date(2026, 9, 4, 15, 32))).toBe('books-phone-2026-10-04-1532');
const long = pairLink('https://comics.skriv.ist', 'https://' + 'a'.repeat(253), secret);
const q = qrPath(long);
expect(q.size).toBeGreaterThan(21 + 8);
expect(q.d).toMatch(/^M\d+ \d+h1v1h-1z/);
expect(isLoopback('http://127.0.0.1:8580')).toBe(true);
expect(isLoopback('http://[::1]:8580')).toBe(true);
expect(isLoopback('https://armarium.example')).toBe(false);
```

- [ ] **Step 2: Run** `cd web && npx vitest run src/lib/pair.test.ts` — Expected: FAIL (module not found).

- [ ] **Step 3: Implement** with `import qrcode from 'qrcode-generator'`; `qrcode(0, 'M')`, `addData(text, 'Byte')`, `make()`.

- [ ] **Step 4: Run** `cd web && npm test && npm run check` — Expected: PASS, 0 errors.

- [ ] **Step 5: Commit** `feat(web): pairing link, token name and QR path helpers`.

---

### Task 5: "Pair a phone" in Admin, and docs

**Files:**
- Modify: `web/src/lib/api.ts` (types `PairReader`, `PairInfo`; `api.pair()`)
- Modify: `web/src/lib/pair.ts` (add `createPairToken`), `web/src/lib/pair.test.ts`
- Create: `web/src/views/PairPhone.svelte`
- Modify: `web/src/views/Admin.svelte` (render `<PairPhone onchange={async () => (tokens = await api.tokens())} />` between "API tokens" and "Connecting apps")
- Modify: `docs/clients.md` ("Pairing a phone" section per spec "Docs")

**Interfaces:**
- Consumes: `GET /api/pair` (Task 3); `pairLink`, `pairTokenName`, `qrPath`, `isLoopback` (Task 4); `api.createToken`, `ApiError` (existing).
- Produces: `interface PairReader { id: string; name: string; url: string; origin: string; corsAllowed: boolean }`, `interface PairInfo { publicUrl: string; readers: PairReader[] }`, `api.pair: () => call<PairInfo>('GET', '/api/pair')`; `createPairToken(id: string, now: Date, create: (name: string) => Promise<{ token: Token; secret: string }>): Promise<{ token: Token; secret: string }>`.

- [ ] **Step 1: Write the failing tests** for `createPairToken` with a fake `create`:
  - first call succeeds → called once with `comics-phone-2026-10-04-1532`.
  - first call rejects `new ApiError(409, 'a token with that name exists')`, second succeeds → second name `comics-phone-2026-10-04-1532-2`.
  - first rejects 409, second rejects 409 → rejects (no third call).
  - first rejects `new ApiError(500, 'x')` → rejects without retry.

- [ ] **Step 2: Run** `cd web && npx vitest run src/lib/pair.test.ts` — Expected: FAIL.

- [ ] **Step 3: Implement `createPairToken` and `api.pair`.**

- [ ] **Step 4: Build `PairPhone.svelte`.** State: `info: PairInfo | null`, `pairing: { reader: PairReader; name: string; link: string; qr: { size: number; d: string } | null; qrError: string } | null`, `error`. Effective base = `info.publicUrl || location.origin`. Exact copy:
  - heading "Pair a phone"; note "Opens Skrivist Comics or Skrivist Books on your phone with this library's OPDS catalogue filled in. Each pairing gets its own token."
  - HTTP block (buttons disabled): "The hosted readers only load HTTPS catalogues. Set public_url to an HTTPS address."
  - warnings: "public_url is not set, so the QR uses this page's address ({base}). A phone may not reach it." / "{base} is this computer's loopback address; a phone can't reach it." / "Add {origin} to cors_origins, or {name} can't load the catalogue."
  - after pairing: "Scan with the phone's camera, then tap Connect in {name}." plus the token name, the link in a `<code>`, a "Done" button.
  - QR: `<svg viewBox="0 0 {size} {size}" width="264" height="264" shape-rendering="crispEdges" role="img" aria-label="Pairing QR code for {name}"><rect width={size} height={size} fill="#fff"/><path d={qr.d} fill="#000"/></svg>`. If `qrPath` throws, show the error text in place of the QR (the token stays and is listed).
  - After a successful create, call `onchange()` so Admin refreshes the token list. Done sets `pairing = null`.
  - Styles in the component's `<style>` block using the existing CSS variables (`--surface`, `--line`, `--amber`, `--text-2`), matching `.secret` in Admin.

- [ ] **Step 5: Update `docs/clients.md`.**

- [ ] **Step 6: Run** `cd web && npm test && npm run check && npm run build`, then `go build ./... && go test ./...` — Expected: all PASS, 0 svelte-check errors.

- [ ] **Step 7: Manual check in the browser.** `make web build`; temp config with `allowed_hosts = ["127.0.0.1", "localhost"]`, `password_hash` from `echo pw | bin/armarium hash-password`, one comics library; run `bin/armarium --config <tmp> serve`, log in, open Admin.
  - Run 1, `public_url` unset: base is `http://127.0.0.1:…`, buttons disabled with the HTTP block text, plus the "public_url is not set" and loopback warnings.
  - Run 2, `public_url = "https://localhost"`: buttons enabled (page still served over HTTP). Press "Skrivist Books": QR appears, token listed, loopback and books CORS warnings shown. Decode a screenshot of the QR (`zbarimg` if installed, otherwise a phone camera) and compare with the printed link. Done clears it.

- [ ] **Step 8: Commit** `feat(web): pair a phone from Admin with a QR code`.

---

### Task 6: books.skriv.ist reads `#opds=`

**Files (repo `~/projects/gand/skrivist/books.skriv.ist`):**
- Create: `public/opds-hash.js`, `src/lib/pair.ts`, `tests/pair.test.ts`
- Modify: `index.html` (`<script src="/opds-hash.js"></script>` as the first `<script>` in `<head>`, before the JSON-LD and the beacon)
- Modify: `src/App.svelte` (`route()`, `catalogueUrl`, pass `initialUrl` to `Catalogue`)
- Modify: `src/components/Catalogue.svelte` (`export let initialUrl = ''` + reactive copy)

**Interfaces:**
- Produces: `takeOpdsLink(win: { location: Location; history: History; sessionStorage: Storage } = window): string` in `src/lib/pair.ts`; sessionStorage key `skrivist.opds`.

- [ ] **Step 1: Write the failing tests** in `tests/pair.test.ts` (jsdom):
  - sessionStorage has `skrivist.opds = 'https://a.example/opds/t/x/v1.2/catalog'` → returns it; key removed.
  - `history.replaceState(null, '', '/#opds=' + encodeURIComponent(U))` → returns `U`; `location.hash === ''`.
  - `'/#opds=' + encodeURIComponent(U) + '&read=' + 'a'.repeat(64)` → returns `U`; hash cleared.
  - `'/#read=' + 'a'.repeat(64)` → returns `''`; hash unchanged.
  - no hash → `''`.
  - scrubber: set hash to `#opds=…`, `await import('../public/opds-hash.js?' + Date.now())` (or read the file and run it with `new Function`) → sessionStorage has the value, hash cleared.

- [ ] **Step 2: Run** `npx vitest run tests/pair.test.ts` — Expected: FAIL.

- [ ] **Step 3: Implement** `public/opds-hash.js` (classic script, IIFE, try/catch, no exports, ES2017 syntax) and `takeOpdsLink` (sessionStorage first, else fragment; strip with `history.replaceState(history.state, '', location.pathname + location.search)`).

- [ ] **Step 4: Wire the app.** `App.svelte`: `let catalogueUrl = ''`; at the top of `route()`: `const link = takeOpdsLink(); if (link) { active = undefined; view = 'catalogues'; catalogueUrl = link; return; }`; pass `initialUrl={catalogueUrl}` to `<Catalogue>`. `Catalogue.svelte`: `export let initialUrl = '';` and a reactive block that, when `initialUrl` is non-empty and differs from the last applied value, sets `url = initialUrl` and clears `username`, `password`, `error`, `notice`. No call to `load()`.

- [ ] **Step 5: Run** `npm test && npm run check && npm run build` — Expected: PASS; `dist/opds-hash.js` exists and the built `index.html` references it before the beacon.

- [ ] **Step 6: Commit** `feat(catalogue): open an OPDS catalogue from an #opds= pairing link`.

---

### Task 7: comics.skriv.ist uses the scrubber

**Files (repo `~/projects/gand/skrivist/comics.skriv.ist`):**
- Create: `static/opds-hash.js` (identical to books'), `src/lib/opds/pair.ts`, `src/lib/opds/pair.test.ts`
- Modify: `src/app.html` (`<script src="%sveltekit.assets%/opds-hash.js"></script>` as the first `<script>` in `<head>`)
- Modify: `src/routes/+page.svelte:130-137` (use `takeOpdsLink()` instead of reading `location.hash`)

**Interfaces:**
- Produces: `takeOpdsLink(win)` with the same signature and behavior as books' (Task 6).

- [ ] **Step 1: Write the failing tests** (node env, so pass a fake `win`: an object with `location: { hash, pathname, search }`, `history: { state: null, replaceState: vi.fn() }`, and a Map-backed `sessionStorage`): sessionStorage value returned and removed; fragment value returned and `replaceState` called with `'/path?x=1'`; no value → `''`, `replaceState` not called.

- [ ] **Step 2: Run** `npx vitest run src/lib/opds/pair.test.ts` — Expected: FAIL.

- [ ] **Step 3: Implement**, copy the scrubber, edit `app.html` and `+page.svelte` (keep the existing `initialCatalogueUrl` / `showCatalogue` assignments).

- [ ] **Step 4: Run** `npm test && npm run check && npm run build` — Expected: PASS; `build/opds-hash.js` exists.

- [ ] **Step 5: Commit** `feat(catalogue): read pairing links through the early fragment scrubber`.

---

### Task 8: End-to-end check on the desktop

No new code unless it finds a bug.

- [ ] **Step 1:** Run Armarium (built from Task 5) with `cors_origins = ["http://127.0.0.1:5173", "http://127.0.0.1:5174"]` and a real comics + books library; run `npm run dev -- --port 5173` in books and `--port 5174` in comics (or the ports their dev scripts use; put the actual ones in `cors_origins`).
- [ ] **Step 2:** In Admin, pair Skrivist Books; take the printed link, replace `https://books.skriv.ist` with the dev origin, open it in the browser pane. Expect: address bar has no fragment, Catalogues view open, URL field filled, user/password empty, nothing fetched until Connect; Connect loads the feed; a book downloads.
- [ ] **Step 3:** With that tab still open, set `location.hash` to a second pairing link's fragment. Expect the form to update to the new URL.
- [ ] **Step 4:** Same for comics (auto-connect on localhost is expected there).
- [ ] **Step 5:** Revoke both tokens in Admin; Connect again in a reader → 401 shown as an error.
- [ ] **Step 6:** Record results in the PR/hand-off notes. Phone scan over HTTPS waits for the DSM reverse proxy and is listed as outstanding.
