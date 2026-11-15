package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"time"

	"company.local/aegis/apps/aegis-api/internal/changes"
	"company.local/aegis/apps/aegis-api/internal/domain"
	"company.local/aegis/apps/aegis-api/internal/repositories"
	"company.local/aegis/apps/aegis-api/internal/risk"
	"github.com/google/uuid"
)

type riskMaterial struct {
	evidence                                                         domain.RiskEvidenceSnapshot
	bindings                                                         domain.RiskSourceBindings
	result                                                           domain.RiskEvaluationResult
	evidenceJSON, bindingsJSON, resultJSON, evidenceHash, resultHash string
}

func riskID(id string) bool {
	u, err := uuid.Parse(id)
	return err == nil && u != uuid.Nil && u.String() == id
}
func riskString(s string) *string { return &s }
func riskNullable(v sql.NullString) *string {
	if !v.Valid {
		return nil
	}
	return riskString(v.String)
}
func riskJSON(v any) string               { b, _ := json.Marshal(v); return string(b) }
func riskSame(a, b any) bool              { return reflect.DeepEqual(a, b) }
func riskTime(raw string) (string, error) { return risk.NormalizeTimestampV1(raw) }
func riskOptionalTime(raw *string) (*string, error) {
	if raw == nil {
		return nil, nil
	}
	v, err := riskTime(*raw)
	return riskString(v), err
}

func riskAuthorize(ctx context.Context, tx *sql.Tx, scope repositories.RiskScope, evaluate bool) error {
	if !riskID(scope.ActorID) {
		return domain.ErrUnauthorized
	}
	if !riskID(scope.DeviceID) || !riskID(scope.EventID) {
		return domain.ErrValidation
	}
	permissions := []string{"backups.read", domain.PermissionAuthorizationRead, domain.PermissionPolicyRead, domain.PermissionRiskRead}
	if evaluate {
		permissions = append(permissions, domain.PermissionRiskEvaluate)
	}
	if err := authorizationActor(ctx, tx, scope.DeviceID, scope.ActorID, permissions...); err != nil {
		return err
	}
	var id string
	return repositoryError(tx.QueryRowContext(ctx, `SELECT id FROM change_events WHERE id=? AND device_id=?`, scope.EventID, scope.DeviceID).Scan(&id))
}

