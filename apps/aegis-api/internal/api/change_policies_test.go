package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"company.local/aegis/apps/aegis-api/internal/domain"
	"company.local/aegis/apps/aegis-api/internal/repositories"
	"github.com/stretchr/testify/require"
)

func policyInput(reference string, deny bool) map[string]any {
	decision := "allow"
	if deny {
		decision = "deny"
	}
	return map[string]any{"reference": reference, "config_type": "running", "rules": []domain.PolicyRule{{Category: "interface", Operation: "add", Decision: "allow"}, {Category: "aaa", Operation: "add", Decision: decision}}}
}

func TestPolicyHTTPFullWorkflow(t *testing.T) {
	for _, deny := range []bool{false, true} {
		t.Run(fmt.Sprint(deny), func(t *testing.T) {
			a := newAuthorizationAPI(t)
			first := a.call(t, "POST", "/backups", "network_operator", map[string]any{"config_type": "running"})
			require.Equal(t, 201, first.status, first.raw)
			created := a.call(t, "POST", "/change-policies", "system_administrator", policyInput(" pol-api-001 ", deny))
			require.Equal(t, 201, created.status, created.raw)
			id := nestedString(t, created.body, "data", "id")
			require.Equal(t, "POL-API-001", nestedString(t, created.body, "data", "reference"))
			require.Equal(t, a.users["system_administrator"], nestedString(t, created.body, "data", "created_by"))
			active := a.call(t, "POST", "/change-policies/"+id+"/activate", "system_administrator", nil)
			require.Equal(t, 200, active.status, active.raw)
			a.connector.SetRunning("hostname LAB\ninterface Gi1\n description approved-uplink\nusername ops secret 0 DO-NOT-LEAK\n")
			second := a.call(t, "POST", "/backups", "network_operator", map[string]any{"config_type": "running"})
			require.Equal(t, 201, second.status, second.raw)
			events := a.call(t, "GET", "/change-events", "network_operator", nil)
			require.Equal(t, 200, events.status, events.raw)
			eventID := events.body["data"].([]any)[0].(map[string]any)["id"].(string)
			cross := requestJSON(t, a.server.Client(), "POST", a.server.URL+"/api/v1/devices/"+a.otherDevice+"/change-events/"+eventID+"/policy-evaluation", a.tokens["system_administrator"], nil)
			require.Equal(t, 404, cross.status, cross.raw)
			before, err := a.store.GetChangeEvent(context.Background(), eventID)
			require.NoError(t, err)
			require.Equal(t, "not_evaluated", before.PolicyStatus)
			*a.now = before.DetectedAt.Add(time.Second)
			calls := a.connector.CallCount()
			path := "/change-events/" + eventID + "/policy-evaluation"
			preview := a.call(t, "POST", path+"/preview", "network_administrator", nil)
			require.Equal(t, 200, preview.status, preview.raw)
			preview2 := a.call(t, "POST", path+"/preview", "network_administrator", map[string]any{})
			require.Equal(t, preview.body["data"], preview2.body["data"])
			after, err := a.store.GetChangeEvent(context.Background(), eventID)
			require.NoError(t, err)
			require.Equal(t, before, after)
			evaluated := a.call(t, "POST", path, "network_administrator", nil)
			require.Equal(t, 200, evaluated.status, evaluated.raw)
			want := "pass"
			if deny {
				want = "violation"
			}
			require.Equal(t, want, nestedString(t, evaluated.body, "data", "result"))
			after, err = a.store.GetChangeEvent(context.Background(), eventID)
			require.NoError(t, err)
			require.Equal(t, want, after.PolicyStatus)
			require.Equal(t, before.AuthorizationStatus, after.AuthorizationStatus)
			evidence := a.call(t, "GET", path, "network_operator", nil)
			require.Equal(t, 200, evidence.status, evidence.raw)
			require.Equal(t, evaluated.body["data"], evidence.body["data"])
			items := a.call(t, "GET", path+"/items?limit=1&offset=0", "network_operator", nil)
			require.Equal(t, 200, items.status, items.raw)
			require.Len(t, items.body["data"].([]any), 1)
			allItems := a.call(t, "GET", path+"/items", "network_operator", nil)
			require.Equal(t, 200, allItems.status, allItems.raw)
			for _, raw := range []string{preview.raw, evaluated.raw, allItems.raw} {
				require.NotContains(t, raw, "DO-NOT-LEAK")
				require.NotContains(t, raw, `"command"`)
				require.NotContains(t, raw, `"context_path"`)
				require.NotContains(t, raw, "storage_path")
			}
			repeat := a.call(t, "POST", path, "network_administrator", nil)
			require.Equal(t, 409, repeat.status, repeat.raw)
			for _, method := range []string{"PUT", "DELETE"} {
				rewrite := a.call(t, method, path, "system_administrator", map[string]any{"result": "pass"})
				require.Equal(t, 405, rewrite.status, rewrite.raw)
			}
			retired := a.call(t, "POST", "/change-policies/"+id+"/retire", "system_administrator", nil)
			require.Equal(t, 200, retired.status, retired.raw)
			historical := a.call(t, "GET", path, "network_operator", nil)
			require.Equal(t, evidence.body["data"], historical.body["data"])
			preview = a.call(t, "POST", path+"/preview", "network_administrator", nil)
			require.Equal(t, "not_evaluated", nestedString(t, preview.body, "data", "outcome"))
			require.Equal(t, "no_active_policy", nestedString(t, preview.body, "data", "reason"))
			require.Equal(t, evidence.body["data"], preview.body["data"].(map[string]any)["existing_evaluation"])
			require.Equal(t, calls, a.connector.CallCount())
			a.connector.SetRunning("hostname LAB\ninterface Gi1\n description next-uplink\nusername ops secret 0 DO-NOT-LEAK\n")
			third := a.call(t, "POST", "/backups", "network_operator", map[string]any{"config_type": "running"})
			require.Equal(t, 201, third.status, third.raw)
			events = a.call(t, "GET", "/change-events", "network_operator", nil)
			newID := events.body["data"].([]any)[0].(map[string]any)["id"].(string)
			require.NotEqual(t, eventID, newID)
			ineligible := a.call(t, "POST", "/change-events/"+newID+"/policy-evaluation", "network_administrator", nil)
			require.Equal(t, 400, ineligible.status, ineligible.raw)
			audits, _, err := a.store.ListAudits(context.Background(), repositories.Page{Limit: 200})
			require.NoError(t, err)
			seen := map[string]bool{}
			for _, audit := range audits {
				seen[audit.Action] = true
				data, _ := json.Marshal(audit)
				require.NotContains(t, string(data), "DO-NOT-LEAK")
			}
			for _, action := range []string{"create", "activate", "retire", "preview", "evaluate", "read"} {
				require.True(t, seen["change_policy."+action], action)
			}
		})
	}
}

