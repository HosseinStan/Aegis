package api_test

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"
)

func TestChangeAuthorizationOpenAPIPathsAndPermissions(t *testing.T) {
	path := filepath.Join("..", "..", "..", "packages", "shared-contracts", "openapi", "aegis-api.yaml")
	document, err := openapi3.NewLoader().LoadFromFile(path)
	require.NoError(t, err)
	require.NoError(t, document.Validate(context.Background()))
	base := "/api/v1/devices/{deviceID}"
	for _, test := range []struct{ path, method, permission string }{
		{"/change-authorizations", http.MethodGet, "changes.authorization.read"},
		{"/change-authorizations", http.MethodPost, "changes.authorization.request"},
		{"/change-authorizations/{authorizationID}", http.MethodGet, "changes.authorization.read"},
		{"/change-authorizations/{authorizationID}/approve", http.MethodPost, "changes.authorization.approve"},
		{"/change-authorizations/{authorizationID}/reject", http.MethodPost, "changes.authorization.approve"},
		{"/change-authorizations/{authorizationID}/revoke", http.MethodPost, "changes.authorization.approve"},
		{"/change-authorizations/{authorizationID}/history", http.MethodGet, "changes.authorization.read"},
		{"/change-events", http.MethodGet, "backups.read"},
		{"/change-events/{eventID}", http.MethodGet, "backups.read"},
		{"/change-events/{eventID}/items", http.MethodGet, "backups.read"},
		{"/change-events/{eventID}/authorize", http.MethodPost, "changes.authorization.reconcile"},
		{"/change-events/{eventID}/authorization", http.MethodGet, "changes.authorization.read"},
	} {
		t.Run(test.method+test.path, func(t *testing.T) {
			item := document.Paths.Value(base + test.path)
			require.NotNil(t, item)
			operation := item.GetOperation(test.method)
			require.NotNil(t, operation)
			require.Contains(t, operation.Description, "devices.read")
			require.Contains(t, operation.Description, test.permission)
			require.Contains(t, operation.Description, "device-group access")
			for _, code := range []string{"401", "403", "404"} {
				require.NotNil(t, operation.Responses.Value(code))
			}
		})
	}
	operation := document.Paths.Value(base + "/change-events/{eventID}/authorize").Post
	for _, text := range []string{"backups.read", "One request links at most one event", "observation time", "not cryptographic proof", "PolicyStatus is untouched"} {
		require.Contains(t, operation.Description, text)
	}
	require.NotNil(t, document.Components.Schemas["ChangeAuthorization"])
	require.NotNil(t, document.Components.Schemas["ChangeEventItem"])
}
