package risk

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"company.local/aegis/apps/aegis-api/internal/changes"
	"company.local/aegis/apps/aegis-api/internal/domain"
	"github.com/stretchr/testify/require"
)

func rid(n int) string { return fmt.Sprintf("00000000-0000-4000-8000-%012d", n) }

const instant = "2026-10-01T12:00:00.000000000Z"

func sampleEvidence(tuples ...domain.RiskTechnicalTuple) domain.RiskEvidenceSnapshot {
	e := domain.RiskEvidenceSnapshot{SchemaVersion: 1, Event: domain.RiskEventSnapshot{EventID: rid(1), DeviceID: rid(2), ConfigType: "running", FromBackupID: rid(3), ToBackupID: rid(4), DetectedAt: instant, CreatedAt: instant, AnalysisVersion: 1, ItemCount: len(tuples), AnalyzedAt: textPtr(instant)}, Items: []domain.RiskItemSignal{}, Authorization: domain.RiskAuthorizationObservation{RecordedStatus: "unaccounted", History: []domain.RiskAuthorizationHistory{}}, Policy: domain.RiskPolicyObservation{RecordedStatus: "not_evaluated"}, Eligibility: domain.RiskEligibility{Persistable: true, Reasons: []string{}}}
	for i, t := range tuples {
		e.Items = append(e.Items, domain.RiskItemSignal{ItemID: rid(100 + i), Ordinal: i, Operation: "add", Category: t.Category, Subcategory: t.Subcategory, ClassificationRule: t.ClassificationRule, ClassificationVersion: 1, CreatedAt: instant, ClassifiedAt: textPtr(instant)})
	}
	return e
}
func authorize(e *domain.RiskEvidenceSnapshot, revoked bool) {
	a := domain.RiskAuthorizationObservation{RecordedStatus: "authorized", Link: &domain.RiskAuthorizationLink{EventID: e.Event.EventID, AuthorizationID: rid(200), LinkedBy: rid(902), LinkedAt: "2026-10-01T12:30:00.000000000Z", Attestation: domain.ChangeAuthorizationAttestation}, Ticket: &domain.RiskAuthorizationTicket{ID: rid(200), DeviceID: e.Event.DeviceID, ConfigType: "running", Status: "approved", AllowedCategories: []string{}, WindowStart: "2026-10-01T11:00:00.000000000Z", WindowEnd: "2026-10-01T13:00:00.000000000Z", RequestedBy: rid(900), RequestedAt: "2026-10-01T10:00:00.000000000Z", ApprovedBy: rid(901), ApprovedAt: "2026-10-01T11:00:00.000000000Z", CreatedAt: "2026-10-01T10:00:00.000000000Z"}}
	for _, i := range e.Items {
		if !slices.Contains(a.Ticket.AllowedCategories, i.Category) {
			a.Ticket.AllowedCategories = append(a.Ticket.AllowedCategories, i.Category)
		}
	}
	a.History = []domain.RiskAuthorizationHistory{
		{ID: rid(210), Action: "requested", ActorID: rid(900), OccurredAt: a.Ticket.RequestedAt},
		{ID: rid(211), Action: "approved", ActorID: rid(901), OccurredAt: a.Ticket.ApprovedAt},
		{ID: rid(212), Action: "linked", ActorID: rid(902), OccurredAt: a.Link.LinkedAt, EventID: textPtr(e.Event.EventID)},
	}
	if revoked {
		a.Ticket.Status = "revoked"
		a.Ticket.RevokedBy = textPtr(rid(901))
		a.Ticket.RevokedAt = textPtr("2026-10-01T14:00:00.000000000Z")
		a.History = append(a.History, domain.RiskAuthorizationHistory{ID: rid(213), Action: "revoked", ActorID: rid(901), OccurredAt: *a.Ticket.RevokedAt})
	}
	e.Authorization = a
}
func policy(t *testing.T, e *domain.RiskEvidenceSnapshot, violation, uncovered bool) {
	t.Helper()
	f := &domain.RiskPolicyFinal{ID: rid(300), EventID: e.Event.EventID, PolicySetID: rid(301), EngineVersion: 1, Result: "pass", EvaluatedBy: rid(901), EvaluatedAt: "2026-10-01T13:00:00.000000000Z", TotalItems: len(e.Items), Items: []domain.PolicyItemResult{}, Definition: domain.PolicyDefinition{EngineVersion: 1, DeviceID: e.Event.DeviceID, ConfigType: e.Event.ConfigType, Rules: []domain.PolicyRule{}}}
	seen := map[string]bool{}
	for i, item := range e.Items {
		decision := "allow"
		if violation && i == 0 {
			decision = "deny"
			f.Result = "violation"
		} else if uncovered && i == 1 {
			decision = "uncovered"
		}
		key := ""
		if decision != "uncovered" {
			key = "policy." + item.Category + "." + item.Operation
			if !seen[key] {
				f.Definition.Rules = append(f.Definition.Rules, domain.PolicyRule{Category: item.Category, Operation: item.Operation, Decision: decision})
				seen[key] = true
			}
		}
		f.Items = append(f.Items, domain.PolicyItemResult{ItemID: item.ItemID, Ordinal: i, Category: item.Category, Operation: item.Operation, Decision: decision, MatchedRuleKey: key})
		switch decision {
		case "allow":
			f.AllowedCount++
		case "deny":
			f.DeniedCount++
		case "uncovered":
			f.UncoveredCount++
		}
	}
	var err error
	f.Definition, err = domain.NormalizePolicyDefinition(f.Definition)
	require.NoError(t, err)
	_, f.DefinitionHash, err = domain.CanonicalPolicyDefinition(f.Definition)
	require.NoError(t, err)
	e.Policy = domain.RiskPolicyObservation{RecordedStatus: f.Result, Final: f}
}

