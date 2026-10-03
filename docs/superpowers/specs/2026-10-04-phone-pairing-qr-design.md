# Phone pairing by QR code

Date: 2026-10-04. Status: approved design, revised after Codex and Grok reviews.

## Goal

Connect Skrivist Comics (comics.skriv.ist) or Skrivist Books (books.skriv.ist) on a
phone to Armarium without typing a server address or a token. The owner opens Admin
on a desktop, picks a reader, and points the phone's camera at a QR code. The phone
opens the reader with the Armarium OPDS catalogue filled in; one tap on Connect
finishes it.

Success: from Admin to a browsable catalogue on the phone with no typing, using only
the phone's built-in camera app, against an HTTPS Armarium.

## Scope

In: an Armarium `public_url` setting, a shared host matcher, an admin-only pairing
endpoint, a "Pair a phone" section in Admin, an early fragment scrubber in both
readers, and `#opds=` handling in books.skriv.ist.

Out: skrivist-mobile, QR scanners inside the readers, short-lived pairing codes,
comics.skriv.ist's Armarium mode (`/armarium`, progress sync; it would need its own
link format), and a configurable reader list. Third-party OPDS apps keep using the
URLs Admin already shows.

## The link

Each QR code encodes one URL. This is the contract between the three apps:

```
<reader origin>/#opds=<encodeURIComponent(<base>/opds/t/<secret>/v1.2/catalog)>
```

`<base>` is the canonical `public_url`, or `location.origin` of the Admin page when
`public_url` is empty. Example:
`https://books.skriv.ist/#opds=https%3A%2F%2Farmarium.example%2Fopds%2Ft%2F<secret>%2Fv1.2%2Fcatalog`

- The link is built by string concatenation, with one trailing slash removed from
  both the reader URL and the base. The catalogue URL is encoded exactly once.
- The catalogue URL uses the token-in-path form, so the reader needs no user name or
  password. Every link in the feeds keeps the prefix.
- The catalogue sits in the fragment, which browsers never send to the reader's host.
- Readers move the value out of the fragment before any other script on the page can
  read it (see "Readers").

## Armarium

### Config: `public_url`

New `public_url` (TOML) and `ARMARIUM_PUBLIC_URL` (added to `applyEnv` and to the
env list at the top of `deploy/armarium.example.toml`). Example:
`https://armarium.akaraduman.synology.me`.

`Config.validate` canonicalises and checks it. Empty (after trimming spaces) is
allowed. Otherwise:

- Parse with `net/url`. Scheme must be `http` or `https`; hostname must be
  non-empty and ASCII (an IDN must be written in punycode, because that is what
  browsers send as `Host`).
- Rejected: user info; a query or fragment, including a bare trailing `?` or `#`
  (`ForceQuery`, `RawFragment`, or a `?`/`#` in the raw string); any path other than
  empty or `/`. Armarium has no base path.
- A port, if present, must be 1–65535. A default port (`https:443`, `http:80`) is
  removed, as browsers do.
- The stored value is `scheme://host[:port]` with no trailing slash.
- The canonical host (`u.Host`, the same shape browsers send in `Host`) must pass
  `config.HostAllowed`. A mismatch is a startup error naming `public_url` and
  `allowed_hosts`.

### Shared host matcher

`hostAllowed` moves from `internal/httpapi/middleware.go` to
`config.HostAllowed(allowed []string, hostport string) bool`, used by both the
middleware and `validate`. Rules:

- An entry with a port (`net.SplitHostPort` succeeds, e.g. `nas.local:8580`,
  `[::1]:8580`) matches that exact host and port, case-insensitively.
- An entry without a port matches that host on any port. This includes bracketed
  IPv6 (`[::1]`) and bare IPv6 (`::1`). Today a portless IPv6 entry matches nothing
  with a port, because the code treats any colon as a port; that bug is fixed here.
- Request hosts are compared without brackets and case-insensitively.

### Endpoint

`GET /api/pair`, Admin access (owner session only, like the token routes):

```json
{
  "publicUrl": "https://armarium.example",
  "readers": [
    {"id": "comics", "name": "Skrivist Comics", "url": "https://comics.skriv.ist", "origin": "https://comics.skriv.ist", "corsAllowed": true},
    {"id": "books",  "name": "Skrivist Books",  "url": "https://books.skriv.ist",  "origin": "https://books.skriv.ist",  "corsAllowed": false}
  ]
}
```

