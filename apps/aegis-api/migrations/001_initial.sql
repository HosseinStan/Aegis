PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS schema_migrations (
    version TEXT PRIMARY KEY,
    applied_at TEXT NOT NULL
);

CREATE TABLE devices (
    id TEXT PRIMARY KEY,
    display_name TEXT NOT NULL,
    hostname TEXT,
    management_ip TEXT NOT NULL,
    management_port INTEGER NOT NULL DEFAULT 22 CHECK (management_port BETWEEN 1 AND 65535),
    vendor TEXT NOT NULL,
    platform TEXT NOT NULL,
    protocol TEXT NOT NULL,
    credential_id TEXT NOT NULL REFERENCES credentials(id) ON DELETE RESTRICT,
    device_group TEXT,
    description TEXT,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    last_success_at TEXT,
    last_failure_at TEXT,
    last_error_code TEXT
);

CREATE UNIQUE INDEX idx_devices_management_endpoint
    ON devices(management_ip, management_port);
CREATE INDEX idx_devices_group ON devices(device_group);

CREATE TABLE credentials (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    auth_type TEXT NOT NULL,
    username_encrypted BLOB,
    password_encrypted BLOB,
    private_key_encrypted BLOB,
    enable_secret_encrypted BLOB,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE users (
    id TEXT PRIMARY KEY,
    external_subject TEXT,
    username TEXT NOT NULL UNIQUE,
    display_name TEXT,
    email TEXT,
    password_hash BLOB,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_users_external_subject
    ON users(external_subject) WHERE external_subject IS NOT NULL;

CREATE TABLE roles (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT
);

CREATE TABLE permissions (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT
);

CREATE TABLE user_roles (
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id TEXT NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, role_id)
);

CREATE TABLE role_permissions (
    role_id TEXT NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission_id TEXT NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    PRIMARY KEY (role_id, permission_id)
);

CREATE TABLE user_device_groups (
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_group TEXT NOT NULL,
    PRIMARY KEY (user_id, device_group)
);

CREATE TABLE audit_logs (
    id TEXT PRIMARY KEY,
    request_id TEXT NOT NULL,
    user_id TEXT,
    username TEXT,
    source_ip TEXT,
    device_id TEXT,
    action TEXT NOT NULL,
    resource_type TEXT,
    resource_id TEXT,
    succeeded INTEGER NOT NULL,
    error_code TEXT,
    duration_ms INTEGER NOT NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX idx_audit_logs_created_at ON audit_logs(created_at DESC);
CREATE INDEX idx_audit_logs_device_id ON audit_logs(device_id);

CREATE TABLE config_backups (
    id TEXT PRIMARY KEY,
    device_id TEXT NOT NULL REFERENCES devices(id) ON DELETE RESTRICT,
    config_type TEXT NOT NULL CHECK (config_type IN ('running', 'startup')),
    storage_path TEXT NOT NULL UNIQUE,
    content_hash TEXT NOT NULL,
    size_bytes INTEGER NOT NULL,
    trigger_type TEXT NOT NULL,
    triggered_by TEXT,
    retrieved_at TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX idx_config_backups_device_created
    ON config_backups(device_id, created_at DESC);

CREATE TABLE backup_schedules (
    id TEXT PRIMARY KEY,
    device_id TEXT REFERENCES devices(id) ON DELETE CASCADE,
    device_group TEXT,
    schedule_expression TEXT NOT NULL,
    config_type TEXT NOT NULL DEFAULT 'running',
    enabled INTEGER NOT NULL DEFAULT 1,
    created_by TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    CHECK (device_id IS NOT NULL OR device_group IS NOT NULL)
);
