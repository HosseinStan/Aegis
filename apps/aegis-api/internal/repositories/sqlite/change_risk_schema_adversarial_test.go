package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"company.local/aegis/apps/aegis-api/internal/domain"
	"company.local/aegis/apps/aegis-api/internal/repositories"
	"company.local/aegis/apps/aegis-api/internal/risk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	sqlitedriver "modernc.org/sqlite"
)

// These helpers are SQL attacker fixtures only, not a trusted materializer,
// application repository API, hash verifier, permission check or replay reader.
func riskAdversarialHeader(tx *sql.Tx, x riskSchemaFixture, override map[string]any) error {
	fields := []string{"id", "event_id", "device_id", "config_type", "revision", "previous_assessment_id", "model_version", "definition_hash", "evidence_schema_version", "evidence_hash", "evidence_json", "source_bindings_json", "result_hash", "result_json", "technical_impact", "known_impact_floor", "evidence_completeness", "review_priority", "priority_rank", "total_items", "known_items", "unknown_items", "distinct_known_categories", "finding_count", "recorded_authorization_status", "authorization_id", "recorded_policy_status", "policy_evaluation_id", "captured_at", "assessed_by"}
	var auth, policy any
	if x.evidence.Authorization.Ticket != nil {
		auth = x.evidence.Authorization.Ticket.ID
	}
	if x.evidence.Policy.Final != nil {
		policy = x.evidence.Policy.Final.ID
	}
	args := []any{x.id, x.event.ID, x.event.DeviceID, x.event.ConfigType, 1, nil, 1, x.definitionHash, 1, x.evidenceHash, x.evidenceJSON, x.bindingsJSON, x.resultHash, x.resultJSON, x.result.TechnicalImpact, x.result.KnownImpactFloor, x.result.EvidenceCompleteness, x.result.ReviewPriority, map[string]int{"normal": 0, "elevated": 1, "urgent": 2}[x.result.ReviewPriority], x.result.TotalItems, x.result.KnownItems, x.result.UnknownItems, x.result.DistinctKnownCategories, len(x.result.Findings), x.evidence.Authorization.RecordedStatus, auth, x.evidence.Policy.RecordedStatus, policy, x.captured, x.f.approver}
	for i, key := range fields {
		if value, ok := override[key]; ok {
			args[i] = value
		}
	}
	_, err := tx.Exec("INSERT INTO change_risk_assessments("+strings.Join(fields, ",")+") VALUES("+strings.TrimSuffix(strings.Repeat("?,", len(fields)), ",")+")", args...)
	return err
}

// Snapshot complete values in stable rowid order; names come from SQLite's
// catalog, are quoted as identifiers, and are never supplied by an API caller.
func riskAdversarialRows(t *testing.T, db *sql.DB, table string) []string {
	t.Helper()
	rows, err := db.Query(`SELECT * FROM "` + strings.ReplaceAll(table, `"`, `""`) + `" ORDER BY rowid`)
	require.NoError(t, err)
	defer rows.Close()
	cols, err := rows.Columns()
	require.NoError(t, err)
	out := []string{}
	for rows.Next() {
		values := make([]any, len(cols))
		targets := make([]any, len(cols))
		for i := range values {
			targets[i] = &values[i]
		}
		require.NoError(t, rows.Scan(targets...))
		raw, err := json.Marshal(values)
		require.NoError(t, err)
		out = append(out, string(raw))
	}
	require.NoError(t, rows.Err())
	return out
}
func riskAdversarialSnapshot(t *testing.T, s *Store) map[string][]string {
	t.Helper()
	rows, err := s.db.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	require.NoError(t, err)
	names := []string{}
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		names = append(names, name)
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	result := map[string][]string{}
	for _, name := range names {
		result[name] = riskAdversarialRows(t, s.db, name)
	}
	return result
}
func riskAdversarialCommit(t *testing.T, x riskSchemaFixture, override map[string]any) {
	t.Helper()
	tx, err := x.f.s.db.Begin()
	require.NoError(t, err)
	defer tx.Rollback()
	require.NoError(t, riskSchemaChildren(tx, x))
	require.NoError(t, riskAdversarialHeader(tx, x, override))
	require.NoError(t, tx.Commit())
}

// Permit rejection at INSERT or deferred COMMIT, but never a successfully
// committed attack. Always compare every original row after exact cleanup.
func riskAdversarialReject(t *testing.T, s *Store, attempt func(*sql.Tx) (string, error)) {
	t.Helper()
	before := riskAdversarialSnapshot(t, s)
	ctx := context.Background()
	conn, err := s.db.Conn(ctx)
	require.NoError(t, err)
	defer conn.Close()
	tx, err := conn.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer tx.Rollback()
	boundary, attackErr := attempt(tx)
	committed := false
	if attackErr == nil {
		boundary = "COMMIT"
		attackErr = tx.Commit()
		committed = true
	}
	if attackErr == nil {
		_ = conn.Close()
		t.Fatalf("SECURITY STOP: forbidden SQL committed at %s", boundary)
	}
	var sqliteErr *sqlitedriver.Error
	require.ErrorAs(t, attackErr, &sqliteErr)
	require.Contains(t, []int{1811, 787, 275, 1555, 2067}, sqliteErr.Code(), "must be a concrete integrity rejection, not SQL syntax or unrelated failure")
	t.Logf("rejected boundary=%s sqlite_code=%d message=%q", boundary, sqliteErr.Code(), sqliteErr.Error())
	if committed {
		_, err = conn.ExecContext(ctx, "ROLLBACK")
		if err != nil {
			var cleanup *sqlitedriver.Error
			require.ErrorAs(t, err, &cleanup)
			require.Equal(t, 1, cleanup.Code())
			require.Equal(t, "SQL logic error: cannot rollback - no transaction is active (1)", cleanup.Error())
		}
	} else {
		require.NoError(t, tx.Rollback())
	}
	require.NoError(t, conn.Close())
	require.Equal(t, before, riskAdversarialSnapshot(t, s), "no partial evidence, collateral replacement/delete, or changed original row may survive")
}
func riskAdversarialJSON(t *testing.T, s *Store, raw, expression string, args ...any) string {
	t.Helper()
	var out string
	all := append([]any{raw}, args...)
	require.NoError(t, s.db.QueryRow("SELECT "+expression, all...).Scan(&out))
	return out
}

