package api_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestComposeAndCIYAML(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	for _, relative := range []string{"compose.yaml", filepath.Join(".github", "workflows", "ci.yaml")} {
		raw, err := os.ReadFile(filepath.Join(root, relative))
		require.NoError(t, err)
		var parsed map[string]any
		require.NoError(t, yaml.Unmarshal(raw, &parsed), relative)
		require.NotEmpty(t, parsed, relative)
	}
}

func TestDockerfileHardeningDirectives(t *testing.T) {
	path := filepath.Join("..", "Dockerfile")
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	content := string(raw)
	require.GreaterOrEqual(t, strings.Count(content, "FROM "), 2)
	require.Contains(t, content, "USER 10001:10001")
	require.Contains(t, content, "HEALTHCHECK")
	require.Contains(t, content, "CGO_ENABLED=0")
}
