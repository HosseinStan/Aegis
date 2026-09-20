package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEvaluationConsoleAssets(t *testing.T) {
	server := httptest.NewServer(TestConsole())
	t.Cleanup(server.Close)

	for _, asset := range []string{"/test-console/index.html", "/test-console/app.js", "/test-console/style.css"} {
		response, err := server.Client().Get(server.URL + asset)
		require.NoError(t, err)
		body, readErr := io.ReadAll(response.Body)
		require.NoError(t, response.Body.Close())
		require.NoError(t, readErr)
		require.Equal(t, http.StatusOK, response.StatusCode)
		require.NotEmpty(t, body)
	}
}
