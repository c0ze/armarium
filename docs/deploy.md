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
   `allowed_hosts`.
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
   with `id` over SSH).
3. Keep `restart: always`: containers set to `unless-stopped` stay down after
   Container Manager restarts.
4. Keep `mem_limit: 256m` and set `max_open_archives = 2` under `[limits]` on
   small NAS CPUs; `workers = 1` under `[scan]` keeps scans gentle.
5. For HTTPS, add a reverse-proxy rule (Control Panel → Login Portal → Advanced →
   Reverse Proxy) from `https://books.example.com` to `http://localhost:8580`,
   add that host to `allowed_hosts`, and set `secure_cookie = true`.

To update, pull the new image in Container Manager and rebuild the project.

## Binary and systemd

Download the binary for your platform from the GitHub release, or build it
(`make`, needs Go and Node). Then:

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
  `secure_cookie = true`.
- Tokens are shown once and stored hashed. Revoke them from **Admin**.
