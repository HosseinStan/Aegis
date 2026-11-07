package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

func TestLabApprovalFileRequiresUniqueLiteralEndpointsAndKeys(t *testing.T) {
	empty, err := loadPins("")
	require.NoError(t, err)
	require.Empty(t, empty)
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	key, err := ssh.NewPublicKey(pub)
	require.NoError(t, err)
	encoded := string(ssh.MarshalAuthorizedKey(key))
	target := func(address, key string) map[string]string {
		return map[string]string{"address": address, "host_key": key}
	}
	one := target("192.0.2.41:22", encoded)
	two := target("[2001:db8::2]:2222", encoded)
	path := filepath.Join(t.TempDir(), "approval.json")
	write := func(entries ...map[string]string) {
		body, err := json.Marshal(map[string]any{"targets": entries})
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path, body, 0o600))
	}
	write(one, two)
	pins, err := loadPins(path)
	require.NoError(t, err)
	require.Len(t, pins, 2)
	for _, test := range []struct {
		name    string
		targets []map[string]string
	}{
		{"duplicate", []map[string]string{one, one}},
		{"three", []map[string]string{one, two, target("192.0.2.42:22", encoded)}},
		{"hostname", []map[string]string{target("switch.example:22", encoded)}},
		{"no host key", []map[string]string{target("192.0.2.41:22", "")}},
		{"invalid port", []map[string]string{target("192.0.2.41:0", encoded)}},
	} {
		t.Run(test.name, func(t *testing.T) {
			write(test.targets...)
			_, err := loadPins(path)
			require.Error(t, err)
		})
	}
}
