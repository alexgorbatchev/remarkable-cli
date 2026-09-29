package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexgorbatchev/go-rmscene"
)

func executeRoot(args ...string) (string, error) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(args)

	err := cmd.Execute()
	return buf.String(), err
}

func TestRootCommand_HelpAndVersion(t *testing.T) {
	t.Setenv("AGENT", "0")
	out, err := executeRoot("--help")
	if err != nil {
		t.Fatalf("help failed: %v", err)
	}
	if !strings.Contains(out, "remarkable-cli") {
		t.Fatalf("expected remarkable-cli in help, got: %s", out)
	}

	outVer, errVer := executeRoot("--version")
	if errVer != nil {
		t.Fatalf("version failed: %v", errVer)
	}
	if outVer != version+"\n" {
		t.Errorf("expected version '%s\\n', got %q", version, outVer)
	}
}

func TestStrokeCommands(t *testing.T) {
	tmpDir := t.TempDir()
	strokePath := filepath.Join(tmpDir, "test.rm")
	_ = os.WriteFile(strokePath, []byte(rmscene.HeaderV6), 0644)

	// 1. Stroke inspect
	t.Setenv("AGENT", "0")
	out, err := executeRoot("stroke", "inspect", strokePath)
	if err != nil {
		t.Fatalf("stroke inspect failed: %v", err)
	}
	if !strings.Contains(out, "Total Blocks:") {
		t.Errorf("expected Total Blocks in inspect output, got %s", out)
	}

	// 2. Stroke export
	outSvgPath := filepath.Join(tmpDir, "exported.svg")
	_, err = executeRoot("stroke", "export", strokePath, "-o", outSvgPath)
	if err != nil {
		t.Fatalf("stroke export failed: %v", err)
	}
	content, err := os.ReadFile(outSvgPath)
	if err != nil || !strings.Contains(string(content), "<svg") {
		t.Fatalf("expected valid SVG file at %s", outSvgPath)
	}
}

func TestAuthPairValidation(t *testing.T) {
	_, err := executeRoot("auth", "pair", "short")
	if err == nil {
		t.Fatal("expected error on short pairing code")
	}
}
