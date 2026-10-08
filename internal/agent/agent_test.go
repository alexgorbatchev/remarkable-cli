package agent

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestIsAgentMode(t *testing.T) {
	tests := []struct {
		envVal string
		want   bool
	}{
		{"", false},
		{"0", false},
		{"false", false},
		{"no", false},
		{"1", true},
		{"true", true},
		{"yes", true},
		{"TRUE", true},
		{"YES", true},
		{" 1 ", true},
	}

	for _, tt := range tests {
		t.Run(tt.envVal, func(t *testing.T) {
			orig := os.Getenv("AGENT")
			defer os.Setenv("AGENT", orig)

			os.Setenv("AGENT", tt.envVal)
			if got := IsAgentMode(); got != tt.want {
				t.Errorf("IsAgentMode() with AGENT=%q = %v, want %v", tt.envVal, got, tt.want)
			}
		})
	}
}

func TestPrintStatus_HumanVsAgent(t *testing.T) {
	orig := os.Getenv("AGENT")
	defer os.Setenv("AGENT", orig)

	// Human mode
	os.Unsetenv("AGENT")
	var bufHuman bytes.Buffer
	PrintStatus(&bufHuman, "ok", "Operation completed")
	PrintStatus(&bufHuman, "error", "Failed to connect")

	if !strings.Contains(bufHuman.String(), "[OK]") {
		t.Errorf("expected [OK] tag in human mode, got: %s", bufHuman.String())
	}
	if !strings.Contains(bufHuman.String(), "[ERROR]") {
		t.Errorf("expected [ERROR] tag in human mode, got: %s", bufHuman.String())
	}

	// Agent mode
	os.Setenv("AGENT", "1")
	var bufAgent bytes.Buffer
	PrintStatus(&bufAgent, "ok", "Operation completed")
	PrintStatus(&bufAgent, "error", "Failed to connect")

	if !strings.Contains(bufAgent.String(), "OK: Operation completed") {
		t.Errorf("expected OK: in agent mode, got: %s", bufAgent.String())
	}
	if !strings.Contains(bufAgent.String(), "ERR: Failed to connect") {
		t.Errorf("expected ERR: in agent mode, got: %s", bufAgent.String())
	}
}

func TestPrintTable_HumanVsAgent(t *testing.T) {
	orig := os.Getenv("AGENT")
	defer os.Setenv("AGENT", orig)

	headers := []string{"ID", "NAME", "TYPE"}
	rows := [][]string{
		{"doc-1", "Daily Planner", "notebook"},
		{"doc-2", "Quick Notes", "notebook"},
	}

	// Human mode
	os.Unsetenv("AGENT")
	var bufHuman bytes.Buffer
	PrintTable(&bufHuman, headers, rows)
	if !strings.Contains(bufHuman.String(), "ID") || !strings.Contains(bufHuman.String(), "Daily Planner") {
		t.Errorf("unexpected human table output: %s", bufHuman.String())
	}

	// Agent mode
	os.Setenv("AGENT", "1")
	var bufAgent bytes.Buffer
	PrintTable(&bufAgent, headers, rows)
	expectedTSV := "ID\tNAME\tTYPE\ndoc-1\tDaily Planner\tnotebook\ndoc-2\tQuick Notes\tnotebook\n"
	if bufAgent.String() != expectedTSV {
		t.Errorf("agent table want %q, got %q", expectedTSV, bufAgent.String())
	}
}

func TestPrintKeyValues_HumanVsAgent(t *testing.T) {
	orig := os.Getenv("AGENT")
	defer os.Setenv("AGENT", orig)

	pairs := []KeyValuePair{
		{Key: "Status", Value: "connected"},
		{Key: "Items", Value: "42"},
	}

	// Human mode
	os.Unsetenv("AGENT")
	var bufHuman bytes.Buffer
	PrintKeyValues(&bufHuman, pairs)
	if !strings.Contains(bufHuman.String(), "Status:") {
		t.Errorf("unexpected human key values: %s", bufHuman.String())
	}

	// Agent mode
	os.Setenv("AGENT", "1")
	var bufAgent bytes.Buffer
	PrintKeyValues(&bufAgent, pairs)
	expected := "Status: connected\nItems: 42\n"
	if bufAgent.String() != expected {
		t.Errorf("agent key values want %q, got %q", expected, bufAgent.String())
	}
}

func TestPrintTree_HumanVsAgent(t *testing.T) {
	orig := os.Getenv("AGENT")
	defer os.Setenv("AGENT", orig)

	tree := &TreeNode{
		Label: "Root",
		Children: []*TreeNode{
			{Label: "Folder A", Children: []*TreeNode{{Label: "Doc 1"}}},
			{Label: "Doc 2"},
		},
	}

	// Human mode
	os.Unsetenv("AGENT")
	var bufHuman bytes.Buffer
	PrintTree(&bufHuman, tree)
	if !strings.Contains(bufHuman.String(), "├── Folder A") || !strings.Contains(bufHuman.String(), "└── Doc 2") {
		t.Errorf("unexpected human tree output: %s", bufHuman.String())
	}

	// Agent mode
	os.Setenv("AGENT", "1")
	var bufAgent bytes.Buffer
	PrintTree(&bufAgent, tree)
	if !strings.Contains(bufAgent.String(), "* Root\n  * Folder A\n    * Doc 1\n  * Doc 2") {
		t.Errorf("unexpected agent tree output: %s", bufAgent.String())
	}
}

