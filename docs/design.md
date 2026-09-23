# Design

## 1. Shape

```
armarium (one Go binary)
├── http server (net/http, Go 1.22+ pattern routing)
│   ├── /api/*          JSON API for the UI and PanelFlow
│   ├── /opds/*         OPDS 1.2 + PSE (Atom XML)
│   ├── /read/*         sanitized EPUB chapters and resources, strict CSP
│   └── /*              embedded Svelte UI (SPA fallback)
├── scanner             walks library roots → SQLite (incremental)
├── formats/            cbz, cbr, epub, pdf readers behind one interface
├── covers              lazy thumbnail cache on disk
└── store               SQLite (modernc.org/sqlite), migrations embedded
```

Configuration comes from a single `armarium.toml` plus env overrides: libraries (root, kind), listen address, allowed hosts, data dir, password hash and token hashes.

## 2. Package layout

```
cmd/armarium/            main: config, wire, serve; `armarium scan`, `armarium token add`
internal/config/
internal/store/       db open, migrations/*.sql, queries
internal/scan/        walker, change detection, throttling
internal/formats/     Archive interface; cbz.go (archive/zip), cbr.go, epub.go, pdf.go
internal/covers/
internal/httpapi/     handlers, middleware (auth, host, csrf, limits)
internal/opds/        feed builders (encoding/xml)
internal/sanitize/    EPUB XHTML allowlist sanitizer
web/                  Svelte 5 + Vite app; `npm run build` → web/dist, embedded
```

Keep files under ~300 lines, one responsibility each.

## 3. Formats

```go
type Container interface {
    Pages() int                             // comics: image entries in natural sort order
    Page(i int) (io.ReadCloser, string, error) // stream + media type
    Close() error
}
```

- **CBZ:** `archive/zip`. Image entries are sorted by natural order (`page2` < `page10`), and `__MACOSX` and dotfiles are ignored. Pages stream straight from the zip entry with no temp files.
- **CBR:** `github.com/nwaples/rardecode/v2` (pure Go). RAR has no random access, so page N means reading forward. rardecode's `RarFS` indexes the file headers once, so a page seeks straight to its entry in non-solid archives, and an LRU of recently opened archives keeps page turns fast. The decoder dictionary is capped at 32 MiB.
- **EPUB:** `archive/zip` plus our own OPF/NCX/nav parsing (small; the existing Python `reader/epub.py` is a good reference for behaviour, not code). Chapters are sanitized before serving (§7).
- **Download-only formats** (MOBI, AZW3, AZW, FB2, DjVu, plus whatever a Calibre library holds): listed, served as files, never opened.
- **Calibre libraries** (`internal/calibre`): `metadata.db` opened read-only; per book the first format present on disk in the order EPUB, CBZ, CBR, PDF, AZW3, MOBI, … becomes the item and the rest go to `item_file` as extra downloads (`/file?format=`, one OPDS acquisition link each). Series come from Calibre, or the first author for books outside a series. Paths from the database are refused if they leave the root.
- **PDF:** v1 serves the file for the browser's own PDF viewer; the page count comes from a light parser that reads the first and last MiB (0 when the page tree sits in a compressed object stream). Rendering PDF pages to images needs MuPDF or pdfium through CGO, which would break the pure-Go, static-binary build, so PDF comics are not streamed page by page.

## 4. Data model (SQLite)

```sql
library(id, name, root, kind CHECK(kind IN ('comics','books')), scan_cron)
series(id, library_id, path, name, sort_name)              -- a folder
item(id, library_id, series_id, path, format, size, mtime,
     title, number, pages, source, added_at, missing_at)   -- a file
tag(id, name) ; item_tag(item_id, tag_id)
progress(item_id PRIMARY KEY, page, locator, status CHECK(status IN
         ('unread','reading','read')), updated_at)
token(id, name, hash, created_at, last_used_at)
```

- `path` is relative to the library root. When a scan no longer finds a file it sets `missing_at` rather than deleting the row, so progress survives a temporary NFS hiccup. Rows missing for more than 30 days are pruned.
- Progress writes are monotonic: `UPDATE ... WHERE excluded.page >= page OR :explicit_reset`. This is enforced in SQL, not just in the client.
- The database runs in WAL mode. The scanner writes in small batches so API writes never hit "database is locked" (a bug found in the current reader).

## 5. HTTP API (JSON)

