# Clients

Armarium speaks OPDS 1.2 (with page streaming) and a small JSON API. Create an API
token under **Admin** for each app.

## OPDS apps

Any OPDS 1.2 client should work (KOReader, Panels, Chunky, …). End-to-end tests so
far cover PanelFlow and comics.skriv.ist; the feeds themselves are checked by the
test suite (valid Atom, link order, PSE attributes, token prefixes).

- Catalog URL: `https://<host>/opds/v1.2/catalog`, with HTTP Basic auth (any user
  name, the token as password).
- Apps without Basic auth: put the token in the path instead,
  `https://<host>/opds/t/<token>/v1.2/catalog`. Every link in the feeds keeps that
  prefix.
- Feeds: libraries → series → items, plus "Continue reading", "Recently added" and
  search (an Atom `{searchTerms}` template and an OpenSearch description).
- Comics carry an OPDS-PSE page-streaming link with `pse:count` and, once you have
  progress, `pse:lastRead`, so PSE readers open on the right page. `{pageNumber}` is
  0-based, as the PSE spec says.
- Books from a Calibre library offer every format Calibre has (EPUB, MOBI, AZW3, …)
  as separate acquisition links.
- There are no redirects anywhere, so strict clients that refuse them work.

## Pairing a phone

Admin → **Pair a phone** shows a QR code that opens Skrivist Comics
(comics.skriv.ist) or Skrivist Books (books.skriv.ist) on a phone with this
library's OPDS catalogue filled in; tap Connect and it's done. Pick the device
(iPhone, Android, iPad or your own name) first: each pairing creates its own token,
named like `books-iphone-2026-10-04-1532`, so you can tell devices apart and revoke
one without the others.

- Set `public_url` to the HTTPS address phones use, e.g.
  `https://armarium.example.com`. Its host must be in `allowed_hosts`. The hosted
  readers refuse plain-HTTP catalogues, so Admin won't pair over HTTP.
- Add the reader origins to `cors_origins` (`https://comics.skriv.ist`,
  `https://books.skriv.ist`): the readers fetch the catalogue from the browser.
- The phone itself must reach `public_url`. If Armarium is only on your home network
  or a VPN such as Tailscale, the phone has to be on it too, and its DNS has to give
  an address the phone can reach. A QR that opens the reader but then spins on
  Connect almost always means the phone can't reach the server.
- The QR carries the token in the link's fragment, which never reaches the reader's
  server. Once paired, OPDS requests carry it in the path, as any token-in-path client
  does, so a reverse proxy's access log will contain it.
- To unpair, revoke the token under API tokens, then forget the shelf on the phone.
  Revoking stops access; it doesn't delete books already downloaded.

## PanelFlow and comics.skriv.ist

These guided-view comic readers have an Armarium mode at `/armarium`: connect with
the Armarium URL and a token, browse comics libraries, and read with pages streamed
from Armarium and progress saved back to it.

- Add the reader's origin to `cors_origins` (for example `https://comics.skriv.ist`).
  The browser calls Armarium directly; no proxy and no cookies are involved.
- An HTTPS reader needs an HTTPS Armarium.
- Set `panelflow_url` and Armarium's item pages link comics straight into the reader.

## Skrivist Books and Comics desktop apps

The macOS and Windows ports use the same direct requests as the hosted readers;
the desktop webview has a different origin. Allow the origins explicitly in your
Armarium config and restart the server:

```toml
cors_origins = [
  "https://comics.skriv.ist",
  "https://books.skriv.ist",
  "tauri://localhost",         # macOS
  "http://tauri.localhost",    # Windows default
  "https://tauri.localhost",   # Windows builds using useHttpsScheme
]
```

Desktop-origin support and this allowlist are included in v0.3.2 and later's
example configuration. Existing installations must merge these origins into
their own config after upgrading; an upgrade does not replace the file.
Do not add `*`, `null`, other custom schemes, or a trailing slash.
Origins are matched exactly. Both apps still need an API token; CORS does not
grant authentication or permit cookies.

In Comics, enter the server's base URL and the token in **Armarium**. In Books,
use **Catalogues** with `https://<host>/opds/v1.2/catalog`, any nonempty username,
and the token as the password. The allowlist covers library requests, downloads,
page images and Comics progress writes. Existing desktop builds need no update
for this server-side fix.