// Capture explicit test metadata from existing real workflows. Commands and
// context never enter these fixtures' risk evidence; production materializer
// authorization/reclassification/limits remain wholly deferred to Phase C.
func riskAdversarialEvidence(t *testing.T, f authorizationFixture, event domain.ChangeEvent) riskSchemaFixture {
	t.Helper()
	ctx := context.Background()
	f.device = event.DeviceID
	current, err := f.s.GetChangeEvent(ctx, event.ID)
	require.NoError(t, err)
	event = current
	canon := func(at time.Time) string { return at.UTC().Format(risk.UTC9) }
	pointer := func(at *time.Time) *string {
		if at == nil {
			return nil
		}
		v := canon(*at)
		return &v
	}
	optionalID := func(id string) *string {
		if id == "" {
			return nil
		}
		return &id
	}
	e := domain.RiskEvidenceSnapshot{SchemaVersion: 1, Event: domain.RiskEventSnapshot{EventID: event.ID, DeviceID: event.DeviceID, ConfigType: event.ConfigType, FromBackupID: event.FromBackupID, ToBackupID: event.ToBackupID, DetectedAt: canon(event.DetectedAt), CreatedAt: canon(event.CreatedAt), AnalysisVersion: event.AnalysisVersion, ItemCount: event.ItemCount, AddedLines: event.AddedLines, RemovedLines: event.RemovedLines, AnalyzedAt: pointer(event.AnalyzedAt)}, Items: []domain.RiskItemSignal{}, Authorization: domain.RiskAuthorizationObservation{RecordedStatus: event.AuthorizationStatus, History: []domain.RiskAuthorizationHistory{}}, Policy: domain.RiskPolicyObservation{RecordedStatus: event.PolicyStatus}, Eligibility: domain.RiskEligibility{Persistable: true, Reasons: []string{}}}
	b := domain.RiskSourceBindings{Event: domain.RiskEventTimeBinding{EventID: event.ID}, Items: []domain.RiskItemTimeBinding{}, Governance: domain.RiskGovernanceTimeBindings{History: []domain.RiskHistoryTimeBinding{}}}
	require.NoError(t, f.s.db.QueryRow(`SELECT detected_at,analyzed_at,created_at FROM change_events WHERE id=?`, event.ID).Scan(&b.Event.DetectedAt, &b.Event.AnalyzedAt, &b.Event.CreatedAt))
	items, total, err := f.s.ListChangeEventItems(ctx, event.ID, repositories.Page{Limit: 200})
	require.NoError(t, err)
	require.Equal(t, event.ItemCount, total)
	for _, i := range items {
		e.Items = append(e.Items, domain.RiskItemSignal{ItemID: i.ID, Ordinal: i.Ordinal, Operation: i.Operation, Category: i.Category, Subcategory: i.Subcategory, ClassificationRule: i.ClassificationRule, ClassificationVersion: i.ClassificationVersion, Sensitive: i.Sensitive, CreatedAt: canon(i.CreatedAt), ClassifiedAt: pointer(i.ClassifiedAt)})
		ib := domain.RiskItemTimeBinding{ItemID: i.ID, Ordinal: i.Ordinal}
		require.NoError(t, f.s.db.QueryRow(`SELECT created_at,classified_at FROM change_event_items WHERE id=?`, i.ID).Scan(&ib.CreatedAt, &ib.ClassifiedAt))
		b.Items = append(b.Items, ib)
	}
	if event.AuthorizationStatus == "authorized" {
		link, err := f.s.GetChangeEventAuthorization(ctx, event.DeviceID, event.ID)
		require.NoError(t, err)
		a, err := f.s.GetChangeAuthorization(ctx, event.DeviceID, link.AuthorizationID)
		require.NoError(t, err)
		require.NotNil(t, a.ApprovedAt)
		e.Authorization.Link = &domain.RiskAuthorizationLink{EventID: link.EventID, AuthorizationID: a.ID, LinkedBy: link.LinkedBy, LinkedAt: canon(link.LinkedAt), Attestation: link.Attestation}
		e.Authorization.Ticket = &domain.RiskAuthorizationTicket{ID: a.ID, DeviceID: a.DeviceID, ConfigType: a.ConfigType, Status: a.Status, AllowedCategories: slices.Clone(a.AllowedCategories), WindowStart: canon(a.WindowStart), WindowEnd: canon(a.WindowEnd), RequestedBy: a.RequestedBy, RequestedAt: canon(a.RequestedAt), ApprovedBy: a.ApprovedBy, ApprovedAt: canon(*a.ApprovedAt), RejectedBy: optionalID(a.RejectedBy), RejectedAt: pointer(a.RejectedAt), RevokedBy: optionalID(a.RevokedBy), RevokedAt: pointer(a.RevokedAt), CreatedAt: canon(a.CreatedAt)}
		tb := &domain.RiskTicketTimeBinding{AuthorizationID: a.ID}
		var rejected, revoked sql.NullString
		require.NoError(t, f.s.db.QueryRow(`SELECT window_start,window_end,requested_at,approved_at,rejected_at,revoked_at,created_at FROM change_authorizations WHERE id=?`, a.ID).Scan(&tb.WindowStart, &tb.WindowEnd, &tb.RequestedAt, &tb.ApprovedAt, &rejected, &revoked, &tb.CreatedAt))
		if rejected.Valid {
			tb.RejectedAt = &rejected.String
		}
		if revoked.Valid {
			tb.RevokedAt = &revoked.String
		}
		b.Governance.Ticket = tb
		lb := &domain.RiskLinkTimeBinding{EventID: event.ID, AuthorizationID: a.ID}
		require.NoError(t, f.s.db.QueryRow(`SELECT linked_at FROM change_event_authorizations WHERE event_id=?`, event.ID).Scan(&lb.LinkedAt))
		b.Governance.Link = lb
		history, count, err := f.s.ListChangeAuthorizationHistory(ctx, event.DeviceID, a.ID, repositories.Page{Limit: 50})
		require.NoError(t, err)
		require.Len(t, history, count)
		for _, h := range history {
			e.Authorization.History = append(e.Authorization.History, domain.RiskAuthorizationHistory{ID: h.ID, Action: h.Action, ActorID: h.ActorUserID, OccurredAt: canon(h.OccurredAt), EventID: optionalID(h.EventID)})
			hb := domain.RiskHistoryTimeBinding{ID: h.ID, Action: h.Action, EventID: optionalID(h.EventID)}
			require.NoError(t, f.s.db.QueryRow(`SELECT occurred_at FROM change_authorization_history WHERE id=?`, h.ID).Scan(&hb.OccurredAt))
			b.Governance.History = append(b.Governance.History, hb)
		}
	}
	if event.PolicyStatus != "not_evaluated" {
		p, err := f.s.GetPolicyEvaluation(ctx, event.DeviceID, event.ID)
		require.NoError(t, err)
		var definition domain.PolicyDefinition
		require.NoError(t, json.Unmarshal([]byte(p.PolicyDefinitionSnapshot), &definition))
		final := &domain.RiskPolicyFinal{ID: p.ID, EventID: event.ID, PolicySetID: p.PolicySetID, DefinitionHash: p.PolicyDefinitionHash, Definition: definition, EngineVersion: p.EngineVersion, Result: p.Result, EvaluatedBy: p.EvaluatedBy, EvaluatedAt: canon(p.EvaluatedAt), TotalItems: p.TotalItems, AllowedCount: p.AllowedCount, DeniedCount: p.DeniedCount, UncoveredCount: p.UncoveredCount, Items: []domain.PolicyItemResult{}}
		rows, count, err := f.s.ListPolicyEvaluationItems(ctx, event.DeviceID, event.ID, repositories.Page{Limit: 200})
		require.NoError(t, err)
		require.Len(t, rows, count)
		for _, i := range rows {
			final.Items = append(final.Items, domain.PolicyItemResult{ItemID: i.ChangeEventItemID, Ordinal: i.Ordinal, Category: i.Category, Operation: i.Operation, Decision: i.Decision, MatchedRuleKey: i.MatchedRuleKey})
		}
		e.Policy.Final = final
		pb := &domain.RiskPolicyTimeBinding{EvaluationID: p.ID}
		require.NoError(t, f.s.db.QueryRow(`SELECT evaluated_at FROM change_policy_evaluations WHERE id=?`, p.ID).Scan(&pb.EvaluatedAt))
		b.Governance.Policy = pb
	}
	x := riskSchemaFixture{f: f, event: event, evidence: e, bindings: b, id: uuid.NewString(), captured: canon(f.now.Add(6 * time.Hour))}
	x.refresh(t)
	return x
}

