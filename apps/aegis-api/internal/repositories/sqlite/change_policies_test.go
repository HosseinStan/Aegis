package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"company.local/aegis/apps/aegis-api/internal/domain"
	"company.local/aegis/apps/aegis-api/internal/policy"
	"company.local/aegis/apps/aegis-api/internal/repositories"
	"company.local/aegis/apps/aegis-api/migrations"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

var completePolicyRules = []domain.PolicyRule{{Category: "aaa", Operation: "add", Decision: "allow"}, {Category: "interface", Operation: "remove", Decision: "allow"}}

func (f authorizationFixture) policy(t *testing.T, rules []domain.PolicyRule) domain.PolicySet {
	t.Helper()
	d, err := domain.NormalizePolicyDefinition(domain.PolicyDefinition{EngineVersion: 1, DeviceID: f.device, ConfigType: "running", Rules: rules})
	require.NoError(t, err)
	_, hash, err := domain.CanonicalPolicyDefinition(d)
	require.NoError(t, err)
	p := domain.PolicySet{ID: uuid.NewString(), Reference: "POL-" + strings.ToUpper(uuid.NewString()), DeviceID: f.device, ConfigType: "running", Status: "draft",
		EngineVersion: 1, Rules: d.Rules, DefinitionHash: hash, CreatedBy: f.approver, CreatedAt: f.now, UpdatedAt: f.now}
	require.NoError(t, f.s.CreatePolicySet(context.Background(), p))
	return p
}
func (f authorizationFixture) activatePolicy(t *testing.T, p domain.PolicySet, at time.Time) domain.PolicySet {
	t.Helper()
	got, err := f.s.ActivatePolicySet(context.Background(), p.DeviceID, p.ID, f.approver, at)
	require.NoError(t, err)
	return got
}

func TestPolicyLifecycleScopeAndConstraints(t *testing.T) {
	f := authorizationStore(t)
	ctx := context.Background()
	p := f.policy(t, completePolicyRules)
	got, err := f.s.GetPolicySet(ctx, f.device, p.ID)
	require.NoError(t, err)
	require.Equal(t, p, got)
	_, err = f.s.GetPolicySet(ctx, f.otherDevice, p.ID)
	require.ErrorIs(t, err, domain.ErrNotFound)
	_, err = f.s.RetirePolicySet(ctx, f.device, p.ID, f.approver, f.now)
	require.ErrorIs(t, err, domain.ErrConflict)
	duplicate := p
	duplicate.ID = uuid.NewString()
	require.ErrorIs(t, f.s.CreatePolicySet(ctx, duplicate), domain.ErrConflict)
	duplicate.Reference = strings.ToLower(p.Reference)
	require.ErrorIs(t, f.s.CreatePolicySet(ctx, duplicate), domain.ErrValidation)
	bad := p
	bad.ID = uuid.NewString()
	bad.Reference = "POL-NEW"
	bad.DefinitionHash = strings.Repeat("0", 64)
	require.ErrorIs(t, f.s.CreatePolicySet(ctx, bad), domain.ErrValidation)
	p = f.activatePolicy(t, p, f.now.Add(time.Hour))
	_, err = f.s.ActivatePolicySet(ctx, f.device, p.ID, f.approver, f.now.Add(time.Hour))
	require.ErrorIs(t, err, domain.ErrConflict)
	next := f.policy(t, completePolicyRules)
	_, err = f.s.ActivatePolicySet(ctx, f.device, next.ID, f.approver, f.now.Add(time.Hour))
	require.ErrorIs(t, err, domain.ErrConflict)
	// Independent device/configuration scopes can each have their own active policy.
	for _, scope := range []struct{ device, config string }{{f.device, "startup"}, {f.otherDevice, "running"}} {
		copy := next
		copy.ID = uuid.NewString()
		copy.Reference = "POL-" + strings.ToUpper(uuid.NewString())
		copy.DeviceID, copy.ConfigType = scope.device, scope.config
		_, copy.DefinitionHash, err = domain.CanonicalPolicyDefinition(copy.Definition())
		require.NoError(t, err)
		require.NoError(t, f.s.CreatePolicySet(ctx, copy))
		f.activatePolicy(t, copy, f.now.Add(time.Hour))
	}
	for _, query := range []string{`UPDATE change_policy_sets SET reference='POL-EDIT' WHERE id=?`, `UPDATE change_policy_sets SET rules_json='[]' WHERE id=?`,
		`UPDATE change_policy_sets SET definition_hash=? WHERE id=?`, `DELETE FROM change_policy_sets WHERE id=?`, `UPDATE change_policy_sets SET status='draft' WHERE id=?`} {
		args := []any{p.ID}
		if strings.Contains(query, "definition_hash") {
			args = []any{strings.Repeat("0", 64), p.ID}
		}
		_, err = f.s.db.Exec(query, args...)
		require.Error(t, err)
	}
	retired, err := f.s.RetirePolicySet(ctx, f.device, p.ID, f.approver, f.now.Add(2*time.Hour))
	require.NoError(t, err)
	require.Equal(t, p.Rules, retired.Rules)
	require.Equal(t, p.ActivatedAt, retired.ActivatedAt)
	require.Equal(t, p.DefinitionHash, retired.DefinitionHash)
	_, err = f.s.ActivatePolicySet(ctx, f.device, p.ID, f.approver, f.now.Add(3*time.Hour))
	require.ErrorIs(t, err, domain.ErrConflict)
	_, err = f.s.RetirePolicySet(ctx, f.device, p.ID, f.approver, f.now.Add(3*time.Hour))
	require.ErrorIs(t, err, domain.ErrConflict)
	f.activatePolicy(t, next, f.now.Add(3*time.Hour))
	list, total, err := f.s.ListPolicySets(ctx, f.device, repositories.PolicySetFilter{ConfigType: "running"}, repositories.Page{Limit: 1, Offset: 20})
	require.NoError(t, err)
	require.Empty(t, list)
	require.Equal(t, 2, total)
	_, _, err = f.s.ListPolicySets(ctx, f.device, repositories.PolicySetFilter{Status: "unknown"}, repositories.Page{Limit: 20})
	require.ErrorIs(t, err, domain.ErrValidation)
	_, _, err = f.s.ListPolicySets(ctx, f.device, repositories.PolicySetFilter{}, repositories.Page{Limit: 201})
	require.ErrorIs(t, err, domain.ErrValidation)
}

