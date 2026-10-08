package agent

import (
	"bytes"
	"errors"
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
			t.Setenv("AGENT", tt.envVal)
			if got := IsAgentMode(); got != tt.want {
				t.Errorf("IsAgentMode() with AGENT=%q = %v, want %v", tt.envVal, got, tt.want)
			}
		})
	}
}

func TestPrintStatus_HumanVsAgent(t *testing.T) {
	t.Run("human", func(t *testing.T) {
		t.Setenv("AGENT", "")
		var bufHuman bytes.Buffer
		if err := PrintStatus(&bufHuman, "ok", "Operation completed"); err != nil {
			t.Fatal(err)
		}
		if err := PrintStatus(&bufHuman, "error", "Failed to connect"); err != nil {
			t.Fatal(err)
		}

		if !strings.Contains(bufHuman.String(), "[OK]") {
			t.Errorf("expected [OK] tag in human mode, got: %s", bufHuman.String())
		}
		if !strings.Contains(bufHuman.String(), "[ERROR]") {
			t.Errorf("expected [ERROR] tag in human mode, got: %s", bufHuman.String())
		}
	})

	t.Run("agent", func(t *testing.T) {
		t.Setenv("AGENT", "1")
		var bufAgent bytes.Buffer
		if err := PrintStatus(&bufAgent, "ok", "Operation completed"); err != nil {
			t.Fatal(err)
		}
		if err := PrintStatus(&bufAgent, "error", "Failed to connect"); err != nil {
			t.Fatal(err)
		}

		if !strings.Contains(bufAgent.String(), "OK: Operation completed") {
			t.Errorf("expected OK: in agent mode, got: %s", bufAgent.String())
		}
		if !strings.Contains(bufAgent.String(), "ERR: Failed to connect") {
			t.Errorf("expected ERR: in agent mode, got: %s", bufAgent.String())
		}
	})
}

func TestPrintTable_HumanVsAgent(t *testing.T) {
	headers := []string{"ID", "NAME", "TYPE"}
	rows := [][]string{
		{"doc-1", "Daily Planner", "notebook"},
		{"doc-2", "Quick Notes", "notebook"},
	}

	t.Run("human", func(t *testing.T) {
		t.Setenv("AGENT", "")
		var bufHuman bytes.Buffer
		if err := PrintTable(&bufHuman, headers, rows); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(bufHuman.String(), "ID") || !strings.Contains(bufHuman.String(), "Daily Planner") {
			t.Errorf("unexpected human table output: %s", bufHuman.String())
		}
	})

	t.Run("agent", func(t *testing.T) {
		t.Setenv("AGENT", "1")
		var bufAgent bytes.Buffer
		if err := PrintTable(&bufAgent, headers, rows); err != nil {
			t.Fatal(err)
		}
		expectedTSV := "ID\tNAME\tTYPE\ndoc-1\tDaily Planner\tnotebook\ndoc-2\tQuick Notes\tnotebook\n"
		if bufAgent.String() != expectedTSV {
			t.Errorf("agent table want %q, got %q", expectedTSV, bufAgent.String())
		}
	})
}

func TestPrintKeyValues_HumanVsAgent(t *testing.T) {
	pairs := []KeyValuePair{
		{Key: "Status", Value: "connected"},
		{Key: "Items", Value: "42"},
	}

	t.Run("human", func(t *testing.T) {
		t.Setenv("AGENT", "")
		var bufHuman bytes.Buffer
		if err := PrintKeyValues(&bufHuman, pairs); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(bufHuman.String(), "Status:") {
			t.Errorf("unexpected human key values: %s", bufHuman.String())
		}
	})

	t.Run("agent", func(t *testing.T) {
		t.Setenv("AGENT", "1")
		var bufAgent bytes.Buffer
		if err := PrintKeyValues(&bufAgent, pairs); err != nil {
			t.Fatal(err)
		}
		expected := "Status: connected\nItems: 42\n"
		if bufAgent.String() != expected {
			t.Errorf("agent key values want %q, got %q", expected, bufAgent.String())
		}
	})
}

func TestPrintTree_HumanVsAgent(t *testing.T) {
	tree := &TreeNode{
		Label: "Root",
		Children: []*TreeNode{
			{Label: "Folder A", Children: []*TreeNode{{Label: "Doc 1"}}},
			{Label: "Doc 2"},
		},
	}

	t.Run("human", func(t *testing.T) {
		t.Setenv("AGENT", "")
		var bufHuman bytes.Buffer
		if err := PrintTree(&bufHuman, tree); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(bufHuman.String(), "├── Folder A") || !strings.Contains(bufHuman.String(), "└── Doc 2") {
			t.Errorf("unexpected human tree output: %s", bufHuman.String())
		}
	})

	t.Run("agent", func(t *testing.T) {
		t.Setenv("AGENT", "1")
		var bufAgent bytes.Buffer
		if err := PrintTree(&bufAgent, tree); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(bufAgent.String(), "* Root\n  * Folder A\n    * Doc 1\n  * Doc 2") {
			t.Errorf("unexpected agent tree output: %s", bufAgent.String())
		}
	})
}

