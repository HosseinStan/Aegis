package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"company.local/aegis/apps/aegis-api/internal/domain"
	"company.local/aegis/apps/aegis-api/internal/repositories"
	"company.local/aegis/apps/aegis-api/migrations"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// Bootstrap an actual historical 007 database from unmodified embedded SQL,
// using checked migration transactions/bookkeeping just like existing upgrade
// tests. Production Open has no stop-at-007 option; it is used for the ACTUAL
// 008 upgrade below, never patched or supplied a mocked migration source.
func riskLegacy007(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "populated-007.db")
	dsn, _, err := normalizeSQLiteDSN(path)
	require.NoError(t, err)
	db, err := sql.Open("sqlite", dsn)
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	t.Cleanup(func() { require.NoError(t, s.Close()) })
	_, err = db.Exec("PRAGMA journal_mode=WAL")
	require.NoError(t, err)
	entries, err := migrations.Files.ReadDir(".")
	require.NoError(t, err)
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".sql") || entry.Name() >= "008" {
			continue
		}
		body, err := migrations.Files.ReadFile(entry.Name())
		require.NoError(t, err)
		tx, err := db.Begin()
		require.NoError(t, err)
		_, err = tx.Exec(string(body))
		require.NoError(t, err)
		_, err = tx.Exec(`INSERT INTO schema_migrations(version,applied_at) VALUES(?,?)`, entry.Name(), "2026-10-01T12:00:00Z")
		require.NoError(t, err)
		require.NoError(t, tx.Commit())
	}
	return s, path
}

