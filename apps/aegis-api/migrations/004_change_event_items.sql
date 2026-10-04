ALTER TABLE change_events ADD COLUMN analysis_version INTEGER NOT NULL DEFAULT 0
    CHECK (analysis_version >= 0);
ALTER TABLE change_events ADD COLUMN analyzed_at TEXT NULL;
ALTER TABLE change_events ADD COLUMN item_count INTEGER NOT NULL DEFAULT 0
    CHECK (item_count >= 0);

CREATE TABLE change_event_items (
    id TEXT PRIMARY KEY,
    change_event_id TEXT NOT NULL REFERENCES change_events(id) ON DELETE CASCADE,
    ordinal INTEGER NOT NULL CHECK (ordinal >= 0),
    operation TEXT NOT NULL CHECK (operation IN ('add', 'remove')),
    context_path_json TEXT NOT NULL DEFAULT '[]',
    command TEXT NOT NULL CHECK (length(command) > 0),
    sensitive INTEGER NOT NULL DEFAULT 0 CHECK (sensitive IN (0, 1)),
    created_at TEXT NOT NULL,
    UNIQUE (change_event_id, ordinal)
);