func TestPolicyFinalEvaluationHistoricalEvidenceAndAuthorizationIndependence(t *testing.T) {
	for _, authorized := range []bool{false, true} {
		for _, deny := range []bool{false, true} {
			name := map[bool]string{false: "unaccounted", true: "authorized"}[authorized] + map[bool]string{false: "-pass", true: "-violation"}[deny]
			t.Run(name, func(t *testing.T) {
				f := authorizationStore(t)
				ctx := context.Background()
				rules := []domain.PolicyRule{{Category: "aaa", Operation: "add", Decision: "allow"}, {Category: "interface", Operation: "remove", Decision: "allow"}}
				if deny {
					rules[0].Decision = "deny"
				}
				p := f.activatePolicy(t, f.policy(t, rules), f.now.Add(time.Hour))
				e := authorizationEvent(t, f.s, f.device)
				if authorized {
					a := f.approve(t, f.request(t))
					require.NoError(t, f.s.LinkChangeEventAuthorization(ctx, f.device, f.link(a, e)))
				}
				before, err := f.s.GetChangeEvent(ctx, e.ID)
				require.NoError(t, err)
				var historyBefore int
				require.NoError(t, f.s.db.QueryRow(`SELECT COUNT(*) FROM change_authorization_history`).Scan(&historyBefore))
				for range 2 {
					preview, err := f.s.PreviewPolicyEvaluation(ctx, f.device, e.ID, f.approver)
					require.NoError(t, err)
					require.Equal(t, p.ID, preview.PolicySetID)
					require.Nil(t, preview.ExistingEvaluation)
				}
				after, err := f.s.GetChangeEvent(ctx, e.ID)
				require.NoError(t, err)
				require.Equal(t, before, after)
				evidence, err := f.s.PersistPolicyEvaluation(ctx, f.device, e.ID, f.approver, f.now.Add(4*time.Hour))
				require.NoError(t, err)
				want := "pass"
				if deny {
					want = "violation"
				}
				require.Equal(t, want, evidence.Result)
				require.Equal(t, 2, evidence.TotalItems)
				after, err = f.s.GetChangeEvent(ctx, e.ID)
				require.NoError(t, err)
				require.Equal(t, want, after.PolicyStatus)
				require.Equal(t, before.AuthorizationStatus, after.AuthorizationStatus)
				before.PolicyStatus = want
				before.UpdatedAt = evidence.EvaluatedAt
				require.Equal(t, before, after)
				loaded, err := f.s.GetPolicyEvaluation(ctx, f.device, e.ID)
				require.NoError(t, err)
				require.Equal(t, evidence, loaded)
				itemEvidence, total, err := f.s.ListPolicyEvaluationItems(ctx, f.device, e.ID, repositories.Page{Limit: 200})
				require.NoError(t, err)
				require.Equal(t, 2, total)
				require.Len(t, itemEvidence, 2)
				original, _, err := f.s.ListChangeEventItems(ctx, e.ID, repositories.Page{Limit: 200})
				require.NoError(t, err)
				var definition domain.PolicyDefinition
				require.NoError(t, json.Unmarshal([]byte(evidence.PolicyDefinitionSnapshot), &definition))
				result, err := policy.Evaluate(definition, original)
				require.NoError(t, err)
				require.Equal(t, evidence.Result, result.Outcome)
				for i, item := range itemEvidence {
					require.Equal(t, i, item.Ordinal)
					require.Equal(t, original[i].ID, item.ChangeEventItemID)
					require.Equal(t, result.ItemResults[i].Decision, item.Decision)
				}
				_, err = f.s.PersistPolicyEvaluation(ctx, f.device, e.ID, f.approver, f.now.Add(5*time.Hour))
				require.ErrorIs(t, err, domain.ErrConflict)
				preview, err := f.s.PreviewPolicyEvaluation(ctx, f.device, e.ID, f.approver)
				require.NoError(t, err)
				require.Equal(t, &evidence, preview.ExistingEvaluation)
				_, err = f.s.RetirePolicySet(ctx, f.device, p.ID, f.approver, f.now.Add(5*time.Hour))
				require.NoError(t, err)
				preview, err = f.s.PreviewPolicyEvaluation(ctx, f.device, e.ID, f.approver)
				require.NoError(t, err)
				require.Equal(t, "no_active_policy", preview.Reason)
				require.Equal(t, &evidence, preview.ExistingEvaluation)
				loaded, err = f.s.GetPolicyEvaluation(ctx, f.device, e.ID)
				require.NoError(t, err)
				require.Equal(t, evidence, loaded)
				itemsAfter, _, err := f.s.ListPolicyEvaluationItems(ctx, f.device, e.ID, repositories.Page{Limit: 200})
				require.NoError(t, err)
				require.Equal(t, itemEvidence, itemsAfter)
				var historyAfter int
				require.NoError(t, f.s.db.QueryRow(`SELECT COUNT(*) FROM change_authorization_history`).Scan(&historyAfter))
				require.Equal(t, historyBefore, historyAfter)
				for _, query := range []string{`DELETE FROM change_policy_evaluations WHERE event_id=?`, `UPDATE change_policy_evaluations SET result='pass' WHERE event_id=?`,
					`DELETE FROM change_policy_evaluation_items WHERE event_id=?`, `UPDATE change_policy_evaluation_items SET decision='deny' WHERE event_id=?`,
					`DELETE FROM change_events WHERE id=?`, `DELETE FROM change_event_items WHERE change_event_id=?`} {
					_, err = f.s.db.Exec(query, e.ID)
					require.Error(t, err)
				}
				for _, query := range []string{`DELETE FROM devices WHERE id=?`, `DELETE FROM users WHERE id=?`} {
					id := f.device
					if strings.Contains(query, "users") {
						id = f.approver
					}
					_, err = f.s.db.Exec(query, id)
					require.Error(t, err)
				}
				_, err = f.s.GetPolicyEvaluation(ctx, f.otherDevice, e.ID)
				require.ErrorIs(t, err, domain.ErrNotFound)
			})
		}
	}
}