- `publicUrl` is the canonical configured value or `""`.
- The reader list is a fixed Go slice in `internal/httpapi`.
- `origin` is `scheme://host` of the reader URL. `corsAllowed` is true when that
  exact string is in `cors_origins` (which the middleware matches exactly against
  the browser's `Origin`, no trailing slash).

No new token route: the UI creates the token with the existing `POST /api/tokens`.

### Admin UI

A "Pair a phone" section between "API tokens" and "Connecting apps" in
`web/src/views/Admin.svelte`. It has its own state (`pairing`), separate from the
existing `created` box.

1. One button per reader from `/api/pair`, labelled "Skrivist Comics" and
   "Skrivist Books", with a line saying the phone gets the OPDS catalogue.
2. Blocking check, before any token is created: if the effective base is `http:`,
   the buttons are disabled with the reason ("The hosted readers only load HTTPS
   catalogues. Set public_url to an HTTPS address."). Both readers refuse plain-HTTP
   catalogues when served over HTTPS, so such a QR could never work.
3. Warnings (never blocking):
   - `public_url` is empty: the page's own address is used, which a phone may not
     reach;
   - the effective host is `localhost`, `127.0.0.1`, `::1` or `[::1]`: a phone can't
     reach it;
   - `corsAllowed` is false: names the exact origin to add to `cors_origins`.
4. Clicking a button creates a token named `<id>-phone-<YYYY-MM-DD-HHMM>`, local
   time, 24-hour, zero-padded. On `ApiError.status === 409` it retries once with
   `-2` appended. If that fails too, it shows the error and no QR.
5. It then shows the QR, the link as selectable text, the token name, and a Done
   button. The token list refreshes. Done sets `pairing` to null, dropping the
   secret, link and QR. Revoking the token in the list stops the phone's access.

QR rendering:

- Library: `qrcode-generator` (MIT, no dependencies), bundled into the embedded UI.
  Nothing loads from a CDN.
- `qrcode(0, 'M')` (version picked automatically), byte mode, the link as data. The
  generator throws when the data does not fit; the error is shown instead of a QR,
  and the token stays in the list.
- The component builds the SVG itself from `getModuleCount()` and `isDark(r, c)`:
  one `<path>` with a `d` string, `fill` attributes only, a 4-module white quiet zone,
  `shape-rendering="crispEdges"`, about 264 px square, white background in both
  themes. No `{@html}`, no `style=` attributes, so the UI's CSP (`style-src 'self'`)
  holds.

Pure functions in `web/src/lib/pair.ts`, so they can be tested without the component:
`pairLink(readerUrl, base, secret)`, `pairTokenName(id, date)`, `qrPath(link)`
(returns the module count and path data, or throws).

### Docs

`docs/clients.md` gets a "Pairing a phone" section: set `public_url` to an HTTPS
address, add the reader origins to `cors_origins`, use Admin → Pair a phone; to
unpair, revoke the token in Admin, then forget the shelf on the phone.
`deploy/armarium.example.toml` gains a commented `public_url` line and the env name.

## Readers

### Early fragment scrubber (both readers)

Both readers load the Cloudflare Web Analytics beacon (deferred). To keep the token
away from it and any other script, each reader gets a small classic script,
`opds-hash.js`, served from its own origin and loaded with a plain
`<script src>` (no `defer`, no `module`) as the first script in `<head>`:

- If `new URLSearchParams(location.hash.slice(1))` has `opds`, store the value in
  `sessionStorage['skrivist.opds']`, then
  `history.replaceState(history.state, '', location.pathname + location.search)`.
- Wrapped in try/catch; on failure it leaves the page alone.

It's an external file because books' CSP allows only `'self'` scripts. Placement:
books `public/opds-hash.js` (referenced from `index.html`), comics
`static/opds-hash.js` (referenced from `src/app.html` via `%sveltekit.assets%`). Both
are precached by the PWA like other static files.

The app then reads the value with a shared rule: take
`sessionStorage['skrivist.opds']` (and remove it), else take `opds` from the current
fragment (covers a fragment-only navigation in an already-open tab, where the
scrubber doesn't run, and strip it the same way).

### books.skriv.ist

- New `src/lib/pair.ts` with `takeOpdsLink(): string` implementing the rule above.
- `App.svelte` gets one dispatcher, `route()`, used both on mount (after `refresh()`)
  and on `hashchange`. It calls `takeOpdsLink()` first. If that returns a URL: close
  any open book (`active = undefined`), set `view = 'catalogues'`, set
  `catalogueUrl` to the value, and skip `parseReadingRoute`. Otherwise it runs the
  existing reading-route logic unchanged. A fragment with both `opds` and `read`
  counts as an `opds` link only.
- `Catalogue.svelte` (legacy `export let` syntax) gets `export let initialUrl = ''`.
  A reactive statement copies a new non-empty `initialUrl` into `url` and clears
  `username`, `password`, `error` and `notice`, so a second pairing link updates an
  already-mounted catalogue. It never calls `load()` by itself: the user taps Connect.

### comics.skriv.ist

- `src/routes/+page.svelte` `onMount` reads the link through the same rule (a small
  `takeOpdsLink()` in `src/lib/opds/`), instead of reading `location.hash` directly.
  Everything after that is unchanged: catalogue panel opens with the URL filled in,
  auto-connect only on localhost.

## Security notes

- The QR holds a long-lived reader token: it can read the library and save
  progress, never administer. Anyone who photographs the screen gets that.
  Mitigations: the QR shows only until Done, every pairing gets its own named
  token, and revoking it stops that phone's access.
- Creating the QR puts the secret only in the `POST /api/tokens` JSON response. Once
  paired, every OPDS request carries it in the path (`/opds/t/<secret>/...`), as
  token-in-path already does: Armarium logs route patterns, not paths, but a reverse
  proxy's access log will contain it. Revoke the token if that log leaks.
- On the phone, books keeps the catalogue URL (with the token) in IndexedDB shelves;
  comics keeps it in IndexedDB shelves and `localStorage['comics.opds.url']`.
  Revoking blocks future requests; it does not delete downloaded books or cached
  feed data.
- Short-lived pairing codes would close the photo risk but need a new exchange
  endpoint and reader changes; left out on purpose.

## Tests

Armarium, Go:

- `config.HostAllowed`: exact host:port; portless entry on any port; `[::1]` and
  `::1` entries against `[::1]:8580`; port-qualified entry rejecting another port;
  case-insensitivity.
- `public_url`: accepted (`https://h`, `https://h/`, `https://h:8443`,
  `http://192.168.1.10:8580`), canonicalised (`https://h:443` → `https://h`,
  trailing slash removed, upper-case scheme/host lowered), rejected (no scheme,
  `ftp:`, user info, `?`, `?q`, `#`, path `/armarium`, port 0 or 70000, non-ASCII
  host, host not in `allowed_hosts`), env override applied.
- Existing `TestHostAllowlist` keeps passing; add an IPv6 case.
- `/api/pair`: 401 anonymous, 403 with a token, 200 with a session; `corsAllowed`
  true for comics (in the harness's `cors_origins`) and false for books; `publicUrl`
  echoes the config.

Armarium, web (vitest, already set up):

- `pairLink`: trailing slashes on both inputs; output contains `%3A%2F%2F` and not
  `%253A`; decoding with `URLSearchParams` returns the exact catalogue URL; a secret
  with `-` and `_`.
- `pairTokenName`: zero padding, 24-hour time.
- `qrPath`: a link with a 253-character host and a 43-character secret produces a
  code; module count matches the expected version range.

books.skriv.ist (vitest):

- `takeOpdsLink`: from sessionStorage (and removed); from the fragment (and
  stripped); `opds` with `read`; `read` only (returns empty, fragment untouched);
  no fragment.
- The scrubber script, run against a jsdom `location`: stores and strips.

comics.skriv.ist (vitest): `takeOpdsLink` from sessionStorage and from the fragment.

Manual:

- Desktop, before HTTPS exists: run Armarium locally with `cors_origins` including
  the dev readers' origins (`http://127.0.0.1:5173` etc.). Create a pairing, then
  open the printed link with the reader origin swapped for the dev server's. Check
  that the fragment is gone from the address bar, the catalogue form is filled in,
  and Connect loads the feed. Comics auto-connects on localhost, so the "no
  auto-fetch" rule is checked on books only.
- Phone: once the DSM reverse-proxy rule for `armarium.akaraduman.synology.me`
  exists, set `public_url` to it and scan with the iPhone camera for both readers.
  This is the success criterion and can't be ticked off before that.
