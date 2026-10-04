package changes

import (
	"strconv"
	"strings"

	"company.local/aegis/apps/aegis-api/internal/domain"
	"github.com/pmezard/go-difflib/difflib"
)

// Item contains only sanitized facts. Configuration text and matching keys
// remain transient inputs and are never included in the returned items.
type Item struct {
	Operation   string
	ContextPath []string
	Command     string
	Sensitive   bool
}

type annotatedLine struct {
	command string
	context []string
	indent  int
}

// Analyze is a deterministic structural transformation, not an IOS grammar or
// classification engine. Unknown commands are handled just like known ones.
func Analyze(before, after string) []Item {
	left, leftKeys := annotate(before)
	right, rightKeys := annotate(after)
	items := make([]Item, 0)
	emit := func(lines []annotatedLine, operation string) {
		for _, line := range lines {
			command, path, sensitive := Sanitize(line.command, line.context)
			items = append(items, Item{Operation: operation, ContextPath: path, Command: command, Sensitive: sensitive})
		}
	}
	// Disable the popularity heuristic: repeated commands are meaningful when
	// scoped to their ancestors, even in large configurations.
	matcher := difflib.NewMatcherWithJunk(leftKeys, rightKeys, false, nil)
	for _, op := range matcher.GetOpCodes() {
		switch op.Tag {
		case 'r':
			emit(left[op.I1:op.I2], domain.ChangeOperationRemove)
			emit(right[op.J1:op.J2], domain.ChangeOperationAdd)
		case 'd':
			emit(left[op.I1:op.I2], domain.ChangeOperationRemove)
		case 'i':
			emit(right[op.J1:op.J2], domain.ChangeOperationAdd)
		}
	}
	return items
}

func annotate(config string) ([]annotatedLine, []string) {
	config = strings.ReplaceAll(strings.ReplaceAll(config, "\r\n", "\n"), "\r", "\n")
	lines := make([]annotatedLine, 0)
	keys := make([]string, 0)
	stack := make([]annotatedLine, 0)
	for _, raw := range strings.Split(config, "\n") {
		command := strings.TrimSpace(raw)
		if command == "!" {
			stack = stack[:0]
			continue
		}
		if command == "" || strings.HasPrefix(command, "!") {
			continue
		}
		indent := indentation(raw)
		for len(stack) > 0 && stack[len(stack)-1].indent >= indent {
			stack = stack[:len(stack)-1]
		}
		path := make([]string, len(stack))
		for i, parent := range stack {
			path[i] = parent.command
		}
		line := annotatedLine{command: command, context: path, indent: indent}
		lines = append(lines, line)
		keys = append(keys, comparisonKey(path, command))
		stack = append(stack, line)
	}
	return lines, keys
}

// Length prefixes keep context boundaries unambiguous without changing raw
// bytes. JSON would collapse distinct invalid UTF-8 sequences to the same key.
func comparisonKey(path []string, command string) string {
	var key strings.Builder
	appendPart := func(part string) {
		key.WriteString(strconv.Itoa(len(part)))
		key.WriteByte(':')
		key.WriteString(part)
	}
	for _, parent := range path {
		appendPart(parent)
	}
	appendPart(command)
	return key.String()
}

func indentation(line string) int {
	column := 0
	for _, r := range line {
		switch r {
		case ' ':
			column++
		case '\t':
			column += 8 - column%8
		default:
			return column
		}
	}
	return column
}
