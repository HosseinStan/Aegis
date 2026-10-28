package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"company.local/aegis/apps/aegis-api/internal/domain"
	"company.local/aegis/apps/aegis-api/internal/repositories"
	"company.local/aegis/apps/aegis-api/internal/risk"
)

const riskHeaderColumns = `id,event_id,device_id,config_type,revision,previous_assessment_id,model_version,definition_hash,evidence_schema_version,evidence_hash,evidence_json,source_bindings_json,result_hash,result_json,technical_impact,known_impact_floor,evidence_completeness,review_priority,priority_rank,total_items,known_items,unknown_items,distinct_known_categories,finding_count,recorded_authorization_status,authorization_id,recorded_policy_status,policy_evaluation_id,captured_at,assessed_by`

type riskArchived struct {
	assessment domain.ChangeRiskAssessment
	material   riskMaterial
}

func riskRegistry(ctx context.Context, tx *sql.Tx, register bool, captured string) error {
	compiled, hash := risk.CanonicalDefinitionV1()
	var model, schema, engine, materializer, size int
	var storedHash, raw, at string
	err := tx.QueryRowContext(ctx, `SELECT model_version,schema_version,engine_version,materializer_version,definition_hash,length(CAST(definition_json AS BLOB)),registered_at FROM change_risk_definitions WHERE model_version=1`).Scan(&model, &schema, &engine, &materializer, &storedHash, &size, &at)
	if errors.Is(err, sql.ErrNoRows) && register {
		_, err = tx.ExecContext(ctx, `INSERT INTO change_risk_definitions(model_version,schema_version,engine_version,materializer_version,definition_hash,definition_json,registered_at) VALUES(1,1,1,1,?,?,?)`, hash, compiled, captured)
		return err
	}
	if err != nil {
		return risk.ErrIntegrity
	}
	if model != 1 || schema != 1 || engine != 1 || materializer != 1 || storedHash != hash || size != len(compiled) {
		return risk.ErrIntegrity
	}
	normalized, err := riskTime(at)
	if err != nil || normalized != at {
		return risk.ErrIntegrity
	}
	if err = tx.QueryRowContext(ctx, `SELECT definition_json FROM change_risk_definitions WHERE model_version=1`).Scan(&raw); err != nil {
		return err
	}
	if raw != compiled {
		return risk.ErrIntegrity
	}
	return nil
}

