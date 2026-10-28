package risk

import (
	"encoding/json"
	"slices"
	"strings"

	"company.local/aegis/apps/aegis-api/internal/domain"
	"github.com/google/uuid"
)

func validID(id string) bool {
	u, err := uuid.Parse(id)
	return err == nil && u != uuid.Nil && u.String() == id
}
func textPtr(s string) *string { return &s }
func impactRank(s string) int {
	switch s {
	case "low":
		return 1
	case "medium":
		return 2
	case "high":
		return 3
	}
	return 0
}
func priorityRank(s string) int {
	switch s {
	case "elevated":
		return 1
	case "urgent":
		return 2
	}
	return 0
}
func tupleFor(item domain.RiskItemSignal) (domain.RiskTechnicalTuple, bool) {
	for _, t := range tuplesV1 {
		if item.Category == t.Category && item.Subcategory == t.Subcategory && item.ClassificationRule == t.ClassificationRule {
			return t, true
		}
	}
	return domain.RiskTechnicalTuple{}, false
}

// CanonicalEvidence returns a deep-detached, validated, deterministically
// ordered projection. The caller's evidence is never modified.
func CanonicalEvidence(in domain.RiskEvidenceSnapshot) (domain.RiskEvidenceSnapshot, string, string, error) {
	var e domain.RiskEvidenceSnapshot
	raw, err := json.Marshal(in)
	if err != nil {
		return e, "", "", ErrIntegrity
	}
	if len(raw) > domain.RiskMaxCanonicalBytes {
		return e, "", "", ErrLimit
	}
	if err := StrictDecode(string(raw), &e); err != nil {
		return e, "", "", err
	}
	// Canonical array fields always encode empty collections as [], never null.
	if e.Items == nil {
		e.Items = []domain.RiskItemSignal{}
	}
	if e.Authorization.History == nil {
		e.Authorization.History = []domain.RiskAuthorizationHistory{}
	}
	if e.Eligibility.Reasons == nil {
		e.Eligibility.Reasons = []string{}
	}
	slices.SortFunc(e.Items, func(a, b domain.RiskItemSignal) int { return a.Ordinal - b.Ordinal })
	slices.SortFunc(e.Authorization.History, func(a, b domain.RiskAuthorizationHistory) int {
		if a.OccurredAt != b.OccurredAt {
			return strings.Compare(a.OccurredAt, b.OccurredAt)
		}
		rank := map[string]int{"requested": 0, "approved": 1, "linked": 2, "revoked": 3}
		if rank[a.Action] != rank[b.Action] {
			return rank[a.Action] - rank[b.Action]
		}
		return strings.Compare(a.ID, b.ID)
	})
	if e.Authorization.Ticket != nil {
		slices.Sort(e.Authorization.Ticket.AllowedCategories)
	}
	if e.Policy.Final != nil {
		slices.SortFunc(e.Policy.Final.Items, func(a, b domain.PolicyItemResult) int { return a.Ordinal - b.Ordinal })
	}
	if err := validateEvidence(e); err != nil {
		return e, "", "", err
	}
	encoded, err := json.Marshal(e)
	if err != nil {
		return e, "", "", ErrIntegrity
	}
	return e, string(encoded), Digest("aegis-risk-evidence-v1", encoded), nil
}

