package risk

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"time"
)

var ErrIntegrity = errors.New("risk evidence integrity failure")
var ErrIneligible = errors.New("risk evidence is ineligible")
var ErrLimit = errors.New("risk evidence exceeds supported limits")

const UTC9 = "2006-01-02T15:04:05.000000000Z"

var timestampV1 = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-5][0-9](?:\.[0-9]{1,9})?(?:Z|[+-](?:[01][0-9]|2[0-3]):[0-5][0-9])$`)

// NormalizeTimestampV1 is a frozen parser ABI. No trimming, coercion or clock.
func NormalizeTimestampV1(raw string) (string, error) {
	if len(raw) < 20 || len(raw) > 35 || !timestampV1.MatchString(raw) {
		return "", ErrIntegrity
	}
	t, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil || t.IsZero() || t.UTC().Year() < 1 || t.UTC().Year() > 9999 {
		return "", ErrIntegrity
	}
	return t.UTC().Format(UTC9), nil
}
func validCanonicalTime(s string) bool {
	v, err := NormalizeTimestampV1(s)
	return err == nil && v == s
}
func Digest(domain string, data []byte) string {
	h := sha256.New()
	_, _ = h.Write([]byte(domain + "\n"))
	_, _ = h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

// StrictDecode rejects duplicates at every nesting level, unknown fields,
// multiple values, and noncanonical types/encodings by byte-for-byte replay.
func StrictDecode[T any](raw string, dst *T) error {
	if len(raw) > 4<<20 {
		return ErrLimit
	}
	tokens := json.NewDecoder(bytes.NewBufferString(raw))
	tokens.UseNumber()
	if err := uniqueJSON(tokens, 0); err != nil {
		return ErrIntegrity
	}
	if _, err := tokens.Token(); err != io.EOF {
		return ErrIntegrity
	}
	dec := json.NewDecoder(bytes.NewBufferString(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return ErrIntegrity
	}
	encoded, err := json.Marshal(dst)
	if err != nil || string(encoded) != raw {
		return ErrIntegrity
	}
	return nil
}
func uniqueJSON(dec *json.Decoder, depth int) error {
	if depth > 32 {
		return ErrIntegrity
	}
	token, err := dec.Token()
	if err != nil {
		return err
	}
	delim, container := token.(json.Delim)
	if !container {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for dec.More() {
			key, err := dec.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return ErrIntegrity
			}
			seen[name] = true
			if err := uniqueJSON(dec, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for dec.More() {
			if err := uniqueJSON(dec, depth+1); err != nil {
				return err
			}
		}
	default:
		return ErrIntegrity
	}
	end, err := dec.Token()
	if err != nil {
		return err
	}
	if (delim == '{' && end != json.Delim('}')) || (delim == '[' && end != json.Delim(']')) {
		return ErrIntegrity
	}
	return nil
}
