package changes

import "strings"

const redacted = "<redacted>"

// Sanitize removes secret-bearing suffixes from the command and every ancestor.
// Context is inspected only in memory; returned paths contain sanitized text.
// Reapplying it is idempotent, including the sensitivity marker.
func Sanitize(command string, context []string) (string, []string, bool) {
	path := make([]string, len(context))
	sensitive := false
	for i, parent := range context {
		var changed bool
		path[i], changed = sanitizeCommand(parent, context[:i])
		sensitive = sensitive || changed
	}
	text, changed := sanitizeCommand(command, context)
	return text, path, sensitive || changed
}

func sanitizeCommand(command string, context []string) (string, bool) {
	command = strings.TrimSpace(command)
	fields := strings.Fields(command)
	lower := make([]string, len(fields))
	for i, field := range fields {
		lower[i] = strings.ToLower(field)
	}
	for i, token := range lower {
		marker := secretToken(token)
		if token == "community" && len(lower) > 0 && lower[0] == "snmp-server" {
			marker = true
		}
		if token == "key" {
			marker = (len(lower) >= 3 && lower[0] == "crypto" && lower[1] == "isakmp") ||
				(lower[0] == "radius-server" || lower[0] == "tacacs-server") ||
				(i == 0 && serverKeyContext(context))
		}
		// SNMPv3 stores authentication and privacy secrets without a password
		// token. Conservatively discard the whole auth/priv suffix.
		if len(lower) > 1 && lower[0] == "snmp-server" && lower[1] == "user" && (token == "auth" || token == "priv") {
			marker = true
		}
		if marker && i+1 < len(fields) {
			return strings.Join(fields[:i+1], " ") + " " + redacted, true
		}
		// Recognizable password/secret assignments can carry their value in the
		// same token rather than the following token.
		for _, separator := range []string{"=", ":"} {
			name, _, found := strings.Cut(token, separator)
			if found && secretToken(name) {
				return strings.Join(append(fields[:i:i], name+separator+redacted), " "), true
			}
		}
	}
	return command, false
}

func secretToken(token string) bool {
	switch token {
	case "secret", "password", "passwd", "key-string", "pre-shared-key", "passphrase", "wpa-psk", "authentication-key", "message-digest-key":
		return true
	}
	return strings.HasSuffix(token, "-password") || strings.HasSuffix(token, "-secret") || strings.HasSuffix(token, "-passphrase")
}

func serverKeyContext(context []string) bool {
	for _, parent := range context {
		fields := strings.Fields(strings.ToLower(parent))
		if len(fields) >= 2 && (fields[0] == "radius" || fields[0] == "tacacs") && fields[1] == "server" {
			return true
		}
	}
	return false
}