func TestRiskAll83ExactTuples(t *testing.T) {
	raw, err := os.ReadFile("testdata/tuples_v1.json")
	require.NoError(t, err)
	var fixtures []struct{ ID, Category, Subcategory, ClassificationRule, RuleID, Impact, Example string }
	// Fixture keys are explicit snake_case; do not rely on Go's underscore matching.
	var records []map[string]string
	require.NoError(t, json.Unmarshal(raw, &records))
	for _, r := range records {
		fixtures = append(fixtures, struct{ ID, Category, Subcategory, ClassificationRule, RuleID, Impact, Example string }{r["id"], r["category"], r["subcategory"], r["classification_rule"], r["rule_id"], r["impact"], r["example"]})
	}
	require.Len(t, fixtures, 83)
	seen := map[string]bool{}
	for _, f := range fixtures {
		t.Run(f.ID, func(t *testing.T) {
			parts := strings.Split(f.Example, " > ")
			command := parts[len(parts)-1]
			parents := parts[:len(parts)-1]
			for _, negate := range []bool{false, true} {
				cmd := command
				if negate {
					cmd = "no " + cmd
				}
				actual := changes.Classify(parents, cmd)
				require.Equal(t, f.Category, actual.Category)
				require.Equal(t, f.Subcategory, actual.Subcategory)
				require.Equal(t, f.ClassificationRule, actual.RuleID)
			}
			tuple := domain.RiskTechnicalTuple{Category: f.Category, Subcategory: f.Subcategory, ClassificationRule: f.ClassificationRule, RuleID: f.RuleID, Impact: f.Impact}
			e := sampleEvidence(tuple)
			for _, op := range []string{"add", "remove"} {
				e.Items[0].Operation = op
				r, err := Evaluate(e)
				require.NoError(t, err)
				require.Equal(t, f.Impact, r.TechnicalImpact)
				var itemFindings []domain.RiskFinding
				for _, finding := range r.Findings {
					if finding.Subject.Kind == "item" {
						itemFindings = append(itemFindings, finding)
					}
				}
				require.Len(t, itemFindings, 1)
				require.Equal(t, f.RuleID, itemFindings[0].RuleID)
			}
			require.False(t, seen[f.ClassificationRule])
			seen[f.ClassificationRule] = true
		})
	}
	require.Len(t, DefinitionV1().TechnicalRules, 83)
}

