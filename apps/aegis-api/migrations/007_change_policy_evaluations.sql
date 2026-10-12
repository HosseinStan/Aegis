-- Additive policy v1 schema. No existing events/items are backfilled or changed.
CREATE TABLE change_policy_sets (
    id TEXT PRIMARY KEY NOT NULL CHECK(length(id) > 0),
    reference TEXT NOT NULL COLLATE NOCASE UNIQUE
        CHECK(length(reference) BETWEEN 3 AND 64 AND reference = upper(reference)
            AND reference NOT GLOB '*[^A-Z0-9._:-]*'),
    device_id TEXT NOT NULL REFERENCES devices(id) ON DELETE RESTRICT,
    config_type TEXT NOT NULL CHECK(config_type IN ('running', 'startup')),
    status TEXT NOT NULL CHECK(status IN ('draft', 'active', 'retired')),
    engine_version INTEGER NOT NULL CHECK(engine_version = 1),
    rules_json TEXT NOT NULL CHECK(json_valid(rules_json) AND json_type(rules_json) = 'array'
        AND json_array_length(rules_json) BETWEEN 1 AND 26),
    definition_hash TEXT NOT NULL CHECK(length(definition_hash) = 64 AND definition_hash NOT GLOB '*[^a-f0-9]*'),
    created_by TEXT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TEXT NOT NULL,
    activated_by TEXT REFERENCES users(id) ON DELETE RESTRICT,
    activated_at TEXT,
    retired_by TEXT REFERENCES users(id) ON DELETE RESTRICT,
    retired_at TEXT,
    updated_at TEXT NOT NULL,
    CHECK((status = 'draft' AND activated_by IS NULL AND activated_at IS NULL AND retired_by IS NULL AND retired_at IS NULL)
        OR (status = 'active' AND activated_by IS NOT NULL AND activated_at IS NOT NULL AND retired_by IS NULL AND retired_at IS NULL)
        OR (status = 'retired' AND activated_by IS NOT NULL AND activated_at IS NOT NULL AND retired_by IS NOT NULL AND retired_at IS NOT NULL))
);
CREATE UNIQUE INDEX change_policy_one_active ON change_policy_sets(device_id, config_type) WHERE status = 'active';
CREATE INDEX change_policy_scope ON change_policy_sets(device_id, config_type, status, created_at, id);

CREATE TABLE change_policy_evaluations (
    id TEXT PRIMARY KEY NOT NULL CHECK(length(id) > 0),
    event_id TEXT NOT NULL UNIQUE REFERENCES change_events(id) ON DELETE RESTRICT,
    policy_set_id TEXT NOT NULL REFERENCES change_policy_sets(id) ON DELETE RESTRICT,
    engine_version INTEGER NOT NULL CHECK(engine_version = 1),
    policy_definition_hash TEXT NOT NULL CHECK(length(policy_definition_hash) = 64 AND policy_definition_hash NOT GLOB '*[^a-f0-9]*'),
    policy_definition_snapshot TEXT NOT NULL CHECK(json_valid(policy_definition_snapshot) AND json_type(policy_definition_snapshot) = 'object'),
    result TEXT NOT NULL CHECK(result IN ('pass', 'violation')),
    total_items INTEGER NOT NULL CHECK(total_items > 0),
    allowed_count INTEGER NOT NULL CHECK(allowed_count >= 0),
    denied_count INTEGER NOT NULL CHECK(denied_count >= 0),
    uncovered_count INTEGER NOT NULL CHECK(uncovered_count >= 0),
    evaluated_by TEXT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    evaluated_at TEXT NOT NULL,
    UNIQUE(id, event_id),
    CHECK(total_items = allowed_count + denied_count + uncovered_count),
    CHECK((result = 'pass' AND allowed_count = total_items AND denied_count = 0 AND uncovered_count = 0)
        OR (result = 'violation' AND denied_count > 0))
);
CREATE TABLE change_policy_evaluation_items (
    evaluation_id TEXT NOT NULL,
    event_id TEXT NOT NULL,
    change_event_item_id TEXT NOT NULL REFERENCES change_event_items(id) ON DELETE RESTRICT,
    ordinal INTEGER NOT NULL CHECK(ordinal >= 0),
    category TEXT NOT NULL CHECK(category IN ('system','interface','switching','routing','access_control','aaa','management','monitoring','services','crypto_vpn','qos','redundancy','other')),
    operation TEXT NOT NULL CHECK(operation IN ('add', 'remove')),
    decision TEXT NOT NULL CHECK(decision IN ('allow', 'deny', 'uncovered')),
    matched_rule_key TEXT NOT NULL,
    PRIMARY KEY(evaluation_id, change_event_item_id),
    UNIQUE(evaluation_id, ordinal),
    FOREIGN KEY(evaluation_id, event_id) REFERENCES change_policy_evaluations(id, event_id) ON DELETE RESTRICT,
    FOREIGN KEY(event_id, ordinal) REFERENCES change_event_items(change_event_id, ordinal) ON DELETE RESTRICT,
    CHECK((decision = 'uncovered' AND matched_rule_key = '')
        OR (decision IN ('allow','deny') AND matched_rule_key = 'policy.' || category || '.' || operation))
);

