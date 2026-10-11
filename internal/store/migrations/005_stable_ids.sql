-- Item and series ids stay unique for good (AUTOINCREMENT): offline readers keep
-- them, so a deleted item's id must never come back as a different book.
CREATE TABLE series_new (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    library_id INTEGER NOT NULL REFERENCES library (id) ON DELETE CASCADE,
    path       TEXT NOT NULL,
    name       TEXT NOT NULL,
    sort_name  TEXT NOT NULL,
    search     TEXT NOT NULL DEFAULT '',
    UNIQUE (library_id, path)
);
INSERT INTO series_new (id, library_id, path, name, sort_name, search)
SELECT id, library_id, path, name, sort_name, search FROM series;
DROP TABLE series;
ALTER TABLE series_new RENAME TO series;

CREATE TABLE item_new (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    library_id INTEGER NOT NULL REFERENCES library (id) ON DELETE CASCADE,
    series_id  INTEGER NOT NULL REFERENCES series (id) ON DELETE CASCADE,
    path       TEXT NOT NULL,
    format     TEXT NOT NULL,
    size       INTEGER NOT NULL,
    mtime      INTEGER NOT NULL,
    title      TEXT NOT NULL,
    sort_title TEXT NOT NULL,
    number     REAL,
    pages      INTEGER NOT NULL DEFAULT 0,
    source     TEXT NOT NULL DEFAULT '',
    added_at   INTEGER NOT NULL,
    missing_at INTEGER,
    author     TEXT NOT NULL DEFAULT '',
    has_cover  INTEGER NOT NULL DEFAULT 1,
    search     TEXT NOT NULL DEFAULT '',
    UNIQUE (library_id, path)
);
INSERT INTO item_new (id, library_id, series_id, path, format, size, mtime, title, sort_title, number,
                      pages, source, added_at, missing_at, author, has_cover, search)
SELECT id, library_id, series_id, path, format, size, mtime, title, sort_title, number,
       pages, source, added_at, missing_at, author, has_cover, search FROM item;
DROP TABLE item;
ALTER TABLE item_new RENAME TO item;
CREATE INDEX item_series ON item (series_id);
CREATE INDEX item_added ON item (added_at);

-- One-time jobs and fingerprints.
CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
