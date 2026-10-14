package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"company.local/aegis/apps/aegis-api/migrations"
	_ "modernc.org/sqlite"
)

type Store struct{ db *sql.DB }

func Open(ctx context.Context, dsn string) (*Store, error) {
	connectionDSN, filename, err := normalizeSQLiteDSN(dsn)
	if err != nil {
		return nil, err
	}
	if filename != "" {
		if err := ensureParent(filename); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", connectionDSN)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	store := &Store{db: db}
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON; PRAGMA journal_mode = WAL; PRAGMA busy_timeout = 5000;"); err != nil {
		db.Close()
		return nil, fmt.Errorf("configure sqlite: %w", err)
	}
	if err := store.Migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

// Keep the filename/URI prefix and unrelated query fields byte-for-byte. The
// driver parses _pragma on EVERY physical Open, including pool replacements.
// Re-encoding a whole URI can change Windows paths or SQLite's literal '+' query
// semantics; only the validated security fields are replaced with fixed values.
func normalizeSQLiteDSN(dsn string) (connectionDSN, filename string, err error) {
	if strings.TrimSpace(dsn) == "" || !utf8.ValidString(dsn) || sqliteDSNControl(dsn) {
		return "", "", errors.New("invalid sqlite DSN: empty or invalid text")
	}
	base, query, _ := strings.Cut(dsn, "?")
	if base == "" {
		return "", "", errors.New("invalid sqlite DSN: missing database identity")
	}
	memory := base == ":memory:"
	if strings.HasPrefix(base, "file:") {
		u, parseErr := url.Parse(base)
		if parseErr != nil || strings.Contains(dsn, "#") || u.User != nil ||
			(u.Host != "" && u.Host != "localhost") {
			return "", "", errors.New("invalid sqlite DSN: ambiguous file URI")
		}
		path := u.Path
		if u.Opaque != "" {
			path, parseErr = url.PathUnescape(u.Opaque)
		}
		if parseErr != nil || path == "" || !utf8.ValidString(path) || sqliteDSNControl(path) {
			return "", "", errors.New("invalid sqlite DSN: invalid file URI path")
		}
		memory = path == ":memory:"
	} else if base != ":memory:" {
		// Do not interpret a Windows drive as a URI scheme, or convert a plain
		// filename (including literal percent signs) into an encoded URI.
		colon := strings.IndexByte(base, ':')
		drive := colon == 1 && ((base[0] >= 'A' && base[0] <= 'Z') || (base[0] >= 'a' && base[0] <= 'z'))
		separator := strings.IndexAny(base, "/\\")
		if colon >= 0 && !drive && (separator < 0 || colon < separator) {
			return "", "", errors.New("invalid sqlite DSN: unsupported scheme or ambiguous filename")
		}
		filename = base
	}
	options, parseErr := url.ParseQuery(query)
	if parseErr != nil {
		return "", "", errors.New("invalid sqlite DSN: malformed query encoding")
	}
	if strings.HasPrefix(base, "file:") && options.Get("mode") == "memory" {
		memory = true
	}
	var fields []string
	seen := make(map[string]bool)
	security := make(map[string]bool)
	if query != "" {
		for _, field := range strings.Split(query, "&") {
			rawKey, rawValue, assigned := strings.Cut(field, "=")
			key, keyErr := url.QueryUnescape(rawKey)
			value, valueErr := url.QueryUnescape(rawValue)
			if !assigned || keyErr != nil || valueErr != nil || key == "" ||
				!utf8.ValidString(key) || !utf8.ValidString(value) ||
				sqliteDSNControl(key) || sqliteDSNControl(value) {
				return "", "", errors.New("invalid sqlite DSN: malformed query field")
			}
			if key != "_pragma" {
				if seen[key] {
					return "", "", errors.New("invalid sqlite DSN: duplicate query option")
				}
				seen[key] = true
				fields = append(fields, field)
				continue
			}
			name, argument, valid := sqliteDSNPragma(value)
			if !valid {
				return "", "", errors.New("invalid sqlite DSN: malformed PRAGMA option")
			}
			// These reviewed options affect cache sizing, file identification,
			// or text encoding, not constraint/isolation/durability enforcement.
			// Keep their existing scalar syntax and original bytes.
			switch name {
			case "cache_size", "application_id", "encoding":
				fields = append(fields, field)
				continue
			}
			if security[name] {
				return "", "", errors.New("invalid sqlite DSN: duplicate security PRAGMA")
			}
			security[name] = true
			switch name {
			case "foreign_keys", "busy_timeout":
				enabled := argument == "1" || argument == "on" || argument == "true" || argument == "yes"
				if (name == "foreign_keys" && !enabled) || (name == "busy_timeout" && argument != "5000") {
					return "", "", errors.New("invalid sqlite DSN: conflicting security PRAGMA")
				}
				// Emit exactly one canonical assignment below.
			case "journal_mode":
				if argument != "" && argument != "wal" && !(memory && argument == "memory") {
					return "", "", errors.New("invalid sqlite DSN: conflicting journal mode")
				}
			case "ignore_check_constraints", "writable_schema", "read_uncommitted", "defer_foreign_keys":
				if argument != "0" && argument != "off" && argument != "false" && argument != "no" {
					return "", "", errors.New("invalid sqlite DSN: unsafe integrity PRAGMA")
				}
			case "synchronous":
				// NORMAL is consistent in WAL but can lose committed evidence on
				// power loss; accept only FULL/EXTRA without changing defaults.
				if argument != "2" && argument != "full" && argument != "3" && argument != "extra" {
					return "", "", errors.New("invalid sqlite DSN: unsafe durability PRAGMA")
				}
			case "recursive_triggers":
				// Explicit OFF can bypass DELETE guards during REPLACE. Do not
				// change the driver's default or existing migration semantics.
				if argument != "1" && argument != "on" && argument != "true" && argument != "yes" {
					return "", "", errors.New("invalid sqlite DSN: unsafe trigger PRAGMA")
				}
			default:
				// A denylist is not fail-closed: e.g. schema_version writes also
				// undermine integrity. Unknown/unreviewed PRAGMAs need review
				// before entering this DSN-only supported subset.
				return "", "", errors.New("invalid sqlite DSN: unsupported PRAGMA option")
			}
			if name != "foreign_keys" && name != "busy_timeout" {
				fields = append(fields, field)
			}
		}
	}
	fields = append(fields, "_pragma="+url.QueryEscape("foreign_keys(1)"),
		"_pragma="+url.QueryEscape("busy_timeout(5000)"))
	return base + "?" + strings.Join(fields, "&"), filename, nil
}

func sqliteDSNControl(text string) bool {
	for _, c := range text {
		if c < 0x20 || c == 0x7f {
			return true
		}
	}
	return false
}

// A single PRAGMA identifier, optional schema, and optional scalar argument.
// Quotes/escapes inside a scalar are supported, but comments, expressions and
// trailing statements cannot disguise a second security assignment.
const sqliteDSNIdentifier = `(?:[A-Za-z_][A-Za-z0-9_]*|"[A-Za-z_][A-Za-z0-9_]*"|'[A-Za-z_][A-Za-z0-9_]*'|\[[A-Za-z_][A-Za-z0-9_]*\])`
const sqliteDSNScalar = `(?:[A-Za-z_][A-Za-z0-9_]*|0[xX][0-9A-Fa-f]+|[+-]?[0-9]+(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?|'(?:[^']|'')*'|"(?:[^"]|"")*"|\[[A-Za-z_][A-Za-z0-9_]*\])`

var sqliteDSNPragmaPattern = regexp.MustCompile(
	`^ *(?:(` + sqliteDSNIdentifier + `) *\. *)?(` + sqliteDSNIdentifier +
		`) *(?:= *(` + sqliteDSNScalar + `)|\( *(` + sqliteDSNScalar + `) *\))? *$`)

func sqliteDSNPragma(text string) (name, argument string, valid bool) {
	match := sqliteDSNPragmaPattern.FindStringSubmatch(text)
	if match == nil {
		return "", "", false
	}
	name = strings.ToLower(strings.Trim(match[2], "\"'[]"))
	argument = match[3]
	if match[4] != "" {
		argument = match[4]
	}
	if len(argument) >= 2 {
		switch argument[0] {
		case '\'', '"':
			quote := argument[:1]
			argument = strings.ReplaceAll(argument[1:len(argument)-1], quote+quote, quote)
		case '[':
			argument = argument[1 : len(argument)-1]
		}
	}
	argument = strings.ToLower(argument)
	return name, argument, true
}

func ensureParent(dsn string) error {
	parent := filepath.Dir(dsn)
	if parent == "." {
		return nil
	}
	return os.MkdirAll(parent, 0o750)
}

func (s *Store) Close() error                   { return s.db.Close() }
func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

func (s *Store) Migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		return fmt.Errorf("create migration table: %w", err)
	}
	entries, err := fs.ReadDir(migrations.Files, ".")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		var exists int
		err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, entry.Name()).Scan(&exists)
		if err != nil {
			return fmt.Errorf("check migration %s: %w", entry.Name(), err)
		}
		if exists > 0 {
			continue
		}
		body, err := migrations.Files.ReadFile(entry.Name())
		if err != nil {
			return fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin migration %s: %w", entry.Name(), err)
		}
		if _, err = tx.ExecContext(ctx, string(body)); err == nil {
			_, err = tx.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES (?, ?)`, entry.Name(), time.Now().UTC().Format(time.RFC3339Nano))
		}
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply migration %s: %w", entry.Name(), err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", entry.Name(), err)
		}
	}
	return nil
}

func mapWriteError(err error) error {
	if err == nil {
		return nil
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "unique constraint") || strings.Contains(message, "foreign key constraint") {
		return fmt.Errorf("%w: %v", errConflict, err)
	}
	return err
}

var errConflict = errors.New("database conflict")
