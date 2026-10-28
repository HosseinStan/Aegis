package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"company.local/aegis/apps/aegis-api/internal/domain"
	"company.local/aegis/apps/aegis-api/internal/repositories"
	"company.local/aegis/apps/aegis-api/internal/risk"
	"company.local/aegis/apps/aegis-api/migrations"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	sqlitedriver "modernc.org/sqlite"
)

type riskSchemaFixture struct {
	f                                                                                              authorizationFixture
	event                                                                                          domain.ChangeEvent
	evidence                                                                                       domain.RiskEvidenceSnapshot
	bindings                                                                                       domain.RiskSourceBindings
	result                                                                                         domain.RiskEvaluationResult
	id, definitionHash, evidenceHash, resultHash, evidenceJSON, bindingsJSON, resultJSON, captured string
}

func riskSchemaSource(t *testing.T, zero bool) riskSchemaFixture {
	t.Helper()
	f := authorizationStore(t)
	var event domain.ChangeEvent
	if !zero {
		event = authorizationEvent(t, f.s, f.device)
	} else {
		now := f.now.Add(2 * time.Hour)
		from := eventBackup(f.device, "running", now.Add(-time.Hour))
		to := eventBackup(f.device, "running", now)
		require.NoError(t, f.s.CreateBackup(context.Background(), from))
		event = eventBetween(from, to)
		event.AnalysisVersion = 1
		event.AnalyzedAt = &now
		require.NoError(t, f.s.CreateBackupWithChangeAnalysis(context.Background(), to, &event, nil))
	}
	items, _, err := f.s.ListChangeEventItems(context.Background(), event.ID, repositories.Page{Limit: 200})
	require.NoError(t, err)
	canonical := func(v time.Time) string { return v.UTC().Format(risk.UTC9) }
	analyzed := canonical(*event.AnalyzedAt)
	e := domain.RiskEvidenceSnapshot{SchemaVersion: 1, Event: domain.RiskEventSnapshot{EventID: event.ID, DeviceID: event.DeviceID, ConfigType: event.ConfigType, FromBackupID: event.FromBackupID, ToBackupID: event.ToBackupID, DetectedAt: canonical(event.DetectedAt), CreatedAt: canonical(event.CreatedAt), AnalysisVersion: 1, ItemCount: len(items), AddedLines: event.AddedLines, RemovedLines: event.RemovedLines, AnalyzedAt: &analyzed}, Items: []domain.RiskItemSignal{}, Authorization: domain.RiskAuthorizationObservation{RecordedStatus: "unaccounted", History: []domain.RiskAuthorizationHistory{}}, Policy: domain.RiskPolicyObservation{RecordedStatus: "not_evaluated"}, Eligibility: domain.RiskEligibility{Persistable: true, Reasons: []string{}}}
	b := domain.RiskSourceBindings{Event: domain.RiskEventTimeBinding{EventID: event.ID}, Items: []domain.RiskItemTimeBinding{}, Governance: domain.RiskGovernanceTimeBindings{History: []domain.RiskHistoryTimeBinding{}}}
	require.NoError(t, f.s.db.QueryRow(`SELECT detected_at,analyzed_at,created_at FROM change_events WHERE id=?`, event.ID).Scan(&b.Event.DetectedAt, &b.Event.AnalyzedAt, &b.Event.CreatedAt))
	for _, item := range items {
		at := canonical(*item.ClassifiedAt)
		e.Items = append(e.Items, domain.RiskItemSignal{ItemID: item.ID, Ordinal: item.Ordinal, Operation: item.Operation, Category: item.Category, Subcategory: item.Subcategory, ClassificationRule: item.ClassificationRule, ClassificationVersion: 1, Sensitive: item.Sensitive, CreatedAt: canonical(item.CreatedAt), ClassifiedAt: &at})
		ib := domain.RiskItemTimeBinding{ItemID: item.ID, Ordinal: item.Ordinal}
		require.NoError(t, f.s.db.QueryRow(`SELECT created_at,classified_at FROM change_event_items WHERE id=?`, item.ID).Scan(&ib.CreatedAt, &ib.ClassifiedAt))
		b.Items = append(b.Items, ib)
	}
	x := riskSchemaFixture{f: f, event: event, evidence: e, bindings: b, id: uuid.NewString(), captured: canonical(f.now.Add(6 * time.Hour))}
	x.refresh(t)
	definition, hash := risk.CanonicalDefinitionV1()
	x.definitionHash = hash
	_, err = f.s.db.Exec(`INSERT INTO change_risk_definitions(model_version,schema_version,engine_version,materializer_version,definition_hash,definition_json,registered_at) VALUES(1,1,1,1,?,?,?)`, hash, definition, x.captured)
	require.NoError(t, err)
	return x
}
func (x *riskSchemaFixture) refresh(t *testing.T) {
	t.Helper()
	var err error
	x.evidence, x.evidenceJSON, x.evidenceHash, err = risk.CanonicalEvidence(x.evidence)
	require.NoError(t, err)
	x.result, err = risk.Evaluate(x.evidence)
	require.NoError(t, err)
	_, x.definitionHash = risk.CanonicalDefinitionV1()
	x.resultJSON, x.resultHash, err = risk.CanonicalResult(x.result, x.definitionHash, x.evidenceHash)
	require.NoError(t, err)
	raw, err := json.Marshal(x.bindings)
	require.NoError(t, err)
	x.bindingsJSON = string(raw)
}
func riskSchemaChildren(tx *sql.Tx, x riskSchemaFixture) error {
	for _, f := range x.result.Findings {
		var itemID any
		if f.Subject.Kind == "item" {
			itemID = f.Subject.ID
		}
		_, err := tx.Exec(`INSERT INTO change_risk_findings(assessment_id,event_id,ordinal,finding_key,rule_id,reason_code,kind,subject_kind,subject_id,source_item_id,item_ordinal,impact,priority_floor,parameters_json) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, x.id, x.event.ID, f.Ordinal, f.Key, f.RuleID, f.ReasonCode, f.Kind, f.Subject.Kind, f.Subject.ID, itemID, f.ItemOrdinal, f.Impact, f.PriorityFloor, "[]")
		if err != nil {
			return err
		}
	}
	return nil
}
func riskSchemaHeader(tx *sql.Tx, x riskSchemaFixture) error {
	fields := []string{"id", "event_id", "device_id", "config_type", "revision", "previous_assessment_id", "model_version", "definition_hash", "evidence_schema_version", "evidence_hash", "evidence_json", "source_bindings_json", "result_hash", "result_json", "technical_impact", "known_impact_floor", "evidence_completeness", "review_priority", "priority_rank", "total_items", "known_items", "unknown_items", "distinct_known_categories", "finding_count", "recorded_authorization_status", "authorization_id", "recorded_policy_status", "policy_evaluation_id", "captured_at", "assessed_by"}
	rank := map[string]int{"normal": 0, "elevated": 1, "urgent": 2}[x.result.ReviewPriority]
	var auth, policy any
	if x.evidence.Authorization.Ticket != nil {
		auth = x.evidence.Authorization.Ticket.ID
	}
	if x.evidence.Policy.Final != nil {
		policy = x.evidence.Policy.Final.ID
	}
	args := []any{x.id, x.event.ID, x.event.DeviceID, x.event.ConfigType, 1, nil, 1, x.definitionHash, 1, x.evidenceHash, x.evidenceJSON, x.bindingsJSON, x.resultHash, x.resultJSON, x.result.TechnicalImpact, x.result.KnownImpactFloor, x.result.EvidenceCompleteness, x.result.ReviewPriority, rank, x.result.TotalItems, x.result.KnownItems, x.result.UnknownItems, x.result.DistinctKnownCategories, len(x.result.Findings), x.evidence.Authorization.RecordedStatus, auth, x.evidence.Policy.RecordedStatus, policy, x.captured, x.f.approver}
	_, err := tx.Exec("INSERT INTO change_risk_assessments("+strings.Join(fields, ",")+") VALUES("+strings.TrimSuffix(strings.Repeat("?,", len(fields)), ",")+")", args...)
	return err
}
func TestRiskSchemaValidHeaderLast(t *testing.T) {
	for _, zero := range []bool{false, true} {
		t.Run(fmt.Sprint(zero), func(t *testing.T) {
			x := riskSchemaSource(t, zero)
			tx, err := x.f.s.db.Begin()
			require.NoError(t, err)
			defer tx.Rollback()
			require.NoError(t, riskSchemaChildren(tx, x))
			require.NoError(t, riskSchemaHeader(tx, x))
			require.NoError(t, tx.Commit())
			var count int
			require.NoError(t, x.f.s.db.QueryRow(`SELECT COUNT(*) FROM change_risk_assessments`).Scan(&count))
			require.Equal(t, 1, count)
		})
	}
}
func TestRiskSchemaUpgradeAndMissingABI(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	dsn, _, err := normalizeSQLiteDSN(path)
	require.NoError(t, err)
	db, err := sql.Open("sqlite", dsn)
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	names, err := migrations.Files.ReadDir(".")
	require.NoError(t, err)
	for _, name := range names {
		if !strings.HasSuffix(name.Name(), ".sql") || name.Name() >= "008" {
			continue
		}
		raw, err := migrations.Files.ReadFile(name.Name())
		require.NoError(t, err)
		_, err = db.Exec(string(raw))
		require.NoError(t, err)
		_, err = db.Exec(`INSERT INTO schema_migrations VALUES(?,?)`, name.Name(), "2026-10-01T12:00:00Z")
		require.NoError(t, err)
	}
	var permissions int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM permissions`).Scan(&permissions))
	require.NoError(t, db.Close())
	s, err := Open(context.Background(), path)
	require.NoError(t, err)
	defer s.Close()
	var after int
	require.NoError(t, s.db.QueryRow(`SELECT COUNT(*) FROM permissions`).Scan(&after))
	require.Equal(t, permissions+2, after)
	var tables int
	require.NoError(t, s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name LIKE 'change_risk_%'`).Scan(&tables))
	require.Equal(t, 3, tables)
	raw, err := migrations.Files.ReadFile("008_change_risk_assessments.sql")
	require.NoError(t, err)
	missing := sql.OpenDB(&riskNoABIConnector{dsn: filepath.Join(t.TempDir(), "missing.db")})
	defer missing.Close()
	_, err = missing.Exec(string(raw))
	require.Error(t, err)
	require.ErrorContains(t, err, "aegis_risk_utc_nano_v1")
	require.NoError(t, missing.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name LIKE 'change_risk_%'`).Scan(&tables))
	require.Zero(t, tables)
}

