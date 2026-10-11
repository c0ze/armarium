# Deploying Armarium

Armarium is one static binary with the web UI inside. Run it in Docker or run the
binary directly; either way it needs a config file and a writable data folder.
Library folders can (and should) be read-only.

## Docker Compose

Images are published for `linux/amd64` and `linux/arm64` as
`ghcr.io/c0ze/armarium` and `docker.io/coze/armarium`, tagged with the version and
`latest`.

```bash
mkdir armarium && cd armarium && mkdir data
curl -LO https://raw.githubusercontent.com/c0ze/armarium/main/deploy/compose.yaml
curl -L -o armarium.toml https://raw.githubusercontent.com/c0ze/armarium/main/deploy/armarium.example.toml
```

1. Edit `compose.yaml`: mount your library folders read-only under `/libraries/…`
   and set `user:` to the owner of `./data` (`id -u` / `id -g`).
2. Edit `armarium.toml`: one `[[library]]` per folder (paths as seen inside the
   container), `listen = "0.0.0.0:8580"`, and every host name you will use in
   `allowed_hosts`. To read on a phone, also set `public_url` (your HTTPS address)
   and `cors_origins` (see [Phones](#phones) below).
3. Set the owner password:
   ```bash
   docker run --rm -i ghcr.io/c0ze/armarium hash-password
   ```
   and paste the output into `password_hash`.
4. `docker compose up -d`, open `http://<host>:8580`, log in, and run a scan
   from **Admin**.

Updating: `docker compose pull && docker compose up -d`. The database migrates
itself on start.

## Synology (Container Manager)

The same compose file works as a Container Manager **Project**:

1. Create `/volume1/docker/armarium/` with a `data` folder and your
   `armarium.toml`, and put `compose.yaml` there.
2. In the compose file, mount shares like `/volume1/comics:/libraries/comics:ro`,
   and set `user: "1026:100"` (the first DSM user and the `users` group; check
   with `id` over SSH). Shared folders grant access through DSM groups, not
   `users`: if covers and pages 404 while the scan finds files, add
   `group_add: ["101"]` (`administrators`, or the group the share's
   permissions name). Test with
   `docker run --rm -u 1026:100 --group-add 101 -v /volume1/comics:/c:ro busybox ls /c`.
3. Keep `restart: always`: containers set to `unless-stopped` stay down after
   Container Manager restarts.
4. Keep `mem_limit: 256m` and set `max_open_archives = 2` under `[limits]` on
   small NAS CPUs; `workers = 1` under `[scan]` keeps scans gentle.
5. For HTTPS, add a reverse-proxy rule (Control Panel → Login Portal → Advanced →
   Reverse Proxy) from `https://books.example.com` to `http://localhost:8580`,
   add that host to `allowed_hosts`, set `public_url = "https://books.example.com"`,
   and set `secure_cookie = true` once you only log in through HTTPS.

To update, pull the new image in Container Manager and rebuild the project.

## Phones

Skrivist Comics and Skrivist Books are web apps served over HTTPS, so they can only
load an HTTPS Armarium, and only from origins Armarium allows:

```toml
allowed_hosts = ["books.example.com"]
public_url    = "https://books.example.com"
cors_origins  = [
  "https://comics.skriv.ist", "https://books.skriv.ist",
  "tauri://localhost", "http://tauri.localhost", "https://tauri.localhost",
]
```

Restart, then **Admin → Pair a phone**: pick a reader and scan the QR code with the
phone's camera. Admin warns when `public_url` is missing, points at this computer,
or a reader's origin is missing from `cors_origins`, and refuses to pair over plain
HTTP. More in [clients.md](clients.md#pairing-a-phone).

The Mac and Windows apps pair from **Admin → This computer** instead, and can use
plain HTTP on your network ([clients.md](clients.md#pairing-this-computer)).

This list also supports Skrivist Books and Comics on macOS and Windows and is
included in the example configuration from v0.3.2. Existing installations must
add the desktop origins to their own `cors_origins` and restart after upgrading;
the update does not replace your configuration. Keep any other reader origins
you already use.

## Forgot the owner password?

Armarium keeps only an argon2id hash of it, so it can't be recovered, only replaced:

```bash
docker exec -i armarium /armarium hash-password      # type the new password, press Enter
```

Paste the printed hash into `password_hash` in `armarium.toml` and restart the
container (`docker compose restart`). Without Docker, run `armarium hash-password`
instead. API tokens are unaffected.

## Binary and systemd

Download the binary for your platform from the GitHub release, or build it
(`make`, needs Go and Node).

On macOS, use the universal DMG (Apple Silicon and Intel). It is signed,
notarized, and stapled; mount it and copy `armarium` and the configuration example
to a folder of your choice. Run the commands from Terminal.

On Linux, install the binary and systemd service:

```bash
install -m 755 armarium ~/.local/bin/
mkdir -p ~/.config/armarium ~/.local/share/armarium
cp deploy/armarium.example.toml ~/.config/armarium/armarium.toml   # set data_dir
cp deploy/armarium.service ~/.config/systemd/user/
systemctl --user daemon-reload && systemctl --user enable --now armarium
```

## Commands

```text
armarium serve                    run the server (default)
armarium scan [library]           scan once and exit
armarium token add <name>         create an API token (printed once)
armarium token list | rm <name>
armarium library list             libraries in the database, configured or not
armarium library rename <old> <new>   keep a library's items under a new name
armarium library rm <name>        delete a library with its items and progress
armarium hash-password            argon2id hash of a password read from stdin
armarium import-kavita --db kavita.db --library <name> --root <path Kavita saw>
armarium import-skrivist --db reader_index.db --library <name>
armarium healthcheck              used by the Docker HEALTHCHECK
armarium version
```

In Docker, run them with `docker exec armarium /armarium <command>` (the image
sets `ARMARIUM_CONFIG=/config/armarium.toml`).

## Moving from Kavita

1. Point an Armarium library at the same folder Kavita used and scan it.
2. Copy Kavita's `kavita.db` (from its config folder) somewhere Armarium can read.
3. `armarium import-kavita --db kavita.db --library Comics --root /comics`, where
   `--root` is the library path as Kavita saw it. Progress is matched by file path;
   the newer of the two sides wins, so re-running is safe.

Try it on a copy first; the importer reads Kavita's `AppUserProgresses`,
`MangaFile` and `Chapter` tables.

## Security notes

- Every request except `/healthz` needs the owner session or an API token.
- `allowed_hosts` must list the names you use; anything else gets `421`.
- Serve over HTTPS if Armarium is reachable from outside your network, and set
  `secure_cookie = true`. Behind a reverse proxy, also list it in
  `trusted_proxies` (below): the cookie is then Secure for visitors who came over
  HTTPS even with `secure_cookie = false`, and login limits count each visitor.
- Five login attempts a minute are allowed per client (30 in all). Without
  `trusted_proxies`, everyone behind the proxy counts as one client.
- Changing `password_hash` logs out every owner session at the next start.
- Tokens are shown once and stored hashed. Revoke them from **Admin**.

## Upgrading to v0.5.0

Back up the data folder (`armarium.db`) first, as for any upgrade. Then:

- **Update the Skrivist readers first.** v0.5.0 numbers comic pages with files at
  the archive root (a cover) before chapter folders, and moves stored reading
  positions and panel corrections to the new numbers once, at the first start.
  Current Skrivist Books and Comics know about this (`pageOrder` on items); an old
  Comics that still remembers a page by its old number would write that number
  back the first time it opens such a comic. The hosted readers update on their
  own; update the Mac and Windows apps before upgrading Armarium.
- **Item and series ids are now never reused** (a one-time table rebuild at the
  first start; it keeps every id).
- **A library missing from the config no longer deletes anything.** Armarium
  refuses to start and names it. Put it back, keep its items under a new name with
  `armarium library rename OLD NEW`, or delete it with `armarium library rm NAME`.
- **Owner sessions end when `password_hash` changes.** The first start after the
  upgrade only records the current hash.
- **`trusted_proxies`** lists reverse proxies whose `X-Forwarded-For` and
  `X-Forwarded-Proto` Armarium believes. On a Synology NAS with Armarium in
  Container Manager behind the DSM reverse proxy, that is the Docker bridge
  gateway, usually:
  ```toml
  trusted_proxies = ["172.17.0.1"]
  ```
  Check it with `docker network inspect bridge`. Without it the cookie is Secure
  only with `secure_cookie = true`, and every proxied visitor shares one login
  budget.
- **Environment variables that override `armarium.toml` are logged.** The Docker
  image sets `ARMARIUM_LISTEN` and `ARMARIUM_DATA_DIR`, which win over the file.
- **`cors_origins` entries are normalised** to the form browsers send (no trailing
  slash, lower case, no default port); an entry with a path is an error.