func TestRiskSchemaCrossScope(t *testing.T) {
	for _, name := range []string{"finding event", "foreign item", "assessment event mismatch", "wrong device", "wrong config", "foreign backups", "foreign ordinal", "foreign model hash", "missing model", "unsupported schema"} {
		t.Run(name, func(t *testing.T) {
			x := riskSchemaSource(t, false)
			otherEvent := authorizationEvent(t, x.f.s, x.f.otherDevice)
			other := riskAdversarialEvidence(t, x.f, otherEvent)
			witness := riskAdversarialEvidence(t, x.f, authorizationEvent(t, x.f.s, x.f.otherDevice))
			riskAdversarialCommit(t, witness, nil)
			foreignBackups := riskAdversarialJSON(t, x.f.s, x.evidenceJSON, `json_set(?,'$.event.from_backup_id',?,'$.event.to_backup_id',?)`, other.event.FromBackupID, other.event.ToBackupID)
			riskAdversarialReject(t, x.f.s, func(tx *sql.Tx) (string, error) {
				children := x
				children.result = domain.RiskEvaluationResult{}
				raw, err := json.Marshal(x.result)
				require.NoError(t, err)
				require.NoError(t, json.Unmarshal(raw, &children.result))
				header := map[string]any{}
				switch name {
				case "finding event":
					children.event.ID = other.event.ID
				case "foreign item":
					children.result.Findings[0].Subject.ID = other.evidence.Items[0].ItemID
					children.result.Findings[0].Key = children.result.Findings[0].RuleID + ":item:" + other.evidence.Items[0].ItemID
				case "assessment event mismatch":
				// Keep the header/technical source valid; change ONLY a non-item
				// child's event so the deferred parent-pair scope is contradicted.
				case "wrong device":
					header["device_id"] = other.event.DeviceID
				case "wrong config":
					header["config_type"] = "startup"
				case "foreign backups":
					header["evidence_json"] = foreignBackups
				case "foreign ordinal":
					children.result.Findings[0].ItemOrdinal = &other.evidence.Items[1].Ordinal
				case "foreign model hash":
					header["definition_hash"] = strings.Repeat("0", 64)
				case "missing model":
					header["model_version"] = 99
				case "unsupported schema":
					header["evidence_schema_version"] = 99
				}
				if name == "assessment event mismatch" {
					for _, finding := range children.result.Findings {
						one := children
						one.result.Findings = []domain.RiskFinding{finding}
						if finding.Subject.Kind != "item" {
							one.event.ID = other.event.ID
						}
						if err := riskSchemaChildren(tx, one); err != nil {
							return "finding INSERT", err
						}
					}
				} else if err := riskSchemaChildren(tx, children); err != nil {
					return "finding INSERT", err
				}
				return "header INSERT", riskAdversarialHeader(tx, x, header)
			})
		})
	}
}

