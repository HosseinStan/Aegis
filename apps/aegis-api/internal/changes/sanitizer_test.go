package changes

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSanitizeSecretForms(t *testing.T) {
	const secret = "SENTINEL_DO_NOT_PERSIST"
	for _, test := range []struct {
		name, command, want string
		context             []string
	}{
		{"enable secret", "enable secret 9 " + secret, "enable secret <redacted>", nil},
		{"enable password", "enable password " + secret, "enable password <redacted>", nil},
		{"username secret", "username admin privilege 15 secret 9 " + secret, "username admin privilege 15 secret <redacted>", nil},
		{"username password", "username admin password 7 " + secret, "username admin password <redacted>", nil},
		{"standalone password", "password " + secret, "password <redacted>", []string{"line vty 0 4"}},
		{"neighbor password", "neighbor 192.0.2.1 password " + secret, "neighbor 192.0.2.1 password <redacted>", []string{"router bgp 65000"}},
		{"SNMP community", "snmp-server community " + secret + " RO", "snmp-server community <redacted>", nil},
		{"RADIUS server key", "radius-server host 192.0.2.1 key 7 " + secret, "radius-server host 192.0.2.1 key <redacted>", nil},
		{"TACACS server key", "tacacs-server host 192.0.2.1 key " + secret, "tacacs-server host 192.0.2.1 key <redacted>", nil},
		{"RADIUS contextual key", "key 7 " + secret, "key <redacted>", []string{"radius server LAB"}},
		{"TACACS contextual key", "key " + secret, "key <redacted>", []string{"tacacs server LAB"}},
		{"key string", "key-string " + secret, "key-string <redacted>", []string{"key chain LAB", "key 1"}},
		{"ISAKMP key", "crypto isakmp key " + secret + " address 192.0.2.1", "crypto isakmp key <redacted>", nil},
		{"NTP key", "ntp authentication-key 1 md5 " + secret, "ntp authentication-key <redacted>", nil},
		{"OSPF authentication", "ip ospf authentication-key " + secret, "ip ospf authentication-key <redacted>", []string{"interface A"}},
		{"OSPF digest", "ip ospf message-digest-key 1 md5 " + secret, "ip ospf message-digest-key <redacted>", []string{"interface A"}},
		{"PPP CHAP", "ppp chap password 0 " + secret, "ppp chap password <redacted>", []string{"interface A"}},
		{"pre shared key", "pre-shared-key local " + secret, "pre-shared-key <redacted>", nil},
		{"passphrase", "passphrase " + secret, "passphrase <redacted>", nil},
		{"WPA PSK", "wpa-psk ascii 0 " + secret, "wpa-psk <redacted>", nil},
		{"explicit unknown token", "vendor-command auth-password " + secret, "vendor-command auth-password <redacted>", nil},
		{"assignment", "vendor password=" + secret, "vendor password=<redacted>", nil},
		{"colon assignment", "vendor secret:" + secret, "vendor secret:<redacted>", nil},
		{"case and whitespace", "  ENABLE\tSECRET 9 " + secret, "ENABLE SECRET <redacted>", nil},
		{"SNMPv3 auth and privacy", "snmp-server user admin group auth sha " + secret + " priv aes 128 " + secret, "snmp-server user admin group auth <redacted>", nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			command, path, sensitive := Sanitize(test.command, test.context)
			require.False(t, strings.Contains(command, secret))
			for _, parent := range path {
				require.False(t, strings.Contains(parent, secret))
			}
			require.True(t, sensitive)
			require.Equal(t, test.want, command)
			again, ancestors, marked := Sanitize(command, path)
			require.Equal(t, command, again)
			require.Equal(t, path, ancestors)
			require.True(t, marked)
		})
	}
}

func TestSanitizeNonSecretCommands(t *testing.T) {
	for _, command := range []string{"crypto key generate rsa modulus 2048", "key 1", "description key management", "unknown command", "password", ""} {
		got, path, sensitive := Sanitize(command, []string{"interface A"})
		require.Equal(t, command, got)
		require.Equal(t, []string{"interface A"}, path)
		require.False(t, sensitive)
	}
}

func TestSanitizeEveryContextElement(t *testing.T) {
	const secret = "SENTINEL_CONTEXT_PRIVATE_VALUE"
	context := []string{"radius server LAB", "key " + secret, "unknown password " + secret}
	command, path, sensitive := Sanitize("child safe", context)
	require.Equal(t, "child safe", command)
	require.True(t, sensitive)
	for _, parent := range path {
		require.False(t, strings.Contains(parent, secret))
	}
	require.Equal(t, []string{"radius server LAB", "key <redacted>", "unknown password <redacted>"}, path)
	// Caller-owned input is not modified, even when multiple ancestors are redacted.
	require.True(t, strings.Contains(context[1], secret))
}
