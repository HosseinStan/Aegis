package api_test

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"
)

func TestChangePolicyOpenAPIContract(t *testing.T) {
	path := filepath.Join("..", "..", "..", "packages", "shared-contracts", "openapi", "aegis-api.yaml")
	doc, err := openapi3.NewLoader().LoadFromFile(path)
	require.NoError(t, err)
	require.NoError(t, doc.Validate(context.Background()))
	base := "/api/v1/devices/{deviceID}"
	for _, tc := range []struct{ path, method, permission string }{
		{"/change-policies", "POST", "manage"}, {"/change-policies", "GET", "read"}, {"/change-policies/{policyID}", "GET", "read"},
		{"/change-policies/{policyID}/activate", "POST", "manage"}, {"/change-policies/{policyID}/retire", "POST", "manage"},
		{"/change-events/{eventID}/policy-evaluation/preview", "POST", "evaluate"}, {"/change-events/{eventID}/policy-evaluation", "POST", "evaluate"},
		{"/change-events/{eventID}/policy-evaluation", "GET", "read"}, {"/change-events/{eventID}/policy-evaluation/items", "GET", "read"},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			item := doc.Paths.Value(base + tc.path)
			require.NotNil(t, item)
			op := item.GetOperation(tc.method)
			require.NotNil(t, op)
			require.Contains(t, op.Description, "devices.read")
			require.Contains(t, op.Description, "changes.policy."+tc.permission)
			require.Contains(t, op.Description, "device-group access")
			if strings.Contains(tc.path, "policy-evaluation") {
				require.Contains(t, op.Description, "backups.read")
			}
			for _, code := range []string{"400", "401", "403", "404"} {
				require.NotNil(t, op.Responses.Value(code))
			}
			if tc.method == http.MethodPost {
				require.NotNil(t, op.Responses.Value("409"))
				require.NotNil(t, op.RequestBody)
			}
		})
	}
	create := doc.Components.Schemas["PolicySetCreate"].Value
	require.NotNil(t, create.AdditionalProperties.Has)
	require.False(t, *create.AdditionalProperties.Has)
	rule := doc.Components.Schemas["PolicyRule"].Value
	require.False(t, *rule.AdditionalProperties.Has)
	require.ElementsMatch(t, []string{"category", "operation", "decision"}, rule.Required)
	require.Len(t, doc.Components.Schemas["PolicyCategory"].Value.Enum, 13)
	for _, name := range []string{"PolicySet", "PolicyDefinition", "PolicyEvaluationPreview", "PolicyItemResult", "ChangePolicyEvaluation", "ChangePolicyEvaluationItem"} {
		require.NotNil(t, doc.Components.Schemas[name])
	}
	final := doc.Paths.Value(base + "/change-events/{eventID}/policy-evaluation").Post.Description
	for _, text := range []string{"Exactly one final evaluation", "authorization_status", "every item explicitly allowed", "Any explicit deny", "not device security", "ActivatedAt <= DetectedAt"} {
		require.Contains(t, final, text)
	}
	for _, name := range []string{"ChangePolicyEvaluation", "ChangePolicyEvaluationItem", "PolicyItemResult"} {
		schema := doc.Components.Schemas[name].Value
		for _, field := range []string{"command", "context_path", "raw_diff", "secret_hash"} {
			require.Nil(t, schema.Properties[field])
		}
	}
}
