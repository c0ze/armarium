CREATE TABLE library (
    id   INTEGER PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    root TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('comics', 'books'))
);

CREATE TABLE series (
    id         INTEGER PRIMARY KEY,
    library_id INTEGER NOT NULL REFERENCES library (id) ON DELETE CASCADE,
    path       TEXT NOT NULL,
    name       TEXT NOT NULL,
    sort_name  TEXT NOT NULL,
    UNIQUE (library_id, path)
);

CREATE TABLE item (
    id         INTEGER PRIMARY KEY,
    library_id INTEGER NOT NULL REFERENCES library (id) ON DELETE CASCADE,
    series_id  INTEGER NOT NULL REFERENCES series (id) ON DELETE CASCADE,
    path       TEXT NOT NULL,
    format     TEXT NOT NULL CHECK (format IN ('cbz', 'cbr', 'epub', 'pdf')),
    size       INTEGER NOT NULL,
    mtime      INTEGER NOT NULL,
    title      TEXT NOT NULL,
    sort_title TEXT NOT NULL,
    number     REAL,
    pages      INTEGER NOT NULL DEFAULT 0,
    source     TEXT NOT NULL DEFAULT '',
    added_at   INTEGER NOT NULL,
    missing_at INTEGER,
    UNIQUE (library_id, path)
);
CREATE INDEX item_series ON item (series_id);
CREATE INDEX item_added ON item (added_at);

CREATE TABLE tag (
    id   INTEGER PRIMARY KEY,
    name TEXT NOT NULL UNIQUE
);

CREATE TABLE item_tag (
    item_id INTEGER NOT NULL REFERENCES item (id) ON DELETE CASCADE,
    tag_id  INTEGER NOT NULL REFERENCES tag (id) ON DELETE CASCADE,
    PRIMARY KEY (item_id, tag_id)
);

CREATE TABLE progress (
    item_id    INTEGER PRIMARY KEY REFERENCES item (id) ON DELETE CASCADE,
    page       INTEGER NOT NULL DEFAULT 0,
    locator    TEXT NOT NULL DEFAULT '',
    status     TEXT NOT NULL DEFAULT 'unread' CHECK (status IN ('unread', 'reading', 'read')),
    updated_at INTEGER NOT NULL
);

CREATE TABLE token (
    id           INTEGER PRIMARY KEY,
    name         TEXT NOT NULL UNIQUE,
    hash         TEXT NOT NULL UNIQUE,
    created_at   INTEGER NOT NULL,
    last_used_at INTEGER
);

CREATE TABLE session (
    hash       TEXT PRIMARY KEY,
    expires_at INTEGER NOT NULL
);
