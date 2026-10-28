package api_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"
)

func TestRiskOpenAPISevenOperationContract(t *testing.T) {
	doc, err := openapi3.NewLoader().LoadFromFile(filepath.Join("..", "..", "..", "packages", "shared-contracts", "openapi", "aegis-api.yaml"))
	require.NoError(t, err)
	require.NoError(t, doc.Validate(context.Background()))
	base := "/api/v1/devices/{deviceID}/change-events/{eventID}"
	operations := 0
	for path, item := range doc.Paths.Map() {
		if !strings.Contains(path, "/risk-assessment") {
			continue
		}
		for method, op := range item.Operations() {
			operations++
			require.Contains(t, []string{base + "/risk-assessment", base + "/risk-assessment/preview", base + "/risk-assessment/findings", base + "/risk-assessments", base + "/risk-assessments/{assessmentID}", base + "/risk-assessments/{assessmentID}/findings"}, path)
			require.Contains(t, []string{"GET", "POST"}, method)
			for _, grant := range []string{"devices.read", "backups.read", "changes.authorization.read", "changes.policy.read", "changes.risk.read", "device-group access", "enabled actor"} {
				require.Contains(t, op.Description, grant)
			}
			if method == "POST" && path == base+"/risk-assessment" {
				require.Contains(t, op.Description, "changes.risk.evaluate AND changes.risk.read")
				require.Contains(t, op.Description, "atomic")
				require.NotNil(t, op.Responses.Value("201"))
			} else {
				require.Nil(t, op.Responses.Value("201"))
			}
			for _, code := range []string{"200", "400", "401", "403", "404", "409", "500", "503"} {
				require.NotNil(t, op.Responses.Value(code), method+" "+path+" "+code)
			}
			if method == "POST" {
				require.Contains(t, op.RequestBody.Value.Description, "4096")
				require.Contains(t, op.RequestBody.Value.Description, "Duplicate")
				unsupportedMediaType := op.Responses.Value("415")
				require.NotNil(t, unsupportedMediaType)
				require.NotNil(t, unsupportedMediaType.Value)
				require.NotNil(t, unsupportedMediaType.Value.Description)
				require.Contains(t, *unsupportedMediaType.Value.Description, "UNSUPPORTED_MEDIA_TYPE")
				require.NotNil(t, unsupportedMediaType.Value.Content["application/json"])
			} else {
				require.Nil(t, op.Responses.Value("415"), "415 addition is restricted to the two risk POSTs")
			}
		}
	}
	require.Equal(t, 7, operations)
	riskUnavailable := doc.Components.Responses["RiskUnavailable"]
	require.NotNil(t, riskUnavailable)
	require.NotNil(t, riskUnavailable.Value)
	require.NotNil(t, riskUnavailable.Value.Description)
	require.Contains(t, *riskUnavailable.Value.Description, "COMMAND_TIMEOUT")
	require.Contains(t, *riskUnavailable.Value.Description, "genuinely canceled")
	for _, name := range []string{"RiskPreviewRequest", "RiskAssessmentRequest"} {
		s := doc.Components.Schemas[name].Value
		require.NotNil(t, s.AdditionalProperties.Has)
		require.False(t, *s.AdditionalProperties.Has)
		if name == "RiskPreviewRequest" {
			require.Empty(t, s.Properties)
		} else {
			require.Len(t, s.Properties, 1)
			require.NotNil(t, s.Properties["expected_latest_assessment_id"])
		}
	}
	for _, name := range []string{"RiskAssessment", "RiskRecord", "RiskPreview", "RiskFinding", "RiskResultSummary", "RiskPreviewResult"} {
		s := doc.Components.Schemas[name].Value
		for _, field := range []string{"command", "context_path", "raw_diff", "source_bindings", "evidence_json", "result_json", "secret_hash", "score"} {
			require.Nil(t, s.Properties[field], name+" "+field)
		}
	}
	require.Equal(t, []any{true}, doc.Components.Schemas["RiskAssessment"].Value.Properties["historical"].Value.Enum)
	require.Equal(t, []any{false}, doc.Components.Schemas["RiskPreview"].Value.Properties["historical"].Value.Enum)
	require.Nil(t, doc.Components.Schemas["RiskResultSummary"].Value.Properties["findings"])
	for _, name := range []string{"RiskLimit", "RiskOffset"} {
		require.NotNil(t, doc.Components.Parameters[name])
	}
}
