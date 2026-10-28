package domain

// Raw bindings contain only scoped identities and original timestamp tokens.
// Canonical evidence contains their normalized instants; no command/context
// text or hashes of source text have a field in this sidecar.
type RiskEventTimeBinding struct {
	EventID    string `json:"event_id"`
	DetectedAt string `json:"detected_at"`
	AnalyzedAt string `json:"analyzed_at"`
	CreatedAt  string `json:"created_at"`
}
type RiskItemTimeBinding struct {
	ItemID       string `json:"item_id"`
	Ordinal      int    `json:"ordinal"`
	CreatedAt    string `json:"created_at"`
	ClassifiedAt string `json:"classified_at"`
}
type RiskTicketTimeBinding struct {
	AuthorizationID string  `json:"authorization_id"`
	WindowStart     string  `json:"window_start"`
	WindowEnd       string  `json:"window_end"`
	RequestedAt     string  `json:"requested_at"`
	ApprovedAt      string  `json:"approved_at"`
	RejectedAt      *string `json:"rejected_at"`
	RevokedAt       *string `json:"revoked_at"`
	CreatedAt       string  `json:"created_at"`
}
type RiskLinkTimeBinding struct {
	EventID         string `json:"event_id"`
	AuthorizationID string `json:"authorization_id"`
	LinkedAt        string `json:"linked_at"`
}
type RiskHistoryTimeBinding struct {
	ID         string  `json:"id"`
	Action     string  `json:"action"`
	EventID    *string `json:"event_id"`
	OccurredAt string  `json:"occurred_at"`
}
type RiskPolicyTimeBinding struct {
	EvaluationID string `json:"evaluation_id"`
	EvaluatedAt  string `json:"evaluated_at"`
}
type RiskGovernanceTimeBindings struct {
	Ticket  *RiskTicketTimeBinding   `json:"ticket"`
	Link    *RiskLinkTimeBinding     `json:"link"`
	History []RiskHistoryTimeBinding `json:"history"`
	Policy  *RiskPolicyTimeBinding   `json:"policy"`
}
type RiskSourceBindings struct {
	Event      RiskEventTimeBinding       `json:"event"`
	Items      []RiskItemTimeBinding      `json:"items"`
	Governance RiskGovernanceTimeBindings `json:"governance"`
}
