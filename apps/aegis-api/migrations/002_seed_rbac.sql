INSERT OR IGNORE INTO permissions(id, name, description) VALUES
    ('perm-devices-read', 'devices.read', 'List and inspect approved devices'),
    ('perm-devices-manage', 'devices.manage', 'Create, update, disable, and test devices'),
    ('perm-configs-read', 'configs.read', 'Read and download device configurations'),
    ('perm-backups-read', 'backups.read', 'Read and compare configuration backups'),
    ('perm-backups-create', 'backups.create', 'Create manual configuration backups'),
    ('perm-backups-schedule', 'backups.schedule', 'Manage configuration backup schedules'),
    ('perm-audit-read', 'audit.read', 'Read security audit records'),
    ('perm-credentials-manage', 'credentials.manage', 'Manage protected device credentials'),
    ('perm-system-manage', 'system.manage', 'Manage global security and application settings');

INSERT OR IGNORE INTO roles(id, name, description) VALUES
    ('role-viewer', 'viewer', 'Read approved device data'),
    ('role-network-operator', 'network_operator', 'Read data and create backups'),
    ('role-network-admin', 'network_administrator', 'Manage network devices and credentials'),
    ('role-system-admin', 'system_administrator', 'Manage all Aegis settings');

INSERT OR IGNORE INTO role_permissions(role_id, permission_id)
SELECT 'role-viewer', id FROM permissions WHERE name IN ('devices.read', 'configs.read');
INSERT OR IGNORE INTO role_permissions(role_id, permission_id)
SELECT 'role-network-operator', id FROM permissions WHERE name IN
    ('devices.read', 'configs.read', 'backups.read', 'backups.create');
INSERT OR IGNORE INTO role_permissions(role_id, permission_id)
SELECT 'role-network-admin', id FROM permissions WHERE name IN
    ('devices.read', 'devices.manage', 'configs.read', 'backups.read',
     'backups.create', 'backups.schedule', 'audit.read', 'credentials.manage');
INSERT OR IGNORE INTO role_permissions(role_id, permission_id)
SELECT 'role-system-admin', id FROM permissions;