func TestPolicyIncompleteAndCorruptEvidenceFailsClosed(t *testing.T) {
	for _, scenario := range []string{"no-policy", "draft", "uncovered", "wrong-config", "wrong-device", "retroactive", "retired", "legacy-items", "legacy-analysis", "zero-items", "missing-item",
		"count", "ordinal", "operation", "class-version", "class-time", "category", "subcategory", "class-rule", "raw-command", "raw-context", "analysis-version", "analysis-time", "hash", "rules"} {
		t.Run(scenario, func(t *testing.T) {
			f := authorizationStore(t)
			ctx := context.Background()
			e := authorizationEvent(t, f.s, f.device)
			rules := completePolicyRules
			if scenario == "uncovered" {
				rules = []domain.PolicyRule{{Category: "interface", Operation: "remove", Decision: "allow"}}
			}
			if scenario != "no-policy" {
				p := f.policy(t, rules)
				if scenario == "wrong-config" || scenario == "wrong-device" {
					// Create a separate, correctly hashed definition in another scope.
					p.ID = uuid.NewString()
					p.Reference = "POL-" + strings.ToUpper(uuid.NewString())
					if scenario == "wrong-config" {
						p.ConfigType = "startup"
					} else {
						p.DeviceID = f.otherDevice
					}
					_, p.DefinitionHash, _ = domain.CanonicalPolicyDefinition(p.Definition())
					require.NoError(t, f.s.CreatePolicySet(ctx, p))
				}
				if scenario != "draft" {
					at := f.now.Add(time.Hour)
					if scenario == "retroactive" {
						at = e.DetectedAt.Add(time.Nanosecond)
					}
					p = f.activatePolicy(t, p, at)
					if scenario == "retired" {
						_, err := f.s.RetirePolicySet(ctx, p.DeviceID, p.ID, f.approver, f.now.Add(2*time.Hour))
						require.NoError(t, err)
					}
				}
				if scenario == "hash" || scenario == "rules" {
					_, err := f.s.db.Exec(`DROP TRIGGER change_policy_definition_immutable`)
					require.NoError(t, err)
					query := `UPDATE change_policy_sets SET definition_hash=? WHERE id=?`
					value := strings.Repeat("0", 64)
					if scenario == "rules" {
						query = `UPDATE change_policy_sets SET rules_json=? WHERE id=?`
						value = `[{"category":"aaa","operation":"add","decision":"allow","wildcard":true}]`
					}
					_, err = f.s.db.Exec(query, value, p.ID)
					require.NoError(t, err)
				}
			}
			queries := map[string]string{
				"missing-item":    `DELETE FROM change_event_items WHERE change_event_id=? AND ordinal=1`,
				"legacy-items":    `UPDATE change_event_items SET classification_version=0,category='unclassified',subcategory='unclassified',classification_rule='',classified_at=NULL WHERE change_event_id=?`,
				"legacy-analysis": `UPDATE change_events SET analysis_version=0,analyzed_at=NULL,item_count=0 WHERE id=?`,
				"zero-items":      `UPDATE change_events SET item_count=0 WHERE id=?`, "count": `UPDATE change_events SET item_count=3 WHERE id=?`,
				"ordinal":          `UPDATE change_event_items SET ordinal=5 WHERE change_event_id=? AND ordinal=1`,
				"operation":        `UPDATE change_event_items SET operation='replace' WHERE change_event_id=?`,
				"class-version":    `UPDATE change_event_items SET classification_version=2 WHERE change_event_id=?`,
				"class-time":       `UPDATE change_event_items SET classified_at=NULL WHERE change_event_id=?`,
				"category":         `UPDATE change_event_items SET category='system' WHERE change_event_id=?`,
				"subcategory":      `UPDATE change_event_items SET subcategory='forged' WHERE change_event_id=?`,
				"class-rule":       `UPDATE change_event_items SET classification_rule='forged' WHERE change_event_id=?`,
				"raw-command":      `UPDATE change_event_items SET command='enable secret TOP-SECRET' WHERE change_event_id=?`,
				"raw-context":      `UPDATE change_event_items SET context_path_json='["enable secret TOP-SECRET"]' WHERE change_event_id=?`,
				"analysis-version": `UPDATE change_events SET analysis_version=2 WHERE id=?`, "analysis-time": `UPDATE change_events SET analyzed_at=NULL WHERE id=?`,
			}
			if query := queries[scenario]; query != "" {
				if scenario == "operation" {
					_, err := f.s.db.Exec(`PRAGMA ignore_check_constraints=ON`)
					require.NoError(t, err)
				}
				if scenario == "zero-items" || scenario == "legacy-analysis" {
					_, err := f.s.db.Exec(`DELETE FROM change_event_items WHERE change_event_id=?`, e.ID)
					require.NoError(t, err)
				}
				_, err := f.s.db.Exec(query, e.ID)
				require.NoError(t, err)
			}
			before, err := f.s.GetChangeEvent(ctx, e.ID)
			require.NoError(t, err)
			preview, err := f.s.PreviewPolicyEvaluation(ctx, f.device, e.ID, f.approver)
			incomplete := map[string]string{"no-policy": "no_active_policy", "draft": "no_active_policy", "uncovered": "uncovered_items", "wrong-config": "no_active_policy", "wrong-device": "no_active_policy", "retroactive": "no_active_policy", "retired": "no_active_policy", "legacy-items": "legacy_classification", "legacy-analysis": "legacy_classification", "zero-items": "invalid_evidence"}
			if reason := incomplete[scenario]; reason != "" {
				require.NoError(t, err)
				require.Equal(t, "not_evaluated", preview.Outcome)
				require.Equal(t, reason, preview.Reason)
			} else {
				require.ErrorIs(t, err, domain.ErrValidation)
			}
			_, err = f.s.PersistPolicyEvaluation(ctx, f.device, e.ID, f.approver, f.now.Add(4*time.Hour))
			require.ErrorIs(t, err, domain.ErrValidation)
			after, err := f.s.GetChangeEvent(ctx, e.ID)
			require.NoError(t, err)
			require.Equal(t, before, after)
			var count int
			require.NoError(t, f.s.db.QueryRow(`SELECT COUNT(*) FROM change_policy_evaluations`).Scan(&count))
			require.Zero(t, count)
		})
	}
}

