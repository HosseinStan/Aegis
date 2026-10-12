// Package policy implements the isolated, deterministic policy v1 evaluator.
package policy

import (
	"slices"
	"strings"

	"company.local/aegis/apps/aegis-api/internal/changes"
	"company.local/aegis/apps/aegis-api/internal/domain"
)

// ValidateItems validates existing evidence, not policy matching dimensions.
// It never returns command/context material, including on corrupt input.
func ValidateItems(items []domain.ChangeEventItem) (string, error) {
	legacy := false
	ids, ordinals := make(map[string]bool), make(map[int]bool)
	for _, item := range items {
		if item.ID == "" || item.ChangeEventID == "" || item.Ordinal < 0 || ids[item.ID] || ordinals[item.Ordinal] ||
			(item.Operation != "add" && item.Operation != "remove") || strings.TrimSpace(item.Command) == "" {
			return "", domain.ErrValidation
		}
		ids[item.ID], ordinals[item.Ordinal] = true, true
		command, path, sensitive := changes.Sanitize(item.Command, item.ContextPath)
		if command != item.Command || !slices.Equal(path, item.ContextPath) || (sensitive && !item.Sensitive) {
			return "", domain.ErrValidation
		}
		if item.ClassificationVersion == 0 {
			if item.Category != domain.ChangeCategoryUnclassified || item.Subcategory != "unclassified" || item.ClassificationRule != "" || item.ClassifiedAt != nil {
				return "", domain.ErrValidation
			}
			legacy = true
			continue
		}
		classification := changes.Classify(item.ContextPath, item.Command)
		if item.ClassificationVersion != domain.ChangeClassificationVersion || item.ClassifiedAt == nil || item.ClassifiedAt.IsZero() ||
			item.Category != classification.Category || item.Subcategory != classification.Subcategory || item.ClassificationRule != classification.RuleID {
			return "", domain.ErrValidation
		}
	}
	if legacy {
		return domain.PolicyReasonLegacy, nil
	}
	return "", nil
}

// Evaluate preserves input order and never reads event state, identity, clock,
// files, network or storage. Only category/operation select a validated rule.
func Evaluate(definition domain.PolicyDefinition, items []domain.ChangeEventItem) (domain.PolicyEvaluationResult, error) {
	result := domain.PolicyEvaluationResult{Outcome: domain.PolicyStatusNotEvaluated, ItemResults: []domain.PolicyItemResult{}}
	canonical, err := domain.NormalizePolicyDefinition(definition)
	if err != nil {
		return result, err
	}
	reason, err := ValidateItems(items)
	if err != nil {
		return result, err
	}
	if reason != "" {
		result.Reason = reason
		return result, nil
	}
	if len(items) == 0 {
		result.Reason = domain.PolicyReasonInvalid
		return result, nil
	}
	rules := make(map[string]string, len(canonical.Rules))
	for _, rule := range canonical.Rules {
		rules[rule.Category+"."+rule.Operation] = rule.Decision
	}
	for _, item := range items {
		key := item.Category + "." + item.Operation
		decision, matched := rules[key]
		matchedKey := ""
		if matched {
			matchedKey = "policy." + key
		} else {
			decision = domain.PolicyUncovered
		}
		result.ItemResults = append(result.ItemResults, domain.PolicyItemResult{ItemID: item.ID, Ordinal: item.Ordinal,
			Category: item.Category, Operation: item.Operation, Decision: decision, MatchedRuleKey: matchedKey})
		switch decision {
		case domain.PolicyAllow:
			result.AllowedCount++
		case domain.PolicyDeny:
			result.DeniedCount++
		case domain.PolicyUncovered:
			result.UncoveredCount++
		}
	}
	result.TotalItems = len(items)
	switch {
	case result.DeniedCount > 0:
		result.Outcome = domain.PolicyStatusViolation
	case result.UncoveredCount > 0:
		result.Reason = domain.PolicyReasonUncovered
	default:
		result.Outcome = domain.PolicyStatusPass
	}
	return result, nil
}