// Check only immutable captured governance facts. A later revocation or a new
// policy final is not retroactively an input to an older assessment.
func riskHistoricalGovernance(ctx context.Context, tx *sql.Tx, e domain.RiskEvidenceSnapshot, b domain.RiskSourceBindings) error {
	if t := e.Authorization.Ticket; t != nil {
		var id, device, config, categories, requestedBy, approvedBy, created, requested, approved, start, end string
		err := tx.QueryRowContext(ctx, `SELECT id,device_id,config_type,allowed_categories_json,requested_by,approved_by,created_at,requested_at,approved_at,window_start,window_end FROM change_authorizations WHERE id=? AND device_id=? AND config_type=?`, t.ID, e.Event.DeviceID, e.Event.ConfigType).Scan(&id, &device, &config, &categories, &requestedBy, &approvedBy, &created, &requested, &approved, &start, &end)
		tb := b.Governance.Ticket
		if err != nil || tb == nil || id != t.ID || device != t.DeviceID || config != t.ConfigType || categories != riskJSON(t.AllowedCategories) || requestedBy != t.RequestedBy || approvedBy != t.ApprovedBy || created != tb.CreatedAt || requested != tb.RequestedAt || approved != tb.ApprovedAt || start != tb.WindowStart || end != tb.WindowEnd {
			return risk.ErrIntegrity
		}
		var l domain.RiskAuthorizationLink
		var raw string
		err = tx.QueryRowContext(ctx, `SELECT event_id,authorization_id,linked_by,linked_at,attestation FROM change_event_authorizations WHERE event_id=? AND authorization_id=?`, e.Event.EventID, t.ID).Scan(&l.EventID, &l.AuthorizationID, &l.LinkedBy, &raw, &l.Attestation)
		if err != nil || b.Governance.Link == nil || raw != b.Governance.Link.LinkedAt {
			return risk.ErrIntegrity
		}
		l.LinkedAt, _ = riskTime(raw)
		if !riskSame(l, *e.Authorization.Link) {
			return risk.ErrIntegrity
		}
		if t.Status == "revoked" {
			var by, at string
			err = tx.QueryRowContext(ctx, `SELECT revoked_by,revoked_at FROM change_authorizations WHERE id=?`, t.ID).Scan(&by, &at)
			if err != nil || t.RevokedBy == nil || by != *t.RevokedBy || tb.RevokedAt == nil || at != *tb.RevokedAt {
				return risk.ErrIntegrity
			}
		}
		for n, h := range e.Authorization.History {
			var action, actor, at string
			var event sql.NullString
			err = tx.QueryRowContext(ctx, `SELECT event_id,action,actor_user_id,occurred_at FROM change_authorization_history WHERE id=? AND authorization_id=?`, h.ID, t.ID).Scan(&event, &action, &actor, &at)
			if err != nil || action != h.Action || actor != h.ActorID || !riskSame(riskNullable(event), h.EventID) || at != b.Governance.History[n].OccurredAt {
				return risk.ErrIntegrity
			}
		}
	}
	if p := e.Policy.Final; p != nil {
		var q domain.RiskPolicyFinal
		var raw, at string
		err := tx.QueryRowContext(ctx, `SELECT `+evaluationColumns+` FROM change_policy_evaluations WHERE id=? AND event_id=? AND length(CAST(policy_definition_snapshot AS BLOB))<=?`, p.ID, e.Event.EventID, domain.RiskMaxCanonicalBytes).Scan(&q.ID, &q.EventID, &q.PolicySetID, &q.EngineVersion, &q.DefinitionHash, &raw, &q.Result, &q.TotalItems, &q.AllowedCount, &q.DeniedCount, &q.UncoveredCount, &q.EvaluatedBy, &at)
		if err != nil || b.Governance.Policy == nil || at != b.Governance.Policy.EvaluatedAt || raw != riskJSON(p.Definition) {
			return risk.ErrIntegrity
		}
		q.Definition = p.Definition
		q.EvaluatedAt, _ = riskTime(at)
		q.Items = []domain.PolicyItemResult{}
		rows, err := tx.QueryContext(ctx, `SELECT change_event_item_id,ordinal,category,operation,decision,matched_rule_key FROM change_policy_evaluation_items WHERE evaluation_id=? AND event_id=? ORDER BY ordinal`, p.ID, p.EventID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var i domain.PolicyItemResult
			if err = rows.Scan(&i.ItemID, &i.Ordinal, &i.Category, &i.Operation, &i.Decision, &i.MatchedRuleKey); err != nil {
				rows.Close()
				return err
			}
			q.Items = append(q.Items, i)
			if len(q.Items) > domain.RiskMaxItems {
				rows.Close()
				return risk.ErrLimit
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		if !riskSame(q, *p) {
			return risk.ErrIntegrity
		}
	}
	return nil
}

func riskReplay(ctx context.Context, tx *sql.Tx, scope repositories.RiskScope, id string) (riskArchived, error) {
	var x riskArchived
	a, m := &x.assessment, &x.material
	var bytes int
	err := tx.QueryRowContext(ctx, `SELECT length(CAST(evidence_json AS BLOB))+length(CAST(source_bindings_json AS BLOB))+length(CAST(result_json AS BLOB)) FROM change_risk_assessments WHERE id=? AND event_id=? AND device_id=?`, id, scope.EventID, scope.DeviceID).Scan(&bytes)
	if err != nil {
		return x, repositoryError(err)
	}
	if bytes > domain.RiskMaxCanonicalBytes {
		return x, risk.ErrLimit
	}
	var typed domain.RiskEvaluationResult
	var priority, findingCount int
	var authStatus, policyStatus string
	var authID, policyID sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT `+riskHeaderColumns+` FROM change_risk_assessments WHERE id=? AND event_id=? AND device_id=?`, id, scope.EventID, scope.DeviceID).Scan(&a.ID, &a.EventID, &a.DeviceID, &a.ConfigType, &a.Revision, &a.PreviousAssessmentID, &a.ModelVersion, &a.DefinitionHash, &a.EvidenceSchemaVersion, &a.EvidenceHash, &m.evidenceJSON, &m.bindingsJSON, &a.ResultHash, &m.resultJSON, &typed.TechnicalImpact, &typed.KnownImpactFloor, &typed.EvidenceCompleteness, &typed.ReviewPriority, &priority, &typed.TotalItems, &typed.KnownItems, &typed.UnknownItems, &typed.DistinctKnownCategories, &findingCount, &authStatus, &authID, &policyStatus, &policyID, &a.CapturedAt, &a.AssessedBy)
	if err != nil {
		return x, err
	}
	_, hash := risk.CanonicalDefinitionV1()
	if !riskID(a.ID) || !riskID(a.AssessedBy) || a.ModelVersion != 1 || a.EvidenceSchemaVersion != 1 || a.DefinitionHash != hash || a.Revision < 1 || findingCount < 0 || findingCount > domain.RiskMaxItems+16 {
		return x, risk.ErrIntegrity
	}
	if err = riskRegistry(ctx, tx, false, ""); err != nil {
		return x, err
	}
	if err = risk.StrictDecode(m.evidenceJSON, &m.evidence); err != nil {
		return x, err
	}
	if err = risk.StrictDecode(m.bindingsJSON, &m.bindings); err != nil {
		return x, err
	}
	if err = risk.StrictDecode(m.resultJSON, &m.result); err != nil {
		return x, err
	}
	e, canonical, evidenceHash, err := risk.CanonicalEvidence(m.evidence)
	if err != nil || canonical != m.evidenceJSON || evidenceHash != a.EvidenceHash {
		return x, risk.ErrIntegrity
	}
	if e.Event.EventID != a.EventID || e.Event.DeviceID != a.DeviceID || e.Event.ConfigType != a.ConfigType || authStatus != e.Authorization.RecordedStatus || policyStatus != e.Policy.RecordedStatus {
		return x, risk.ErrIntegrity
	}
	var expectedAuth, expectedPolicy *string
	if e.Authorization.Ticket != nil {
		expectedAuth = riskString(e.Authorization.Ticket.ID)
	}
	if e.Policy.Final != nil {
		expectedPolicy = riskString(e.Policy.Final.ID)
	}
	if !riskSame(riskNullable(authID), expectedAuth) || !riskSame(riskNullable(policyID), expectedPolicy) {
		return x, risk.ErrIntegrity
	}
	if err = riskBindings(e, m.bindings, a.CapturedAt); err != nil {
		return x, err
	}
	replayed, err := risk.Evaluate(e)
	if err != nil {
		return x, err
	}
	resultJSON, resultHash, err := risk.CanonicalResult(replayed, hash, evidenceHash)
	if err != nil || resultJSON != m.resultJSON || resultHash != a.ResultHash {
		return x, risk.ErrIntegrity
	}
	typed.Findings = m.result.Findings
	if !riskSame(typed, m.result) || priority != map[string]int{"normal": 0, "elevated": 1, "urgent": 2}[m.result.ReviewPriority] || findingCount != len(m.result.Findings) {
		return x, risk.ErrIntegrity
	}
	// Complete ordered SQL child projection, not just child count/hash.
	rows, err := tx.QueryContext(ctx, `SELECT event_id,ordinal,finding_key,rule_id,reason_code,kind,subject_kind,subject_id,source_item_id,item_ordinal,impact,priority_floor,parameters_json FROM change_risk_findings WHERE assessment_id=? ORDER BY ordinal`, id)
	if err != nil {
		return x, err
	}
	n := 0
	for rows.Next() {
		var f domain.RiskFinding
		var event, params string
		var source sql.NullString
		if err = rows.Scan(&event, &f.Ordinal, &f.Key, &f.RuleID, &f.ReasonCode, &f.Kind, &f.Subject.Kind, &f.Subject.ID, &source, &f.ItemOrdinal, &f.Impact, &f.PriorityFloor, &params); err != nil {
			rows.Close()
			return x, err
		}
		f.Parameters = []domain.RiskSafeParameter{}
		var expectedSource *string
		if f.Subject.Kind == "item" {
			expectedSource = riskString(f.Subject.ID)
		}
		if n >= len(m.result.Findings) || event != a.EventID || params != "[]" || !riskSame(riskNullable(source), expectedSource) || !riskSame(f, m.result.Findings[n]) {
			rows.Close()
			return x, risk.ErrIntegrity
		}
		n++
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return x, err
	}
	if n != len(m.result.Findings) {
		return x, risk.ErrIntegrity
	}
	technical, bindings, err := riskTechnical(ctx, tx, scope)
	if err != nil {
		return x, err
	}
	if !riskSame(technical.Event, e.Event) || !riskSame(technical.Items, e.Items) || !riskSame(bindings.Event, m.bindings.Event) || !riskSame(bindings.Items, m.bindings.Items) {
		return x, risk.ErrIntegrity
	}
	if err = riskHistoricalGovernance(ctx, tx, e, m.bindings); err != nil {
		return x, err
	}
	// Validate every edge and contiguous revision count through this record.
	var count, bad int
	err = tx.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(SUM(CASE WHEN r.revision=1 THEN r.previous_assessment_id IS NOT NULL ELSE NOT EXISTS(SELECT 1 FROM change_risk_assessments p WHERE p.id=r.previous_assessment_id AND p.event_id=r.event_id AND p.device_id=r.device_id AND p.config_type=r.config_type AND p.revision=r.revision-1 AND p.captured_at<=r.captured_at) END),0) FROM change_risk_assessments r WHERE r.event_id=? AND r.revision<=?`, a.EventID, a.Revision).Scan(&count, &bad)
	if err != nil {
		return x, err
	}
	if count != a.Revision || bad != 0 {
		return x, risk.ErrIntegrity
	}
	a.Result = m.result
	a.Historical = true
	m.evidenceHash = evidenceHash
	m.resultHash = resultHash
	return x, nil
}