func TestRiskGovernanceAndUnknownDominance(t *testing.T) {
	low := tuplesV1[25]
	high := tuplesV1[0]
	medium := tuplesV1[23]
	unknown := tuplesV1[40]
	cases := []struct {
		name                                      string
		tuples                                    []domain.RiskTechnicalTuple
		auth, pass, violation, revoked, uncovered bool
		impact, floor, complete, priority         string
	}{
		{"G01", []domain.RiskTechnicalTuple{low}, true, true, false, false, false, "low", "low", "complete", "normal"},
		{"G02", []domain.RiskTechnicalTuple{low}, false, true, false, false, false, "low", "low", "partial", "elevated"},
		{"G03", []domain.RiskTechnicalTuple{low}, true, false, true, false, false, "low", "low", "complete", "urgent"},
		{"G04", []domain.RiskTechnicalTuple{low}, false, false, true, false, false, "low", "low", "partial", "urgent"},
		{"G05", []domain.RiskTechnicalTuple{high}, true, true, false, false, false, "high", "high", "complete", "elevated"},
		{"G06", []domain.RiskTechnicalTuple{high, unknown}, true, true, false, false, false, "unknown", "high", "partial", "elevated"},
		{"G07", nil, false, false, false, false, false, "unknown", "", "partial", "elevated"},
		{"G08", []domain.RiskTechnicalTuple{low}, true, true, false, true, false, "low", "low", "complete", "urgent"},
		{"G09", []domain.RiskTechnicalTuple{medium}, true, false, false, false, false, "medium", "medium", "partial", "elevated"},
		{"uncovered-violation", []domain.RiskTechnicalTuple{low, high}, true, false, true, false, true, "high", "high", "partial", "urgent"},
		{"unknown-only", []domain.RiskTechnicalTuple{unknown}, false, false, false, false, false, "unknown", "", "partial", "elevated"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := sampleEvidence(c.tuples...)
			if c.auth {
				authorize(&e, c.revoked)
			}
			if c.pass || c.violation {
				policy(t, &e, c.violation, c.uncovered)
			}
			before, err := json.Marshal(e)
			require.NoError(t, err)
			r, err := Evaluate(e)
			require.NoError(t, err)
			require.Equal(t, c.impact, r.TechnicalImpact)
			if c.floor == "" {
				require.Nil(t, r.KnownImpactFloor)
			} else {
				require.Equal(t, c.floor, *r.KnownImpactFloor)
			}
			require.Equal(t, c.complete, r.EvidenceCompleteness)
			require.Equal(t, c.priority, r.ReviewPriority)
			after, err := json.Marshal(e)
			require.NoError(t, err)
			require.Equal(t, string(before), string(after))
			for i, f := range r.Findings {
				require.Equal(t, i, f.Ordinal)
			}
		})
	}
}
func TestRiskCanonicalPermutationAndFailures(t *testing.T) {
	e := sampleEvidence(tuplesV1[25], tuplesV1[0])
	authorize(&e, false)
	policy(t, &e, false, false)
	_, canon, hash, err := CanonicalEvidence(e)
	require.NoError(t, err)
	r, err := Evaluate(e)
	require.NoError(t, err)
	result, _, err := CanonicalResult(r, "definition", hash)
	require.NoError(t, err)
	slices.Reverse(e.Items)
	slices.Reverse(e.Authorization.History)
	slices.Reverse(e.Policy.Final.Items)
	_, again, againHash, err := CanonicalEvidence(e)
	require.NoError(t, err)
	require.Equal(t, canon, again)
	require.Equal(t, hash, againHash)
	r2, err := Evaluate(e)
	require.NoError(t, err)
	out, _, err := CanonicalResult(r2, "definition", hash)
	require.NoError(t, err)
	require.Equal(t, result, out)
	for _, mutate := range []func(*domain.RiskEvidenceSnapshot){
		func(e *domain.RiskEvidenceSnapshot) { e.Items[0].ItemID = e.Items[1].ItemID },
		func(e *domain.RiskEvidenceSnapshot) { e.Items[0].Ordinal = 7 },
		func(e *domain.RiskEvidenceSnapshot) { e.Items[0].ClassificationRule = "canary-raw-secret" },
		func(e *domain.RiskEvidenceSnapshot) { e.Event.ItemCount++ },
		func(e *domain.RiskEvidenceSnapshot) { e.Event.AnalyzedAt = nil },
		func(e *domain.RiskEvidenceSnapshot) { e.Event.AnalysisVersion = 0 },
		func(e *domain.RiskEvidenceSnapshot) { e.Items[0].ClassificationVersion = 2 },
		func(e *domain.RiskEvidenceSnapshot) { e.Items[0].Operation = "override" },
		func(e *domain.RiskEvidenceSnapshot) { e.Policy.RecordedStatus = "pass" },
		func(e *domain.RiskEvidenceSnapshot) { e.Authorization.RecordedStatus = "authorized" },
	} {
		bad := sampleEvidence(tuplesV1[25], tuplesV1[0])
		mutate(&bad)
		got, err := Evaluate(bad)
		require.Error(t, err)
		encoded, _ := json.Marshal(got)
		require.NotContains(t, string(encoded), "canary-raw-secret")
		require.NotContains(t, err.Error(), "canary-raw-secret")
	}
	d := DefinitionV1()
	d.TechnicalRules[0].Impact = "low"
	d.AcceptedAnalysisVersions[0] = 999
	require.NotEqual(t, d.TechnicalRules[0], DefinitionV1().TechnicalRules[0])
	require.Equal(t, []int{1}, DefinitionV1().AcceptedAnalysisVersions)
	var decoded domain.RiskEvidenceSnapshot
	require.NoError(t, StrictDecode(canon, &decoded))
	require.Error(t, StrictDecode(strings.Replace(canon, `"schema_version":1`, `"schema_version":1,"schema_version":1`, 1), &decoded))
	require.Error(t, StrictDecode(canon+"{}", &decoded))
}
func TestRiskTimestampABI(t *testing.T) {
	good := map[string]string{"2026-10-01T12:00:00Z": instant, "2026-10-01T15:30:00+03:30": instant, "2026-10-01T12:00:00.000000001Z": "2026-10-01T12:00:00.000000001Z"}
	for raw, want := range good {
		got, err := NormalizeTimestampV1(raw)
		require.NoError(t, err)
		require.Equal(t, want, got)
	}
	for _, raw := range []string{"", "2026-02-30T00:00:00Z", "2026-10-01t12:00:00Z", "2026-10-01T12:00:60Z", "2026-10-01T12:00:00.0000000001Z", "2026-10-01T12:00:00+24:00", "2026-10-01T12:00:00+03:60", "0001-01-01T00:00:00Z", "0000-01-01T12:00:00Z", "9999-12-31T23:59:59-23:59"} {
		_, err := NormalizeTimestampV1(raw)
		require.Error(t, err)
	}
}

