package api

import "company.local/aegis/apps/aegis-api/internal/domain"

// Persisted results are historical, even on the latest route. Full finding
// sets are obtained only through bounded ordinal paging; canonical evidence
// and timestamp binding sidecars have no field in this HTTP projection.
type riskResultSummary struct {
	TechnicalImpact         string               `json:"technical_impact"`
	KnownImpactFloor        *string              `json:"known_impact_floor"`
	EvidenceCompleteness    string               `json:"evidence_completeness"`
	ReviewPriority          string               `json:"review_priority"`
	TotalItems              int                  `json:"total_items"`
	KnownItems              int                  `json:"known_items"`
	UnknownItems            int                  `json:"unknown_items"`
	DistinctKnownCategories int                  `json:"distinct_known_categories"`
	FindingCount            int                  `json:"finding_count"`
	GovernanceFindings      []domain.RiskFinding `json:"governance_findings"`
}

type riskAssessmentDTO struct {
	ID                    string            `json:"id"`
	EventID               string            `json:"event_id"`
	DeviceID              string            `json:"device_id"`
	ConfigType            string            `json:"config_type"`
	Revision              int               `json:"revision"`
	PreviousAssessmentID  *string           `json:"previous_assessment_id"`
	ModelVersion          int               `json:"model_version"`
	EvidenceSchemaVersion int               `json:"evidence_schema_version"`
	DefinitionHash        string            `json:"definition_hash"`
	EvidenceHash          string            `json:"evidence_hash"`
	ResultHash            string            `json:"result_hash"`
	CapturedAt            string            `json:"captured_at"`
	AssessedBy            string            `json:"assessed_by"`
	Historical            bool              `json:"historical"`
	Result                riskResultSummary `json:"result"`
}

type riskRecordDTO struct {
	Created    bool              `json:"created"`
	Assessment riskAssessmentDTO `json:"assessment"`
}

func riskAssessmentResponse(a domain.ChangeRiskAssessment) riskAssessmentDTO {
	governance := []domain.RiskFinding{}
	for _, f := range a.Result.Findings {
		if f.Kind == "governance" {
			governance = append(governance, f)
		}
	}
	return riskAssessmentDTO{
		ID: a.ID, EventID: a.EventID, DeviceID: a.DeviceID, ConfigType: a.ConfigType,
		Revision: a.Revision, PreviousAssessmentID: a.PreviousAssessmentID, ModelVersion: a.ModelVersion,
		EvidenceSchemaVersion: a.EvidenceSchemaVersion, DefinitionHash: a.DefinitionHash,
		EvidenceHash: a.EvidenceHash, ResultHash: a.ResultHash, CapturedAt: a.CapturedAt,
		AssessedBy: a.AssessedBy, Historical: true,
		Result: riskResultSummary{TechnicalImpact: a.Result.TechnicalImpact, KnownImpactFloor: a.Result.KnownImpactFloor,
			EvidenceCompleteness: a.Result.EvidenceCompleteness, ReviewPriority: a.Result.ReviewPriority,
			TotalItems: a.Result.TotalItems, KnownItems: a.Result.KnownItems, UnknownItems: a.Result.UnknownItems,
			DistinctKnownCategories: a.Result.DistinctKnownCategories, FindingCount: len(a.Result.Findings), GovernanceFindings: governance},
	}
}
