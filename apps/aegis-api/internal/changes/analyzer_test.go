package changes

import (
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAnalyze(t *testing.T) {
	item := func(op, command string, path ...string) Item {
		return Item{Operation: op, Command: command, ContextPath: append([]string{}, path...)}
	}
	for _, test := range []struct {
		name, before, after string
		want                []Item
	}{
		{"identical", "hostname LAB\n", "hostname LAB\n", []Item{}},
		{"top level addition", "", "hostname LAB\n", []Item{item("add", "hostname LAB")}},
		{"top level removal", "hostname LAB\n", "", []Item{item("remove", "hostname LAB")}},
		{"replacement order", "hostname OLD\nservice old\n", "hostname NEW\nservice new\n",
			[]Item{item("remove", "hostname OLD"), item("remove", "service old"), item("add", "hostname NEW"), item("add", "service new")}},
		{"interface context", "interface Gi1/0/1\n description old\n", "interface Gi1/0/1\n description new\n shutdown\n",
			[]Item{item("remove", "description old", "interface Gi1/0/1"), item("add", "description new", "interface Gi1/0/1"), item("add", "shutdown", "interface Gi1/0/1")}},
		{"nested contexts", "router bgp 65000\n address-family ipv4\n  neighbor 192.0.2.1 activate\n",
			"router bgp 65000\n address-family ipv4\n  neighbor 192.0.2.2 activate\n",
			[]Item{item("remove", "neighbor 192.0.2.1 activate", "router bgp 65000", "address-family ipv4"), item("add", "neighbor 192.0.2.2 activate", "router bgp 65000", "address-family ipv4")}},
		{"identical children have different identities", "interface A\n shutdown\ninterface B\n",
			"interface A\ninterface B\n shutdown\n", []Item{item("remove", "shutdown", "interface A"), item("add", "shutdown", "interface B")}},
		{"only second identical child changes", "interface A\n shutdown\n!\ninterface B\n shutdown\n",
			"interface A\n shutdown\n!\ninterface B\n no shutdown\n", []Item{item("remove", "shutdown", "interface B"), item("add", "no shutdown", "interface B")}},
		{"malformed text remains distinct", "unknown \xff\n", "unknown \xfe\n",
			[]Item{item("remove", "unknown \xff"), item("add", "unknown \xfe")}},
		{"separator resets context", "", "interface A\n shutdown\n ! \n description detached\n",
			[]Item{item("add", "interface A"), item("add", "shutdown", "interface A"), item("add", "description detached")}},
		{"comments and blanks ignored", "hostname LAB\n! old comment\n\n!\n", "! new comment\nhostname LAB\n\n\n!\n", []Item{}},
		{"CRLF normalized", "interface A\n shutdown\n", "interface A\r\n shutdown\r\n", []Item{}},
		{"CR normalized", "interface A\n shutdown\n", "interface A\r shutdown\r", []Item{}},
		{"outer whitespace canonicalized", "hostname LAB\n", " hostname LAB  \n", []Item{}},
		{"unknown commands and decreasing indentation", "", "  unknown root\n    opaque child\n   sibling\nnext root\n\tother child\n",
			[]Item{item("add", "unknown root"), item("add", "opaque child", "unknown root"), item("add", "sibling", "unknown root"), item("add", "next root"), item("add", "other child", "next root")}},
		{"comment does not become context", "", "interface A\n ! note\n shutdown\n",
			[]Item{item("add", "interface A"), item("add", "shutdown", "interface A")}},
		{"context rename changes child identity", "interface A\n shutdown\n", "interface B\n shutdown\n",
			[]Item{item("remove", "interface A"), item("remove", "shutdown", "interface A"), item("add", "interface B"), item("add", "shutdown", "interface B")}},
	} {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, Analyze(test.before, test.after))
			require.Equal(t, Analyze(test.before, test.after), Analyze(test.before, test.after))
		})
	}
}

func TestAnalyzeMatchesBeforeRedaction(t *testing.T) {
	const first = "SENTINEL_FIRST_PRIVATE_VALUE"
	const second = "SENTINEL_SECOND_PRIVATE_VALUE"
	items := Analyze("enable secret 9 "+first+"\n", "enable secret 9 "+second+"\n")
	require.Equal(t, 2, len(items)) // Redacted text is equal, but raw inputs changed.
	for i, item := range items {
		require.True(t, item.Sensitive)
		require.False(t, strings.Contains(item.Command, first) || strings.Contains(item.Command, second))
		require.Equal(t, "enable secret <redacted>", item.Command)
		require.Equal(t, []string{"remove", "add"}[i], item.Operation)
	}
}

func TestAnalyzeRedactsSecretAncestors(t *testing.T) {
	const secret = "SENTINEL_ANCESTOR_PRIVATE_VALUE"
	items := Analyze("", "unknown password "+secret+"\n child safe\n")
	require.Equal(t, 2, len(items))
	for _, item := range items {
		require.True(t, item.Sensitive)
		require.False(t, strings.Contains(item.Command, secret))
		for _, parent := range item.ContextPath {
			require.False(t, strings.Contains(parent, secret))
		}
	}
	require.Equal(t, []string{"unknown password <redacted>"}, items[1].ContextPath)
}

func FuzzAnalyzeDeterministic(f *testing.F) {
	f.Add("interface A\n shutdown\n", "interface B\r\n shutdown\r\n")
	f.Add("\x00\t malformed\r!", "!\nunknown password example\n")
	f.Fuzz(func(t *testing.T, before, after string) {
		if !reflect.DeepEqual(Analyze(before, after), Analyze(before, after)) {
			t.Fatal("analysis was not deterministic")
		}
	})
}
