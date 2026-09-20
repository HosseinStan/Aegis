package cisco_ios_ssh

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"company.local/aegis/apps/aegis-api/internal/domain"
)

var (
	hostnameVersionPattern = regexp.MustCompile(`(?m)^([^\s]+) uptime is (.+)$`)
	versionPattern         = regexp.MustCompile(`(?mi)Cisco IOS(?: XE)? Software.*Version\s+([^,\s]+)`)
	modelPattern           = regexp.MustCompile(`(?mi)^cisco\s+(\S+).*processor`)
	serialPattern          = regexp.MustCompile(`(?mi)(?:SN|System Serial Number)\s*:\s*([^\s,]+)`)
)

func makeConfigResult(device domain.Device, kind, content string) domain.ConfigResult {
	hash := sha256.Sum256([]byte(content))
	return domain.ConfigResult{DeviceID: device.ID, Hostname: device.Hostname, Vendor: device.Vendor, Platform: device.Platform, Protocol: device.Protocol, ConfigurationType: kind, RetrievedAt: time.Now().UTC(), ContentHash: fmt.Sprintf("%x", hash), Config: content}
}

func parseFacts(version, inventory string) domain.DeviceFacts {
	facts := domain.DeviceFacts{Software: "Cisco IOS", RawSummary: firstLines(version, 5)}
	if match := hostnameVersionPattern.FindStringSubmatch(version); len(match) == 3 {
		facts.Hostname, facts.Uptime = match[1], match[2]
	}
	if match := versionPattern.FindStringSubmatch(version); len(match) == 2 {
		facts.Version = match[1]
	}
	if match := modelPattern.FindStringSubmatch(version); len(match) == 2 {
		facts.Model = match[1]
	}
	if match := serialPattern.FindStringSubmatch(inventory); len(match) == 2 {
		facts.SerialNumber = match[1]
	}
	return facts
}

func parseInterfaces(output string) []domain.InterfaceStatus {
	var result []domain.InterfaceStatus
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 6 || strings.EqualFold(fields[0], "port") || strings.HasPrefix(fields[0], "--") {
			continue
		}
		statusIndex := -1
		for i := 1; i < len(fields); i++ {
			if isInterfaceStatus(fields[i]) {
				statusIndex = i
				break
			}
		}
		if statusIndex < 1 || len(fields)-statusIndex < 5 {
			continue
		}
		item := domain.InterfaceStatus{Name: fields[0], Description: strings.Join(fields[1:statusIndex], " "), Status: fields[statusIndex], VLAN: fields[statusIndex+1], Duplex: fields[statusIndex+2], Speed: fields[statusIndex+3], Type: strings.Join(fields[statusIndex+4:], " ")}
		result = append(result, item)
	}
	return result
}

func isInterfaceStatus(value string) bool {
	switch strings.ToLower(value) {
	case "connected", "notconnect", "disabled", "err-disabled", "inactive", "monitoring", "sfpabsent":
		return true
	default:
		return false
	}
}

func firstLines(value string, count int) string {
	lines := strings.Split(strings.TrimSpace(value), "\n")
	if len(lines) > count {
		lines = lines[:count]
	}
	return strings.Join(lines, "\n")
}

var _ = strconv.IntSize