// These are security stop-gate assertions, not examples of tolerated behavior.
func TestRiskSchemaRejectsContradictions(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*riskSchemaFixture)
	}{
		{"event canonical plus 1ns", func(x *riskSchemaFixture) { x.evidence.Event.DetectedAt = "2026-10-04T13:00:00.000000001Z" }},
		{"event raw equivalent but not original", func(x *riskSchemaFixture) { x.bindings.Event.DetectedAt = "2026-10-04T16:30:00+03:30" }},
		{"item canonical plus 1ns", func(x *riskSchemaFixture) { x.evidence.Items[0].CreatedAt = "2026-10-04T13:00:00.000000001Z" }},
		{"binding item identity", func(x *riskSchemaFixture) { x.bindings.Items[0].ItemID = uuid.NewString() }},
		{"changed ordinal binding", func(x *riskSchemaFixture) { x.bindings.Items[0].Ordinal = 1 }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			x := riskSchemaSource(t, false)
			c.mutate(&x)
			x.refresh(t)
			tx, err := x.f.s.db.Begin()
			require.NoError(t, err)
			defer tx.Rollback()
			require.NoError(t, riskSchemaChildren(tx, x))
			require.Error(t, riskSchemaHeader(tx, x))
			require.NoError(t, tx.Rollback())
		})
	}
	for _, c := range []string{"missing child", "altered child", "altered child priority floor", "missing typed key", "duplicate JSON key", "unknown JSON key", "extra child"} {
		t.Run(c, func(t *testing.T) {
			x := riskSchemaSource(t, false)
			original, err := json.Marshal(x.result)
			require.NoError(t, err)
			// Detach the whole child projection, including slice and pointer fields.
			// Keep the header's original result/JSON untouched by every mutation.
			children := x
			children.result = domain.RiskEvaluationResult{}
			require.NoError(t, json.Unmarshal(original, &children.result))
			tx, err := x.f.s.db.Begin()
			require.NoError(t, err)
			defer tx.Rollback()
			switch c {
			case "missing child":
				children.result.Findings = children.result.Findings[:len(children.result.Findings)-1]
			case "altered child":
				require.NotNil(t, x.result.Findings[0].Impact)
				altered := "high"
				require.NotEqual(t, *x.result.Findings[0].Impact, altered)
				children.result.Findings[0].Impact = &altered
				t.Logf("child impact contradiction: %s -> %s; header unchanged", *x.result.Findings[0].Impact, altered)
			case "altered child priority floor":
				require.NotNil(t, x.result.Findings[0].PriorityFloor)
				altered := "urgent"
				require.NotEqual(t, *x.result.Findings[0].PriorityFloor, altered)
				children.result.Findings[0].PriorityFloor = &altered
				t.Logf("child priority contradiction: %s -> %s; header unchanged", *x.result.Findings[0].PriorityFloor, altered)
			case "missing typed key":
				x.evidenceJSON = strings.Replace(x.evidenceJSON, `"added_lines":2,`, "", 1)
			case "duplicate JSON key":
				x.evidenceJSON = strings.Replace(x.evidenceJSON, `"schema_version":1,`, `"schema_version":1,"schema_version":1,`, 1)
			case "unknown JSON key":
				x.evidenceJSON = strings.Replace(x.evidenceJSON, `"schema_version":1,`, `"schema_version":1,"attacker":1,`, 1)
			case "extra child":
				extra := children.result.Findings[len(children.result.Findings)-1]
				extra.Ordinal++
				extra.RuleID = "risk.attacker.v1"
				extra.ReasonCode = extra.RuleID
				extra.Key = extra.RuleID + ":" + extra.Subject.Kind + ":" + extra.Subject.ID
				children.result.Findings = append(children.result.Findings, extra)
			}
			if strings.HasPrefix(c, "altered child") {
				altered, err := json.Marshal(children.result)
				require.NoError(t, err)
				require.NotEqual(t, string(original), string(altered))
				require.Equal(t, x.resultJSON, string(original), "sealed header/result remains original")
			}
			require.NoError(t, riskSchemaChildren(tx, children))
			err = riskSchemaHeader(tx, x)
			require.Error(t, err)
			if strings.HasPrefix(c, "altered child") {
				var sqliteErr *sqlitedriver.Error
				require.ErrorAs(t, err, &sqliteErr)
				require.Equal(t, 1811, sqliteErr.Code(), "SQLITE_CONSTRAINT_TRIGGER at header INSERT")
				require.ErrorContains(t, err, "risk finding projection")
				t.Logf("header INSERT rejected contradictory child: code=%d; rolling back", sqliteErr.Code())
			}
			require.NoError(t, tx.Rollback())
			for _, table := range []string{"change_risk_assessments", "change_risk_findings"} {
				var count int
				require.NoError(t, x.f.s.db.QueryRow("SELECT COUNT(*) FROM "+table).Scan(&count))
				require.Zero(t, count, "rollback leaves no partial %s", table)
			}
			unchanged, err := json.Marshal(x.result)
			require.NoError(t, err)
			require.Equal(t, string(original), string(unchanged), "original fixture must not alias altered child")
		})
	}
}