func validateEvidence(e domain.RiskEvidenceSnapshot) error {
	event := e.Event
	for _, id := range []string{event.EventID, event.DeviceID, event.FromBackupID, event.ToBackupID} {
		if !validID(id) {
			return ErrIntegrity
		}
	}
	if e.SchemaVersion != 1 || (event.ConfigType != "running" && event.ConfigType != "startup") || event.FromBackupID == event.ToBackupID || event.AddedLines < 0 || event.RemovedLines < 0 || event.ItemCount != len(e.Items) || event.ItemCount < 0 || !validCanonicalTime(event.DetectedAt) || !validCanonicalTime(event.CreatedAt) {
		return ErrIntegrity
	}
	if len(e.Items) > domain.RiskMaxItems {
		return ErrLimit
	}
	if event.AnalysisVersion != 1 || event.AnalyzedAt == nil || !validCanonicalTime(*event.AnalyzedAt) {
		return ErrIntegrity
	}
	seen := map[string]bool{}
	for i, item := range e.Items {
		if !validID(item.ItemID) || seen[item.ItemID] || item.Ordinal != i || (item.Operation != "add" && item.Operation != "remove") || item.ClassificationVersion != 1 || item.ClassifiedAt == nil || !validCanonicalTime(*item.ClassifiedAt) || !validCanonicalTime(item.CreatedAt) {
			return ErrIntegrity
		}
		seen[item.ItemID] = true
		if _, ok := tupleFor(item); !ok {
			return ErrIntegrity
		}
	}
	// Unsupported source summaries use a separate preview contract; they must
	// never be disguised as a partially validated current-version item set.
	if !e.Eligibility.Persistable || len(e.Eligibility.Reasons) != 0 {
		return ErrIneligible
	}
	if err := validateAuthorization(e); err != nil {
		return err
	}
	return validatePolicy(e)
}
func validateAuthorization(e domain.RiskEvidenceSnapshot) error {
	a := e.Authorization
	if a.RecordedStatus == "unaccounted" {
		if a.Link != nil || a.Ticket != nil || len(a.History) != 0 {
			return ErrIntegrity
		}
		return nil
	}
	if a.RecordedStatus != "authorized" || a.Link == nil || a.Ticket == nil {
		return ErrIntegrity
	}
	l, t := a.Link, a.Ticket
	for _, id := range []string{l.EventID, l.AuthorizationID, l.LinkedBy, t.ID, t.DeviceID, t.RequestedBy, t.ApprovedBy} {
		if !validID(id) {
			return ErrIntegrity
		}
	}
	if l.EventID != e.Event.EventID || l.AuthorizationID != t.ID || t.DeviceID != e.Event.DeviceID || t.ConfigType != e.Event.ConfigType || l.Attestation != domain.ChangeAuthorizationAttestation || t.RequestedBy == t.ApprovedBy || l.LinkedBy == t.RequestedBy || t.RejectedBy != nil || t.RejectedAt != nil {
		return ErrIntegrity
	}
	for _, at := range []string{l.LinkedAt, t.WindowStart, t.WindowEnd, t.RequestedAt, t.ApprovedAt, t.CreatedAt} {
		if !validCanonicalTime(at) {
			return ErrIntegrity
		}
	}
	if t.WindowStart >= t.WindowEnd || t.RequestedAt > t.ApprovedAt || t.ApprovedAt > l.LinkedAt || e.Event.DetectedAt < t.WindowStart || e.Event.DetectedAt >= t.WindowEnd {
		return ErrIntegrity
	}
	if len(t.AllowedCategories) == 0 || len(t.AllowedCategories) > 13 {
		return ErrIntegrity
	}
	for i, c := range t.AllowedCategories {
		if !domain.IsChangeAuthorizationCategory(c) || (i > 0 && c == t.AllowedCategories[i-1]) {
			return ErrIntegrity
		}
	}
	for _, item := range e.Items {
		if !slices.Contains(t.AllowedCategories, item.Category) {
			return ErrIntegrity
		}
	}
	if t.Status == "revoked" {
		if t.RevokedBy == nil || !validID(*t.RevokedBy) || t.RevokedAt == nil || !validCanonicalTime(*t.RevokedAt) || *t.RevokedAt < l.LinkedAt {
			return ErrIntegrity
		}
	} else if t.Status != "approved" || t.RevokedBy != nil || t.RevokedAt != nil {
		return ErrIntegrity
	}
	if len(a.History) < 3 || len(a.History) > 10000 {
		return ErrIntegrity
	}
	ids := map[string]bool{}
	requested, approved, linked, revoked := false, false, false, false
	for _, h := range a.History {
		if !validID(h.ID) || ids[h.ID] || !validID(h.ActorID) || !validCanonicalTime(h.OccurredAt) {
			return ErrIntegrity
		}
		ids[h.ID] = true
		switch h.Action {
		case "requested":
			if requested || h.ActorID != t.RequestedBy || h.OccurredAt != t.RequestedAt || h.EventID != nil {
				return ErrIntegrity
			}
			requested = true
		case "approved":
			if approved || h.ActorID != t.ApprovedBy || h.OccurredAt != t.ApprovedAt || h.EventID != nil {
				return ErrIntegrity
			}
			approved = true
		case "linked":
			if h.EventID == nil || !validID(*h.EventID) {
				return ErrIntegrity
			}
			if *h.EventID == e.Event.EventID {
				if linked || h.ActorID != l.LinkedBy || h.OccurredAt != l.LinkedAt {
					return ErrIntegrity
				}
				linked = true
			}
		case "revoked":
			if revoked || t.RevokedBy == nil || h.ActorID != *t.RevokedBy || h.OccurredAt != *t.RevokedAt || h.EventID != nil {
				return ErrIntegrity
			}
			revoked = true
		default:
			return ErrIntegrity
		}
	}
	if !requested || !approved || !linked || (revoked != (t.Status == "revoked")) {
		return ErrIntegrity
	}
	return nil
}
func validatePolicy(e domain.RiskEvidenceSnapshot) error {
	p := e.Policy
	if p.RecordedStatus == "not_evaluated" {
		if p.Final != nil {
			return ErrIntegrity
		}
		return nil
	}
	f := p.Final
	if (p.RecordedStatus != "pass" && p.RecordedStatus != "violation") || f == nil || f.EventID != e.Event.EventID || f.Result != p.RecordedStatus || f.EngineVersion != 1 || f.Definition.EngineVersion != 1 || f.Definition.DeviceID != e.Event.DeviceID || f.Definition.ConfigType != e.Event.ConfigType || !validCanonicalTime(f.EvaluatedAt) {
		return ErrIntegrity
	}
	for _, id := range []string{f.ID, f.EventID, f.PolicySetID, f.EvaluatedBy} {
		if !validID(id) {
			return ErrIntegrity
		}
	}
	_, hash, err := domain.CanonicalPolicyDefinition(f.Definition)
	if err != nil || hash != f.DefinitionHash || f.TotalItems != len(e.Items) || f.TotalItems == 0 || len(f.Items) != f.TotalItems {
		return ErrIntegrity
	}
	allowed, denied, uncovered := 0, 0, 0
	for i, out := range f.Items {
		item := e.Items[i]
		if out.ItemID != item.ItemID || out.Ordinal != item.Ordinal || out.Category != item.Category || out.Operation != item.Operation {
			return ErrIntegrity
		}
		expected := "uncovered"
		key := ""
		for _, r := range f.Definition.Rules {
			if r.Category == item.Category && r.Operation == item.Operation {
				expected = r.Decision
				key = "policy." + r.Category + "." + r.Operation
			}
		}
		if out.Decision != expected || out.MatchedRuleKey != key {
			return ErrIntegrity
		}
		switch expected {
		case "allow":
			allowed++
		case "deny":
			denied++
		case "uncovered":
			uncovered++
		}
	}
	if f.AllowedCount != allowed || f.DeniedCount != denied || f.UncoveredCount != uncovered {
		return ErrIntegrity
	}
	if (p.RecordedStatus == "pass" && (allowed != f.TotalItems || denied != 0 || uncovered != 0)) || (p.RecordedStatus == "violation" && denied == 0) {
		return ErrIntegrity
	}
	return nil
}