func TestPolicyHTTPPermissionsScopeAndStrictInput(t *testing.T) {
	a := newAuthorizationAPI(t)
	for _, role := range []string{"viewer", "network_operator", "network_administrator"} {
		res := a.call(t, "POST", "/change-policies", role, policyInput("POL-BAD", false))
		require.Equal(t, 403, res.status, res.raw)
	}
	created := a.call(t, "POST", "/change-policies", "system_administrator", policyInput("POL-RBAC", false))
	require.Equal(t, 201, created.status, created.raw)
	id := nestedString(t, created.body, "data", "id")
	duplicate := a.call(t, "POST", "/change-policies", "system_administrator", policyInput(" pol-rbac ", false))
	require.Equal(t, 409, duplicate.status, duplicate.raw)
	for role, want := range map[string]int{"viewer": 403, "network_operator": 200, "network_administrator": 200, "system_administrator": 200} {
		res := a.call(t, "GET", "/change-policies/"+id, role, nil)
		require.Equal(t, want, res.status, res.raw)
	}
	for _, role := range []string{"viewer", "network_operator", "network_administrator"} {
		res := a.call(t, "POST", "/change-policies/"+id+"/activate", role, nil)
		require.Equal(t, 403, res.status, res.raw)
	}
	for _, endpoint := range []string{"/change-events/missing/policy-evaluation", "/change-events/missing/policy-evaluation/preview"} {
		for _, role := range []string{"viewer", "network_operator"} {
			res := a.call(t, "POST", endpoint, role, nil)
			require.Equal(t, 403, res.status, res.raw)
		}
	}
	for _, endpoint := range []string{"/change-policies", "/change-policies/" + id, "/change-events/missing/policy-evaluation", "/change-events/missing/policy-evaluation/items"} {
		res := a.call(t, "GET", endpoint, "", nil)
		require.Equal(t, 401, res.status, res.raw)
	}
	for _, endpoint := range []string{"/change-policies/" + id, "/change-policies/" + id + "/activate", "/change-events/missing/policy-evaluation"} {
		method := "GET"
		if strings.HasSuffix(endpoint, "activate") {
			method = "POST"
		}
		res := requestJSON(t, a.server.Client(), method, a.server.URL+"/api/v1/devices/"+a.otherDevice+endpoint, a.tokens["system_administrator"], nil)
		require.Equal(t, 404, res.status, res.raw)
	}
	for _, query := range []string{"?limit=0", "?limit=201", "?offset=-1", "?status=invalid", "?config_type=candidate"} {
		res := a.call(t, "GET", "/change-policies"+query, "network_operator", nil)
		require.Equal(t, 400, res.status, res.raw)
	}
}