func TestRiskSchemaRevisionChain(t *testing.T) {
	// Governance is monotonic; use a REAL late authorization to create B, never
	// fabricate a historical A recurrence. SQL index inventory is checked below.
	x := riskSchemaSource(t, false)
	riskAdversarialCommit(t, x, nil)
	a := x.f.approve(t, x.f.request(t))
	link := x.f.link(a, x.event)
	link.LinkedAt = x.f.now.Add(6*time.Hour + time.Minute)
	require.NoError(t, x.f.s.LinkChangeEventAuthorization(context.Background(), x.f.device, link))
	b := riskAdversarialEvidence(t, x.f, x.event)
	b.captured = x.f.now.Add(6*time.Hour + 2*time.Minute).Format(risk.UTC9)
	require.NotEqual(t, x.evidenceHash, b.evidenceHash)
	otherEvent := authorizationEvent(t, x.f.s, x.f.otherDevice)
	other := riskAdversarialEvidence(t, x.f, otherEvent)
	riskAdversarialCommit(t, other, nil)
	fresh := riskAdversarialEvidence(t, x.f, authorizationEvent(t, x.f.s, x.f.otherDevice))
	for _, name := range []string{"duplicate root", "wrong first number", "skipped revision", "nonexistent predecessor", "foreign predecessor", "root nonnull predecessor"} {
		t.Run(name, func(t *testing.T) {
			attack := b
			attack.id = uuid.NewString()
			override := map[string]any{"revision": 2, "previous_assessment_id": x.id}
			switch name {
			case "duplicate root":
				override["revision"] = 1
				override["previous_assessment_id"] = nil
			case "wrong first number":
				attack = fresh
				attack.id = uuid.NewString()
				override["revision"] = 2
				override["previous_assessment_id"] = nil
			case "skipped revision":
				override["revision"] = 3
			case "nonexistent predecessor":
				override["previous_assessment_id"] = uuid.NewString()
			case "foreign predecessor":
				override["previous_assessment_id"] = other.id
			case "root nonnull predecessor":
				attack = fresh
				attack.id = uuid.NewString()
				override["revision"] = 1
			}
			riskAdversarialReject(t, x.f.s, func(tx *sql.Tx) (string, error) {
				if err := riskSchemaChildren(tx, attack); err != nil {
					return "finding INSERT", err
				}
				return "header INSERT", riskAdversarialHeader(tx, attack, override)
			})
		})
	}
	riskAdversarialCommit(t, b, map[string]any{"revision": 2, "previous_assessment_id": x.id})
	for _, name := range []string{"duplicate successor", "stale predecessor", "unchanged latest", "revision identity UPDATE", "sealed DELETE"} {
		t.Run(name, func(t *testing.T) {
			riskAdversarialReject(t, x.f.s, func(tx *sql.Tx) (string, error) {
				if name == "revision identity UPDATE" {
					_, err := tx.Exec(`UPDATE OR REPLACE change_risk_assessments SET id=? WHERE id=?`, uuid.NewString(), b.id)
					return "UPDATE OR REPLACE", err
				}
				if name == "sealed DELETE" {
					_, err := tx.Exec(`DELETE FROM change_risk_assessments WHERE id=?`, b.id)
					return "DELETE", err
				}
				attack := b
				attack.id = uuid.NewString()
				revision := 3
				predecessor := b.id
				if name == "duplicate successor" {
					revision = 2
					predecessor = x.id
				}
				if name == "stale predecessor" {
					predecessor = x.id
				}
				if err := riskSchemaChildren(tx, attack); err != nil {
					return "finding INSERT", err
				}
				return "header INSERT", riskAdversarialHeader(tx, attack, map[string]any{"revision": revision, "previous_assessment_id": predecessor})
			})
		})
	}
	rows, err := x.f.s.db.Query(`SELECT name FROM pragma_index_list('change_risk_assessments') WHERE "unique"=1`)
	require.NoError(t, err)
	indexes := []string{}
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		indexes = append(indexes, name)
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	for _, name := range indexes {
		r, err := x.f.s.db.Query(`SELECT name FROM pragma_index_info(?) ORDER BY seqno`, name)
		require.NoError(t, err)
		columns := []string{}
		for r.Next() {
			var col string
			require.NoError(t, r.Scan(&col))
			columns = append(columns, col)
		}
		require.NoError(t, r.Err())
		require.NoError(t, r.Close())
		sorted := slices.Clone(columns)
		slices.Sort(sorted)
		require.NotEqual(t, []string{"definition_hash", "event_id", "evidence_hash"}, sorted)
		t.Logf("unique index %s columns=%v", name, columns)
	}
	t.Log("valid R1(A) -> R2(B) committed using real late authorization; full A->B->A recurrence NOT COVERED: current source-consistent governance is monotonic, no forged model or governance used")
}

