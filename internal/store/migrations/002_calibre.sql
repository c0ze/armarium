-- Calibre libraries: authors, covers from cover.jpg, download-only formats
-- (MOBI, AZW3, ...) and extra formats of the same book. The format CHECK goes:
-- formats are validated in code, and a new one should not need a migration.
CREATE TABLE item_new (
    id         INTEGER PRIMARY KEY,
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
    UNIQUE (library_id, path)
);
INSERT INTO item_new (id, library_id, series_id, path, format, size, mtime, title, sort_title, number,
                      pages, source, added_at, missing_at, has_cover)
SELECT id, library_id, series_id, path, format, size, mtime, title, sort_title, number,
       pages, source, added_at, missing_at, format != 'pdf'
FROM item;
DROP TABLE item;
ALTER TABLE item_new RENAME TO item;
CREATE INDEX item_series ON item (series_id);
CREATE INDEX item_added ON item (added_at);

-- Other formats of the same Calibre book, offered as extra downloads.
CREATE TABLE item_file (
    item_id INTEGER NOT NULL REFERENCES item (id) ON DELETE CASCADE,
    format  TEXT NOT NULL,
    path    TEXT NOT NULL,
    size    INTEGER NOT NULL,
    PRIMARY KEY (item_id, format)
);