To diagnose a rejected preflight without exposing a token:

```sh
curl -i -X OPTIONS 'https://<host>/api/libraries' \
  -H 'Origin: tauri://localhost' \
  -H 'Access-Control-Request-Method: GET' \
  -H 'Access-Control-Request-Headers: authorization'
```

Expect HTTP 204 with `Access-Control-Allow-Origin: tauri://localhost` and
Authorization in `Access-Control-Allow-Headers`. Repeat with the Windows origin
and `/opds/v1.2/catalog` for Books. A 404/405 or missing allow-origin header means
the running server/proxy has not enabled that origin. TLS, DNS and URL errors
can also prevent a connection even when CORS is configured correctly.

## skriv.ist (Skrivist)

Skrivist's OPDS browser works with Armarium's feeds for EPUBs. Its server-side
proxy refuses private and Tailscale addresses, so Armarium must be reachable on a
public HTTPS host name.

## JSON API

For your own tools; authenticate with `Authorization: Bearer <token>`.

```text
GET  /api/libraries
GET  /api/libraries/{id}/series?q=&offset=&limit=
GET  /api/series/{id}                    series + items in reading order
GET  /api/items?library=&series=&q=&status=&tag=&source=&sort=&offset=&limit=
GET  /api/items/{id}                     item (with progress) + library
GET  /api/items/{id}/cover               JPEG thumbnail
GET  /api/items/{id}/pages/{n}           comic page n (1-based)
GET  /api/items/{id}/pages/{n}/panels    shared correction or null
PUT  /api/items/{id}/pages/{n}/panels    save shared panels with expected revision
GET  /api/items/{id}/file[?format=mobi]  the file (Range supported)
GET  /api/items/{id}/toc                 EPUB table of contents
PUT  /api/items/{id}/progress            {"page": n} | {"status": "read"|"unread"}
```

Progress only moves forward: a lower `page` is ignored (`"applied": false`) unless
you send `"reset": true`; `{"status": "unread"}` resets it. Details in
[design.md](design.md).

## Shared comic panel corrections

Clients supporting the panel editor can save geometry and reading order once on
Armarium and reuse it on other devices. Corrections belong to the comic page,
not a token or account. Any authenticated reader may edit them. Generic OPDS
clients continue reading normally; they must implement this API to use panels.
Skrivist Comics uses it for streaming and Armarium OPDS downloads/phone pairing.

`GET /api/items/{id}/pages/{n}/panels` returns `{"correction": null}` when there
is no record, otherwise a correction object. Responses are authenticated and
`no-store`. Pages are 1-based; only CBZ/CBR comic pages are supported.

A PUT sends this shape (`rect` is normalized X, Y, width, height):

```json
{
  "imageHash": "<lowercase SHA-256 of the page image bytes>",
  "revision": 0,
  "enabled": true,
  "pageSize": { "w": 1200, "h": 1800 },
  "direction": "ltr",
  "panels": [{ "id": 0, "rect": [0.05, 0.05, 0.9, 0.4] }],
  "automaticPanels": [{ "id": 0, "rect": [0.04, 0.04, 0.91, 0.41] }],
  "automaticCandidates": [[0.04, 0.04, 0.91, 0.41]],
  "detector": { "model": "frame-detector", "revision": "model-revision" }
}
```

Array order is the explicit reading order; IDs must be unique nonnegative
integers. Up to 100 edited panels are allowed, each inside the page and at least
0.5% wide/high. Geometry, bounds and body size are validated. The server hashes
the original page before accepting the annotation; images are not uploaded.

Revision 0 creates a record. Later saves send the revision returned by GET;
successful writes increment it and set `updatedAt` (Unix seconds). HTTP 409 means
the page bytes changed or a newer correction exists: reload before editing.
This compare-and-swap is atomic, so stale clients cannot erase a newer edit.
`enabled:true` with an empty panel array means intentional whole-page reading.
`enabled:false` writes a retained reset tombstone; automatic detection resumes.
Do not send `updatedAt` in PUT requests; timestamps come from the server.

Migration 004 creates the SQLite annotation table automatically. Back up the
database and comic files as usual. Metadata survives app/device changes and
rescans; replaced images are distinguished by their content hash. Original
predictions/order and detector identity remain available alongside reviewed
annotations for training and regression evaluation. Model retraining and
publishing validated weights remain a separate process.