-- Narrow guards: only forward lifecycle transitions; definitions and recorded
-- evidence cannot be rewritten. This is not protection against a DB admin.
CREATE TRIGGER change_policy_definition_immutable BEFORE UPDATE ON change_policy_sets
WHEN NEW.id != OLD.id OR NEW.reference != OLD.reference OR NEW.device_id != OLD.device_id
 OR NEW.config_type != OLD.config_type OR NEW.engine_version != OLD.engine_version
 OR NEW.rules_json != OLD.rules_json OR NEW.definition_hash != OLD.definition_hash
 OR NEW.created_by != OLD.created_by OR NEW.created_at != OLD.created_at
 OR NOT ((OLD.status = 'draft' AND NEW.status = 'active') OR (OLD.status = 'active' AND NEW.status = 'retired'))
 OR (OLD.status = 'active' AND (NEW.activated_by IS NOT OLD.activated_by OR NEW.activated_at IS NOT OLD.activated_at))
BEGIN SELECT RAISE(ABORT, 'immutable policy definition or invalid lifecycle'); END;
CREATE TRIGGER change_policy_no_delete BEFORE DELETE ON change_policy_sets
BEGIN SELECT RAISE(ABORT, 'immutable policy identity'); END;
CREATE TRIGGER change_policy_evaluation_no_update BEFORE UPDATE ON change_policy_evaluations
BEGIN SELECT RAISE(ABORT, 'immutable evaluation evidence'); END;
CREATE TRIGGER change_policy_evaluation_scope BEFORE INSERT ON change_policy_evaluations
WHEN NOT EXISTS (SELECT 1 FROM change_events e JOIN change_policy_sets p
    ON p.device_id = e.device_id AND p.config_type = e.config_type
    WHERE e.id = NEW.event_id AND p.id = NEW.policy_set_id AND p.status = 'active'
      AND p.engine_version = NEW.engine_version AND p.definition_hash = NEW.policy_definition_hash
      AND json_extract(NEW.policy_definition_snapshot, '$.engine_version') = p.engine_version
      AND json_extract(NEW.policy_definition_snapshot, '$.device_id') = p.device_id
      AND json_extract(NEW.policy_definition_snapshot, '$.config_type') = p.config_type
      AND json_extract(NEW.policy_definition_snapshot, '$.rules') = p.rules_json)
BEGIN SELECT RAISE(ABORT, 'invalid evaluation policy scope'); END;
CREATE TRIGGER change_policy_evaluation_no_delete BEFORE DELETE ON change_policy_evaluations
BEGIN SELECT RAISE(ABORT, 'immutable evaluation evidence'); END;
CREATE TRIGGER change_policy_evaluation_item_no_update BEFORE UPDATE ON change_policy_evaluation_items
BEGIN SELECT RAISE(ABORT, 'immutable item evidence'); END;
CREATE TRIGGER change_policy_evaluation_item_no_delete BEFORE DELETE ON change_policy_evaluation_items
BEGIN SELECT RAISE(ABORT, 'immutable item evidence'); END;
CREATE TRIGGER change_policy_evaluation_item_scope BEFORE INSERT ON change_policy_evaluation_items
WHEN NOT EXISTS (SELECT 1 FROM change_event_items i WHERE i.id = NEW.change_event_item_id
    AND i.change_event_id = NEW.event_id AND i.ordinal = NEW.ordinal
    AND i.category = NEW.category AND i.operation = NEW.operation)
