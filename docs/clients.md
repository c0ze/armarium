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
  Entries are matched as a browser sends them; Armarium normalises case, default
  ports and a trailing slash, and refuses entries with a path.
- The phone itself must reach `public_url`. If Armarium is only on your home network
  or a VPN such as Tailscale, the phone has to be on it too, and its DNS has to give
  an address the phone can reach. A QR that opens the reader but then spins on
  Connect almost always means the phone can't reach the server.
- The QR carries the token in the link's fragment, which never reaches the reader's
  server. After Connect the readers send it as an `Authorization: Bearer` header to
  the plain `/opds/v1.2/…` URLs and keep it apart from every URL they store, so it
  does not end up in a proxy's access log. (Generic token-in-path OPDS apps still
  put it in the path.)
- To unpair, revoke the token under API tokens, then forget the shelf on the phone.
  Revoking stops access; it doesn't delete books already downloaded.

### Pairing this computer

The **This computer** section of the same panel pairs the Skrivist Books and
Comics apps for Mac and Windows. It creates a token named like
`books-desktop-2026-10-11-0930` and an `skrivist-books://pair#opds=…` link: open it
on this computer (or paste it into the app's catalogue field), then press Connect
in the app. The desktop apps can use plain HTTP on your network, so this works
without `public_url` or HTTPS.

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

Pair from Admin (above), or connect by hand. In Comics, enter the server's base
URL and the token in **Armarium**. In either app, a token URL pasted as the
catalogue, `http(s)://<host>/opds/t/<token>/v1.2/catalog`, connects and keeps the
token on the device (it moves out of the URL on Connect); a Basic password
(any username, the token as password) works too but lasts only for that session.
The allowlist covers library requests, downloads, page images, Comics progress
and panel writes.

- **HTTP on your network works.** The macOS app's origin is `tauri://localhost`,
  the Windows app's `http://tauri.localhost`; neither is an HTTPS page, so a
  `http://192.168.x.x:8580` server is fine. Its address must be in
  `allowed_hosts`, and `listen` must not be `127.0.0.1` for other machines.
- **HTTP 421** ("unknown host") means the host name or address the app used is
  not in `allowed_hosts`. The apps show it as such.
- **macOS 15 and later** ask once whether Skrivist may find devices on your local
  network. If that was declined, allow it under System Settings › Privacy &
  Security › Local Network.

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

## CrossPoint Reader (Xteink X3/X4 and others)

[CrossPoint Reader](https://github.com/crosspoint-reader/crosspoint-reader) e-ink
readers browse Armarium over OPDS and sync reading positions with it over the
KOReader sync protocol (KOSync). Create one API token for the device under
**Admin**; it serves both.

- **OPDS:** Settings → System → OPDS Servers → Add Server. URL
  `http://<host>:8580/opds/v1.2/catalog`, any user name, the token as the
  password. CrossPoint downloads the EPUB link of each entry; it can't read
  comics or PDFs. Plain HTTP is fine on your home network: CrossPoint sends
  the token as Basic auth, the same as it would over HTTPS to a proxy.
- **Progress sync:** Settings → System → KOReader Sync. **Sync Server URL**
  `http://<host>:8580/kosync`, any user name, the same token as the password,
  then **Authenticate** (not **Sign Up**: Armarium has no sign-up). Set
  **Document Matching** to **Binary**. **Send Document Metadata** is optional
  (see below).
- A sync from the reader moves the book's Armarium progress to the uploaded
  chapter and position, so it shows under "Continue reading" here and in OPDS.
  It follows the web reader's rules: the book counts as read once the last
  chapter is past 95%, and a sync never moves a book backwards or marks a read
  book unread.
- If you read on in Armarium's web reader afterwards, the next sync on the device
  offers that position, at the start of its chapter.
- **How Armarium knows the book.** A KOSync client names a book by an id
  computed from the file. With Binary matching that is KOReader's partial MD5
  of the file, which Armarium records whenever it serves an EPUB, so books
  downloaded through Armarium's OPDS match. For books put on the device another
  way (USB, the Calibre plugin) or with Filename matching, turn on **Send
  Document Metadata**: Armarium then matches an unknown id by the book's title (and
  author, when several share it).
- Positions for a book Armarium can't match are still stored and handed back to
  any device that asks, like any KOSync server.
- KOReader itself is not supported yet: it authenticates with an MD5 of the
  password only, which Armarium can't check against its stored token digests.

## skriv.ist (Skrivist)

Skrivist's OPDS browser works with Armarium's feeds for EPUBs. Its server-side
proxy refuses private and Tailscale addresses, so Armarium must be reachable on a
public HTTPS host name.

## JSON API

For your own tools; authenticate with `Authorization: Bearer <token>`. A
`?token=` query parameter also authenticates a single request, but feed links
never carry it, so OPDS clients use Basic, Bearer or the path form instead.

```text
GET  /api/libraries
GET  /api/libraries/{id}/series?q=&offset=&limit=
GET  /api/series/{id}                    series + items in reading order
GET  /api/items?library=&series=&q=&status=&tag=&source=&sort=&offset=&limit=
GET  /api/items/{id}                     item (with progress) + library
GET  /api/items/{id}/cover               JPEG thumbnail
GET  /api/items/{id}/pages               comic page names in reading order
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
not a token or account. Any reader with a Bearer token may edit them (Basic
credentials are read-only from other origins, as for every write). Generic OPDS
clients continue reading normally; they must implement this API to use panels.
Skrivist Comics uses it for streaming and for CBZ/CBR downloaded over OPDS.

Page numbers follow Armarium's page order: pages at the archive root (a cover)
first, then chapter folders, each in natural order. A client that unpacked the
file itself matches its pages to these numbers by entry name with
`GET /api/items/{id}/pages`, which lists `{"name": …}` in order. Items report
`"pageOrder": 2`; numbers stored before v0.5.0 (root files last) were remapped
once at upgrade.

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
0.5% wide/high. Geometry and bounds are validated, and a body may be up to
256 KiB (the largest valid correction is about 100 KiB); a larger one gets
HTTP 413. The server hashes the original page before accepting the annotation;
images are not uploaded.

Revision 0 creates a record. Later saves send the revision returned by GET;
successful writes increment it and set `updatedAt` (Unix seconds).
Progress `updatedAt` likewise grows with every applied write, even within one
second, so clients can compare it to tell which side changed last. HTTP 409 means
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
