# Project website

The public project site is **https://c0ze.github.io/armarium/**. Its source is in
`site/`, separate from the embedded application UI in `web/`.

It is plain HTML, CSS, and a small JavaScript enhancement for installation tabs
and copy buttons. There is no build step, package installation, analytics, or
third-party runtime request. Bebas Neue is self-hosted under its bundled OFL
license in `site/assets/`. Cover illustrations are original HTML and SVG artwork;
the library preview is illustrative, not a screenshot of a personal collection.

## Preview

From the repository root:

```sh
python3 -m http.server 8594 --bind 127.0.0.1
```

Open `http://127.0.0.1:8594/site/`. Check desktop and narrow phone layouts,
installation tabs (including keyboard arrows), copy buttons, and the OPDS
disclosure. With JavaScript disabled, both installation methods remain visible.

## Publish

GitHub Pages uses **GitHub Actions** as its publishing source. The
`.github/workflows/pages.yml` workflow uploads only `site/` and publishes it to
the `github-pages` environment. It runs when changes to `site/` or that workflow
reach `main`, and can also be started manually from Actions. No release or
application build is required.

Keep the site’s setup commands, formats, and pairing instructions aligned with
`docs/deploy.md`, `docs/clients.md`, and `deploy/armarium.example.toml`. In
particular, distinguish local progress in downloaded OPDS titles from server
progress through Armarium’s web UI, page streaming, and Comics Armarium mode.

The CSS, script, font, and favicon use relative URLs so the project path
`/armarium/` works without rewriting. If the site moves, also update the canonical
and Open Graph URLs in `site/index.html`, the README link, and this document.
