# Phone pairing by QR code

Date: 2026-10-04. Status: approved design, not built.

## Goal

Connect Skrivist Comics (comics.skriv.ist) or Skrivist Books (books.skriv.ist) on a
phone to Armarium without typing a server address or a token. The owner opens Admin
on a desktop, picks a reader, and points the phone's camera at a QR code. The phone
opens the reader with the Armarium catalogue filled in; one tap on Connect finishes it.

Success: from Admin to a browsable catalogue on the phone with no typing, using only
the phone's built-in camera app.

## Scope

In: an Armarium `public_url` setting, an admin-only pairing endpoint, a "Pair a phone"
section in Admin, and `#opds=` link handling in books.skriv.ist.

Out: skrivist-mobile, QR scanners inside the readers, short-lived pairing codes, and
comics.skriv.ist's Armarium mode (`/armarium`, progress sync), which would need its
own link format. Third-party OPDS apps keep using the URLs Admin already shows.

## The link

Each QR code encodes one URL. This is the contract between the three apps:

```
<reader URL>#opds=<encodeURIComponent(<public URL>/opds/t/<secret>/v1.2/catalog)>
```

Example: `https://books.skriv.ist/#opds=https%3A%2F%2Farmarium.example%2Fopds%2Ft%2F<secret>%2Fv1.2%2Fcatalog`

- The catalogue URL uses the token-in-path form, so the reader needs no user name
  or password. Every link in the feeds keeps the prefix.
- The catalogue sits in the fragment. Browsers never send fragments to the reader's
  host, and Armarium sets `Referrer-Policy: no-referrer`.
- A reader that handles the link removes the fragment from the address bar and
  history before doing anything else.

## Armarium

### Config

New `public_url` (`ARMARIUM_PUBLIC_URL`): the address phones and HTTPS readers use,
e.g. `https://armarium.akaraduman.synology.me`. Validation, in `Config.validate`:

- http or https, with a host; no user info, query or fragment. A trailing slash is
  removed. Armarium has no base path, so any other path is an error too.
- Its host must pass the same `allowed_hosts` check the server applies to requests
  (`hostAllowed`'s rules), or the phone would get 421 "unknown host". A mismatch is a
  startup error that names both settings.

Empty is allowed: pairing then uses the address the Admin page was loaded from.

### Endpoint

`GET /api/pair`, Admin access (owner session only, like the token routes):

```json
{
  "publicUrl": "https://armarium.example",
  "readers": [
    {"id": "comics", "name": "Skrivist Comics", "url": "https://comics.skriv.ist/", "corsAllowed": true},
    {"id": "books",  "name": "Skrivist Books",  "url": "https://books.skriv.ist/",  "corsAllowed": false}
  ]
}
```

- `publicUrl` is the configured value or `""`.
- The reader list is a fixed Go slice in `internal/httpapi`, in one place.
- `corsAllowed` is true when the reader URL's origin is in `cors_origins`. Both
  readers fetch the catalogue cross-origin, so without it the phone connects and
  fails.

No new token route: the UI creates the token with the existing `POST /api/tokens`.

### Admin UI

A "Pair a phone" section next to "API tokens" in `web/src/views/Admin.svelte`:

1. One button per reader from `/api/pair`.
2. Clicking it creates a token named `<id>-phone-<YYYY-MM-DD-HHMM>` (local time). On
   409 (name taken) it retries once with `-2` appended.
3. It then shows the QR code (an inline SVG), the link as selectable text, the token
   name, and a Done button. Done clears the secret from component state. The token
   list refreshes so the new token shows there; revoking it there unpairs the phone.
4. Warnings, shown above the QR when they apply:
   - the address is `http://` (the HTTPS readers refuse plain-HTTP catalogues);
   - `publicUrl` is empty (the page's own address is used, which a phone may not reach);
   - `corsAllowed` is false (names the origin to add to `cors_origins`).

The warnings never block the QR; the owner may know better (e.g. a tailnet address).

The QR is generated in the browser with `qrcode-generator` (MIT, no dependencies),
bundled into the embedded UI. Nothing loads from the network, the existing CSP
(`script-src 'self'`) holds, and the secret never appears in a request URL to
Armarium. Error correction level M; the link is about 150 characters, which fits
comfortably.

The link builder is a small pure function in `web/src/lib/pair.ts`
(`pairLink(readerUrl, baseUrl, secret)`), so it can be tested without the component.

### Docs

`docs/clients.md` gets a short "Pairing a phone" section: set `public_url`, add the
reader origins to `cors_origins`, use Admin → Pair a phone.
`deploy/armarium.example.toml` gains a commented `public_url` line.

## books.skriv.ist

Match what comics.skriv.ist already does (`src/routes/+page.svelte`, `onMount`):

- On startup, before `route()` reads the hash, read `opds` from
  `new URLSearchParams(location.hash.slice(1))`. If present, `history.replaceState`
  the URL without the fragment, switch `view` to `'catalogues'`, and pass the value
  to `Catalogue` as a new `initialUrl` prop.
- `Catalogue.svelte` fills its URL field from `initialUrl`, leaves user name and
  password empty, and waits for Connect. It does not fetch on its own: like comics,
  a link must not make the app fetch an arbitrary URL without a tap.
- `#read=<id>` links keep working. A hash with both `opds` and `read` is treated as
  an `opds` link only.

## comics.skriv.ist

No code change. It already reads `#opds=`, strips it, opens the catalogue panel
with the URL filled in, and connects on tap. It auto-connects only on localhost;
that stays.

## Security notes

- The QR holds a long-lived reader token: it can read the library and save progress,
  never administer. Anyone who photographs the screen gets that. Mitigations: the QR
  shows only until Done, every pairing has its own named token, and revoking it in
  Admin cuts that phone off.
- Short-lived pairing codes would close the photo risk but need a new exchange
  endpoint and reader changes; left out on purpose.
- The readers already store the catalogue URL (with the token) in localStorage;
  pairing doesn't change that.

## Tests

Armarium (Go):
- `config`: `public_url` accepted forms, trailing slash trimmed, rejected forms
  (no scheme, user info, query, fragment, path), host not in `allowed_hosts`.
- `httpapi`: `/api/pair` is 401 anonymous and 403 with a token; with a session it
  returns `publicUrl` and the right `corsAllowed` per reader.

Armarium (web): `pairLink` encoding, trailing slashes on both URLs, a secret with
`-` and `_` (the web package already runs vitest).

books.skriv.ist (vitest): extract the hash parsing into a function in `src/lib`
and test `opds` alone, `opds` with `read`, `read` alone, and no hash.

Manual: on desktop, run Armarium locally with `public_url` set to the LAN address and
open the generated link in a browser against `npm run dev` of each reader (HTTP on
both sides). A real phone scan needs Armarium on HTTPS, which waits on the DSM
reverse-proxy rule for `armarium.akaraduman.synology.me`.