func TestTerminalWidthAndSeparator(t *testing.T) {
	orig := os.Getenv("AGENT")
	defer os.Setenv("AGENT", orig)

	_ = TerminalWidth()

	// Human separator
	os.Unsetenv("AGENT")
	var bufHuman bytes.Buffer
	PrintSeparator(&bufHuman)
	if len(bufHuman.String()) == 0 {
		t.Error("expected non-empty separator in human mode")
	}

	// Agent separator (should be omitted)
	os.Setenv("AGENT", "1")
	var bufAgent bytes.Buffer
	PrintSeparator(&bufAgent)
	if len(bufAgent.String()) != 0 {
		t.Errorf("expected empty separator in agent mode, got %q", bufAgent.String())
	}
}

func TestPrintTree_Nil(t *testing.T) {
	var buf bytes.Buffer
	PrintTree(&buf, nil)
	if buf.Len() != 0 {
		t.Errorf("expected empty for nil tree")
	}
}

func TestPrintTable_Escaping(t *testing.T) {
	orig := os.Getenv("AGENT")
	defer os.Setenv("AGENT", orig)
	os.Setenv("AGENT", "1")

	headers := []string{"ID", "NAME", "TYPE", "MODIFIED"}
	rows := [][]string{
		{"id-1", "Notes\tDocumentType\nfake-id\tInjected", "DocumentType", "1700000000000"},
	}

	var buf bytes.Buffer
	PrintTable(&buf, headers, rows)
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("PrintTable produced %d lines, want 2 (header + 1 row): %q", len(lines), buf.String())
	}
	for i, line := range lines {
		fields := strings.Split(line, "\t")
		if len(fields) != len(headers) {
			t.Errorf("line %d has %d fields, want %d: %q", i, len(fields), len(headers), line)
		}
	}
}

func TestPrintTable_HumanModeEscaping(t *testing.T) {
	orig := os.Getenv("AGENT")
	defer os.Setenv("AGENT", orig)
	os.Unsetenv("AGENT")

	headers := []string{"ID", "NAME"}
	rows := [][]string{
		{"id-1", "Escape\x1b[2J\a and tab\tand\nnewline"},
	}

	var buf bytes.Buffer
	PrintTable(&buf, headers, rows)
	out := buf.String()

	if strings.Contains(out, "\x1b") {
		t.Errorf("human PrintTable output contains raw ESC: %q", out)
	}
	if strings.Contains(out, "\a") {
		t.Errorf("human PrintTable output contains raw BEL: %q", out)
	}
	if !strings.Contains(out, `Escape\x1b[2J\a and tab\tand\nnewline`) {
		t.Errorf("human PrintTable output does not contain expected escaped cell: %q", out)
	}
}


func TestPrintTree_Escaping(t *testing.T) {
	orig := os.Getenv("AGENT")
	defer os.Setenv("AGENT", orig)
	os.Setenv("AGENT", "1")

	tree := &TreeNode{
		Label: "/",
		Children: []*TreeNode{
			{Label: "a\n* forged"},
		},
	}

	var buf bytes.Buffer
	PrintTree(&buf, tree)
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("PrintTree produced %d lines, want 2 bullets: %q", len(lines), buf.String())
	}
	if !strings.Contains(lines[1], `a\n* forged`) {
		t.Errorf("bullet label not escaped: %q", lines[1])
	}
}

func TestPrintableText(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "plain ASCII", in: "Chapter 1: Getting Started", want: "Chapter 1: Getting Started"},
		{name: "empty", in: "", want: ""},
		{name: "backslash", in: `C:\docs\plan.pdf`, want: `C:\\docs\\plan.pdf`},
		{name: "C0 controls", in: "bell\a backspace\b lf\n cr\r tab\t vtab\v ff\f esc\x1b", want: `bell\a backspace\b lf\n cr\r tab\t vtab\v ff\f esc\x1b`},
		{name: "NUL and DEL", in: "start\x00middle\x7fend", want: `start\x00middle\x7fend`},
		{name: "C1 controls", in: "a\u0080b\u009fc", want: `a\u0080b\u009fc`},
		{name: "unicode format characters", in: "soft\u00adhyphen zero\u200bwidth right-to-left\u202eoverride", want: `soft\u00adhyphen zero\u200bwidth right-to-left\u202eoverride`},
		{name: "line separator", in: "first\u2028second", want: `first\u2028second`},
		{name: "private use", in: "icon:\ue000", want: `icon:\ue000`},
		{name: "invalid UTF-8 bytes", in: "bad:\xff\xfe", want: `bad:\xff\xfe`},
		{name: "truncated UTF-8 sequence", in: "lead:\xe2\x80", want: `lead:\xe2\x80`},
		{name: "printable non-ASCII letters", in: "Grüße, Welt! 🚀 — 100%", want: "Grüße, Welt! 🚀 — 100%"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := PrintableText(tc.in)
			if got != tc.want {
				t.Errorf("PrintableText(%q) = %q, want %q", tc.in, got, tc.want)
			}
			if decoded := unescapeText(t, got); decoded != tc.in {
				t.Errorf("decoding PrintableText(%q) = %q gives %q", tc.in, got, decoded)
			}
		})
	}
}

func unescapeText(t *testing.T, s string) string {
	t.Helper()
	decoded, err := UnescapeText(s)
	if err != nil {
		t.Fatalf("UnescapeText(%q): %v", s, err)
	}
	return decoded
}
