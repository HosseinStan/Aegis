package api_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"
)

func TestCanonicalOpenAPIContract(t *testing.T) {
	path := filepath.Join("..", "..", "..", "packages", "shared-contracts", "openapi", "aegis-api.yaml")
	document, err := openapi3.NewLoader().LoadFromFile(path)
	require.NoError(t, err)
	require.NoError(t, document.Validate(context.Background()))
}