func TestPolicyHTTPStrictInput(t *testing.T) {
	a := newAuthorizationAPI(t)
	id := "missing"
	for _, field := range []string{"id", "device_id", "status", "engine_version", "definition_hash", "created_by", "created_at", "activated_by", "activated_at", "retired_by", "retired_at", "updated_at"} {
		input := policyInput("POL-FORGED", false)
		input[field] = "forged"
		res := a.call(t, "POST", "/change-policies", "system_administrator", input)
		require.Equal(t, 400, res.status, res.raw)
	}
	for _, endpoint := range []string{"/change-policies/" + id + "/activate", "/change-policies/" + id + "/retire", "/change-events/missing/policy-evaluation", "/change-events/missing/policy-evaluation/preview"} {
		// A fresh server per action keeps this input matrix below the unchanged
		// production rate-limit burst, without sleeps or disabling security.
		a = newAuthorizationAPI(t)
		for _, field := range []string{"policy_set_id", "outcome", "result", "policy_definition_hash", "evaluated_by", "evaluated_at"} {
			res := a.call(t, "POST", endpoint, "system_administrator", map[string]any{field: "forged"})
			require.Equal(t, 400, res.status, res.raw)
		}
	}
	for _, body := range []string{`null`, `[]`, `{} {}`, `{"reference":"POL-JSON","config_type":"running","rules":[{"category":"aaa","operation":"add","decision":"allow","id":"forged"}]}`} {
		req, err := http.NewRequest("POST", a.server.URL+"/api/v1/devices/"+a.device+"/change-policies", strings.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+a.tokens["system_administrator"])
		res, err := a.server.Client().Do(req)
		require.NoError(t, err)
		data, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		require.NoError(t, res.Body.Close())
		require.Equal(t, 400, res.StatusCode, string(data))
	}
}

func TestPolicyHTTPLiveGrantsAndGroups(t *testing.T) {
	a := newAuthorizationAPI(t)
	created := a.call(t, "POST", "/change-policies", "system_administrator", policyInput("POL-LIVE", false))
	require.Equal(t, 201, created.status, created.raw)
	id := nestedString(t, created.body, "data", "id")
	db, err := sql.Open("sqlite", a.path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	_, err = db.Exec(`INSERT INTO user_device_groups VALUES (?,'group-B')`, a.users["network_administrator"])
	require.NoError(t, err)
	for _, endpoint := range []string{"/change-policies", "/change-policies/" + id, "/change-events/missing/policy-evaluation", "/change-events/missing/policy-evaluation/items"} {
		res := a.call(t, "GET", endpoint, "network_administrator", nil)
		require.Equal(t, 403, res.status, res.raw)
	}
	res := a.call(t, "POST", "/change-events/missing/policy-evaluation/preview", "network_administrator", nil)
	require.Equal(t, 403, res.status, res.raw)
	_, err = db.Exec(`DELETE FROM user_roles WHERE user_id=?`, a.users["system_administrator"])
	require.NoError(t, err)
	res = a.call(t, "GET", "/change-policies/"+id, "system_administrator", nil)
	require.Equal(t, 403, res.status, res.raw)
	res = a.call(t, "POST", "/change-policies/"+id+"/activate", "system_administrator", nil)
	require.Equal(t, 403, res.status, res.raw)
	_, err = db.Exec(`UPDATE users SET enabled=0 WHERE id=?`, a.users["network_operator"])
	require.NoError(t, err)
	res = a.call(t, "GET", "/change-policies/"+id, "network_operator", nil)
	require.Contains(t, []int{401, 403}, res.status, res.raw)
}
