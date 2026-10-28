package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"company.local/aegis/apps/aegis-api/internal/domain"
	"company.local/aegis/apps/aegis-api/internal/repositories"
	"company.local/aegis/apps/aegis-api/internal/risk"
	"github.com/google/uuid"
)

var _ repositories.ChangeRiskRepository = (*Store)(nil)

func riskCapture(at time.Time) (string, error) {
	if at.IsZero() {
		return "", domain.ErrValidation
	}
	return riskTime(at.UTC().Format(risk.UTC9))
}
func riskPage(page repositories.Page) bool {
	return page.Limit >= 1 && page.Limit <= 200 && page.Offset >= 0 && page.Offset <= 10000
}
func riskLatest(ctx context.Context, tx *sql.Tx, scope repositories.RiskScope) (string, error) {
	var id string
	err := tx.QueryRowContext(ctx, `SELECT id FROM change_risk_assessments WHERE event_id=? AND device_id=? ORDER BY revision DESC LIMIT 1`, scope.EventID, scope.DeviceID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return id, err
}
func riskResolve(ctx context.Context, tx *sql.Tx, scope repositories.RiskScope, id string) (riskArchived, error) {
	if id == "" {
		var err error
		id, err = riskLatest(ctx, tx, scope)
		if err != nil {
			return riskArchived{}, err
		}
		if id == "" {
			return riskArchived{}, domain.ErrNotFound
		}
	}
	if !riskID(id) {
		return riskArchived{}, domain.ErrValidation
	}
	return riskReplay(ctx, tx, scope, id)
}

func (s *Store) PreviewRisk(ctx context.Context, scope repositories.RiskScope, at time.Time) (repositories.RiskPreview, error) {
	var result repositories.RiskPreview
	captured, err := riskCapture(at)
	if err != nil {
		return result, riskFailure(err)
	}
	err = s.riskTransaction(ctx, func(tx *sql.Tx) error {
		if err := riskAuthorize(ctx, tx, scope, false); err != nil {
			return err
		}
		m, err := riskMaterialize(ctx, tx, scope, captured)
		if errors.Is(err, risk.ErrIneligible) {
			result = riskIneligiblePreview(m.evidence)
			return nil
		}
		if err != nil {
			return err
		}
		result = repositories.RiskPreview{ModelVersion: 1, Eligibility: m.evidence.Eligibility, Result: m.result}
		return nil
	})
	if err != nil {
		return repositories.RiskPreview{}, err
	}
	return result, nil
}

func riskActionAudit(ctx context.Context, tx *sql.Tx, scope repositories.RiskScope, id, captured string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO audit_logs(id,request_id,user_id,device_id,action,resource_type,resource_id,succeeded,duration_ms,created_at) VALUES(?,?,?,?,'change_risk.evaluate','change_risk_assessment',?,1,0,?)`, uuid.NewString(), uuid.NewString(), scope.ActorID, scope.DeviceID, id, captured)
	return err
}

func riskInsert(ctx context.Context, tx *sql.Tx, a domain.ChangeRiskAssessment, m riskMaterial) error {
	for _, f := range m.result.Findings {
		var itemID *string
		if f.Subject.Kind == "item" {
			itemID = riskString(f.Subject.ID)
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO change_risk_findings(assessment_id,event_id,ordinal,finding_key,rule_id,reason_code,kind,subject_kind,subject_id,source_item_id,item_ordinal,impact,priority_floor,parameters_json) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, a.ID, a.EventID, f.Ordinal, f.Key, f.RuleID, f.ReasonCode, f.Kind, f.Subject.Kind, f.Subject.ID, itemID, f.ItemOrdinal, f.Impact, f.PriorityFloor, "[]")
		if err != nil {
			return err
		}
	}
	var authID, policyID *string
	if m.evidence.Authorization.Ticket != nil {
		authID = riskString(m.evidence.Authorization.Ticket.ID)
	}
	if m.evidence.Policy.Final != nil {
		policyID = riskString(m.evidence.Policy.Final.ID)
	}
	args := []any{a.ID, a.EventID, a.DeviceID, a.ConfigType, a.Revision, a.PreviousAssessmentID, a.ModelVersion, a.DefinitionHash, a.EvidenceSchemaVersion, a.EvidenceHash, m.evidenceJSON, m.bindingsJSON, a.ResultHash, m.resultJSON, m.result.TechnicalImpact, m.result.KnownImpactFloor, m.result.EvidenceCompleteness, m.result.ReviewPriority, map[string]int{"normal": 0, "elevated": 1, "urgent": 2}[m.result.ReviewPriority], m.result.TotalItems, m.result.KnownItems, m.result.UnknownItems, m.result.DistinctKnownCategories, len(m.result.Findings), m.evidence.Authorization.RecordedStatus, authID, m.evidence.Policy.RecordedStatus, policyID, a.CapturedAt, a.AssessedBy}
	_, err := tx.ExecContext(ctx, `INSERT INTO change_risk_assessments(`+riskHeaderColumns+`) VALUES(`+strings.TrimSuffix(strings.Repeat("?,", len(args)), ",")+")", args...)
	return err
}

func (s *Store) RecordRisk(ctx context.Context, scope repositories.RiskScope, expected *string, at time.Time) (repositories.RiskRecord, error) {
	var result repositories.RiskRecord
	if expected != nil && !riskID(*expected) {
		return result, riskFailure(domain.ErrValidation)
	}
	captured, err := riskCapture(at)
	if err != nil {
		return result, riskFailure(err)
	}
	err = s.riskTransaction(ctx, func(tx *sql.Tx) error {
		if err := riskAuthorize(ctx, tx, scope, true); err != nil {
			return err
		}
		m, err := riskMaterialize(ctx, tx, scope, captured)
		if err != nil {
			return err
		}
		latestID, err := riskLatest(ctx, tx, scope)
		if err != nil {
			return err
		}
		revision := 1
		var predecessor *string
		_, definitionHash := risk.CanonicalDefinitionV1()
		if latestID == "" {
			if expected != nil {
				return domain.ErrConflict
			}
		} else {
			latest, err := riskReplay(ctx, tx, scope, latestID)
			if err != nil {
				return err
			}
			a := latest.assessment
			// Compare semantic hashes AND exact validated canonical bytes, including
			// raw timestamp bindings. Never select an older matching fingerprint.
			same := a.ModelVersion == 1 && a.DefinitionHash == definitionHash && a.EvidenceHash == m.evidenceHash && a.ResultHash == m.resultHash && latest.material.evidenceJSON == m.evidenceJSON && latest.material.bindingsJSON == m.bindingsJSON && latest.material.resultJSON == m.resultJSON
			if same {
				allowed := (expected != nil && (*expected == a.ID || (a.PreviousAssessmentID != nil && *expected == *a.PreviousAssessmentID))) || (expected == nil && a.Revision == 1)
				if !allowed {
					return domain.ErrConflict
				}
				if err := riskActionAudit(ctx, tx, scope, a.ID, captured); err != nil {
					return err
				}
				result = repositories.RiskRecord{Assessment: a}
				return nil
			}
			if expected == nil || *expected != a.ID || captured < a.CapturedAt {
				return domain.ErrConflict
			}
			revision = a.Revision + 1
			predecessor = riskString(a.ID)
		}
		if err = riskRegistry(ctx, tx, true, captured); err != nil {
			return err
		}
		a := domain.ChangeRiskAssessment{ID: uuid.NewString(), EventID: scope.EventID, DeviceID: scope.DeviceID, ConfigType: m.evidence.Event.ConfigType, Revision: revision, PreviousAssessmentID: predecessor, ModelVersion: 1, EvidenceSchemaVersion: 1, DefinitionHash: definitionHash, EvidenceHash: m.evidenceHash, ResultHash: m.resultHash, CapturedAt: captured, AssessedBy: scope.ActorID, Historical: true, Result: m.result}
		if err = riskInsert(ctx, tx, a, m); err != nil {
			return err
		}
		if err = riskActionAudit(ctx, tx, scope, a.ID, captured); err != nil {
			return err
		}
		// Read our own sealed result through the identical historical trust path.
		verified, err := riskReplay(ctx, tx, scope, a.ID)
		if err != nil {
			return err
		}
		result = repositories.RiskRecord{Created: true, Assessment: verified.assessment}
		return nil
	})
	if err != nil {
		return repositories.RiskRecord{}, err
	}
	return result, nil
}

func (s *Store) GetRisk(ctx context.Context, scope repositories.RiskScope, id string) (domain.ChangeRiskAssessment, error) {
	var result domain.ChangeRiskAssessment
	err := s.riskTransaction(ctx, func(tx *sql.Tx) error {
		if err := riskAuthorize(ctx, tx, scope, false); err != nil {
			return err
		}
		x, err := riskResolve(ctx, tx, scope, id)
		if err != nil {
			return err
		}
		result = x.assessment
		return nil
	})
	if err != nil {
		return domain.ChangeRiskAssessment{}, err
	}
	return result, nil
}
func (s *Store) ListRisks(ctx context.Context, scope repositories.RiskScope, page repositories.Page) ([]domain.ChangeRiskAssessment, int, error) {
	if !riskPage(page) {
		return nil, 0, riskFailure(domain.ErrValidation)
	}
	result := []domain.ChangeRiskAssessment{}
	total := 0
	err := s.riskTransaction(ctx, func(tx *sql.Tx) error {
		if err := riskAuthorize(ctx, tx, scope, false); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM change_risk_assessments WHERE event_id=? AND device_id=?`, scope.EventID, scope.DeviceID).Scan(&total); err != nil {
			return err
		}
		rows, err := tx.QueryContext(ctx, `SELECT id FROM change_risk_assessments WHERE event_id=? AND device_id=? ORDER BY revision DESC LIMIT ? OFFSET ?`, scope.EventID, scope.DeviceID, page.Limit, page.Offset)
		if err != nil {
			return err
		}
		ids := []string{}
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, id)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, id := range ids {
			x, err := riskReplay(ctx, tx, scope, id)
			if err != nil {
				return err
			}
			result = append(result, x.assessment)
		}
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	return result, total, nil
}
func (s *Store) ListRiskFindings(ctx context.Context, scope repositories.RiskScope, id string, page repositories.Page) (repositories.RiskFindingPage, error) {
	var result repositories.RiskFindingPage
	if !riskPage(page) {
		return result, riskFailure(domain.ErrValidation)
	}
	err := s.riskTransaction(ctx, func(tx *sql.Tx) error {
		if err := riskAuthorize(ctx, tx, scope, false); err != nil {
			return err
		}
		x, err := riskResolve(ctx, tx, scope, id)
		if err != nil {
			return err
		}
		findings := x.assessment.Result.Findings
		start := min(page.Offset, len(findings))
		end := start + min(page.Limit, len(findings)-start)
		result = repositories.RiskFindingPage{AssessmentID: x.assessment.ID, Historical: true, Total: len(findings), Items: append([]domain.RiskFinding{}, findings[start:end]...)}
		return nil
	})
	if err != nil {
		return repositories.RiskFindingPage{}, err
	}
	return result, nil
}
