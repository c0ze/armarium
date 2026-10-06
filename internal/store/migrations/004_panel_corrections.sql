-- Human-reviewed geometry and reading order belong to the comic, not a client.
CREATE TABLE panel_correction (
    item_id INTEGER NOT NULL REFERENCES item (id) ON DELETE CASCADE,
    page INTEGER NOT NULL CHECK (page >= 1),
    revision INTEGER NOT NULL CHECK (revision >= 1),
    document TEXT NOT NULL,
    PRIMARY KEY (item_id, page)
);