// Source commands and contexts exist only in this function's transient locals.
// Admission is measured in SQLite BLOB bytes BEFORE reading those values.
func riskTechnical(ctx context.Context, tx *sql.Tx, scope repositories.RiskScope) (domain.RiskEvidenceSnapshot, domain.RiskSourceBindings, error) {
	e := domain.RiskEvidenceSnapshot{SchemaVersion: 1, Items: []domain.RiskItemSignal{}, Authorization: domain.RiskAuthorizationObservation{History: []domain.RiskAuthorizationHistory{}}, Eligibility: domain.RiskEligibility{Persistable: true, Reasons: []string{}}}
	b := domain.RiskSourceBindings{Items: []domain.RiskItemTimeBinding{}, Governance: domain.RiskGovernanceTimeBindings{History: []domain.RiskHistoryTimeBinding{}}}
	v := &e.Event
	var analyzed sql.NullString
	err := tx.QueryRowContext(ctx, `SELECT id,device_id,config_type,from_backup_id,to_backup_id,detected_at,created_at,analysis_version,item_count,added_lines,removed_lines,analyzed_at,authorization_status,policy_status FROM change_events WHERE id=? AND device_id=?`, scope.EventID, scope.DeviceID).Scan(&v.EventID, &v.DeviceID, &v.ConfigType, &v.FromBackupID, &v.ToBackupID, &b.Event.DetectedAt, &b.Event.CreatedAt, &v.AnalysisVersion, &v.ItemCount, &v.AddedLines, &v.RemovedLines, &analyzed, &e.Authorization.RecordedStatus, &e.Policy.RecordedStatus)
	if err != nil {
		return e, b, repositoryError(err)
	}
	b.Event.EventID = v.EventID
	b.Event.AnalyzedAt = analyzed.String
	for _, id := range []string{v.EventID, v.DeviceID, v.FromBackupID, v.ToBackupID} {
		if !riskID(id) {
			return e, b, risk.ErrIntegrity
		}
	}
	if v.FromBackupID == v.ToBackupID || (v.ConfigType != "running" && v.ConfigType != "startup") || v.ItemCount < 0 || v.AddedLines < 0 || v.RemovedLines < 0 {
		return e, b, risk.ErrIntegrity
	}
	if v.DetectedAt, err = riskTime(b.Event.DetectedAt); err != nil {
		return e, b, err
	}
	if v.CreatedAt, err = riskTime(b.Event.CreatedAt); err != nil {
		return e, b, err
	}
	if v.AnalyzedAt, err = riskOptionalTime(riskNullable(analyzed)); err != nil {
		return e, b, err
	}
	if v.AnalysisVersion == 1 && v.AnalyzedAt == nil {
		return e, b, risk.ErrIntegrity
	}
	if v.AnalysisVersion < 0 || (v.AnalysisVersion == 0 && (analyzed.Valid || v.ItemCount != 0)) {
		return e, b, risk.ErrIntegrity
	}
	if v.AnalysisVersion != 1 {
		e.Eligibility = domain.RiskEligibility{Reasons: []string{"unsupported_analysis_version"}}
	}
	var previous string
	for _, id := range []string{v.FromBackupID, v.ToBackupID} {
		var raw string
		err = tx.QueryRowContext(ctx, `SELECT created_at FROM config_backups WHERE id=? AND device_id=? AND config_type=?`, id, v.DeviceID, v.ConfigType).Scan(&raw)
		if errors.Is(err, sql.ErrNoRows) {
			return e, b, risk.ErrIntegrity
		}
		if err != nil {
			return e, b, err
		}
		instant, err := riskTime(raw)
		if err != nil {
			return e, b, err
		}
		if previous != "" && (instant < previous || instant > v.DetectedAt) {
			return e, b, risk.ErrIntegrity
		}
		previous = instant
	}
	var count, bytes int64
	err = tx.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(SUM(length(CAST(context_path_json AS BLOB))+length(CAST(command AS BLOB))+length(CAST(id AS BLOB))+length(CAST(category AS BLOB))+length(CAST(subcategory AS BLOB))+length(CAST(classification_rule AS BLOB))+length(CAST(created_at AS BLOB))+COALESCE(length(CAST(classified_at AS BLOB)),0)),0) FROM change_event_items WHERE change_event_id=?`, scope.EventID).Scan(&count, &bytes)
	if err != nil {
		return e, b, err
	}
	if count != int64(v.ItemCount) {
		return e, b, risk.ErrIntegrity
	}
	if count > domain.RiskMaxItems || bytes > domain.RiskMaxSourceBytes {
		return e, b, risk.ErrLimit
	}
	rows, err := tx.QueryContext(ctx, `SELECT `+changeEventItemColumns+` FROM change_event_items WHERE change_event_id=? ORDER BY ordinal`, scope.EventID)
	if err != nil {
		return e, b, err
	}
	defer rows.Close()
	seen := map[string]bool{}
	for rows.Next() {
		var item domain.RiskItemSignal
		var binding domain.RiskItemTimeBinding
		var eventID, pathJSON, command string
		var classified sql.NullString
		var sensitive int
		if err = rows.Scan(&item.ItemID, &eventID, &item.Ordinal, &item.Operation, &pathJSON, &command, &sensitive, &binding.CreatedAt, &item.ClassificationVersion, &item.Category, &item.Subcategory, &item.ClassificationRule, &classified); err != nil {
			return e, b, err
		}
		if !riskID(item.ItemID) || seen[item.ItemID] || eventID != v.EventID || item.Ordinal != len(e.Items) || (item.Operation != "add" && item.Operation != "remove") || sensitive < 0 || sensitive > 1 || strings.TrimSpace(command) == "" {
			return e, b, risk.ErrIntegrity
		}
		seen[item.ItemID] = true
		item.Sensitive = sensitive == 1
		binding.ItemID = item.ItemID
		binding.Ordinal = item.Ordinal
		binding.ClassifiedAt = classified.String
		if item.CreatedAt, err = riskTime(binding.CreatedAt); err != nil {
			return e, b, err
		}
		if item.ClassifiedAt, err = riskOptionalTime(riskNullable(classified)); err != nil {
			return e, b, err
		}
		var path []string
		if err = json.Unmarshal([]byte(pathJSON), &path); err != nil || path == nil || len(path) > 10000 {
			return e, b, risk.ErrIntegrity
		}
		sanitized, safePath, isSensitive := changes.Sanitize(command, path)
		if command != sanitized || !slices.Equal(path, safePath) || (isSensitive && !item.Sensitive) {
			return e, b, risk.ErrIntegrity
		}
		if item.ClassificationVersion == 1 {
			if item.ClassifiedAt == nil {
				return e, b, risk.ErrIntegrity
			}
			reproduced := changes.Classify(path, command)
			if reproduced.Category != item.Category || reproduced.Subcategory != item.Subcategory || reproduced.RuleID != item.ClassificationRule {
				return e, b, risk.ErrIntegrity
			}
		} else {
			if item.ClassificationVersion < 0 || (item.ClassificationVersion == 0 && (classified.Valid || item.Category != "unclassified" || item.Subcategory != "unclassified" || item.ClassificationRule != "")) {
				return e, b, risk.ErrIntegrity
			}
			e.Eligibility.Persistable = false
			if !slices.Contains(e.Eligibility.Reasons, "unsupported_classification_version") {
				e.Eligibility.Reasons = append(e.Eligibility.Reasons, "unsupported_classification_version")
			}
		}
		e.Items = append(e.Items, item)
		b.Items = append(b.Items, binding)
	}
	if err = rows.Err(); err != nil {
		return e, b, err
	}
	if len(e.Items) != v.ItemCount {
		return e, b, risk.ErrIntegrity
	}
	return e, b, nil
}

func riskGovernance(ctx context.Context, tx *sql.Tx, e *domain.RiskEvidenceSnapshot, b *domain.RiskSourceBindings) error {
	var l domain.RiskAuthorizationLink
	var lb domain.RiskLinkTimeBinding
	err := tx.QueryRowContext(ctx, `SELECT event_id,authorization_id,linked_by,linked_at,attestation FROM change_event_authorizations WHERE event_id=?`, e.Event.EventID).Scan(&l.EventID, &l.AuthorizationID, &l.LinkedBy, &lb.LinkedAt, &l.Attestation)
	if errors.Is(err, sql.ErrNoRows) {
		if e.Authorization.RecordedStatus != "unaccounted" {
			return risk.ErrIntegrity
		}
	} else {
		if err != nil {
			return err
		}
		if e.Authorization.RecordedStatus != "authorized" {
			return risk.ErrIntegrity
		}
		if l.LinkedAt, err = riskTime(lb.LinkedAt); err != nil {
			return err
		}
		lb.EventID = l.EventID
		lb.AuthorizationID = l.AuthorizationID
		var t domain.RiskAuthorizationTicket
		var tb domain.RiskTicketTimeBinding
		var categories string
		var approvedBy, approvedAt, rejectedBy, rejectedAt, revokedBy, revokedAt sql.NullString
		var size int
		if err = tx.QueryRowContext(ctx, `SELECT length(CAST(allowed_categories_json AS BLOB)) FROM change_authorizations WHERE id=?`, l.AuthorizationID).Scan(&size); err != nil {
			return risk.ErrIntegrity
		}
		if size > 4096 {
			return risk.ErrLimit
		}
		err = tx.QueryRowContext(ctx, `SELECT id,device_id,config_type,status,allowed_categories_json,window_start,window_end,requested_by,requested_at,approved_by,approved_at,rejected_by,rejected_at,revoked_by,revoked_at,created_at FROM change_authorizations WHERE id=? AND device_id=? AND config_type=?`, l.AuthorizationID, e.Event.DeviceID, e.Event.ConfigType).Scan(&t.ID, &t.DeviceID, &t.ConfigType, &t.Status, &categories, &tb.WindowStart, &tb.WindowEnd, &t.RequestedBy, &tb.RequestedAt, &approvedBy, &approvedAt, &rejectedBy, &rejectedAt, &revokedBy, &revokedAt, &tb.CreatedAt)
		if err != nil {
			return risk.ErrIntegrity
		}
		if !approvedBy.Valid || !approvedAt.Valid {
			return risk.ErrIntegrity
		}
		if err = json.Unmarshal([]byte(categories), &t.AllowedCategories); err != nil || t.AllowedCategories == nil {
			return risk.ErrIntegrity
		}
		t.ApprovedBy = approvedBy.String
		t.RejectedBy = riskNullable(rejectedBy)
		t.RevokedBy = riskNullable(revokedBy)
		tb.AuthorizationID = t.ID
		tb.ApprovedAt = approvedAt.String
		tb.RejectedAt = riskNullable(rejectedAt)
		tb.RevokedAt = riskNullable(revokedAt)
		for _, pair := range []struct {
			raw string
			to  *string
		}{{tb.WindowStart, &t.WindowStart}, {tb.WindowEnd, &t.WindowEnd}, {tb.RequestedAt, &t.RequestedAt}, {tb.ApprovedAt, &t.ApprovedAt}, {tb.CreatedAt, &t.CreatedAt}} {
			if *pair.to, err = riskTime(pair.raw); err != nil {
				return err
			}
		}
		if t.RejectedAt, err = riskOptionalTime(tb.RejectedAt); err != nil {
			return err
		}
		if t.RevokedAt, err = riskOptionalTime(tb.RevokedAt); err != nil {
			return err
		}
		e.Authorization.Link = &l
		e.Authorization.Ticket = &t
		b.Governance.Link = &lb
		b.Governance.Ticket = &tb
		var count int
		if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM change_authorization_history WHERE authorization_id=?`, t.ID).Scan(&count); err != nil {
			return err
		}
		if count > 4 {
			return risk.ErrIntegrity
		}
		rows, err := tx.QueryContext(ctx, `SELECT id,event_id,action,actor_user_id,occurred_at FROM change_authorization_history WHERE authorization_id=? ORDER BY aegis_risk_utc_nano_v1(occurred_at),CASE action WHEN 'requested' THEN 0 WHEN 'approved' THEN 1 WHEN 'linked' THEN 2 WHEN 'revoked' THEN 3 ELSE 4 END,id`, t.ID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var h domain.RiskAuthorizationHistory
			var hb domain.RiskHistoryTimeBinding
			var event sql.NullString
			if err = rows.Scan(&h.ID, &event, &h.Action, &h.ActorID, &hb.OccurredAt); err != nil {
				rows.Close()
				return err
			}
			h.EventID = riskNullable(event)
			hb.ID = h.ID
			hb.Action = h.Action
			hb.EventID = h.EventID
			if h.OccurredAt, err = riskTime(hb.OccurredAt); err != nil {
				rows.Close()
				return err
			}
			if h.EventID != nil && *h.EventID != e.Event.EventID {
				rows.Close()
				return risk.ErrIntegrity
			}
			e.Authorization.History = append(e.Authorization.History, h)
			b.Governance.History = append(b.Governance.History, hb)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		if len(e.Authorization.History) != count {
			return risk.ErrIntegrity
		}
	}
	var p domain.RiskPolicyFinal
	var rawDefinition, rawAt string
	var size int
	err = tx.QueryRowContext(ctx, `SELECT length(CAST(policy_definition_snapshot AS BLOB)) FROM change_policy_evaluations WHERE event_id=?`, e.Event.EventID).Scan(&size)
	if errors.Is(err, sql.ErrNoRows) {
		if e.Policy.RecordedStatus != "not_evaluated" {
			return risk.ErrIntegrity
		}
		return nil
	}
	if err != nil {
		return err
	}
	if size > domain.RiskMaxCanonicalBytes {
		return risk.ErrLimit
	}
	err = tx.QueryRowContext(ctx, `SELECT `+evaluationColumns+` FROM change_policy_evaluations WHERE event_id=?`, e.Event.EventID).Scan(&p.ID, &p.EventID, &p.PolicySetID, &p.EngineVersion, &p.DefinitionHash, &rawDefinition, &p.Result, &p.TotalItems, &p.AllowedCount, &p.DeniedCount, &p.UncoveredCount, &p.EvaluatedBy, &rawAt)
	if err != nil {
		return err
	}
	if e.Policy.RecordedStatus != p.Result {
		return risk.ErrIntegrity
	}
	if err = risk.StrictDecode(rawDefinition, &p.Definition); err != nil {
		return err
	}
	if p.EvaluatedAt, err = riskTime(rawAt); err != nil {
		return err
	}
	var device, config, hash, rules string
	var engine int
	err = tx.QueryRowContext(ctx, `SELECT device_id,config_type,engine_version,definition_hash,rules_json FROM change_policy_sets WHERE id=? AND length(CAST(rules_json AS BLOB))<=?`, p.PolicySetID, domain.RiskMaxCanonicalBytes).Scan(&device, &config, &engine, &hash, &rules)
	if err != nil || device != e.Event.DeviceID || config != e.Event.ConfigType || engine != p.EngineVersion || hash != p.DefinitionHash {
		return risk.ErrIntegrity
	}
	var archivedRules []domain.PolicyRule
	if err = json.Unmarshal([]byte(rules), &archivedRules); err != nil || !riskSame(archivedRules, p.Definition.Rules) {
		return risk.ErrIntegrity
	}
	p.Items = []domain.PolicyItemResult{}
	var count int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM change_policy_evaluation_items WHERE evaluation_id=?`, p.ID).Scan(&count); err != nil {
		return err
	}
	if count != len(e.Items) {
		return risk.ErrIntegrity
	}
	rows, err := tx.QueryContext(ctx, `SELECT event_id,change_event_item_id,ordinal,category,operation,decision,matched_rule_key FROM change_policy_evaluation_items WHERE evaluation_id=? ORDER BY ordinal`, p.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var i domain.PolicyItemResult
		var event string
		var key sql.NullString
		if err = rows.Scan(&event, &i.ItemID, &i.Ordinal, &i.Category, &i.Operation, &i.Decision, &key); err != nil {
			rows.Close()
			return err
		}
		if !key.Valid {
			rows.Close()
			return risk.ErrIntegrity
		}
		i.MatchedRuleKey = key.String
		if event != e.Event.EventID || i.Ordinal != len(p.Items) {
			rows.Close()
			return risk.ErrIntegrity
		}
		p.Items = append(p.Items, i)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(p.Items) != count {
		return risk.ErrIntegrity
	}
	e.Policy.Final = &p
	b.Governance.Policy = &domain.RiskPolicyTimeBinding{EvaluationID: p.ID, EvaluatedAt: rawAt}
	return nil
}

// Replay checks captured governance against its own copied facts, never a live
// ticket status or active policy. Raw timestamp tokens must normalize exactly.
func riskBindings(e domain.RiskEvidenceSnapshot, b domain.RiskSourceBindings, captured string) error {
	if b.Items == nil || b.Governance.History == nil {
		return risk.ErrIntegrity
	}
	check := func(raw, canonical string) bool { v, err := riskTime(raw); return err == nil && v == canonical }
	valid := func(v string) bool { return check(v, v) && v <= captured }
	if !check(captured, captured) || !valid(e.Event.DetectedAt) || !valid(e.Event.CreatedAt) || e.Event.AnalyzedAt == nil || !valid(*e.Event.AnalyzedAt) || *e.Event.AnalyzedAt < e.Event.DetectedAt || e.Event.CreatedAt < e.Event.DetectedAt {
		return risk.ErrIntegrity
	}
	if b.Event.EventID != e.Event.EventID || !check(b.Event.DetectedAt, e.Event.DetectedAt) || !check(b.Event.CreatedAt, e.Event.CreatedAt) || !check(b.Event.AnalyzedAt, *e.Event.AnalyzedAt) || len(b.Items) != len(e.Items) {
		return risk.ErrIntegrity
	}
	for n, i := range e.Items {
		ib := b.Items[n]
		if i.ClassifiedAt == nil || !valid(i.CreatedAt) || !valid(*i.ClassifiedAt) || i.CreatedAt < e.Event.DetectedAt || *i.ClassifiedAt < i.CreatedAt || ib.ItemID != i.ItemID || ib.Ordinal != i.Ordinal || !check(ib.CreatedAt, i.CreatedAt) || !check(ib.ClassifiedAt, *i.ClassifiedAt) {
			return risk.ErrIntegrity
		}
	}
	a, g := e.Authorization, b.Governance
	if a.Ticket == nil {
		if g.Ticket != nil || g.Link != nil || len(g.History) != 0 {
			return risk.ErrIntegrity
		}
	} else {
		t, tb, l, lb := a.Ticket, g.Ticket, a.Link, g.Link
		if tb == nil || l == nil || lb == nil || tb.AuthorizationID != t.ID || lb.EventID != l.EventID || lb.AuthorizationID != l.AuthorizationID || !check(lb.LinkedAt, l.LinkedAt) || !valid(l.LinkedAt) || l.LinkedAt < e.Event.DetectedAt || t.ApprovedAt > e.Event.DetectedAt || t.CreatedAt > t.RequestedAt {
			return risk.ErrIntegrity
		}
		start, err := time.Parse(risk.UTC9, t.WindowStart)
		if err != nil {
			return risk.ErrIntegrity
		}
		end, err := time.Parse(risk.UTC9, t.WindowEnd)
		if err != nil || end.Sub(start) > 24*time.Hour {
			return risk.ErrIntegrity
		}
		for _, p := range []struct{ raw, at string }{{tb.WindowStart, t.WindowStart}, {tb.WindowEnd, t.WindowEnd}, {tb.RequestedAt, t.RequestedAt}, {tb.ApprovedAt, t.ApprovedAt}, {tb.CreatedAt, t.CreatedAt}} {
			if !check(p.raw, p.at) {
				return risk.ErrIntegrity
			}
		}
		for _, at := range []string{t.RequestedAt, t.ApprovedAt, t.CreatedAt} {
			if !valid(at) {
				return risk.ErrIntegrity
			}
		}
		for _, p := range []struct{ raw, at *string }{{tb.RejectedAt, t.RejectedAt}, {tb.RevokedAt, t.RevokedAt}} {
			if (p.raw == nil) != (p.at == nil) {
				return risk.ErrIntegrity
			}
			if p.raw != nil && (!check(*p.raw, *p.at) || !valid(*p.at)) {
				return risk.ErrIntegrity
			}
		}
		if len(g.History) != len(a.History) {
			return risk.ErrIntegrity
		}
		for n, h := range a.History {
			hb := g.History[n]
			if hb.ID != h.ID || hb.Action != h.Action || !riskSame(hb.EventID, h.EventID) || !check(hb.OccurredAt, h.OccurredAt) || !valid(h.OccurredAt) {
				return risk.ErrIntegrity
			}
		}
	}
	if e.Policy.Final == nil {
		if g.Policy != nil {
			return risk.ErrIntegrity
		}
	} else {
		p, pb := e.Policy.Final, g.Policy
		if pb == nil || pb.EvaluationID != p.ID || !check(pb.EvaluatedAt, p.EvaluatedAt) || !valid(p.EvaluatedAt) || p.EvaluatedAt < e.Event.DetectedAt {
			return risk.ErrIntegrity
		}
	}
	return nil
}

func riskMaterialize(ctx context.Context, tx *sql.Tx, scope repositories.RiskScope, captured string) (riskMaterial, error) {
	var m riskMaterial
	e, b, err := riskTechnical(ctx, tx, scope)
	if err != nil {
		return m, err
	}
	m.evidence = e
	m.bindings = b
	if !e.Eligibility.Persistable {
		return m, risk.ErrIneligible
	}
	if err = riskGovernance(ctx, tx, &e, &b); err != nil {
		return m, err
	}
	if err = riskBindings(e, b, captured); err != nil {
		return m, err
	}
	m.evidence, m.evidenceJSON, m.evidenceHash, err = risk.CanonicalEvidence(e)
	if err != nil {
		return m, err
	}
	m.bindings = b
	m.bindingsJSON = riskJSON(b)
	if m.result, err = risk.Evaluate(m.evidence); err != nil {
		return m, err
	}
	_, definitionHash := risk.CanonicalDefinitionV1()
	m.resultJSON, m.resultHash, err = risk.CanonicalResult(m.result, definitionHash, m.evidenceHash)
	if err != nil {
		return m, err
	}
	if len(m.evidenceJSON)+len(m.bindingsJSON)+len(m.resultJSON) > domain.RiskMaxCanonicalBytes {
		return m, risk.ErrLimit
	}
	return m, nil
}

func riskIneligiblePreview(e domain.RiskEvidenceSnapshot) repositories.RiskPreview {
	result := domain.RiskEvaluationResult{TechnicalImpact: "unknown", EvidenceCompleteness: "partial", ReviewPriority: "elevated", TotalItems: len(e.Items), Findings: []domain.RiskFinding{}}
	rank := map[string]int{"low": 1, "medium": 2, "high": 3}
	categories := map[string]bool{}
	for _, i := range e.Items {
		known := false
		if i.ClassificationVersion == 1 {
			for _, tuple := range risk.DefinitionV1().TechnicalRules {
				if i.Category == tuple.Category && i.Subcategory == tuple.Subcategory && i.ClassificationRule == tuple.ClassificationRule && tuple.Impact != "unknown" {
					known = true
					result.KnownItems++
					categories[i.Category] = true
					if result.KnownImpactFloor == nil || rank[tuple.Impact] > rank[*result.KnownImpactFloor] {
						result.KnownImpactFloor = riskString(tuple.Impact)
					}
					break
				}
			}
		}
		if !known {
			result.UnknownItems++
		}
	}
	result.DistinctKnownCategories = len(categories)
	return repositories.RiskPreview{ModelVersion: 1, Eligibility: e.Eligibility, Result: result}
}