func TestPolicyTimeComparisonUsesAbsoluteInstants(t *testing.T) {
	for _, delta := range []time.Duration{-time.Nanosecond, 0, time.Nanosecond} {
		t.Run(delta.String(), func(t *testing.T) {
			f := authorizationStore(t)
			ctx := context.Background()
			e := authorizationEvent(t, f.s, f.device)
			detected := f.now.Add(2 * time.Hour).Add(123456789 * time.Nanosecond)
			_, err := f.s.db.Exec(`UPDATE change_events SET detected_at=? WHERE id=?`, detected.In(time.FixedZone("offset", -7*3600)).Format(time.RFC3339Nano), e.ID)
			require.NoError(t, err)
			p := f.policy(t, completePolicyRules)
			f.activatePolicy(t, p, detected.Add(delta).In(time.FixedZone("other", 3*3600+1800)))
			preview, err := f.s.PreviewPolicyEvaluation(ctx, f.device, e.ID, f.approver)
			require.NoError(t, err)
			if delta > 0 {
				require.Equal(t, "not_evaluated", preview.Outcome)
			} else {
				require.Equal(t, "pass", preview.Outcome)
			}
		})
	}
}

func TestPolicyTransactionRollbackIncludingCommitFailure(t *testing.T) {
	for _, stage := range []string{"evaluation", "item", "event", "commit"} {
		t.Run(stage, func(t *testing.T) {
			f := authorizationStore(t)
			ctx := context.Background()
			f.activatePolicy(t, f.policy(t, completePolicyRules), f.now.Add(time.Hour))
			e := authorizationEvent(t, f.s, f.device)
			before, err := f.s.GetChangeEvent(ctx, e.ID)
			require.NoError(t, err)
			var query string
			switch stage {
			case "evaluation":
				query = `CREATE TRIGGER policy_fault BEFORE INSERT ON change_policy_evaluations BEGIN SELECT RAISE(ABORT,'injected'); END`
			case "item":
				query = `CREATE TRIGGER policy_fault BEFORE INSERT ON change_policy_evaluation_items WHEN NEW.ordinal=1 BEGIN SELECT RAISE(ABORT,'injected'); END`
			case "event":
				query = `CREATE TRIGGER policy_fault BEFORE UPDATE OF policy_status ON change_events BEGIN SELECT RAISE(ABORT,'injected'); END`
			case "commit":
				_, err = f.s.db.Exec(`CREATE TABLE policy_commit_fault(id TEXT REFERENCES users(id) DEFERRABLE INITIALLY DEFERRED)`)
				require.NoError(t, err)
				query = `CREATE TRIGGER policy_fault AFTER INSERT ON change_policy_evaluations BEGIN INSERT INTO policy_commit_fault VALUES ('missing-actor'); END`
			}
			_, err = f.s.db.Exec(query)
			require.NoError(t, err)
			_, err = f.s.PersistPolicyEvaluation(ctx, f.device, e.ID, f.approver, f.now.Add(4*time.Hour))
			require.Error(t, err)
			require.Zero(t, f.s.db.Stats().InUse, "failed commit must release its pinned connection")
			after, err := f.s.GetChangeEvent(ctx, e.ID)
			require.NoError(t, err)
			require.Equal(t, before, after)
			for _, table := range []string{"change_policy_evaluations", "change_policy_evaluation_items"} {
				var count int
				require.NoError(t, f.s.db.QueryRow(`SELECT COUNT(*) FROM `+table).Scan(&count))
				require.Zero(t, count)
			}
			_, err = f.s.db.Exec(`DROP TRIGGER policy_fault`)
			require.NoError(t, err)
			_, err = f.s.PersistPolicyEvaluation(ctx, f.device, e.ID, f.approver, f.now.Add(4*time.Hour))
			require.NoError(t, err)
			got, err := f.s.GetPolicyEvaluation(ctx, f.device, e.ID)
			require.NoError(t, err)
			require.Equal(t, "pass", got.Result)
			require.Zero(t, f.s.db.Stats().InUse)
		})
	}
}

