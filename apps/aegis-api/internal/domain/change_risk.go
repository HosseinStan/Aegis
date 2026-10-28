package domain

// Risk contracts carry metadata only. Commands, context paths and configurations
// deliberately have no representation here.
const (
	RiskModelVersion        = 1
	RiskEngineVersion       = 1
	RiskSchemaVersion       = 1
	RiskMaterializerVersion = 1
	PermissionRiskRead      = "changes.risk.read"
	PermissionRiskEvaluate  = "changes.risk.evaluate"
	RiskMaxItems            = 10000
	RiskMaxSourceBytes      = 5 << 20
	RiskMaxCanonicalBytes   = 4 << 20
)

type RiskTechnicalTuple struct {
	Category           string `json:"category"`
	Subcategory        string `json:"subcategory"`
	ClassificationRule string `json:"classification_rule"`
	RuleID             string `json:"rule_id"`
	Impact             string `json:"impact"`
}
type RiskAggregationContract struct {
	ImpactMode            string `json:"impact_mode"`
	PriorityMode          string `json:"priority_mode"`
	FindingOrder          string `json:"finding_order"`
	DuplicateMode         string `json:"duplicate_mode"`
	NoItemsImpact         string `json:"no_items_impact"`
	DefaultReviewPriority string `json:"default_review_priority"`
	CriticalEnabled       bool   `json:"critical_enabled"`
}
type RiskPriorityRule struct {
	RuleID    string `json:"rule_id"`
	Predicate string `json:"predicate"`
	Floor     string `json:"floor"`
}
type RiskExplanationTemplate struct {
	ReasonCode string `json:"reason_code"`
	Text       string `json:"text"`
}
type RiskDefinition struct {
	ModelVersion                   int                       `json:"model_version"`
	SchemaVersion                  int                       `json:"schema_version"`
	EngineVersion                  int                       `json:"engine_version"`
	MaterializerVersion            int                       `json:"materializer_version"`
	AcceptedAnalysisVersions       []int                     `json:"accepted_analysis_versions"`
	AcceptedClassificationVersions []int                     `json:"accepted_classification_versions"`
	Aggregation                    RiskAggregationContract   `json:"aggregation"`
	TechnicalRules                 []RiskTechnicalTuple      `json:"technical_rules"`
	PriorityRules                  []RiskPriorityRule        `json:"priority_rules"`
	ExplanationTemplates           []RiskExplanationTemplate `json:"explanation_templates"`
}
type RiskEventSnapshot struct {
	EventID         string  `json:"event_id"`
	DeviceID        string  `json:"device_id"`
	ConfigType      string  `json:"config_type"`
	FromBackupID    string  `json:"from_backup_id"`
	ToBackupID      string  `json:"to_backup_id"`
	DetectedAt      string  `json:"detected_at"`
	CreatedAt       string  `json:"created_at"`
	AnalysisVersion int     `json:"analysis_version"`
	ItemCount       int     `json:"item_count"`
	AddedLines      int     `json:"added_lines"`
	RemovedLines    int     `json:"removed_lines"`
	AnalyzedAt      *string `json:"analyzed_at"`
}
type RiskItemSignal struct {
	ItemID                string  `json:"item_id"`
	Ordinal               int     `json:"ordinal"`
	Operation             string  `json:"operation"`
	Category              string  `json:"category"`
	Subcategory           string  `json:"subcategory"`
	ClassificationRule    string  `json:"classification_rule"`
	ClassificationVersion int     `json:"classification_version"`
	Sensitive             bool    `json:"sensitive"`
	CreatedAt             string  `json:"created_at"`
	ClassifiedAt          *string `json:"classified_at"`
}
type RiskAuthorizationLink struct {
	EventID         string `json:"event_id"`
	AuthorizationID string `json:"authorization_id"`
	LinkedBy        string `json:"linked_by"`
	LinkedAt        string `json:"linked_at"`
	Attestation     string `json:"attestation"`
}
type RiskAuthorizationTicket struct {
	ID                string   `json:"id"`
	DeviceID          string   `json:"device_id"`
	ConfigType        string   `json:"config_type"`
	Status            string   `json:"status"`
	AllowedCategories []string `json:"allowed_categories"`
	WindowStart       string   `json:"window_start"`
	WindowEnd         string   `json:"window_end"`
	RequestedBy       string   `json:"requested_by"`
	RequestedAt       string   `json:"requested_at"`
	ApprovedBy        string   `json:"approved_by"`
	ApprovedAt        string   `json:"approved_at"`
	RejectedBy        *string  `json:"rejected_by"`
	RejectedAt        *string  `json:"rejected_at"`
	RevokedBy         *string  `json:"revoked_by"`
	RevokedAt         *string  `json:"revoked_at"`
	CreatedAt         string   `json:"created_at"`
}
type RiskAuthorizationHistory struct {
	ID         string  `json:"id"`
	Action     string  `json:"action"`
	ActorID    string  `json:"actor_id"`
	OccurredAt string  `json:"occurred_at"`
	EventID    *string `json:"event_id"`
}
type RiskAuthorizationObservation struct {
	RecordedStatus string                     `json:"recorded_status"`
	Link           *RiskAuthorizationLink     `json:"link"`
	Ticket         *RiskAuthorizationTicket   `json:"ticket"`
	History        []RiskAuthorizationHistory `json:"history"`
}
type RiskPolicyFinal struct {
	ID             string             `json:"id"`
	EventID        string             `json:"event_id"`
	PolicySetID    string             `json:"policy_set_id"`
	DefinitionHash string             `json:"definition_hash"`
	Definition     PolicyDefinition   `json:"definition"`
	EngineVersion  int                `json:"engine_version"`
	Result         string             `json:"result"`
	EvaluatedBy    string             `json:"evaluated_by"`
	EvaluatedAt    string             `json:"evaluated_at"`
	TotalItems     int                `json:"total_items"`
	AllowedCount   int                `json:"allowed_count"`
	DeniedCount    int                `json:"denied_count"`
	UncoveredCount int                `json:"uncovered_count"`
	Items          []PolicyItemResult `json:"items"`
}
type RiskPolicyObservation struct {
	RecordedStatus string           `json:"recorded_status"`
	Final          *RiskPolicyFinal `json:"final"`
}
type RiskEligibility struct {
	Persistable bool     `json:"persistable"`
	Reasons     []string `json:"reasons"`
}
type RiskEvidenceSnapshot struct {
	SchemaVersion int                          `json:"schema_version"`
	Event         RiskEventSnapshot            `json:"event"`
	Items         []RiskItemSignal             `json:"items"`
	Authorization RiskAuthorizationObservation `json:"authorization"`
	Policy        RiskPolicyObservation        `json:"policy"`
	Eligibility   RiskEligibility              `json:"eligibility"`
}
type RiskSubject struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}
type RiskSafeParameter struct {
	Name      string  `json:"name"`
	Kind      string  `json:"kind"`
	EnumValue *string `json:"enum_value"`
	IntValue  *int    `json:"int_value"`
	UUIDValue *string `json:"uuid_value"`
}
type RiskFinding struct {
	Ordinal       int                 `json:"ordinal"`
	Key           string              `json:"key"`
	RuleID        string              `json:"rule_id"`
	ReasonCode    string              `json:"reason_code"`
	Kind          string              `json:"kind"`
	Subject       RiskSubject         `json:"subject"`
	ItemOrdinal   *int                `json:"item_ordinal"`
	Impact        *string             `json:"impact"`
	PriorityFloor *string             `json:"priority_floor"`
	Parameters    []RiskSafeParameter `json:"parameters"`
}
type RiskEvaluationResult struct {
	TechnicalImpact         string        `json:"technical_impact"`
	KnownImpactFloor        *string       `json:"known_impact_floor"`
	EvidenceCompleteness    string        `json:"evidence_completeness"`
	ReviewPriority          string        `json:"review_priority"`
	TotalItems              int           `json:"total_items"`
	KnownItems              int           `json:"known_items"`
	UnknownItems            int           `json:"unknown_items"`
	DistinctKnownCategories int           `json:"distinct_known_categories"`
	Findings                []RiskFinding `json:"findings"`
}
type ChangeRiskAssessment struct {
	ID                    string               `json:"id"`
	EventID               string               `json:"event_id"`
	DeviceID              string               `json:"device_id"`
	ConfigType            string               `json:"config_type"`
	Revision              int                  `json:"revision"`
	PreviousAssessmentID  *string              `json:"previous_assessment_id"`
	ModelVersion          int                  `json:"model_version"`
	EvidenceSchemaVersion int                  `json:"evidence_schema_version"`
	DefinitionHash        string               `json:"definition_hash"`
	EvidenceHash          string               `json:"evidence_hash"`
	ResultHash            string               `json:"result_hash"`
	CapturedAt            string               `json:"captured_at"`
	AssessedBy            string               `json:"assessed_by"`
	Historical            bool                 `json:"historical"`
	Result                RiskEvaluationResult `json:"result"`
}