BEGIN SELECT RAISE(ABORT, 'invalid item evidence scope'); END;

-- Final policy evidence depends on the original technical source, not just
-- its foreign-key identity. Only finalized events are frozen; ordinary backup
-- creation and non-finalized Task 2/3 evidence retain their existing behavior.
CREATE TRIGGER change_policy_source_item_no_insert BEFORE INSERT ON change_event_items
WHEN EXISTS (SELECT 1 FROM change_policy_evaluations WHERE event_id = NEW.change_event_id)
BEGIN SELECT RAISE(ABORT, 'immutable finalized policy source item'); END;
CREATE TRIGGER change_policy_source_item_no_update BEFORE UPDATE ON change_event_items
WHEN EXISTS (SELECT 1 FROM change_policy_evaluations
    WHERE event_id = OLD.change_event_id OR event_id = NEW.change_event_id)
BEGIN SELECT RAISE(ABORT, 'immutable finalized policy source item'); END;
CREATE TRIGGER change_policy_source_item_no_delete BEFORE DELETE ON change_event_items
WHEN EXISTS (SELECT 1 FROM change_policy_evaluations WHERE event_id = OLD.change_event_id)
BEGIN SELECT RAISE(ABORT, 'immutable finalized policy source item'); END;

CREATE TRIGGER change_policy_source_event_immutable BEFORE UPDATE ON change_events
WHEN EXISTS (SELECT 1 FROM change_policy_evaluations WHERE event_id = OLD.id)
 AND (NEW.id IS NOT OLD.id OR NEW.device_id IS NOT OLD.device_id
    OR NEW.config_type IS NOT OLD.config_type OR NEW.from_backup_id IS NOT OLD.from_backup_id
    OR NEW.to_backup_id IS NOT OLD.to_backup_id OR NEW.detected_at IS NOT OLD.detected_at
    OR NEW.added_lines IS NOT OLD.added_lines OR NEW.removed_lines IS NOT OLD.removed_lines
    OR NEW.analysis_version IS NOT OLD.analysis_version OR NEW.analyzed_at IS NOT OLD.analyzed_at
    OR NEW.item_count IS NOT OLD.item_count OR NEW.created_at IS NOT OLD.created_at)
BEGIN SELECT RAISE(ABORT, 'immutable finalized policy source event'); END;

-- The evaluation header/items precede the initial event update in the same
-- transaction. Permit that one matching, complete transition, never a rewrite.
-- AuthorizationStatus and UpdatedAt are deliberately not frozen.
CREATE TRIGGER change_policy_final_status_immutable BEFORE UPDATE OF policy_status ON change_events
WHEN EXISTS (SELECT 1 FROM change_policy_evaluations WHERE event_id = OLD.id)
 AND (OLD.policy_status != 'not_evaluated'
    OR NEW.policy_status IS NOT (SELECT result FROM change_policy_evaluations WHERE event_id = OLD.id)
    OR (SELECT COUNT(*) FROM change_policy_evaluation_items WHERE event_id = OLD.id)
        != (SELECT total_items FROM change_policy_evaluations WHERE event_id = OLD.id))
BEGIN SELECT RAISE(ABORT, 'immutable final policy status'); END;

INSERT INTO permissions(id, name, description) VALUES
 ('changes.policy.read', 'changes.policy.read', 'Read scoped policy definitions and evaluation evidence'),
 ('changes.policy.manage', 'changes.policy.manage', 'Create, activate and retire scoped immutable policies'),
 ('changes.policy.evaluate', 'changes.policy.evaluate', 'Preview and explicitly evaluate scoped observed changes');
INSERT INTO role_permissions(role_id, permission_id)
SELECT r.id, p.id FROM roles r CROSS JOIN permissions p WHERE
 (r.name = 'network_operator' AND p.name = 'changes.policy.read') OR
 (r.name = 'network_administrator' AND p.name IN ('changes.policy.read','changes.policy.evaluate')) OR
 (r.name = 'system_administrator' AND p.name IN ('changes.policy.read','changes.policy.manage','changes.policy.evaluate'));