func TestPolicyConcurrentIndependentStores(t *testing.T) {
	for _, operation := range []string{"activation", "evaluation", "retirement", "permission-revocation", "activation-evaluation"} {
		t.Run(operation, func(t *testing.T) {
			f := authorizationStore(t)
			ctx := context.Background()
			p := f.policy(t, completePolicyRules)
			next := f.policy(t, completePolicyRules)
			if operation != "activation" && operation != "activation-evaluation" {
				p = f.activatePolicy(t, p, f.now.Add(time.Hour))
			}
			e := authorizationEvent(t, f.s, f.device)
			var path string
			require.NoError(t, f.s.db.QueryRow(`SELECT file FROM pragma_database_list WHERE name='main'`).Scan(&path))
			other, err := Open(ctx, path)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, other.Close()) })
			start := make(chan struct{})
			results := make(chan error, 2)
			var wg sync.WaitGroup
			for i := range 2 {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					<-start
					store := f.s
					if i == 1 {
						store = other
					}
					var err error
					switch {
					case operation == "activation":
						target := p
						if i == 1 {
							target = next
						}
						_, err = store.ActivatePolicySet(ctx, f.device, target.ID, f.approver, f.now.Add(time.Hour))
					case i == 1 && operation == "retirement":
						_, err = store.RetirePolicySet(ctx, f.device, p.ID, f.approver, f.now.Add(4*time.Hour))
					case i == 1 && operation == "permission-revocation":
						_, err = store.db.ExecContext(ctx, `DELETE FROM user_roles WHERE user_id=?`, f.approver)
						err = authorizationWriteError(err)
					case i == 1 && operation == "activation-evaluation":
						_, err = store.ActivatePolicySet(ctx, f.device, p.ID, f.approver, f.now.Add(time.Hour))
					default:
						_, err = store.PersistPolicyEvaluation(ctx, f.device, e.ID, f.approver, f.now.Add(4*time.Hour))
					}
					results <- err
				}(i)
			}
			close(start)
			wg.Wait()
			close(results)
			success := 0
			for err := range results {
				if err == nil {
					success++
				} else {
					require.True(t, errors.Is(err, domain.ErrConflict) || errors.Is(err, domain.ErrForbidden) || errors.Is(err, domain.ErrValidation), "unexpected race error: %v", err)
				}
			}
			var count, items int
			require.NoError(t, f.s.db.QueryRow(`SELECT COUNT(*) FROM change_policy_evaluations WHERE event_id=?`, e.ID).Scan(&count))
			require.NoError(t, f.s.db.QueryRow(`SELECT COUNT(*) FROM change_policy_evaluation_items WHERE event_id=?`, e.ID).Scan(&items))
			require.LessOrEqual(t, count, 1)
			require.Equal(t, count*2, items)
			got, err := f.s.GetChangeEvent(ctx, e.ID)
			require.NoError(t, err)
			require.Equal(t, count == 1, got.PolicyStatus == "pass")
			if operation == "evaluation" {
				require.Equal(t, 1, success)
				require.Equal(t, 1, count)
			}
			if operation == "activation" {
				require.Equal(t, 1, success)
				var active int
				require.NoError(t, f.s.db.QueryRow(`SELECT COUNT(*) FROM change_policy_sets WHERE status='active'`).Scan(&active))
				require.Equal(t, 1, active)
			}
		})
	}
}

