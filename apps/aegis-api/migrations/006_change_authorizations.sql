CREATE TABLE change_authorizations (
    id TEXT PRIMARY KEY NOT NULL,
    reference TEXT NOT NULL COLLATE NOCASE UNIQUE
        CHECK (length(reference) BETWEEN 3 AND 64 AND reference = upper(reference)
               AND reference NOT GLOB '*[^A-Z0-9._:-]*'),
    device_id TEXT NOT NULL REFERENCES devices(id) ON DELETE RESTRICT,
    config_type TEXT NOT NULL CHECK (config_type IN ('running', 'startup')),
    allowed_categories_json TEXT NOT NULL CHECK (json_valid(allowed_categories_json)
        AND json_type(allowed_categories_json) = 'array' AND json_array_length(allowed_categories_json) BETWEEN 1 AND 13),
    window_start TEXT NOT NULL CHECK (length(window_start) = 30 AND substr(window_start, 30, 1) = 'Z'),
    window_end TEXT NOT NULL CHECK (length(window_end) = 30 AND substr(window_end, 30, 1) = 'Z'),
    status TEXT NOT NULL CHECK (status IN ('pending', 'approved', 'rejected', 'revoked')),
    requested_by TEXT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    requested_at TEXT NOT NULL,
    approved_by TEXT REFERENCES users(id) ON DELETE RESTRICT,
    approved_at TEXT,
    rejected_by TEXT REFERENCES users(id) ON DELETE RESTRICT,
    rejected_at TEXT,
    revoked_by TEXT REFERENCES users(id) ON DELETE RESTRICT,
    revoked_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    CHECK (julianday(window_start) IS NOT NULL AND julianday(window_end) IS NOT NULL
        AND window_start < window_end
        AND julianday(window_end) - julianday(window_start) <= 1.0),
    CHECK (approved_by IS NULL OR approved_by <> requested_by),
    CHECK ((status = 'pending' AND approved_by IS NULL AND approved_at IS NULL
            AND rejected_by IS NULL AND rejected_at IS NULL AND revoked_by IS NULL AND revoked_at IS NULL)
        OR (status = 'approved' AND approved_by IS NOT NULL AND approved_at IS NOT NULL
            AND rejected_by IS NULL AND rejected_at IS NULL AND revoked_by IS NULL AND revoked_at IS NULL)
        OR (status = 'rejected' AND approved_by IS NULL AND approved_at IS NULL
            AND rejected_by IS NOT NULL AND rejected_at IS NOT NULL AND revoked_by IS NULL AND revoked_at IS NULL)
        OR (status = 'revoked' AND approved_by IS NOT NULL AND approved_at IS NOT NULL
            AND rejected_by IS NULL AND rejected_at IS NULL AND revoked_by IS NOT NULL AND revoked_at IS NOT NULL))
);

CREATE INDEX idx_change_authorizations_device_requested
    ON change_authorizations(device_id, requested_at DESC, id DESC);

CREATE TABLE change_event_authorizations (
    event_id TEXT PRIMARY KEY NOT NULL REFERENCES change_events(id) ON DELETE RESTRICT,
    authorization_id TEXT NOT NULL UNIQUE REFERENCES change_authorizations(id) ON DELETE RESTRICT,
    linked_by TEXT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    linked_at TEXT NOT NULL,
    attestation TEXT NOT NULL CHECK (attestation = 'reviewed_change_items_against_ticket')
);

CREATE TABLE change_authorization_history (
    id TEXT PRIMARY KEY NOT NULL,
    authorization_id TEXT NOT NULL REFERENCES change_authorizations(id) ON DELETE RESTRICT,
    event_id TEXT REFERENCES change_events(id) ON DELETE RESTRICT,
    action TEXT NOT NULL CHECK (action IN ('requested', 'approved', 'rejected', 'revoked', 'linked')),
    actor_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    occurred_at TEXT NOT NULL,
    UNIQUE (authorization_id, action),
    CHECK ((action = 'linked' AND event_id IS NOT NULL) OR (action <> 'linked' AND event_id IS NULL))
);

CREATE INDEX idx_change_authorization_history_authorization_time
    ON change_authorization_history(authorization_id, occurred_at);

INSERT OR IGNORE INTO permissions(id, name, description) VALUES
    ('perm-change-auth-read', 'changes.authorization.read', 'Inspect change authorization evidence'),
    ('perm-change-auth-request', 'changes.authorization.request', 'Request scoped change authorization'),
    ('perm-change-auth-approve', 'changes.authorization.approve', 'Independently approve, reject or revoke requests'),
    ('perm-change-auth-reconcile', 'changes.authorization.reconcile', 'Explicitly attest and link observed change events');

INSERT OR IGNORE INTO role_permissions(role_id, permission_id)
SELECT r.id, p.id FROM roles r CROSS JOIN permissions p
WHERE (r.name = 'network_operator' AND p.name IN ('changes.authorization.read', 'changes.authorization.request'))
   OR (r.name = 'network_administrator' AND p.name IN ('changes.authorization.read', 'changes.authorization.request', 'changes.authorization.reconcile'))
   OR (r.name = 'system_administrator' AND p.name IN ('changes.authorization.read', 'changes.authorization.request', 'changes.authorization.approve', 'changes.authorization.reconcile'));