func TestTerminalWidthAndSeparator(t *testing.T) {
	_ = TerminalWidth()

	t.Run("human", func(t *testing.T) {
		t.Setenv("AGENT", "")
		var bufHuman bytes.Buffer
		if err := PrintSeparator(&bufHuman); err != nil {
			t.Fatal(err)
		}
		if len(bufHuman.String()) == 0 {
			t.Error("expected non-empty separator in human mode")
		}
	})

	t.Run("agent", func(t *testing.T) {
		t.Setenv("AGENT", "1")
		var bufAgent bytes.Buffer
		if err := PrintSeparator(&bufAgent); err != nil {
			t.Fatal(err)
		}
		if len(bufAgent.String()) != 0 {
			t.Errorf("expected empty separator in agent mode, got %q", bufAgent.String())
		}
	})
}

func TestPrintTree_Nil(t *testing.T) {
	var buf bytes.Buffer
	if err := PrintTree(&buf, nil); err != nil {
		t.Fatal(err)
	}
	if buf.Len() != 0 {
		t.Errorf("expected empty for nil tree")
	}
}

func TestPrintTable_Escaping(t *testing.T) {
	t.Setenv("AGENT", "1")

	headers := []string{"ID", "NAME", "TYPE", "MODIFIED"}
	rows := [][]string{
		{"id-1", "Notes\tDocumentType\nfake-id\tInjected", "DocumentType", "1700000000000"},
	}

	var buf bytes.Buffer
	if err := PrintTable(&buf, headers, rows); err != nil {
		t.Fatal(err)
	}
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
	t.Setenv("AGENT", "")

	headers := []string{"ID", "NAME"}
	rows := [][]string{
		{"id-1", "Escape\x1b[2J\a and tab\tand\nnewline"},
	}

	var buf bytes.Buffer
	if err := PrintTable(&buf, headers, rows); err != nil {
		t.Fatal(err)
	}
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

func TestPrintKeyValues_HumanAlignment(t *testing.T) {
	t.Setenv("AGENT", "")

	pairs := []KeyValuePair{
		{Key: "ID", Value: "abc"},
		{Key: "Name", Value: "Planner"},
		{Key: "Modified", Value: "1700"},
	}

	var buf bytes.Buffer
	if err := PrintKeyValues(&buf, pairs); err != nil {
		t.Fatal(err)
	}

	want := "ID:        abc\nName:      Planner\nModified:  1700\n"
	if buf.String() != want {
		t.Errorf("human key values want %q, got %q", want, buf.String())
	}

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}

	valCol := -1
	for i, line := range lines {
		colonIdx := strings.Index(line, ":")
		if colonIdx == -1 {
			t.Fatalf("line %d missing colon: %q", i, line)
		}
		valIdx := strings.Index(line[colonIdx+1:], pairs[i].Value) + colonIdx + 1
		if valCol == -1 {
			valCol = valIdx
		} else if valIdx != valCol {
			t.Errorf("line %d value starts at column %d, want column %d (line: %q)", i, valIdx, valCol, line)
		}
	}
}

func TestPrintTable_HeaderVerbatim(t *testing.T) {
	t.Setenv("AGENT", "")

	headers := []string{"LINK #", "TARGET PAGE"}
	rows := [][]string{{"0", "1"}}

	var buf bytes.Buffer
	if err := PrintTable(&buf, headers, rows); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if strings.Contains(out, "LINK  #") {
		t.Errorf("header auto-formatted with extra space: %q", out)
	}
	if !strings.Contains(out, "LINK #") {
		t.Errorf("header missing verbatim text: %q", out)
	}
}



func TestPrintTree_Escaping(t *testing.T) {
	t.Setenv("AGENT", "1")

	tree := &TreeNode{
		Label: "/",
		Children: []*TreeNode{
			{Label: "a\n* forged"},
		},
	}

	var buf bytes.Buffer
	if err := PrintTree(&buf, tree); err != nil {
		t.Fatal(err)
	}
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

type testFailWriter struct{}

func (f testFailWriter) Write(p []byte) (int, error) {
	return 0, errors.New("simulated write failure")
}

func TestAgentPrimitives_WriteError(t *testing.T) {
	fw := testFailWriter{}
	headers := []string{"A", "B"}
	rows := [][]string{{"1", "2"}}
	pairs := []KeyValuePair{{Key: "k", Value: "v"}}
	tree := &TreeNode{Label: "root", Children: []*TreeNode{{Label: "child"}}}

	for _, mode := range []string{"0", "1"} {
		t.Run("mode="+mode, func(t *testing.T) {
			t.Setenv("AGENT", mode)

			if mode == "0" {
				if err := PrintSeparator(fw); err == nil {
					t.Errorf("PrintSeparator must return error on write failure")
				}
			}
			if err := PrintStatus(fw, "ok", "msg"); err == nil {
				t.Errorf("PrintStatus must return error on write failure")
			}
			if err := PrintTable(fw, headers, rows); err == nil {
				t.Errorf("PrintTable must return error on write failure")
			}
			if err := PrintKeyValues(fw, pairs); err == nil {
				t.Errorf("PrintKeyValues must return error on write failure")
			}
			if err := PrintTree(fw, tree); err == nil {
				t.Errorf("PrintTree must return error on write failure")
			}
		})
	}
}

