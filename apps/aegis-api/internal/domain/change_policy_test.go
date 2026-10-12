package domain

import (
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPolicyRulesAndReferenceValidation(t *testing.T) {
	base := PolicySetInput{Reference: " pol-lab:01 ", ConfigType: "running", Rules: []PolicyRule{{" SYSTEM ", "add", "allow"}, {"aaa", "remove", "deny"}}}
	before := slices.Clone(base.Rules)
	got, err := NormalizePolicyInput(base)
	require.NoError(t, err)
	require.Equal(t, "POL-LAB:01", got.Reference)
	require.Equal(t, []PolicyRule{{"aaa", "remove", "deny"}, {"system", "add", "allow"}}, got.Rules)
	require.Equal(t, before, base.Rules)
	for _, ref := range []string{"", "ab", strings.Repeat("A", 65), "POL\r", "\nPOL", "pol space", "POL/1", "PÖL", "POLK", "POL'; DROP TABLE users;--"} {
		in := base
		in.Reference = ref
		_, err := NormalizePolicyInput(in)
		require.ErrorIs(t, err, ErrValidation, ref)
	}
	for _, rules := range [][]PolicyRule{
		nil, {}, {{"unclassified", "add", "allow"}}, {{"unknown", "add", "allow"}}, {{"*", "add", "allow"}},
		{{"system", "*", "allow"}}, {{"system", "Add", "allow"}}, {{"system", "add", "ALLOW"}}, {{"system", "add", "uncovered"}},
		{{"system", "add", "allow"}, {" SYSTEM ", "add", "allow"}}, {{"system", "add", "allow"}, {"system", "add", "deny"}},
		make([]PolicyRule, 27),
	} {
		in := base
		in.Rules = rules
		_, err := NormalizePolicyInput(in)
		require.ErrorIs(t, err, ErrValidation)
	}
	for _, config := range []string{"", "RUNNING", "candidate", "*"} {
		in := base
		in.ConfigType = config
		_, err := NormalizePolicyInput(in)
		require.ErrorIs(t, err, ErrValidation)
	}
	var rules []PolicyRule
	for _, category := range []string{"system", "interface", "switching", "routing", "access_control", "aaa", "management", "monitoring", "services", "crypto_vpn", "qos", "redundancy", "other"} {
		for _, op := range []string{"add", "remove"} {
			rules = append(rules, PolicyRule{category, op, "allow"})
		}
	}
	base.Rules = rules
	got, err = NormalizePolicyInput(base)
	require.NoError(t, err)
	require.Len(t, got.Rules, 26)
}

func TestPolicyCanonicalDefinitionIdentity(t *testing.T) {
	d, err := NormalizePolicyDefinition(PolicyDefinition{1, "device-1", "running", []PolicyRule{{"system", "remove", "deny"}, {"aaa", "add", "allow"}}})
	require.NoError(t, err)
	json, hash, err := CanonicalPolicyDefinition(d)
	require.NoError(t, err)
	require.Equal(t, `{"engine_version":1,"device_id":"device-1","config_type":"running","rules":[{"category":"aaa","operation":"add","decision":"allow"},{"category":"system","operation":"remove","decision":"deny"}]}`, json)
	require.Regexp(t, `^[a-f0-9]{64}$`, hash)
	reversed := d
	reversed.Rules = slices.Clone(d.Rules)
	slices.Reverse(reversed.Rules)
	normal, err := NormalizePolicyDefinition(reversed)
	require.NoError(t, err)
	json2, hash2, err := CanonicalPolicyDefinition(normal)
	require.NoError(t, err)
	require.Equal(t, json, json2)
	require.Equal(t, hash, hash2)
	_, _, err = CanonicalPolicyDefinition(reversed)
	require.ErrorIs(t, err, ErrValidation)
	for _, mutate := range []func(*PolicyDefinition){
		func(d *PolicyDefinition) { d.DeviceID = "device-2" }, func(d *PolicyDefinition) { d.ConfigType = "startup" },
		func(d *PolicyDefinition) { d.Rules[0].Decision = "deny" },
	} {
		changed := d
		changed.Rules = slices.Clone(d.Rules)
		mutate(&changed)
		_, next, err := CanonicalPolicyDefinition(changed)
		require.NoError(t, err)
		require.NotEqual(t, hash, next)
	}
	for _, version := range []int{0, 2, -1} {
		changed := d
		changed.EngineVersion = version
		_, _, err := CanonicalPolicyDefinition(changed)
		require.ErrorIs(t, err, ErrValidation)
	}
	set := PolicySet{ID: "id", Reference: "POL-1", DeviceID: d.DeviceID, ConfigType: d.ConfigType, Rules: d.Rules, EngineVersion: 1}
	_, identical, err := CanonicalPolicyDefinition(set.Definition())
	require.NoError(t, err)
	require.Equal(t, hash, identical)
	set.ID, set.Reference, set.Status, set.CreatedBy = "other-id", "POL-2", PolicyRetired, "other-user"
	_, identical, err = CanonicalPolicyDefinition(set.Definition())
	require.NoError(t, err)
	require.Equal(t, hash, identical)
}