func TestRiskSchemaEveryTechnicalColumnAndCollisions(t *testing.T) {
	x := riskSchemaSource(t, false)
	riskAdversarialCommit(t, x, nil)
	same := authorizationEvent(t, x.f.s, x.f.device)
	foreign := authorizationEvent(t, x.f.s, x.f.otherDevice)
	// Each value is valid for its ordinary column type. Rejection must be an
	// integrity trigger/constraint, never a misspelled column or syntax error.
	eventChanges := map[string]any{"id": uuid.NewString(), "device_id": x.f.otherDevice, "config_type": "startup", "from_backup_id": same.FromBackupID, "to_backup_id": same.ToBackupID, "detected_at": "2026-10-04T13:00:00.000000001Z", "added_lines": x.event.AddedLines + 1, "removed_lines": x.event.RemovedLines + 1, "created_at": "2026-10-04T13:00:00.000000001Z", "analysis_version": 2, "analyzed_at": "2026-10-04T13:00:00.000000001Z", "item_count": x.event.ItemCount + 1}
	eventKeys := make([]string, 0, len(eventChanges))
	for key := range eventChanges {
		eventKeys = append(eventKeys, key)
	}
	slices.Sort(eventKeys)
	for _, column := range eventKeys {
		t.Run("event "+column, func(t *testing.T) {
			riskAdversarialReject(t, x.f.s, func(tx *sql.Tx) (string, error) {
				_, err := tx.Exec("UPDATE change_events SET "+column+"=? WHERE id=?", eventChanges[column], x.event.ID)
				return "technical event UPDATE", err
			})
		})
	}
	item := x.evidence.Items[0]
	itemChanges := map[string]any{"id": uuid.NewString(), "change_event_id": same.ID, "ordinal": 9, "operation": "add", "context_path_json": "[\"interface LAB\"]", "command": "description changed", "sensitive": 1, "created_at": "2026-10-04T13:00:00.000000001Z", "classification_version": 2, "category": "routing", "subcategory": "ospf", "classification_rule": "routing.ospf", "classified_at": "2026-10-04T13:00:00.000000001Z"}
	itemKeys := make([]string, 0, len(itemChanges))
	for key := range itemChanges {
		itemKeys = append(itemKeys, key)
	}
	slices.Sort(itemKeys)
	for _, column := range itemKeys {
		t.Run("item "+column, func(t *testing.T) {
			riskAdversarialReject(t, x.f.s, func(tx *sql.Tx) (string, error) {
				_, err := tx.Exec("UPDATE change_event_items SET "+column+"=? WHERE id=?", itemChanges[column], item.ItemID)
				return "technical item UPDATE", err
			})
		})
	}
	for _, query := range []struct {
		name, sql string
		args      []any
	}{
		{"event to_backup UPDATE OR REPLACE", `UPDATE OR REPLACE change_events SET to_backup_id=? WHERE id=?`, []any{x.event.ToBackupID, same.ID}},
		{"event identity UPDATE OR REPLACE", `UPDATE OR REPLACE change_events SET id=? WHERE id=?`, []any{x.event.ID, foreign.ID}},
		{"item ID UPDATE OR REPLACE", `UPDATE OR REPLACE change_event_items SET id=? WHERE change_event_id=? AND ordinal=0`, []any{item.ItemID, same.ID}},
		{"item ordinal INSERT OR REPLACE", `INSERT OR REPLACE INTO change_event_items SELECT ?,change_event_id,ordinal,operation,context_path_json,command,sensitive,created_at,classification_version,category,subcategory,classification_rule,classified_at FROM change_event_items WHERE id=?`, []any{uuid.NewString(), item.ItemID}},
		{"source event DELETE", `DELETE FROM change_events WHERE id=?`, []any{x.event.ID}},
		{"source item DELETE", `DELETE FROM change_event_items WHERE id=?`, []any{item.ItemID}},
		{"header alternate revision identity REPLACE", `INSERT OR REPLACE INTO change_risk_assessments SELECT ?,event_id,device_id,config_type,revision,previous_assessment_id,model_version,definition_hash,evidence_schema_version,evidence_hash,evidence_json,source_bindings_json,result_hash,result_json,technical_impact,known_impact_floor,evidence_completeness,review_priority,priority_rank,total_items,known_items,unknown_items,distinct_known_categories,finding_count,recorded_authorization_status,authorization_id,recorded_policy_status,policy_evaluation_id,captured_at,assessed_by FROM change_risk_assessments WHERE id=?`, []any{uuid.NewString(), x.id}},
	} {
		t.Run(query.name, func(t *testing.T) {
			riskAdversarialReject(t, x.f.s, func(tx *sql.Tx) (string, error) { _, err := tx.Exec(query.sql, query.args...); return query.name, err })
		})
	}
	// Exercise both finding UNIQUE collisions BEFORE a header exists, rather
	// than allowing the sealed-parent guard to hide the alternate keys.
	unsealedSource := riskAdversarialEvidence(t, x.f, same)
	for _, name := range []string{"finding ordinal", "finding key", "finding source item"} {
		t.Run(name, func(t *testing.T) {
			riskAdversarialReject(t, x.f.s, func(tx *sql.Tx) (string, error) {
				unsealed := unsealedSource
				unsealed.id = uuid.NewString()
				if err := riskSchemaChildren(tx, unsealed); err != nil {
					return "finding seed INSERT", err
				}
				f := unsealed.result.Findings[0]
				ordinal := 8
				rule, kind, subjectKind, subjectID := "risk.attacker.v1", "technical", "item", f.Subject.ID
				var source, itemOrdinal, impact, floor any = f.Subject.ID, 0, "low", "normal"
				if name == "finding ordinal" {
					ordinal = 0
					kind = "summary"
					subjectKind = "event"
					subjectID = unsealed.event.ID
					source = nil
					itemOrdinal = nil
					impact = nil
					floor = nil
				}
				if name == "finding key" {
					f = unsealed.result.Findings[2]
					rule = f.RuleID
					kind = f.Kind
					subjectKind = f.Subject.Kind
					subjectID = f.Subject.ID
					source = nil
					itemOrdinal = nil
					impact = f.Impact
					floor = f.PriorityFloor
				}
				key := rule + ":" + subjectKind + ":" + subjectID
				_, err := tx.Exec(`INSERT OR REPLACE INTO change_risk_findings(assessment_id,event_id,ordinal,finding_key,rule_id,reason_code,kind,subject_kind,subject_id,source_item_id,item_ordinal,impact,priority_floor,parameters_json) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, unsealed.id, unsealed.event.ID, ordinal, key, rule, rule, kind, subjectKind, subjectID, source, itemOrdinal, impact, floor, "[]")
				return "alternate finding UNIQUE INSERT OR REPLACE", err
			})
		})
	}
	var recursive int
	require.NoError(t, x.f.s.db.QueryRow(`PRAGMA recursive_triggers`).Scan(&recursive))
	require.Zero(t, recursive)
}

func riskAdversarialGoverned(t *testing.T, revoked, violation bool) riskSchemaFixture {
	t.Helper()
	x := riskSchemaSource(t, false)
	ctx := context.Background()
	a := x.f.approve(t, x.f.request(t))
	require.NoError(t, x.f.s.LinkChangeEventAuthorization(ctx, x.f.device, x.f.link(a, x.event)))
	rules := slices.Clone(completePolicyRules)
	if violation {
		rules[0].Decision = "deny"
	}
	x.f.activatePolicy(t, x.f.policy(t, rules), x.f.now.Add(time.Hour))
	_, err := x.f.s.PersistPolicyEvaluation(ctx, x.f.device, x.event.ID, x.f.approver, x.f.now.Add(4*time.Hour))
	require.NoError(t, err)
	if revoked {
		_, err = x.f.s.TransitionChangeAuthorization(ctx, x.f.device, a.ID, x.f.approver, "revoked", x.f.now.Add(5*time.Hour))
		require.NoError(t, err)
	}
	return riskAdversarialEvidence(t, x.f, x.event)
}
func TestRiskSchemaGovernanceAndVersions(t *testing.T) {
	for _, scenario := range []struct {
		name               string
		revoked, violation bool
	}{{"approved PASS nullable optional facts", false, false}, {"approved VIOLATION", false, true}, {"revoked VIOLATION", true, true}} {
		t.Run(scenario.name, func(t *testing.T) {
			x := riskAdversarialGoverned(t, scenario.revoked, scenario.violation)
			riskAdversarialCommit(t, x, nil)
			if scenario.violation || scenario.revoked {
				require.Equal(t, "urgent", x.result.ReviewPriority)
			}
			t.Logf("real workflow captured status=%s policy=%s and sealed; optional rejected timestamps remain NULL", x.evidence.Authorization.Ticket.Status, x.evidence.Policy.RecordedStatus)
		})
	}
	for _, name := range []string{"missing required authorization field", "invalid approved timestamp NULL", "wrong link identity", "wrong authorization reference", "wrong policy reference", "missing policy field", "typed governance mismatch", "duplicate governance key", "unknown governance key", "result counter mismatch", "unsupported evidence JSON version", "revoked timestamp NULL"} {
		t.Run(name, func(t *testing.T) {
			x := riskAdversarialGoverned(t, name == "revoked timestamp NULL", true)
			override := map[string]any{}
			switch name {
			case "missing required authorization field":
				override["evidence_json"] = riskAdversarialJSON(t, x.f.s, x.evidenceJSON, `json_remove(?,'$.authorization.ticket.approved_at')`)
			case "invalid approved timestamp NULL":
				override["evidence_json"] = riskAdversarialJSON(t, x.f.s, x.evidenceJSON, `json_set(?,'$.authorization.ticket.approved_at',NULL)`)
				override["source_bindings_json"] = riskAdversarialJSON(t, x.f.s, x.bindingsJSON, `json_set(?,'$.governance.ticket.approved_at',NULL)`)
			case "wrong link identity":
				override["evidence_json"] = riskAdversarialJSON(t, x.f.s, x.evidenceJSON, `json_set(?,'$.authorization.link.authorization_id',?)`, uuid.NewString())
			case "wrong authorization reference":
				override["authorization_id"] = uuid.NewString()
			case "wrong policy reference":
				override["policy_evaluation_id"] = uuid.NewString()
			case "missing policy field":
				override["evidence_json"] = riskAdversarialJSON(t, x.f.s, x.evidenceJSON, `json_remove(?,'$.policy.final.evaluated_at')`)
			case "typed governance mismatch":
				override["recorded_authorization_status"] = "unaccounted"
				override["authorization_id"] = nil
			case "duplicate governance key":
				override["evidence_json"] = strings.Replace(x.evidenceJSON, `"recorded_status":"authorized",`, `"recorded_status":"authorized","recorded_status":"authorized",`, 1)
			case "unknown governance key":
				override["evidence_json"] = riskAdversarialJSON(t, x.f.s, x.evidenceJSON, `json_set(?,'$.authorization.ticket.attacker',1)`)
			case "result counter mismatch":
				override["result_json"] = riskAdversarialJSON(t, x.f.s, x.resultJSON, `json_set(?,'$.known_items',1)`)
			case "unsupported evidence JSON version":
				override["evidence_json"] = riskAdversarialJSON(t, x.f.s, x.evidenceJSON, `json_set(?,'$.schema_version',2)`)
			case "revoked timestamp NULL":
				override["evidence_json"] = riskAdversarialJSON(t, x.f.s, x.evidenceJSON, `json_set(?,'$.authorization.ticket.revoked_at',NULL)`)
				override["source_bindings_json"] = riskAdversarialJSON(t, x.f.s, x.bindingsJSON, `json_set(?,'$.governance.ticket.revoked_at',NULL)`)
			}
			riskAdversarialReject(t, x.f.s, func(tx *sql.Tx) (string, error) {
				if err := riskSchemaChildren(tx, x); err != nil {
					return "finding INSERT", err
				}
				return "header INSERT", riskAdversarialHeader(tx, x, override)
			})
		})
	}
	t.Run("absent v1 registry", func(t *testing.T) {
		f := authorizationStore(t)
		x := riskAdversarialEvidence(t, f, authorizationEvent(t, f.s, f.device))
		riskAdversarialReject(t, f.s, func(tx *sql.Tx) (string, error) {
			if err := riskSchemaChildren(tx, x); err != nil {
				return "finding INSERT", err
			}
			return "header INSERT", riskAdversarialHeader(tx, x, nil)
		})
	})
	t.Run("unsupported registry schema", func(t *testing.T) {
		x := riskSchemaSource(t, false)
		definition, _ := risk.CanonicalDefinitionV1()
		definition = riskAdversarialJSON(t, x.f.s, definition, `json_set(?,'$.model_version',2)`)
		riskAdversarialReject(t, x.f.s, func(tx *sql.Tx) (string, error) {
			_, err := tx.Exec(`INSERT INTO change_risk_definitions(model_version,schema_version,engine_version,materializer_version,definition_hash,definition_json,registered_at) VALUES(2,2,1,1,?,?,?)`, strings.Repeat("1", 64), definition, x.captured)
			return "unsupported registry INSERT", err
		})
	})
}

func TestRiskSchemaZeroItemLegacyEligibility(t *testing.T) {
	x := riskSchemaSource(t, true)
	riskAdversarialCommit(t, x, nil)
	before := riskAdversarialSnapshot(t, x.f.s)
	ctx := context.Background()
	a := x.f.approve(t, x.f.request(t))
	withTicket := riskAdversarialSnapshot(t, x.f.s)
	err := x.f.s.LinkChangeEventAuthorization(ctx, x.f.device, x.f.link(a, x.event))
	require.ErrorIs(t, err, domain.ErrValidation)
	require.Equal(t, withTicket, riskAdversarialSnapshot(t, x.f.s))
	t.Log("zero-item authorization: existing Task 4 ineligible, not a risk source-freeze conflict")
	x.f.activatePolicy(t, x.f.policy(t, completePolicyRules), x.f.now.Add(time.Hour))
	withPolicy := riskAdversarialSnapshot(t, x.f.s)
	_, err = x.f.s.PersistPolicyEvaluation(ctx, x.f.device, x.event.ID, x.f.approver, x.f.now.Add(4*time.Hour))
	require.Error(t, err)
	require.Equal(t, withPolicy, riskAdversarialSnapshot(t, x.f.s))
	t.Logf("zero-item final policy: existing workflow ineligible (%v)", err)
	for _, table := range []string{"change_events", "change_event_items", "change_risk_assessments", "change_risk_findings"} {
		require.Equal(t, before[table], riskAdversarialRows(t, x.f.s.db, table))
	}
	riskAdversarialReject(t, x.f.s, func(tx *sql.Tx) (string, error) {
		_, err := tx.Exec(`INSERT INTO change_event_items(id,change_event_id,ordinal,operation,command,created_at) VALUES(?,?,0,'add','description lab',?)`, uuid.NewString(), x.event.ID, x.captured)
		return "zero-item source INSERT", err
	})
}

// Append-only regressions for the approved TWO-predicate migration correction.
// The previously failing TestRiskSchemaRevisionChain above is unchanged.
func TestRiskSchemaNullableTicketCommittedHistory(t *testing.T) {
	x := riskSchemaSource(t, false)
	riskAdversarialCommit(t, x, nil)
	ctx := context.Background()
	assertPrefix := func(table string, before []string) {
		after := riskAdversarialRows(t, x.f.s.db, table)
		require.GreaterOrEqual(t, len(after), len(before))
		require.Equal(t, before, after[:len(before)], "all preceding historical %s rows remain byte-identical", table)
	}
	rootHeader := riskAdversarialRows(t, x.f.s.db, "change_risk_assessments")
	rootFindings := riskAdversarialRows(t, x.f.s.db, "change_risk_findings")
	a := x.f.approve(t, x.f.request(t))
	link := x.f.link(a, x.event)
	link.LinkedAt = x.f.now.Add(6*time.Hour + time.Minute)
	require.NoError(t, x.f.s.LinkChangeEventAuthorization(ctx, x.f.device, link))
	b := riskAdversarialEvidence(t, x.f, x.event)
	b.captured = link.LinkedAt.Add(time.Minute).Format(risk.UTC9)
	require.Nil(t, b.evidence.Authorization.Ticket.RejectedAt)
	require.Nil(t, b.evidence.Authorization.Ticket.RevokedAt)
	require.Nil(t, b.bindings.Governance.Ticket.RejectedAt)
	require.Nil(t, b.bindings.Governance.Ticket.RevokedAt)
	riskAdversarialCommit(t, b, map[string]any{"revision": 2, "previous_assessment_id": x.id})
	assertPrefix("change_risk_assessments", rootHeader)
	assertPrefix("change_risk_findings", rootFindings)
	t.Log("approved ticket with rejected_at=NULL/revoked_at=NULL: real R2 COMMIT succeeded; complete historical R1 header/findings byte-identical")
	beforeRevocationHeaders := riskAdversarialRows(t, x.f.s.db, "change_risk_assessments")
	beforeRevocationFindings := riskAdversarialRows(t, x.f.s.db, "change_risk_findings")
	revokedAt := link.LinkedAt.Add(2 * time.Minute)
	_, err := x.f.s.TransitionChangeAuthorization(ctx, x.f.device, a.ID, x.f.approver, "revoked", revokedAt)
	require.NoError(t, err)
	c := riskAdversarialEvidence(t, x.f, x.event)
	c.captured = revokedAt.Add(time.Minute).Format(risk.UTC9)
	require.Equal(t, "revoked", c.evidence.Authorization.Ticket.Status)
	require.Nil(t, c.evidence.Authorization.Ticket.RejectedAt)
	require.NotNil(t, c.evidence.Authorization.Ticket.RevokedAt)
	require.Equal(t, revokedAt.Format(risk.UTC9), *c.evidence.Authorization.Ticket.RevokedAt)
	require.Equal(t, "urgent", c.result.ReviewPriority)
	riskAdversarialCommit(t, c, map[string]any{"revision": 3, "previous_assessment_id": b.id})
	assertPrefix("change_risk_assessments", beforeRevocationHeaders)
	assertPrefix("change_risk_findings", beforeRevocationFindings)
	t.Log("legitimately revoked ticket with rejected_at=NULL/non-NULL past revoked_at: R3 COMMIT succeeded; historical R1/R2 header/findings byte-identical (not A->B->A recurrence)")
}

func TestRiskSchemaNullableTicketNegatives(t *testing.T) {
	for _, name := range []string{"future rejected evidence", "future actual revoked source", "NULL source forged nonNULL evidence", "nonNULL source forged NULL evidence", "missing optional JSON key", "wrong exact raw sidecar", "canonical revoked plus 1ns", "forged ticket status", "forged approved actor", "forged ticket identity"} {
		t.Run(name, func(t *testing.T) {
			revoked := name == "future actual revoked source" || name == "nonNULL source forged NULL evidence" || name == "wrong exact raw sidecar" || name == "canonical revoked plus 1ns"
			x := riskAdversarialGoverned(t, revoked, false)
			// A real historical witness survives every failed target assessment.
			witness := riskAdversarialEvidence(t, x.f, authorizationEvent(t, x.f.s, x.f.otherDevice))
			riskAdversarialCommit(t, witness, nil)
			override := map[string]any{}
			switch name {
			case "future rejected evidence":
				// An approved/revoked source cannot legitimately be rejected too.
				// Do not corrupt the source or disable CHECKs to force that state.
				require.Nil(t, x.evidence.Authorization.Ticket.RejectedAt)
				future := x.f.now.Add(7 * time.Hour).Format(risk.UTC9)
				override["evidence_json"] = riskAdversarialJSON(t, x.f.s, x.evidenceJSON, `json_set(?,'$.authorization.ticket.rejected_at',?)`, future)
				override["source_bindings_json"] = riskAdversarialJSON(t, x.f.s, x.bindingsJSON, `json_set(?,'$.governance.ticket.rejected_at',?)`, future)
			case "future actual revoked source":
				require.NotNil(t, x.evidence.Authorization.Ticket.RevokedAt)
				at, err := time.Parse(risk.UTC9, *x.evidence.Authorization.Ticket.RevokedAt)
				require.NoError(t, err)
				x.captured = at.Add(-time.Nanosecond).Format(risk.UTC9)
				t.Log("actual source revoked_at is 1ns after the attempted capture; evidence and raw bindings still match the real source")
			case "NULL source forged nonNULL evidence":
				require.Nil(t, x.evidence.Authorization.Ticket.RevokedAt)
				forged := x.f.now.Add(5 * time.Hour).Format(risk.UTC9)
				override["evidence_json"] = riskAdversarialJSON(t, x.f.s, x.evidenceJSON, `json_set(?,'$.authorization.ticket.revoked_at',?)`, forged)
				override["source_bindings_json"] = riskAdversarialJSON(t, x.f.s, x.bindingsJSON, `json_set(?,'$.governance.ticket.revoked_at',?)`, forged)
			case "nonNULL source forged NULL evidence":
				require.NotNil(t, x.evidence.Authorization.Ticket.RevokedAt)
				override["evidence_json"] = riskAdversarialJSON(t, x.f.s, x.evidenceJSON, `json_set(?,'$.authorization.ticket.revoked_at',NULL)`)
				override["source_bindings_json"] = riskAdversarialJSON(t, x.f.s, x.bindingsJSON, `json_set(?,'$.governance.ticket.revoked_at',NULL)`)
			case "missing optional JSON key":
				override["evidence_json"] = riskAdversarialJSON(t, x.f.s, x.evidenceJSON, `json_remove(?,'$.authorization.ticket.revoked_at')`)
			case "wrong exact raw sidecar":
				at, err := time.Parse(risk.UTC9, *x.evidence.Authorization.Ticket.RevokedAt)
				require.NoError(t, err)
				equivalent := at.In(time.FixedZone("fixture-offset", 3*60*60+30*60)).Format(time.RFC3339Nano)
				require.NotEqual(t, x.bindings.Governance.Ticket.RevokedAt, &equivalent)
				override["source_bindings_json"] = riskAdversarialJSON(t, x.f.s, x.bindingsJSON, `json_set(?,'$.governance.ticket.revoked_at',?)`, equivalent)
			case "canonical revoked plus 1ns":
				at, err := time.Parse(risk.UTC9, *x.evidence.Authorization.Ticket.RevokedAt)
				require.NoError(t, err)
				override["evidence_json"] = riskAdversarialJSON(t, x.f.s, x.evidenceJSON, `json_set(?,'$.authorization.ticket.revoked_at',?)`, at.Add(time.Nanosecond).Format(risk.UTC9))
			case "forged ticket status":
				override["evidence_json"] = riskAdversarialJSON(t, x.f.s, x.evidenceJSON, `json_set(?,'$.authorization.ticket.status','pending')`)
			case "forged approved actor":
				override["evidence_json"] = riskAdversarialJSON(t, x.f.s, x.evidenceJSON, `json_set(?,'$.authorization.ticket.approved_by',?)`, uuid.NewString())
			case "forged ticket identity":
				override["evidence_json"] = riskAdversarialJSON(t, x.f.s, x.evidenceJSON, `json_set(?,'$.authorization.ticket.id',?)`, uuid.NewString())
			}
			riskAdversarialReject(t, x.f.s, func(tx *sql.Tx) (string, error) {
				if err := riskSchemaChildren(tx, x); err != nil {
					return "finding INSERT", err
				}
				return "header INSERT", riskAdversarialHeader(tx, x, override)
			})
		})
	}
}
