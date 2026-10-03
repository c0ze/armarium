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
library's OPDS catalogue filled in; tap Connect and it's done. Each pairing creates
its own token, named like `books-phone-2026-10-04-1532`.

- Set `public_url` to the HTTPS address phones use, e.g.
  `https://armarium.example.com`. Its host must be in `allowed_hosts`. The hosted
  readers refuse plain-HTTP catalogues, so Admin won't pair over HTTP.
- Add the reader origins to `cors_origins` (`https://comics.skriv.ist`,
  `https://books.skriv.ist`): the readers fetch the catalogue from the browser.
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
GET  /api/items/{id}/file[?format=mobi]  the file (Range supported)
GET  /api/items/{id}/toc                 EPUB table of contents
PUT  /api/items/{id}/progress            {"page": n} | {"status": "read"|"unread"}
```

Progress only moves forward: a lower `page` is ignored (`"applied": false`) unless
you send `"reset": true`; `{"status": "unread"}` resets it. Details in
[design.md](design.md).
