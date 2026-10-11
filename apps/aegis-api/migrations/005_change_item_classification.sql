ALTER TABLE change_event_items ADD COLUMN classification_version INTEGER NOT NULL DEFAULT 0
    CHECK (classification_version >= 0);
ALTER TABLE change_event_items ADD COLUMN category TEXT NOT NULL DEFAULT 'unclassified'
    CHECK (length(category) > 0);
ALTER TABLE change_event_items ADD COLUMN subcategory TEXT NOT NULL DEFAULT 'unclassified'
    CHECK (length(subcategory) > 0);
ALTER TABLE change_event_items ADD COLUMN classification_rule TEXT NOT NULL DEFAULT '';
ALTER TABLE change_event_items ADD COLUMN classified_at TEXT NULL;
