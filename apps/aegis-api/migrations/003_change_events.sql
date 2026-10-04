CREATE UNIQUE INDEX idx_config_backups_identity_scope
    ON config_backups(id, device_id, config_type);

CREATE TABLE change_events (
    id TEXT PRIMARY KEY,
    device_id TEXT NOT NULL REFERENCES devices(id) ON DELETE RESTRICT,
    config_type TEXT NOT NULL CHECK (config_type IN ('running', 'startup')),
    from_backup_id TEXT NOT NULL,
    to_backup_id TEXT NOT NULL UNIQUE,
    detected_at TEXT NOT NULL,
    added_lines INTEGER NOT NULL DEFAULT 0 CHECK (added_lines >= 0),
    removed_lines INTEGER NOT NULL DEFAULT 0 CHECK (removed_lines >= 0),
    authorization_status TEXT NOT NULL DEFAULT 'unaccounted'
        CHECK (authorization_status IN ('unaccounted', 'authorized')),
    policy_status TEXT NOT NULL DEFAULT 'not_evaluated'
        CHECK (policy_status IN ('not_evaluated', 'pass', 'violation')),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    CHECK (from_backup_id <> to_backup_id),
    FOREIGN KEY (from_backup_id, device_id, config_type)
        REFERENCES config_backups(id, device_id, config_type) ON DELETE RESTRICT,
    FOREIGN KEY (to_backup_id, device_id, config_type)
        REFERENCES config_backups(id, device_id, config_type) ON DELETE RESTRICT
);

CREATE INDEX idx_change_events_device_detected
    ON change_events(device_id, detected_at DESC);
CREATE INDEX idx_change_events_authorization_detected
    ON change_events(authorization_status, detected_at DESC);
CREATE INDEX idx_change_events_policy_detected
    ON change_events(policy_status, detected_at DESC);
