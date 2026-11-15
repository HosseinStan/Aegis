package repositories

import (
	"context"
	"errors"
	"time"

	"company.local/aegis/apps/aegis-api/internal/domain"
)

var ErrRiskUnavailable = errors.New("risk assessment temporarily unavailable")

// RiskFailure's public message never contains source text or driver diagnostics.
// Restricted callers can unwrap the original AND cleanup causes for diagnosis.
type RiskFailure struct{ Kind, Cause error }

func (e *RiskFailure) Error() string   { return e.Kind.Error() }
func (e *RiskFailure) Unwrap() []error { return []error{e.Kind, e.Cause} }

// ActorID is the authenticated server principal, not a cached permission list.
type RiskScope struct{ DeviceID, EventID, ActorID string }
type RiskPreview struct {
	ModelVersion int                         `json:"model_version"`
	Historical   bool                        `json:"historical"`
	Eligibility  domain.RiskEligibility      `json:"eligibility"`
	Result       domain.RiskEvaluationResult `json:"result"`
}
type RiskRecord struct {
	Created    bool                        `json:"created"`
	Assessment domain.ChangeRiskAssessment `json:"assessment"`
}
type RiskFindingPage struct {
	AssessmentID string               `json:"assessment_id"`
	Historical   bool                 `json:"historical"`
	Total        int                  `json:"total"`
	Items        []domain.RiskFinding `json:"items"`
}

// No caller result/model/hash fields are accepted. Every method reauthorizes
// the live actor and device scope in its own consistent database transaction.
type ChangeRiskRepository interface {
	PreviewRisk(context.Context, RiskScope, time.Time) (RiskPreview, error)
	RecordRisk(context.Context, RiskScope, *string, time.Time) (RiskRecord, error)
	GetRisk(context.Context, RiskScope, string) (domain.ChangeRiskAssessment, error)
	ListRisks(context.Context, RiskScope, Page) ([]domain.ChangeRiskAssessment, int, error)
	ListRiskFindings(context.Context, RiskScope, string, Page) (RiskFindingPage, error)
}
