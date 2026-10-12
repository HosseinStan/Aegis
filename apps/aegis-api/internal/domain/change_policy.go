package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strings"
	"time"
)

const (
	ChangePolicyEngineVersion = 1
	PolicyDraft               = "draft"
	PolicyActive              = "active"
	PolicyRetired             = "retired"
	PolicyAllow               = "allow"
	PolicyDeny                = "deny"
	PolicyUncovered           = "uncovered"
	PermissionPolicyRead      = "changes.policy.read"
	PermissionPolicyManage    = "changes.policy.manage"
	PermissionPolicyEvaluate  = "changes.policy.evaluate"
	PolicyReasonNoActive      = "no_active_policy"
	PolicyReasonUncovered     = "uncovered_items"
	PolicyReasonLegacy        = "legacy_classification"
	PolicyReasonInvalid       = "invalid_evidence"
)

type PolicyRule struct {
	Category  string `json:"category"`
	Operation string `json:"operation"`
	Decision  string `json:"decision"`
}

type PolicySetInput struct {
	Reference  string       `json:"reference"`
	ConfigType string       `json:"config_type"`
	Rules      []PolicyRule `json:"rules"`
}

// Field order is part of the v1 canonical definition encoding. Lifecycle and
// identity metadata are deliberately excluded from this reproducible digest.
type PolicyDefinition struct {
	EngineVersion int          `json:"engine_version"`
	DeviceID      string       `json:"device_id"`
	ConfigType    string       `json:"config_type"`
	Rules         []PolicyRule `json:"rules"`
}

type PolicySet struct {
	ID             string       `json:"id"`
	Reference      string       `json:"reference"`
	DeviceID       string       `json:"device_id"`
	ConfigType     string       `json:"config_type"`
	Status         string       `json:"status"`
	EngineVersion  int          `json:"engine_version"`
	Rules          []PolicyRule `json:"rules"`
	DefinitionHash string       `json:"definition_hash"`
	CreatedBy      string       `json:"created_by"`
	CreatedAt      time.Time    `json:"created_at"`
	ActivatedBy    string       `json:"activated_by,omitempty"`
	ActivatedAt    *time.Time   `json:"activated_at,omitempty"`
	RetiredBy      string       `json:"retired_by,omitempty"`
	RetiredAt      *time.Time   `json:"retired_at,omitempty"`
	UpdatedAt      time.Time    `json:"updated_at"`
}

func (p PolicySet) Definition() PolicyDefinition {
	return PolicyDefinition{p.EngineVersion, p.DeviceID, p.ConfigType, slices.Clone(p.Rules)}
}

func NormalizePolicyInput(in PolicySetInput) (PolicySetInput, error) {
	if strings.ContainsAny(in.Reference, "\r\n") {
		return in, ErrValidation
	}
	in.Reference = strings.TrimSpace(in.Reference)
	if len(in.Reference) < 3 || len(in.Reference) > 64 {
		return in, ErrValidation
	}
	for _, c := range in.Reference {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("._:-", c)) {
			return in, ErrValidation
		}
	}
	in.Reference = strings.ToUpper(in.Reference)
	definition, err := NormalizePolicyDefinition(PolicyDefinition{ChangePolicyEngineVersion, "scope", in.ConfigType, in.Rules})
	in.Rules = definition.Rules
	return in, err
}

func NormalizePolicyDefinition(d PolicyDefinition) (PolicyDefinition, error) {
	if d.EngineVersion != ChangePolicyEngineVersion || strings.TrimSpace(d.DeviceID) == "" ||
		(d.ConfigType != "running" && d.ConfigType != "startup") || len(d.Rules) < 1 || len(d.Rules) > 26 {
		return PolicyDefinition{}, ErrValidation
	}
	d.Rules = slices.Clone(d.Rules)
	seen := make(map[string]bool, len(d.Rules))
	for i, r := range d.Rules {
		r.Category = strings.ToLower(strings.TrimSpace(r.Category))
		if !IsChangeAuthorizationCategory(r.Category) || (r.Operation != "add" && r.Operation != "remove") ||
			(r.Decision != PolicyAllow && r.Decision != PolicyDeny) || seen[r.Category+"."+r.Operation] {
			return PolicyDefinition{}, ErrValidation
		}
		seen[r.Category+"."+r.Operation] = true
		d.Rules[i] = r
	}
	slices.SortFunc(d.Rules, func(a, b PolicyRule) int {
		if cmp := strings.Compare(a.Category, b.Category); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.Operation, b.Operation)
	})
	return d, nil
}

// CanonicalPolicyDefinition does not repair stored definitions. Writers first
// normalize input; readers reject noncanonical or unsupported stored values.
func CanonicalPolicyDefinition(d PolicyDefinition) (string, string, error) {
	normalized, err := NormalizePolicyDefinition(d)
	if err != nil || !slices.Equal(d.Rules, normalized.Rules) {
		return "", "", ErrValidation
	}
	data, err := json.Marshal(normalized)
	if err != nil {
		return "", "", ErrValidation
	}
	hash := sha256.Sum256(data)
	return string(data), hex.EncodeToString(hash[:]), nil
}

type PolicyItemResult struct {
	ItemID         string `json:"item_id"`
	Ordinal        int    `json:"ordinal"`
	Category       string `json:"category"`
	Operation      string `json:"operation"`
	Decision       string `json:"decision"`
	MatchedRuleKey string `json:"matched_rule_key"`
}

type PolicyEvaluationResult struct {
	Outcome        string             `json:"outcome"`
	ItemResults    []PolicyItemResult `json:"item_results"`
	TotalItems     int                `json:"total_items"`
	AllowedCount   int                `json:"allowed_count"`
	DeniedCount    int                `json:"denied_count"`
	UncoveredCount int                `json:"uncovered_count"`
	Reason         string             `json:"reason,omitempty"`
}

type ChangePolicyEvaluation struct {
	ID                       string    `json:"id"`
	EventID                  string    `json:"event_id"`
	PolicySetID              string    `json:"policy_set_id"`
	EngineVersion            int       `json:"engine_version"`
	PolicyDefinitionHash     string    `json:"policy_definition_hash"`
	PolicyDefinitionSnapshot string    `json:"policy_definition_snapshot"`
	Result                   string    `json:"result"`
	TotalItems               int       `json:"total_items"`
	AllowedCount             int       `json:"allowed_count"`
	DeniedCount              int       `json:"denied_count"`
	UncoveredCount           int       `json:"uncovered_count"`
	EvaluatedBy              string    `json:"evaluated_by"`
	EvaluatedAt              time.Time `json:"evaluated_at"`
}

type ChangePolicyEvaluationItem struct {
	EvaluationID      string `json:"evaluation_id"`
	EventID           string `json:"event_id"`
	ChangeEventItemID string `json:"change_event_item_id"`
	Ordinal           int    `json:"ordinal"`
	Category          string `json:"category"`
	Operation         string `json:"operation"`
	Decision          string `json:"decision"`
	MatchedRuleKey    string `json:"matched_rule_key"`
}

type PolicyEvaluationPreview struct {
	EventID              string `json:"event_id"`
	PolicySetID          string `json:"policy_set_id,omitempty"`
	PolicyDefinitionHash string `json:"policy_definition_hash,omitempty"`
	PolicyEvaluationResult
	ExistingEvaluation *ChangePolicyEvaluation `json:"existing_evaluation,omitempty"`
}