func TestRiskSchemaPopulatedUpgrade007(t *testing.T) {
	s, path := riskLegacy007(t)
	ctx := context.Background()
	devices := changeEventDevices(t, s)
	now := time.Date(2026, 10, 4, 11, 0, 0, 0, time.UTC)
	f := authorizationFixture{s: s, device: devices[0].ID, otherDevice: devices[1].ID, now: now}
	for _, id := range []*string{&f.requester, &f.approver} {
		*id = uuid.NewString()
		require.NoError(t, s.CreateUser(ctx, domain.User{ID: *id, Username: *id, Enabled: true, CreatedAt: now, UpdatedAt: now}))
		require.NoError(t, s.AssignRole(ctx, *id, "system_administrator"))
	}
	_, err := s.db.Exec(`INSERT INTO roles(id,name,description) VALUES('role-risk-upgrade-custom','custom_observer','Unprivileged legacy custom role')`)
	require.NoError(t, err)
	for _, role := range []string{"viewer", "network_operator", "network_administrator", "custom_observer"} {
		id := uuid.NewString()
		require.NoError(t, s.CreateUser(ctx, domain.User{ID: id, Username: id, Enabled: true, CreatedAt: now, UpdatedAt: now}))
		require.NoError(t, s.AssignRole(ctx, id, role))
		_, err = s.db.Exec(`INSERT INTO user_device_groups(user_id,device_group) VALUES(?,?)`, id, "lab-upgrade")
		require.NoError(t, err)
	}
	var histories []struct {
		f       authorizationFixture
		a       domain.ChangeAuthorization
		event   domain.ChangeEvent
		policy  domain.ChangePolicyEvaluation
		items   []domain.ChangePolicyEvaluationItem
		history []domain.ChangeAuthorizationHistory
		link    domain.ChangeEventAuthorization
	}
	for _, violation := range []bool{false, true} {
		local := f
		if violation {
			local.device = f.otherDevice
		}
		a := local.approve(t, local.request(t))
		event := authorizationEvent(t, s, local.device)
		link := local.link(a, event)
		require.NoError(t, s.LinkChangeEventAuthorization(ctx, local.device, link))
		rules := slices.Clone(completePolicyRules)
		if violation {
			rules[0].Decision = "deny"
		}
		p := local.activatePolicy(t, local.policy(t, rules), now.Add(time.Hour))
		evaluation, err := s.PersistPolicyEvaluation(ctx, local.device, event.ID, local.approver, now.Add(4*time.Hour))
		require.NoError(t, err)
		want := "pass"
		if violation {
			want = "violation"
			a, err = s.TransitionChangeAuthorization(ctx, local.device, a.ID, local.approver, "revoked", now.Add(5*time.Hour))
			require.NoError(t, err)
		}
		require.Equal(t, want, evaluation.Result)
		_, err = s.RetirePolicySet(ctx, local.device, p.ID, local.approver, now.Add(5*time.Hour+time.Minute))
		require.NoError(t, err)
		a, err = s.GetChangeAuthorization(ctx, local.device, a.ID)
		require.NoError(t, err)
		history, total, err := s.ListChangeAuthorizationHistory(ctx, local.device, a.ID, repositories.Page{Limit: 50})
		require.NoError(t, err)
		require.Len(t, history, total)
		items, total, err := s.ListPolicyEvaluationItems(ctx, local.device, event.ID, repositories.Page{Limit: 200})
		require.NoError(t, err)
		require.Len(t, items, total)
		histories = append(histories, struct {
			f       authorizationFixture
			a       domain.ChangeAuthorization
			event   domain.ChangeEvent
			policy  domain.ChangePolicyEvaluation
			items   []domain.ChangePolicyEvaluationItem
			history []domain.ChangeAuthorizationHistory
			link    domain.ChangeEventAuthorization
		}{local, a, event, evaluation, items, history, link})
	}
	before := riskAdversarialSnapshot(t, s)
	require.NotEmpty(t, before["change_authorization_history"])
	require.Len(t, before["change_policy_evaluations"], 2)
	require.NotEmpty(t, before["role_permissions"])
	type schemaEntry struct {
		name, kind string
		sql        sql.NullString
	}
	originalSchema := []schemaEntry{}
	rows, err := s.db.Query(`SELECT name,type,sql FROM sqlite_master WHERE type IN ('table','index','trigger') AND name NOT LIKE 'sqlite_%' ORDER BY type,name`)
	require.NoError(t, err)
	for rows.Next() {
		var entry schemaEntry
		require.NoError(t, rows.Scan(&entry.name, &entry.kind, &entry.sql))
		originalSchema = append(originalSchema, entry)
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	require.NoError(t, s.Close())
	upgraded, err := Open(ctx, path)
	require.NoError(t, err)
	defer upgraded.Close()
	after := riskAdversarialSnapshot(t, upgraded)
	for table, oldRows := range before {
		switch table {
		case "permissions", "role_permissions", "schema_migrations":
			for _, old := range oldRows {
				require.Contains(t, after[table], old, "original %s row retained byte-for-byte", table)
			}
		default:
			require.Equal(t, oldRows, after[table], "all old %s identities and values unchanged", table)
		}
	}
	require.Len(t, after["permissions"], len(before["permissions"])+2)
	require.Len(t, after["role_permissions"], len(before["role_permissions"])+5)
	require.Len(t, after["schema_migrations"], len(before["schema_migrations"])+1)
	for _, entry := range originalSchema {
		var kind string
		var ddl sql.NullString
		require.NoError(t, upgraded.db.QueryRow(`SELECT type,sql FROM sqlite_master WHERE name=?`, entry.name).Scan(&kind, &ddl))
		require.Equal(t, entry.kind, kind)
		require.Equal(t, entry.sql, ddl, "old schema/triggers/indexes unchanged")
	}
	var permissions []string
	rows, err = upgraded.db.Query(`SELECT name FROM permissions WHERE name LIKE 'changes.risk.%' ORDER BY name`)
	require.NoError(t, err)
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		permissions = append(permissions, name)
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	require.Equal(t, []string{"changes.risk.evaluate", "changes.risk.read"}, permissions)
	roleMatrix := map[string][]string{}
	rows, err = upgraded.db.Query(`SELECT r.name,p.name FROM roles r JOIN role_permissions rp ON rp.role_id=r.id JOIN permissions p ON p.id=rp.permission_id WHERE p.name LIKE 'changes.risk.%' ORDER BY r.name,p.name`)
	require.NoError(t, err)
	for rows.Next() {
		var role, permission string
		require.NoError(t, rows.Scan(&role, &permission))
		roleMatrix[role] = append(roleMatrix[role], permission)
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	require.Equal(t, map[string][]string{"network_operator": {"changes.risk.read"}, "network_administrator": {"changes.risk.evaluate", "changes.risk.read"}, "system_administrator": {"changes.risk.evaluate", "changes.risk.read"}}, roleMatrix, "viewer and custom observer must gain no risk rights")
	for _, table := range []string{"change_risk_definitions", "change_risk_assessments", "change_risk_findings"} {
		require.Empty(t, after[table], "no model/evidence backfill")
	}
	var count int
	require.NoError(t, upgraded.db.QueryRow(`SELECT COUNT(*) FROM pragma_foreign_key_check`).Scan(&count))
	require.Zero(t, count)
	require.NoError(t, upgraded.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name LIKE 'change_risk_%'`).Scan(&count))
	require.Equal(t, 12, count)
	for _, old := range histories {
		a, err := upgraded.GetChangeAuthorization(ctx, old.f.device, old.a.ID)
		require.NoError(t, err)
		require.Equal(t, old.a, a)
		link, err := upgraded.GetChangeEventAuthorization(ctx, old.f.device, old.event.ID)
		require.NoError(t, err)
		require.Equal(t, old.link, link)
		history, total, err := upgraded.ListChangeAuthorizationHistory(ctx, old.f.device, old.a.ID, repositories.Page{Limit: 50})
		require.NoError(t, err)
		require.Len(t, history, total)
		require.Equal(t, old.history, history)
		evaluation, err := upgraded.GetPolicyEvaluation(ctx, old.f.device, old.event.ID)
		require.NoError(t, err)
		require.Equal(t, old.policy, evaluation)
		items, total, err := upgraded.ListPolicyEvaluationItems(ctx, old.f.device, old.event.ID, repositories.Page{Limit: 200})
		require.NoError(t, err)
		require.Len(t, items, total)
		require.Equal(t, old.items, items)
	}
	require.NoError(t, upgraded.Migrate(ctx))
	require.Equal(t, after, riskAdversarialSnapshot(t, upgraded), "second production Migrate is idempotent")
	t.Logf("populated 007->008: %d original tables / %d original schema objects retained; PASS+VIOLATION, links/revocation/history/users/groups/roles/grants unchanged; exactly 2 risk permissions/5 role grants; 12 risk indexes; zero backfill/FK violations", len(before), len(originalSchema))
}