func TestRiskSchemaOrphanCommitAndFKNegativeControl(t *testing.T) {
	x := riskSchemaSource(t, false)
	conn, err := x.f.s.db.Conn(context.Background())
	require.NoError(t, err)
	defer conn.Close()
	physical := storeConnectionSettings(t, conn, "wal")
	tx, err := conn.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	require.NoError(t, riskSchemaChildren(tx, x))
	err = tx.Commit()
	var sqliteErr *sqlitedriver.Error
	require.ErrorAs(t, err, &sqliteErr)
	require.Equal(t, 787, sqliteErr.Code(), "child-only COMMIT must fail SQLITE_CONSTRAINT_FOREIGNKEY")
	t.Logf("FK-ON child-only COMMIT rejected: code=%d", sqliteErr.Code())
	// modernc v1.54.0 itself rolls back this failed driver COMMIT. Do not
	// broadly ignore cleanup errors: assert the exact already-finished state.
	_, err = conn.ExecContext(context.Background(), "ROLLBACK")
	require.ErrorAs(t, err, &sqliteErr)
	require.Equal(t, 1, sqliteErr.Code())
	require.Equal(t, "SQL logic error: cannot rollback - no transaction is active (1)", sqliteErr.Error())
	t.Logf("same-connection cleanup state verified: code=%d message=%q", sqliteErr.Code(), sqliteErr.Error())
	var count int
	for _, table := range []string{"change_risk_assessments", "change_risk_findings"} {
		require.NoError(t, conn.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM "+table).Scan(&count))
		require.Zero(t, count, "no orphan or partial %s may survive failed COMMIT", table)
	}
	// BEGIN on the same physical connection also proves no unresolved backend
	// transaction remains. A complete valid risk transaction must still commit.
	tx, err = conn.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	defer tx.Rollback()
	require.NoError(t, riskSchemaChildren(tx, x))
	require.NoError(t, riskSchemaHeader(tx, x))
	require.NoError(t, tx.Commit())
	require.NoError(t, conn.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM change_risk_findings`).Scan(&count))
	require.Equal(t, len(x.result.Findings), count)
	require.NoError(t, conn.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM change_risk_assessments`).Scan(&count))
	require.Equal(t, 1, count)
	_, err = conn.ExecContext(context.Background(), `INSERT INTO user_roles(user_id,role_id) VALUES(?,?)`, uuid.NewString(), uuid.NewString())
	require.ErrorAs(t, err, &sqliteErr)
	require.Equal(t, 787, sqliteErr.Code(), "follow-up invalid FK write must still fail")
	require.Same(t, physical, storeConnectionSettings(t, conn, "wal"))
	require.NoError(t, conn.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM pragma_foreign_key_check`).Scan(&count))
	require.Zero(t, count)
	t.Log("zero partial/orphan rows; same physical connection accepted valid transaction, rejected invalid FK, FK=1 busy=5000 WAL retained")

	// Separate disposable database and pinned physical connection, never the
	// positive fixture or any operational DB. Discard while FK-OFF so this
	// connection cannot return to even its isolated application's idle pool.
	t.Run("isolated FK-OFF negative control", func(t *testing.T) {
		negative := riskSchemaSource(t, false)
		negativeConn, err := negative.f.s.db.Conn(context.Background())
		require.NoError(t, err)
		defer storeDiscardConnection(t, negativeConn)
		storeConnectionSettings(t, negativeConn, "wal")
		_, err = negativeConn.ExecContext(context.Background(), "PRAGMA foreign_keys=OFF")
		require.NoError(t, err)
		var enabled int
		require.NoError(t, negativeConn.QueryRowContext(context.Background(), "PRAGMA foreign_keys").Scan(&enabled))
		require.Zero(t, enabled, "disable before BEGIN, never mid-transaction")
		negativeTx, err := negativeConn.BeginTx(context.Background(), nil)
		require.NoError(t, err)
		defer negativeTx.Rollback()
		require.NoError(t, riskSchemaChildren(negativeTx, negative))
		require.NoError(t, negativeTx.Commit())
		var orphanCount, headerCount int
		require.NoError(t, negativeConn.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM change_risk_findings`).Scan(&orphanCount))
		require.Equal(t, len(negative.result.Findings), orphanCount)
		require.NoError(t, negativeConn.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM change_risk_assessments`).Scan(&headerCount))
		require.Zero(t, headerCount)
		t.Logf("separate FK-OFF database committed %d orphan children with %d headers; physical connection discarded without pool return", orphanCount, headerCount)
	})
}

func TestRiskSchemaSealedSourceAndReplaceRoutes(t *testing.T) {
	for _, zero := range []bool{false, true} {
		t.Run(fmt.Sprint(zero), func(t *testing.T) {
			x := riskSchemaSource(t, zero)
			tx, err := x.f.s.db.Begin()
			require.NoError(t, err)
			require.NoError(t, riskSchemaChildren(tx, x))
			require.NoError(t, riskSchemaHeader(tx, x))
			require.NoError(t, tx.Commit())
			queries := []string{
				`UPDATE change_risk_assessments SET review_priority='urgent'`, `DELETE FROM change_risk_assessments`,
				`INSERT OR REPLACE INTO change_risk_assessments SELECT * FROM change_risk_assessments`,
				`UPDATE OR REPLACE change_risk_assessments SET id=id`,
				`UPDATE change_risk_findings SET parameters_json='[]'`, `DELETE FROM change_risk_findings`,
				`INSERT OR REPLACE INTO change_risk_findings SELECT * FROM change_risk_findings`,
				`UPDATE OR REPLACE change_risk_findings SET ordinal=ordinal`,
				`INSERT OR REPLACE INTO change_risk_definitions SELECT * FROM change_risk_definitions`,
				`UPDATE OR REPLACE change_risk_definitions SET model_version=model_version`, `DELETE FROM change_risk_definitions`,
				`UPDATE change_events SET added_lines=added_lines+1 WHERE id=?`, `DELETE FROM change_events WHERE id=?`,
				`INSERT OR REPLACE INTO change_events SELECT * FROM change_events WHERE id=?`,
				`INSERT OR REPLACE INTO change_events SELECT 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa',device_id,config_type,from_backup_id,to_backup_id,detected_at,added_lines,removed_lines,authorization_status,policy_status,created_at,updated_at,analysis_version,analyzed_at,item_count FROM change_events WHERE id=?`,
				`INSERT INTO change_event_items(id,change_event_id,ordinal,operation,command,created_at) VALUES('new',?,100,'add','description lab','2026-10-04T13:00:00Z')`,
			}
			if !zero {
				queries = append(queries, `UPDATE change_event_items SET command='description altered' WHERE change_event_id=?`, `DELETE FROM change_event_items WHERE change_event_id=?`, `INSERT OR REPLACE INTO change_event_items SELECT * FROM change_event_items WHERE change_event_id=?`, `UPDATE OR REPLACE change_event_items SET ordinal=ordinal WHERE change_event_id=?`)
			}
			for _, q := range queries {
				var args []any
				if strings.Contains(q, "?") {
					args = []any{x.event.ID}
				}
				_, err = x.f.s.db.Exec(q, args...)
				require.Error(t, err, q)
			}
			// A conflicting source-item ID from another event is also guarded before
			// REPLACE's implicit delete with recursive_triggers at its default OFF.
			if !zero {
				other := authorizationEvent(t, x.f.s, x.event.DeviceID)
				_, err = x.f.s.db.Exec(`INSERT OR REPLACE INTO change_event_items SELECT id,?,ordinal,operation,context_path_json,command,sensitive,created_at,classification_version,category,subcategory,classification_rule,classified_at FROM change_event_items WHERE change_event_id=?`, other.ID, x.event.ID)
				require.Error(t, err)
			}
			var recursive int
			require.NoError(t, x.f.s.db.QueryRow(`PRAGMA recursive_triggers`).Scan(&recursive))
			require.Zero(t, recursive)
		})
	}
}

