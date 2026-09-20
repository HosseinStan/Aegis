package security

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClientIPUsesForwardedChainOnlyForTrustedPeer(t *testing.T) {
	trusted, err := ParseCIDRs([]string{"172.16.0.0/12"})
	require.NoError(t, err)
	request := httptest.NewRequest("GET", "http://aegis.internal", nil)
	request.RemoteAddr = "172.20.0.3:12345"
	request.Header.Set("X-Forwarded-For", "203.0.113.10, 172.20.0.2")
	require.Equal(t, "203.0.113.10", ClientIP(request, trusted))
	request.RemoteAddr = "198.51.100.5:12345"
	require.Equal(t, "198.51.100.5", ClientIP(request, trusted))
}
