package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLabDataRootIsPrivateAndReopensWithSameDPAPIKey(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	path := filepath.Join(base, "lab")
	first, err := openLabStorage(path)
	require.NoError(t, err)
	defer clear(first.key)
	_, err = openLabStorage(path)
	require.Error(t, err)
	require.NoError(t, first.lock.Close())
	reopened, err := openLabStorage(path)
	require.NoError(t, err)
	defer clear(reopened.key)
	defer reopened.lock.Close()
	require.True(t, bytes.Equal(first.key, reopened.key))
	keyFile, err := os.ReadFile(filepath.Join(path, "credential-key.dpapi"))
	require.NoError(t, err)
	require.False(t, bytes.Contains(keyFile, first.key))
	repo := filepath.Join(base, "repo")
	require.NoError(t, os.Mkdir(repo, 0o700))
	require.NoError(t, os.Mkdir(filepath.Join(repo, ".git"), 0o700))
	_, err = openLabStorage(filepath.Join(repo, "lab"))
	require.Error(t, err)
}