func TestRiskCanonicalGoldenV1(t *testing.T) {
	e := sampleEvidence(tuplesV1[25], tuplesV1[0])
	authorize(&e, false)
	policy(t, &e, false, false)
	definition, dh := CanonicalDefinitionV1()
	_, evidence, eh, err := CanonicalEvidence(e)
	require.NoError(t, err)
	r, err := Evaluate(e)
	require.NoError(t, err)
	result, rh, err := CanonicalResult(r, dh, eh)
	require.NoError(t, err)
	for name, data := range map[string]string{"definition": definition, "evidence": evidence, "result": result} {
		expected, err := os.ReadFile("testdata/" + name + "_v1.json")
		require.NoError(t, err)
		golden := strings.TrimSuffix(strings.TrimSuffix(string(expected), "\n"), "\r")
		require.Equal(t, golden, data, "versioned canonical bytes must not drift")
	}
	require.Equal(t, "f90c9840ec0e7e638ef9e369199d6b1959ade4d5c10589aa8b1ad78e18219837", dh)
	require.Equal(t, "113c46233a394893fc4fd01cefe800dec51fe792e1d1e168cf37bb3aa6293ace", eh)
	require.Equal(t, "a3683e702d3faaf4beca413219000d88300398fc93d0ec8147b4e67dfef55450", rh)
}
func FuzzRiskTimestamp(f *testing.F) {
	f.Add("2026-10-01T12:00:00.000000001Z")
	f.Add("not-a-timestamp")
	f.Fuzz(func(t *testing.T, raw string) {
		got, err := NormalizeTimestampV1(raw)
		if err == nil {
			again, err := NormalizeTimestampV1(got)
			require.NoError(t, err)
			require.Equal(t, got, again)
			require.Len(t, got, 30)
		}
	})
}
func FuzzRiskStrictCanonical(f *testing.F) {
	f.Add(`{"technical_impact":"low"}`)
	f.Add(`{"technical_impact":"low","technical_impact":"high"}`)
	f.Fuzz(func(t *testing.T, raw string) {
		if len(raw) > 4<<20 {
			return
		}
		var got domain.RiskEvaluationResult
		if StrictDecode(raw, &got) == nil {
			encoded, err := json.Marshal(got)
			require.NoError(t, err)
			require.Equal(t, raw, string(encoded))
		}
	})
}
