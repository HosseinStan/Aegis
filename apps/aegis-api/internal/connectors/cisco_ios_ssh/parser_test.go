package cisco_ios_ssh

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSanitizeOutput(t *testing.T) {
	raw := "\x1b[2Joffice-sw1#show running-config\r\nversion 17.9\r\nhostname office-sw1\r\nend\r\noffice-sw1#"
	clean := sanitizeOutput(raw, "show running-config")
	require.Equal(t, "version 17.9\nhostname office-sw1\nend\n", clean)
}

func TestParseFactsAndInterfaces(t *testing.T) {
	version := "OFFICE-SW1 uptime is 2 weeks, 3 days\nCisco IOS XE Software, Version 17.09.04a\ncisco C9300-24T (X86) processor"
	inventory := "NAME: chassis\nPID: C9300-24T, VID: V01, SN: FCW12345678"
	facts := parseFacts(version, inventory)
	require.Equal(t, "OFFICE-SW1", facts.Hostname)
	require.Equal(t, "17.09.04a", facts.Version)
	require.Equal(t, "C9300-24T", facts.Model)
	require.Equal(t, "FCW12345678", facts.SerialNumber)

	output := "Port Name Status Vlan Duplex Speed Type\nGi1/0/1 Office AP connected 20 a-full a-1000 10/100/1000BaseTX\nGi1/0/2 notconnect 1 auto auto 10/100/1000BaseTX"
	interfaces := parseInterfaces(output)
	require.Len(t, interfaces, 2)
	require.Equal(t, "Office AP", interfaces[0].Description)
	require.Equal(t, "connected", interfaces[0].Status)
}

func TestDetectCiscoCLIError(t *testing.T) {
	require.True(t, ciscoErrorPattern.MatchString("% Invalid input detected at '^' marker."))
}