func Evaluate(in domain.RiskEvidenceSnapshot) (domain.RiskEvaluationResult, error) {
	e, _, _, err := CanonicalEvidence(in)
	if err != nil {
		return domain.RiskEvaluationResult{}, err
	}
	r := domain.RiskEvaluationResult{TechnicalImpact: "unknown", EvidenceCompleteness: "complete", ReviewPriority: "normal", TotalItems: len(e.Items), Findings: []domain.RiskFinding{}}
	categories := map[string]bool{}
	add := func(kind, rule, subjectKind, id string, ordinal *int, impact, floor *string) {
		r.Findings = append(r.Findings, domain.RiskFinding{Key: rule + ":" + subjectKind + ":" + id, RuleID: rule, ReasonCode: rule, Kind: kind, Subject: domain.RiskSubject{Kind: subjectKind, ID: id}, ItemOrdinal: ordinal, Impact: impact, PriorityFloor: floor, Parameters: []domain.RiskSafeParameter{}})
		if floor != nil && priorityRank(*floor) > priorityRank(r.ReviewPriority) {
			r.ReviewPriority = *floor
		}
	}
	for _, item := range e.Items {
		tuple, _ := tupleFor(item)
		floor := "normal"
		kind := "technical"
		if tuple.Impact == "unknown" {
			r.UnknownItems++
			r.EvidenceCompleteness = "partial"
			floor = "elevated"
			kind = "uncertainty"
		} else {
			r.KnownItems++
			categories[item.Category] = true
			if r.KnownImpactFloor == nil || impactRank(tuple.Impact) > impactRank(*r.KnownImpactFloor) {
				r.KnownImpactFloor = textPtr(tuple.Impact)
			}
			if tuple.Impact == "high" {
				floor = "elevated"
			}
		}
		n := item.Ordinal
		add(kind, tuple.RuleID, "item", item.ItemID, &n, textPtr(tuple.Impact), textPtr(floor))
	}
	r.DistinctKnownCategories = len(categories)
	if r.TotalItems > 0 && r.UnknownItems == 0 {
		r.TechnicalImpact = *r.KnownImpactFloor
	}
	if r.TotalItems == 0 {
		r.EvidenceCompleteness = "partial"
		add("uncertainty", "risk.evidence.no_items.v1", "event", e.Event.EventID, nil, textPtr("unknown"), textPtr("elevated"))
	}
	if e.Authorization.RecordedStatus == "unaccounted" {
		r.EvidenceCompleteness = "partial"
		add("governance", "risk.gov.unaccounted.v1", "event", e.Event.EventID, nil, nil, textPtr("elevated"))
	} else {
		add("governance", "risk.gov.authorized_recorded.v1", "authorization", e.Authorization.Ticket.ID, nil, nil, nil)
		if e.Authorization.Ticket.Status == "revoked" {
			add("governance", "risk.gov.authorization_revoked.v1", "authorization", e.Authorization.Ticket.ID, nil, nil, textPtr("urgent"))
		}
	}
	switch e.Policy.RecordedStatus {
	case "not_evaluated":
		r.EvidenceCompleteness = "partial"
		add("governance", "risk.gov.policy_not_evaluated.v1", "event", e.Event.EventID, nil, nil, textPtr("elevated"))
	case "pass":
		add("governance", "risk.gov.policy_pass_recorded.v1", "policy", e.Policy.Final.ID, nil, nil, nil)
	case "violation":
		add("governance", "risk.gov.policy_violation.v1", "policy", e.Policy.Final.ID, nil, nil, textPtr("urgent"))
		if e.Policy.Final.UncoveredCount > 0 {
			r.EvidenceCompleteness = "partial"
			add("uncertainty", "risk.evidence.policy_uncovered.v1", "policy", e.Policy.Final.ID, nil, nil, textPtr("urgent"))
		}
	}
	if len(categories) > 1 {
		add("summary", "risk.summary.multiple_categories.v1", "event", e.Event.EventID, nil, nil, nil)
	}
	if r.ReviewPriority == "normal" {
		add("summary", "risk.summary.normal_review.v1", "event", e.Event.EventID, nil, nil, nil)
	}
	rank := map[string]int{"technical": 0, "governance": 1, "uncertainty": 2, "summary": 3}
	slices.SortFunc(r.Findings, func(a, b domain.RiskFinding) int {
		if rank[a.Kind] != rank[b.Kind] {
			return rank[a.Kind] - rank[b.Kind]
		}
		ao, bo := -1, -1
		if a.ItemOrdinal != nil {
			ao = *a.ItemOrdinal
		}
		if b.ItemOrdinal != nil {
			bo = *b.ItemOrdinal
		}
		if ao != bo {
			return ao - bo
		}
		for _, pair := range [][2]string{{a.RuleID, b.RuleID}, {a.Subject.Kind, b.Subject.Kind}, {a.Subject.ID, b.Subject.ID}, {a.ReasonCode, b.ReasonCode}} {
			if cmp := strings.Compare(pair[0], pair[1]); cmp != 0 {
				return cmp
			}
		}
		return 0
	})
	for i := range r.Findings {
		r.Findings[i].Ordinal = i
	}
	return r, nil
}
func CanonicalResult(result domain.RiskEvaluationResult, definitionHash, evidenceHash string) (string, string, error) {
	encoded, err := json.Marshal(result)
	if err != nil {
		return "", "", ErrIntegrity
	}
	// Both hashes bind the full deterministic result, not actor/capture metadata.
	bound, err := json.Marshal(struct {
		DefinitionHash string                      `json:"definition_hash"`
		EvidenceHash   string                      `json:"evidence_hash"`
		Result         domain.RiskEvaluationResult `json:"result"`
	}{definitionHash, evidenceHash, result})
	if err != nil {
		return "", "", ErrIntegrity
	}
	return string(encoded), Digest("aegis-risk-result-v1", bound), nil
}