func TestRiskSchemaAllowsRealLateAuthorizationAndPolicy(t *testing.T) {
	for _, policyFirst := range []bool{false, true} {
		t.Run(fmt.Sprint(policyFirst), func(t *testing.T) {
			x := riskSchemaSource(t, false)
			ctx := context.Background()
			a := x.f.approve(t, x.f.request(t))
			require.NotEqual(t, a.RequestedBy, a.ApprovedBy)
			require.NotNil(t, a.ApprovedAt)
			require.True(t, a.ApprovedAt.Before(x.event.DetectedAt))
			require.False(t, x.event.DetectedAt.Before(a.WindowStart))
			require.True(t, x.event.DetectedAt.Before(a.WindowEnd))
			p := x.f.activatePolicy(t, x.f.policy(t, completePolicyRules), x.f.now.Add(time.Hour))
			captured, err := time.Parse(risk.UTC9, x.captured)
			require.NoError(t, err)
			require.True(t, captured.After(x.event.UpdatedAt))
			// Both late workflows occur AFTER the recorded risk capture. The
			// approved window constrains detection, not retrospective link time.
			first, second := captured.Add(time.Minute), captured.Add(2*time.Minute)
			tx, err := x.f.s.db.Begin()
			require.NoError(t, err)
			require.NoError(t, riskSchemaChildren(tx, x))
			require.NoError(t, riskSchemaHeader(tx, x))
			require.NoError(t, tx.Commit())
			snapshot := func() [2]string {
				var value [2]string
				require.NoError(t, x.f.s.db.QueryRow(`SELECT json_array(id,event_id,device_id,config_type,revision,previous_assessment_id,model_version,definition_hash,evidence_schema_version,evidence_hash,evidence_json,source_bindings_json,result_hash,result_json,technical_impact,known_impact_floor,evidence_completeness,review_priority,priority_rank,total_items,known_items,unknown_items,distinct_known_categories,finding_count,recorded_authorization_status,authorization_id,recorded_policy_status,policy_evaluation_id,captured_at,assessed_by) FROM change_risk_assessments WHERE id=?`, x.id).Scan(&value[0]))
				require.NoError(t, x.f.s.db.QueryRow(`SELECT json_group_array(json_array(assessment_id,event_id,ordinal,finding_key,rule_id,reason_code,kind,subject_kind,subject_id,source_item_id,item_ordinal,impact,priority_floor,parameters_json)) FROM (SELECT * FROM change_risk_findings WHERE assessment_id=? ORDER BY ordinal)`, x.id).Scan(&value[1]))
				return value
			}
			original := snapshot()
			var linkedAt time.Time
			var finalized domain.ChangePolicyEvaluation
			var finalItems []domain.ChangePolicyEvaluationItem
			auth := func(at time.Time) {
				before, err := x.f.s.GetChangeEvent(ctx, x.event.ID)
				require.NoError(t, err)
				require.Equal(t, domain.AuthorizationStatusUnaccounted, before.AuthorizationStatus)
				currentTicket, err := x.f.s.GetChangeAuthorization(ctx, x.f.device, a.ID)
				require.NoError(t, err)
				link := x.f.link(a, x.event)
				link.LinkedAt = at
				require.True(t, link.LinkedAt.After(before.UpdatedAt), "never backdate relative to policy/event update")
				require.True(t, link.LinkedAt.After(currentTicket.UpdatedAt))
				if policyFirst {
					require.NotEmpty(t, finalized.ID)
					require.True(t, link.LinkedAt.After(finalized.EvaluatedAt))
				}
				require.NotEqual(t, a.RequestedBy, link.LinkedBy)
				require.Equal(t, x.f.approver, link.LinkedBy)
				require.Equal(t, domain.ChangeAuthorizationAttestation, link.Attestation)
				require.NoError(t, x.f.s.LinkChangeEventAuthorization(ctx, x.f.device, link))
				linkedAt = link.LinkedAt
				after, err := x.f.s.GetChangeEvent(ctx, x.event.ID)
				require.NoError(t, err)
				require.Equal(t, domain.AuthorizationStatusAuthorized, after.AuthorizationStatus)
				require.Equal(t, before.PolicyStatus, after.PolicyStatus)
				require.Equal(t, linkedAt, after.UpdatedAt)
				storedLink, err := x.f.s.GetChangeEventAuthorization(ctx, x.f.device, x.event.ID)
				require.NoError(t, err)
				require.Equal(t, link, storedLink)
				require.Equal(t, original, snapshot())
			}
			policy := func(at time.Time) {
				before, err := x.f.s.GetChangeEvent(ctx, x.event.ID)
				require.NoError(t, err)
				require.True(t, at.After(before.UpdatedAt))
				finalized, err = x.f.s.PersistPolicyEvaluation(ctx, x.f.device, x.event.ID, x.f.approver, at)
				require.NoError(t, err)
				require.Equal(t, "pass", finalized.Result)
				var count int
				finalItems, count, err = x.f.s.ListPolicyEvaluationItems(ctx, x.f.device, x.event.ID, repositories.Page{Limit: 200})
				require.NoError(t, err)
				require.Equal(t, x.event.ItemCount, count)
				after, err := x.f.s.GetChangeEvent(ctx, x.event.ID)
				require.NoError(t, err)
				require.Equal(t, before.AuthorizationStatus, after.AuthorizationStatus)
				require.Equal(t, domain.PolicyStatusPass, after.PolicyStatus)
				require.Equal(t, original, snapshot())
			}
			if policyFirst {
				policy(first)
				auth(second)
			} else {
				auth(first)
				policy(second)
			}
			revokedAt := second.Add(time.Minute)
			revoked, err := x.f.s.TransitionChangeAuthorization(ctx, x.f.device, a.ID, x.f.approver, "revoked", revokedAt)
			require.NoError(t, err)
			require.Equal(t, domain.ChangeAuthorizationRevoked, revoked.Status)
			require.NotNil(t, revoked.RevokedAt)
			require.Equal(t, revokedAt, *revoked.RevokedAt)
			history, total, err := x.f.s.ListChangeAuthorizationHistory(ctx, x.f.device, a.ID, repositories.Page{Limit: 50})
			require.NoError(t, err)
			require.Equal(t, 4, total)
			require.Len(t, history, 4)
			for _, h := range history {
				if h.Action == "linked" {
					require.Equal(t, linkedAt, h.OccurredAt)
					require.Equal(t, x.event.ID, h.EventID)
				}
				if h.Action == "revoked" {
					require.Equal(t, revokedAt, h.OccurredAt)
				}
			}
			require.Equal(t, original, snapshot())
			_, err = x.f.s.RetirePolicySet(ctx, x.f.device, p.ID, x.f.approver, revokedAt.Add(time.Minute))
			require.NoError(t, err)
			storedPolicy, err := x.f.s.GetPolicyEvaluation(ctx, x.f.device, x.event.ID)
			require.NoError(t, err)
			require.Equal(t, finalized, storedPolicy)
			storedItems, count, err := x.f.s.ListPolicyEvaluationItems(ctx, x.f.device, x.event.ID, repositories.Page{Limit: 200})
			require.NoError(t, err)
			require.Equal(t, x.event.ItemCount, count)
			require.Equal(t, finalItems, storedItems)
			_, err = x.f.s.db.Exec(`UPDATE change_policy_evaluations SET result=result WHERE id=?`, finalized.ID)
			require.Error(t, err, "Task 5 immutable finalized evidence must remain enforced")
			finalEvent, err := x.f.s.GetChangeEvent(ctx, x.event.ID)
			require.NoError(t, err)
			require.Equal(t, domain.AuthorizationStatusAuthorized, finalEvent.AuthorizationStatus)
			require.Equal(t, domain.PolicyStatusPass, finalEvent.PolicyStatus)
			require.Equal(t, original, snapshot())
			t.Logf("policyFirst=%t request=%s approval=%s detection=%s window=[%s,%s) riskCapture=%s link=%s policy=%s revocation=%s; history/typed risk header/findings and policy evidence preserved", policyFirst, a.RequestedAt.Format(risk.UTC9), a.ApprovedAt.Format(risk.UTC9), x.event.DetectedAt.Format(risk.UTC9), a.WindowStart.Format(risk.UTC9), a.WindowEnd.Format(risk.UTC9), captured.Format(risk.UTC9), linkedAt.Format(risk.UTC9), finalized.EvaluatedAt.Format(risk.UTC9), revokedAt.Format(risk.UTC9))
		})
	}
}