func TestPolicyRepositoryRechecksLiveActorAndGroup(t *testing.T) {
	for _, mutation := range []string{"disabled", "grants", "group", "backups"} {
		t.Run(mutation, func(t *testing.T) {
			f := authorizationStore(t)
			ctx := context.Background()
			p := f.activatePolicy(t, f.policy(t, completePolicyRules), f.now.Add(time.Hour))
			e := authorizationEvent(t, f.s, f.device)
			var query string
			switch mutation {
			case "disabled":
				query = `UPDATE users SET enabled=0 WHERE id=?`
			case "grants":
				query = `DELETE FROM user_roles WHERE user_id=?`
			case "group":
				_, err := f.s.db.Exec(`UPDATE devices SET device_group='group-A' WHERE id=?`, f.device)
				require.NoError(t, err)
				// Non-system actor with explicit policy grants and a restricted group.
				_, err = f.s.db.Exec(`DELETE FROM user_roles WHERE user_id=?`, f.approver)
				require.NoError(t, err)
				require.NoError(t, f.s.AssignRole(ctx, f.approver, "network_administrator"))
				query = `INSERT INTO user_device_groups VALUES (?,'other-group')`
			case "backups":
				_, err := f.s.db.Exec(`DELETE FROM user_roles WHERE user_id=?`, f.approver)
				require.NoError(t, err)
				_, err = f.s.db.Exec(`INSERT INTO roles(id,name,description) VALUES ('policy-only','policy-only','test')`)
				require.NoError(t, err)
				_, err = f.s.db.Exec(`INSERT INTO role_permissions SELECT 'policy-only',id FROM permissions WHERE name IN ('devices.read','changes.policy.evaluate','changes.policy.manage')`)
				require.NoError(t, err)
				query = `INSERT INTO user_roles VALUES (?,'policy-only')`
			}
			_, err := f.s.db.Exec(query, f.approver)
			require.NoError(t, err)
			_, err = f.s.PreviewPolicyEvaluation(ctx, f.device, e.ID, f.approver)
			require.ErrorIs(t, err, domain.ErrForbidden)
			_, err = f.s.PersistPolicyEvaluation(ctx, f.device, e.ID, f.approver, f.now.Add(4*time.Hour))
			require.ErrorIs(t, err, domain.ErrForbidden)
			if mutation != "backups" {
				_, err = f.s.RetirePolicySet(ctx, f.device, p.ID, f.approver, f.now.Add(4*time.Hour))
				require.ErrorIs(t, err, domain.ErrForbidden)
			}
		})
	}
}

