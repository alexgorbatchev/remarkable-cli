package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func executeCommand(args ...string) (string, error) {
	buf := new(bytes.Buffer)
	cmd := newRootCommand()
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(args)

	err := cmd.Execute()
	return buf.String(), err
}

func TestRootCommand_HelpAndVersion(t *testing.T) {
	t.Setenv("AGENT", "0")
	out, err := executeCommand("--help")
	if err != nil {
		t.Fatalf("help failed: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("expected help output")
	}

	outVer, errVer := executeCommand("--version")
	if errVer != nil {
		t.Fatalf("version failed: %v", errVer)
	}
	if outVer != version+"\n" {
		t.Errorf("expected '%s\\n', got %q", version, outVer)
	}
}

func TestListCommand_Empty(t *testing.T) {
	t.Setenv("AGENT", "0")
	tmpDir := t.TempDir()

	out, err := executeCommand("list", "--output-dir", tmpDir)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if !strings.Contains(out, "[INFO] No planner days captured") {
		t.Errorf("expected info output, got %q", out)
	}
}

func TestListCommand_Populated(t *testing.T) {
	tmpDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmpDir, "2026-09-28-day.png"), []byte("png"), 0o644)

	t.Setenv("AGENT", "0")
	out, err := executeCommand("list", "--output-dir", tmpDir)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if !strings.Contains(out, "2026-09-28") {
		t.Errorf("expected date in output, got %q", out)
	}

	// Agent mode: one per line
	t.Setenv("AGENT", "1")
	outAgent, errAgent := executeCommand("list", "--output-dir", tmpDir)
	if errAgent != nil {
		t.Fatalf("agent list failed: %v", errAgent)
	}
	if strings.TrimSpace(outAgent) != "2026-09-28" {
		t.Errorf("expected plain date line, got %q", outAgent)
	}
}

func TestSyncCommand_SkipExisting(t *testing.T) {
	tmpDir := t.TempDir()
	// Pre-create both day and notes
	_ = os.WriteFile(filepath.Join(tmpDir, "2026-09-28-day.png"), []byte("png"), 0o644)
	_ = os.WriteFile(filepath.Join(tmpDir, "2026-09-28-notes.png"), []byte("png"), 0o644)

	t.Setenv("AGENT", "0")
	out, err := executeCommand("sync", "2026-09-28", "--output-dir", tmpDir)
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}
	if !strings.Contains(out, "[SKIPPED]") || !strings.Contains(out, "2026-09-28 day (already on disk)") {
		t.Errorf("expected skipped output, got %q", out)
	}

	// Agent mode
	t.Setenv("AGENT", "1")
	outAgent, errAgent := executeCommand("sync", "2026-09-28", "--output-dir", tmpDir)
	if errAgent != nil {
		t.Fatalf("agent sync failed: %v", errAgent)
	}
	if !strings.Contains(outAgent, "skipped:") {
		t.Errorf("expected agent skipped output, got %q", outAgent)
	}
}
