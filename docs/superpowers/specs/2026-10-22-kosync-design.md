# KOReader progress sync (KOSync) — design

## Why

The owner reads on an Xteink X4 running CrossPoint Reader 1.6.5. CrossPoint
already browses and downloads from Armarium over OPDS, but its reading
position only goes to a KOReader sync server, so it never reaches Armarium's
"Continue reading" or the web reader. Armarium speaks KOSync itself, so the X4
(and any other CrossPoint device) syncs straight to it.

## Goals

- A KOSync server under `/kosync`: CrossPoint's server URL is
  `http://<host>:8580/kosync`.
- X4 → Armarium: an uploaded position moves the book's Armarium progress
  (chapter + position in it), so it shows in the UI and in OPDS "Continue
  reading".
- Armarium → X4: when Armarium's progress for a book is newer than the last
  KOSync upload (read in the web reader), a download returns it at chapter
  granularity.
- KOSync round trip: whatever a device uploads, any device gets back verbatim,
  even for a document Armarium can't match to an item.

## Non-goals

- Sign-up. `POST /users/create` is refused; credentials are an API token.
- KOReader's own app. It sends only `x-auth-user` / `x-auth-key` (an MD5 of
  the password), which can't be checked against stored SHA-256 token digests.
  CrossPoint also sends HTTP Basic, which is what Armarium checks.
- Multiple users. Armarium has one owner; the KOSync user name is ignored.
- Comics and PDFs. CrossPoint syncs EPUBs only.

## Auth and CSRF

- Credentials: Basic auth with any user name and an API token as the password,
  exactly as for OPDS. Bearer tokens work too. Routes use the existing `Reader`
  access level; `/users/auth` answers 200 `{"authorized":"OK"}`.
- A Basic credential is ambient, so writes normally need an Origin from
  `allowed_hosts`. CrossPoint sends none. A `/kosync/` request that carries an
  `X-Auth-User` header is treated like a Bearer token: a browser can't add that
  header cross-site without a preflight, and `/kosync/` gets no CORS headers,
  so no preflight passes. The JSON-only body rule still applies.

## Matching a document to a book

KOSync names a book by a document id: either KOReader's partial MD5 of the
file ("Binary" in CrossPoint) or an MD5 of the file name ("Filename", the
CrossPoint default). The table `kosync_document` maps ids to items:

1. **On download.** When Armarium serves an EPUB file (OPDS acquisition or
   `/api/items/{id}/file`), it computes the partial MD5 (12 reads of at most
   1 KiB) and links that id to the item. Armarium serves files byte for byte,
   so the X4's Binary id matches.
2. **On upload, from metadata.** If CrossPoint's "send metadata" is on, an
   unknown id comes with `{filename, title, authors}`. Armarium looks up EPUB
   items whose folded title (and author, when given) match, at most 10. One
   whose file's partial MD5 equals the id is linked; otherwise exactly one
   title (+author) match is linked (this covers Filename ids).

Docs recommend Binary matching plus downloading through Armarium's OPDS.

## Positions

CrossPoint's progress is a KOReader XPointer, `/body/DocFragment[N]/body/…`,
with N the 1-based spine index, plus an overall `percentage`.

- Upload: chapter = N − 1 (from the percentage when there's no DocFragment),
  scroll = clamp(percentage × chapters − chapter, 0, 1). Saved with the
  existing monotonic `SaveProgress` (page = chapter + 1, web reader locator
  `{"chapter","scroll"}`), the same rules as the web reader's saves.
- Download: the stored KOSync record, unless the linked item's progress was
  updated after it (then `/body/DocFragment[page]/body`, percentage =
  (chapter + scroll) / chapters, device "Armarium"). After an upload the
  record's time is set to at least the progress time it caused, so an upload
  never echoes back as "newer".
- No record and no newer Armarium progress: 404, which CrossPoint treats as
  "no remote progress".