func TestPolicyMigrationPreservesAllLegacyEvidenceAndGrants(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "pre-policy.db")
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	for _, name := range []string{"001_initial.sql", "002_seed_rbac.sql", "003_change_events.sql", "004_change_event_items.sql", "005_change_item_classification.sql", "006_change_authorizations.sql"} {
		body, err := migrations.Files.ReadFile(name)
		require.NoError(t, err)
		_, err = db.Exec(string(body))
		require.NoError(t, err)
		_, err = db.Exec(`INSERT INTO schema_migrations VALUES (?,?)`, name, timestamp(time.Now()))
		require.NoError(t, err)
	}
	devices := changeEventDevices(t, s)
	now := time.Date(2026, 10, 4, 11, 0, 0, 0, time.UTC)
	f := authorizationFixture{s: s, device: devices[0].ID, otherDevice: devices[1].ID, now: now, requester: uuid.NewString(), approver: uuid.NewString()}
	for _, id := range []string{f.requester, f.approver} {
		require.NoError(t, s.CreateUser(ctx, domain.User{ID: id, Username: id, Enabled: true, CreatedAt: now, UpdatedAt: now}))
		require.NoError(t, s.AssignRole(ctx, id, "system_administrator"))
	}
	from := eventBackup(f.device, "startup", now)
	to := eventBackup(f.device, "startup", now.Add(time.Hour))
	legacy := eventBetween(from, to)
	require.NoError(t, s.CreateBackup(ctx, from))
	require.NoError(t, s.CreateBackupWithChangeEvent(ctx, to, &legacy))
	next, analyzed, oldItems := analysisFixture(t, s, f.otherDevice)
	require.NoError(t, s.CreateBackupWithChangeAnalysis(ctx, next, &analyzed, oldItems))
	e := authorizationEvent(t, s, f.device)
	a := f.approve(t, f.request(t))
	require.NoError(t, s.LinkChangeEventAuthorization(ctx, f.device, f.link(a, e)))
	e, err = s.GetChangeEvent(ctx, e.ID)
	require.NoError(t, err)
	link, err := s.GetChangeEventAuthorization(ctx, f.device, e.ID)
	require.NoError(t, err)
	history, _, err := s.ListChangeAuthorizationHistory(ctx, f.device, a.ID, repositories.Page{Limit: 200})
	require.NoError(t, err)
	var root, grants int
	require.NoError(t, db.QueryRow(`SELECT rootpage FROM sqlite_master WHERE name='change_events'`).Scan(&root))
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM role_permissions`).Scan(&grants))
	oldGrants := policyMigrationGrantIdentities(t, db)
	require.NoError(t, s.Close())
	s, err = Open(ctx, path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Close()) })
	require.NoError(t, s.Migrate(ctx))
	for _, event := range []domain.ChangeEvent{legacy, analyzed, e} {
		got, err := s.GetChangeEvent(ctx, event.ID)
		require.NoError(t, err)
		require.Equal(t, event, got)
	}
	items, _, err := s.ListChangeEventItems(ctx, analyzed.ID, repositories.Page{Limit: 200})
	require.NoError(t, err)
	require.Equal(t, oldItems, items)
	gotLink, err := s.GetChangeEventAuthorization(ctx, f.device, e.ID)
	require.NoError(t, err)
	require.Equal(t, link, gotLink)
	gotHistory, _, err := s.ListChangeAuthorizationHistory(ctx, f.device, a.ID, repositories.Page{Limit: 200})
	require.NoError(t, err)
	require.Equal(t, history, gotHistory)
	var newRoot, count int
	require.NoError(t, s.db.QueryRow(`SELECT rootpage FROM sqlite_master WHERE name='change_events'`).Scan(&newRoot))
	require.Equal(t, root, newRoot)
	assertApprovedTaskSixMigrations(t, s.db)
	require.NoError(t, s.db.QueryRow(`SELECT COUNT(*) FROM role_permissions`).Scan(&count))
	require.Equal(t, grants+6+5, count, "six approved policy grants plus five approved risk grants")
	newGrants := policyMigrationGrantIdentities(t, s.db)
	require.Subset(t, newGrants, oldGrants, "every original role/permission identity pair survives")
	grantRows, err := s.db.Query(`SELECT r.name||':'||p.name FROM role_permissions rp JOIN roles r ON r.id=rp.role_id JOIN permissions p ON p.id=rp.permission_id WHERE p.name LIKE 'changes.policy.%' OR p.name LIKE 'changes.risk.%' ORDER BY r.name,p.name`)
	require.NoError(t, err)
	var addedGrants []string
	for grantRows.Next() {
		var grant string
		require.NoError(t, grantRows.Scan(&grant))
		addedGrants = append(addedGrants, grant)
	}
	require.NoError(t, grantRows.Err())
	require.NoError(t, grantRows.Close())
	require.Equal(t, []string{
		"network_administrator:changes.policy.evaluate",
		"network_administrator:changes.policy.read",
		"network_administrator:changes.risk.evaluate",
		"network_administrator:changes.risk.read",
		"network_operator:changes.policy.read",
		"network_operator:changes.risk.read",
		"system_administrator:changes.policy.evaluate",
		"system_administrator:changes.policy.manage",
		"system_administrator:changes.policy.read",
		"system_administrator:changes.risk.evaluate",
		"system_administrator:changes.risk.read",
	}, addedGrants, "exact approved 007+008 role grants; no broader grant drift")
	for _, table := range []string{"change_policy_sets", "change_policy_evaluations", "change_policy_evaluation_items"} {
		require.NoError(t, s.db.QueryRow(`SELECT COUNT(*) FROM `+table).Scan(&count))
		require.Zero(t, count)
	}
	rows, err := s.db.Query(`PRAGMA foreign_key_check`)
	require.NoError(t, err)
	require.False(t, rows.Next())
	require.NoError(t, rows.Close())
	for role, want := range map[string]int{"viewer": 0, "network_operator": 1, "network_administrator": 2, "system_administrator": 3} {
		require.NoError(t, s.db.QueryRow(`SELECT COUNT(*) FROM role_permissions rp JOIN roles r ON r.id=rp.role_id JOIN permissions p ON p.id=rp.permission_id WHERE r.name=? AND p.name LIKE 'changes.policy.%'`, role).Scan(&count))
		require.Equal(t, want, count)
	}
}

func policyMigrationGrantIdentities(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.Query(`SELECT json_array(role_id,permission_id) FROM role_permissions ORDER BY role_id,permission_id`)
	require.NoError(t, err)
	defer rows.Close()
	var grants []string
	for rows.Next() {
		var grant string
		require.NoError(t, rows.Scan(&grant))
		grants = append(grants, grant)
	}
	require.NoError(t, rows.Err())
	return grants
}
