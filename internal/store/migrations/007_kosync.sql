-- KOReader sync (KOSync): one row per document id a device has used. item_id
-- links it to the book it names, once known; the rest is the last position a
-- device uploaded (updated_at 0: linked on download, nothing uploaded yet).
CREATE TABLE kosync_document (
    document   TEXT PRIMARY KEY,
    item_id    INTEGER REFERENCES item (id) ON DELETE SET NULL,
    progress   TEXT NOT NULL DEFAULT '',
    percentage REAL NOT NULL DEFAULT 0,
    device     TEXT NOT NULL DEFAULT '',
    device_id  TEXT NOT NULL DEFAULT '',
    updated_at INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX kosync_document_item ON kosync_document (item_id);
