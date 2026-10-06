# Armarium

A small, self-hosted library server for **comics and ebooks**: point it at your
folders or your Calibre library and read them in the browser, in OPDS apps, or in
guided-view comic readers.

An *armarium* was the cupboard where a medieval monastery kept its books.

**[Project website](https://c0ze.github.io/armarium/)** — why Armarium exists,
basic setup, and reading with Skrivist Books and Comics.

- **Comics:** CBZ and CBR, streamed page by page (OPDS page streaming and a JSON API);
  series come from folders, issue numbers from file names.
- **Books:** EPUB and PDF read in the browser; MOBI, AZW3, FB2, DjVu and anything
  else Calibre holds are offered for download.
- **Calibre libraries:** reads `metadata.db` directly for titles, authors, series,
  tags and covers. One book per item, with its other formats as extra downloads.
  A lightweight stand-in for Calibre-Web's browsing and OPDS.
- **Reading progress** per item that only moves forward unless you reset it, shared
  by the web UI, OPDS page streaming and connected readers.
- **Web UI:** library grid with search and filters, an EPUB reader, a quick comic
  viewer, and an admin page for scanning and API tokens. Works on phones.
- **Phone pairing:** Admin shows a QR code; scan it with the phone's camera and
  [Skrivist Comics](https://comics.skriv.ist) or [Skrivist Books](https://books.skriv.ist)
  opens with your library connected. No addresses or tokens to type.
- **Light on hardware:** one static binary with pure-Go SQLite, about 30–50 MB of
  memory while scanning ten thousand books, incremental scans that only open new or
  changed files, and no background indexing unless you ask for it.
- **Careful with untrusted files:** EPUB content goes through an allowlist sanitizer
  and a strict Content-Security-Policy; archives are bounded in size, entries and
  pixels; every request needs a session or token; Host allowlist, CSRF and CORS
  rules are tested.

What it is not: a metadata manager, a scraper, a multi-user service or a store.
Libraries are mounted read-only; Armarium only writes to its data folder.

## Quick start

```bash
docker run --rm -i ghcr.io/c0ze/armarium hash-password   # owner password hash
```

Then follow [docs/deploy.md](docs/deploy.md) for Docker Compose, Synology and
systemd. Every setting is explained in
[deploy/armarium.example.toml](deploy/armarium.example.toml).

### Read on your phone

1. Put Armarium behind HTTPS (a reverse proxy such as Synology's, Caddy or
   Tailscale Serve) and set three things in `armarium.toml`:
   ```toml
   allowed_hosts = ["armarium.example.com"]
   public_url    = "https://armarium.example.com"
   cors_origins  = [
     "https://comics.skriv.ist", "https://books.skriv.ist",
     "tauri://localhost", "http://tauri.localhost", "https://tauri.localhost",
   ]
   ```
2. Restart, log in, open **Admin → Pair a phone** and pick Skrivist Comics or
   Skrivist Books.
3. Point the phone's camera at the QR code, open the link, tap **Connect**.

Each pairing gets its own token; revoke it in Admin to unpair. Details in
[docs/clients.md](docs/clients.md#pairing-a-phone).

The example configuration also allows the Skrivist macOS and Windows apps.
When upgrading an existing installation to v0.3.2 or later, add the three desktop
origins above to your existing `cors_origins` and restart Armarium; upgrading
the binary or Docker image preserves your configuration.

From source (Go 1.25+, Node 22+):

```bash
make                      # builds web/dist and bin/armarium
cp deploy/armarium.example.toml armarium.toml
bin/armarium hash-password
bin/armarium serve        # http://127.0.0.1:8580
```

## Apps

- **OPDS readers** (KOReader, Panels, Chunky, …): `/opds/v1.2/catalog` with Basic
  auth, or a token in the path for apps without it.
- **Skrivist Comics and Skrivist Books** (comics.skriv.ist, books.skriv.ist):
  offline-first readers in the browser; pair them from Admin with a QR code.
- **PanelFlow / comics.skriv.ist Armarium mode:** guided panel-by-panel reading with
  progress sync.
- **Your own tools:** a small JSON API with bearer tokens.

See [docs/clients.md](docs/clients.md). Coming from Kavita? `armarium import-kavita`
copies read progress ([docs/deploy.md](docs/deploy.md#moving-from-kavita)).

## Development

```bash
make check    # go vet, staticcheck (if installed), Go tests, svelte-check, UI tests
```

Architecture, data model and security design: [docs/design.md](docs/design.md).
The GitHub Pages website lives in `site/`; see
[docs/github-pages.md](docs/github-pages.md) to preview and publish it.
Release builds and macOS signing: [docs/releasing.md](docs/releasing.md).

## License

MIT, see [LICENSE](LICENSE).
