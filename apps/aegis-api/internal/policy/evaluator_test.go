package policy

import (
	"encoding/json"
	"slices"
	"testing"
	"time"

	"company.local/aegis/apps/aegis-api/internal/changes"
	"company.local/aegis/apps/aegis-api/internal/domain"
	"github.com/stretchr/testify/require"
)

func policyItem(id, command, operation string, ordinal int, path ...string) domain.ChangeEventItem {
	command, path, sensitive := changes.Sanitize(command, path)
	c := changes.Classify(path, command)
	at := time.Date(2026, 10, 10, 10, 0, 0, 0, time.UTC)
	return domain.ChangeEventItem{ID: id, ChangeEventID: "event", Ordinal: ordinal, Command: command, ContextPath: path, Sensitive: sensitive,
		Operation: operation, ClassificationVersion: 1, Category: c.Category, Subcategory: c.Subcategory, ClassificationRule: c.RuleID, ClassifiedAt: &at}
}

func TestEvaluateOutcomesAndImmutableOrder(t *testing.T) {
	items := []domain.ChangeEventItem{policyItem("one", "hostname LAB", "add", 1), policyItem("two", "enable secret 0 private-secret", "remove", 0)}
	before, err := json.Marshal(items)
	require.NoError(t, err)
	for _, tc := range []struct {
		name                   string
		rules                  []domain.PolicyRule
		outcome, reason        string
		allow, deny, uncovered int
	}{
		{"all allowed", []domain.PolicyRule{{Category: "system", Operation: "add", Decision: "allow"}, {Category: "aaa", Operation: "remove", Decision: "allow"}}, "pass", "", 2, 0, 0},
		{"deny", []domain.PolicyRule{{Category: "system", Operation: "add", Decision: "allow"}, {Category: "aaa", Operation: "remove", Decision: "deny"}}, "violation", "", 1, 1, 0},
		{"deny and uncovered", []domain.PolicyRule{{Category: "aaa", Operation: "remove", Decision: "deny"}}, "violation", "", 0, 1, 1},
		{"partial allow", []domain.PolicyRule{{Category: "system", Operation: "add", Decision: "allow"}}, "not_evaluated", "uncovered_items", 1, 0, 1},
		{"wrong operation", []domain.PolicyRule{{Category: "system", Operation: "remove", Decision: "allow"}}, "not_evaluated", "uncovered_items", 0, 0, 2},
		{"other is not wildcard", []domain.PolicyRule{{Category: "other", Operation: "add", Decision: "allow"}}, "not_evaluated", "uncovered_items", 0, 0, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			definition := domain.PolicyDefinition{EngineVersion: 1, DeviceID: "device", ConfigType: "running", Rules: tc.rules}
			result, err := Evaluate(definition, items)
			require.NoError(t, err)
			require.Equal(t, tc.outcome, result.Outcome)
			require.Equal(t, tc.reason, result.Reason)
			require.Equal(t, 2, result.TotalItems)
			require.Equal(t, tc.allow, result.AllowedCount)
			require.Equal(t, tc.deny, result.DeniedCount)
			require.Equal(t, tc.uncovered, result.UncoveredCount)
			require.Equal(t, "one", result.ItemResults[0].ItemID)
			require.Equal(t, 1, result.ItemResults[0].Ordinal)
			for _, r := range result.ItemResults {
				if r.Decision == "uncovered" {
					require.Empty(t, r.MatchedRuleKey)
				} else {
					require.Equal(t, "policy."+r.Category+"."+r.Operation, r.MatchedRuleKey)
				}
			}
			reversed := definition
			reversed.Rules = slices.Clone(tc.rules)
			slices.Reverse(reversed.Rules)
			again, err := Evaluate(reversed, items)
			require.NoError(t, err)
			require.Equal(t, result, again)
			after, err := json.Marshal(items)
			require.NoError(t, err)
			require.Equal(t, before, after)
			encoded, err := json.Marshal(result)
			require.NoError(t, err)
			require.NotContains(t, string(encoded), "private-secret")
			require.NotContains(t, string(encoded), "command")
		})
	}
}