```
GET  /api/session                     {authenticated, admin, passwordSet, panelflowUrl}
POST /api/login | /api/logout         owner password → session cookie
GET  /api/libraries
GET  /api/libraries/{id}/series?q=&offset=&limit=   series with counts + coverItemId
GET  /api/series/{id}                 series + items in reading order
GET  /api/items?library=&series=&q=&status=&tag=&source=&sort=&offset=&limit=
GET  /api/items/{id}                  {item (with progress), library}
GET  /api/facets?library=             sources (with labels) and tags
GET  /api/items/{id}/cover            lazy JPEG thumbnail
GET  /api/items/{id}/pages/{n}        comic page image (1-based), ETag
GET  /api/items/{id}/file             original file (Range; ?inline=1 for PDF)
GET  /api/items/{id}/toc              EPUB {chapters, toc}
PUT  /api/items/{id}/progress         {page?, locator?, status?, reset?} → {progress, applied}
GET|POST /api/scan                    status | start {library?}      (admin)
GET|POST /api/tokens, DELETE /api/tokens/{id}                        (admin)
GET  /healthz                         no auth, no Host check
GET  /read/base.css                   reader defaults
GET  /read/{id}/chapter/{n}           sanitized chapter n (0-based spine index), strict CSP
GET  /read/{id}/res/{idx}             manifest item; image (not SVG)/CSS/font only
/opds/v1.2/… and /opds/t/{token}/v1.2/…   OPDS (§6)
```

Numeric IDs are parsed as int64; out-of-range or garbage input returns 404, never 500.
Sort is one of `title`, `added`, `recent` (default: series order). Progress `page` is the
1-based page (comics) or chapter (EPUB) the reader is on; reaching the last one, or
`status: "read"`, marks the item read; `status: "unread"` is the reset. Backward writes
are ignored in SQL and reported as `applied: false`.

## 6. OPDS and PanelFlow

- `/opds/v1.2/catalog` → libraries → series (paged navigation feeds) → items (acquisition feed), plus "Continue reading", "Recently added" and search. Each comic entry carries a PSE link `…/items/{id}/pse/{pageNumber}` (0-based, per the PSE spec) with `pse:count` and `pse:lastRead`.
- **Auth for OPDS:** HTTP Basic (any username, token as password), `?token=`, or the token in the path: `/opds/t/{token}/v1.2/catalog`. Every link keeps the request's prefix, so path-token clients never lose it. Tokens are stored hashed (SHA-256 of a 256-bit secret) and never logged.
- Feed-level links come before entries and the first `rel="search"` is a direct `{searchTerms}` template (skrivist.app's parser needs both). See [clients.md](clients.md).
- **PanelFlow integration.** PanelFlow (and comics.skriv.ist, which shares its frontend) now has an **Armarium mode** on the JSON API above, so no Kavita shim was built. Browser readers call Armarium directly; `cors_origins` lists their origins (no credentials, preflight answered before auth).

## 7. Security

- **Middleware order:** limits (body size, header timeout, `ReadHeaderTimeout`/`ReadTimeout`) → Host allowlist → CORS (configured reader origins only) → per-route guard: auth (session cookie or token) and access level (reader / owner-only admin) → CSRF (on non-GET: `Content-Type: application/json`; ambient credentials, i.e. the cookie or Basic, need an `Origin` of our own hosts; explicit tokens may come from a `cors_origins` origin) → handler.
- **Session:** a single password (argon2id hash in config) gives an HttpOnly, SameSite=Strict, Secure (when served over TLS) cookie.
- **Untrusted content:** the EPUB sanitizer is an allowlist on `golang.org/x/net/html` tokens. It drops `script`, `style`, `svg`, `math`, `iframe`, `object`, `form` and every `on*` attribute, and it drops `srcset` and any URL that isn't a same-book resource. `/read/*` responses carry `Content-Security-Policy: default-src 'none'; img-src 'self'; style-src 'self'; font-src 'self'` and `nosniff`. The UI shows chapters in a sandboxed iframe as a second layer.
- **Decompression caps:** a maximum uncompressed entry size (e.g. 64 MiB), maximum image dimensions checked with `image.DecodeConfig` before any decode, and a maximum entries per archive.
- **Concurrency:** a semaphore on archive opens. Slow clients can't hold a slot forever: there's a write deadline per response.
- Every one of these gets a test. Known attacks on book servers (XSS through `svg`/`style` in EPUBs, raw resource serving, CSRF, DNS rebinding, integer overflow in IDs) are regression tests.

## 8. UI

**Svelte 5 (runes) + Vite + TypeScript**, built as a static SPA (plain Vite with a ~50-line history router, no router dependency) and embedded in the binary. EPUB chapters load in an `<iframe sandbox="allow-same-origin">` (no scripts; same origin so the parent can style it and read the scroll position).

Why Svelte rather than React or Vue:
- It's the framework PanelFlow already uses (SvelteKit), so the components, i18n approach and tooling carry over.
- It gives the smallest runtime of the three for a little app.
- Svelte 5 is where much of the current momentum is.

The trade-off: React is still the most popular by a wide margin, and Solid is the other "hot" small option. For a solo project that shares a codebase with PanelFlow, Svelte wins.

Styling uses plain CSS variables (the PanelFlow theme tokens), with no UI kit. The grid lazy-loads covers and pages results 60 at a time ("Load more").

## 9. Deployment

See [deploy.md](deploy.md): Docker (the `scratch` image, multi-arch), Docker
Compose or a Synology Container Manager project, the static binary with a systemd
user unit, and progress importers for Kavita and the Skrivist Books reader.
