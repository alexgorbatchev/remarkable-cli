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