func TestEvaluateAllCategoryOperationPairs(t *testing.T) {
	commands := map[string]string{"system": "hostname LAB", "interface": "interface Gi1", "switching": "vlan 20", "routing": "router bgp 65000",
		"access_control": "access-list 100 permit ip any any", "aaa": "aaa new-model", "management": "ip ssh version 2", "monitoring": "logging host 192.0.2.1",
		"services": "service dhcp", "crypto_vpn": "crypto isakmp policy 10", "qos": "policy-map WAN", "redundancy": "standby 10 ip 192.0.2.1", "other": "future-command"}
	for category, command := range commands {
		for _, operation := range []string{"add", "remove"} {
			t.Run(category+operation, func(t *testing.T) {
				item := policyItem("one", command, operation, 0)
				require.Equal(t, category, item.Category)
				for _, decision := range []string{"allow", "deny"} {
					result, err := Evaluate(domain.PolicyDefinition{EngineVersion: 1, DeviceID: "device", ConfigType: "running", Rules: []domain.PolicyRule{{Category: category, Operation: operation, Decision: decision}}}, []domain.ChangeEventItem{item})
					require.NoError(t, err)
					require.Equal(t, decision, result.ItemResults[0].Decision)
				}
			})
		}
	}
}

func TestEvaluateInvalidAndLegacyEvidence(t *testing.T) {
	d := domain.PolicyDefinition{EngineVersion: 1, DeviceID: "device", ConfigType: "running", Rules: []domain.PolicyRule{{Category: "system", Operation: "add", Decision: "allow"}}}
	item := policyItem("one", "hostname LAB", "add", 0)
	for name, mutate := range map[string]func(*domain.ChangeEventItem){
		"operation": func(i *domain.ChangeEventItem) { i.Operation = "replace" }, "id": func(i *domain.ChangeEventItem) { i.ID = "" },
		"ordinal": func(i *domain.ChangeEventItem) { i.Ordinal = -1 }, "category": func(i *domain.ChangeEventItem) { i.Category = "aaa" },
		"subcategory": func(i *domain.ChangeEventItem) { i.Subcategory = "forged" }, "rule": func(i *domain.ChangeEventItem) { i.ClassificationRule = "forged" },
		"unsupported version": func(i *domain.ChangeEventItem) { i.ClassificationVersion = 2 }, "missing time": func(i *domain.ChangeEventItem) { i.ClassifiedAt = nil },
		"zero time": func(i *domain.ChangeEventItem) { at := time.Time{}; i.ClassifiedAt = &at }, "raw command": func(i *domain.ChangeEventItem) { i.Command = "enable secret raw-secret" },
		"raw context": func(i *domain.ChangeEventItem) { i.ContextPath = []string{"enable secret raw-secret"} }, "empty command": func(i *domain.ChangeEventItem) { i.Command = " " },
	} {
		t.Run(name, func(t *testing.T) {
			bad := item
			mutate(&bad)
			_, err := Evaluate(d, []domain.ChangeEventItem{bad})
			require.ErrorIs(t, err, domain.ErrValidation)
		})
	}
	_, err := Evaluate(d, []domain.ChangeEventItem{item, item})
	require.ErrorIs(t, err, domain.ErrValidation)
	legacy := item
	legacy.ClassificationVersion = 0
	legacy.Category = "unclassified"
	legacy.Subcategory = "unclassified"
	legacy.ClassificationRule = ""
	legacy.ClassifiedAt = nil
	result, err := Evaluate(d, []domain.ChangeEventItem{legacy})
	require.NoError(t, err)
	require.Equal(t, "legacy_classification", result.Reason)
	require.Equal(t, "not_evaluated", result.Outcome)
	result, err = Evaluate(d, nil)
	require.NoError(t, err)
	require.Equal(t, "invalid_evidence", result.Reason)
	other := policyItem("other", "future-command", "add", 0)
	result, err = Evaluate(domain.PolicyDefinition{EngineVersion: 1, DeviceID: "device", ConfigType: "running", Rules: []domain.PolicyRule{{Category: "other", Operation: "add", Decision: "allow"}}}, []domain.ChangeEventItem{other})
	require.NoError(t, err)
	require.Equal(t, "pass", result.Outcome)
}

func FuzzEvaluateDeterministic(f *testing.F) {
	for _, seed := range []string{"hostname LAB", "enable secret raw-secret", "router bgp 65000", "", "\x00", "ip route 192.0.2.0"} {
		f.Add(seed, uint8(0))
	}
	f.Fuzz(func(t *testing.T, command string, operation uint8) {
		if len(command) > 4096 {
			t.Skip()
		}
		op := "add"
		if operation%2 != 0 {
			op = "remove"
		}
		item := policyItem("id", command, op, 0)
		d := domain.PolicyDefinition{EngineVersion: 1, DeviceID: "device", ConfigType: "running", Rules: []domain.PolicyRule{{Category: "system", Operation: op, Decision: "allow"}}}
		before, _ := json.Marshal(item)
		a, ea := Evaluate(d, []domain.ChangeEventItem{item})
		b, eb := Evaluate(d, []domain.ChangeEventItem{item})
		require.Equal(t, ea, eb)
		require.Equal(t, a, b)
		after, _ := json.Marshal(item)
		require.Equal(t, before, after)
	})
}
