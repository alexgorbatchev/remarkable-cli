package main

import (
	"strings"
	"testing"
)

// TestDocInspectReadFailures runs doc inspect against a cloud that fails the
// read of the content file or, for --pages, the second read of the document
// manifest, which follows the one that resolves the document, and on a
// document that lists no content file. Each must exit with the cause's status
// and name it, never print a document with no pages or with every page empty.
func TestDocInspectReadFailures(t *testing.T) {
	cases := []struct {
		name       string
		failedPath string
		okRequests int
		args       []string
		wantStatus int
		// wantText lists parts of the error message that name the failure.
		wantText []string
	}{
		{"content", "/sync/v3/files/content-hash", 0, []string{"doc", "inspect", "doc-1", "--no-cache"}, exitCloudUnavailable, []string{"fetching content schema", "injected cloud failure"}},
		{"pages manifest", "/sync/v3/files/doc-hash", 1, []string{"doc", "inspect", "doc-1", "--pages", "--no-cache"}, exitCloudUnavailable, []string{"fetching document manifest", "injected cloud failure"}},
		{"missing content", "", 0, []string{"doc", "inspect", "nocontent-1", "--no-cache"}, exitMissing, []string{"fetching content schema", "content entry not found for item nocontent-1"}},
	}
	for _, mode := range errorModes {
		t.Run(mode.agent, func(t *testing.T) {
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					ts, configPath := setupCLITestEnvFailing(t, tc.failedPath, tc.okRequests)
					defer ts.Close()
					got := runMainProcess(t, mainRun{
						args: tc.args,
						env:  map[string]string{"AGENT": mode.agent, "REMARKABLE_HOST": ts.URL, "REMARKABLE_CONFIG": configPath},
					})
					if got.exitCode != tc.wantStatus {
						t.Errorf("exit status = %d, want %d; stderr: %s", got.exitCode, tc.wantStatus, got.stderr)
					}
					if got.stdout != "" {
						t.Errorf("stdout = %q, want empty", got.stdout)
					}
					if !strings.HasPrefix(got.stderr, mode.prefix) {
						t.Errorf("stderr = %q, want an error prefixed %q", got.stderr, mode.prefix)
					}
					for _, text := range tc.wantText {
						if !strings.Contains(got.stderr, text) {
							t.Errorf("stderr = %q, want it to name %q", got.stderr, text)
						}
					}
				})
			}
		})
	}
}

// TestPDFReadFailures runs PDF commands on a page PDFium cannot load. They
// run in process because a child process would load the PDF engine again;
// TestMainExitStatus shows that main exits with exitStatus of the error.
// doc search must not drop the page from its results, and doc links must not
// report the page as having no links.
func TestPDFReadFailures(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantText string
	}{
		{"search", []string{"doc", "search", "unloadable-1", "needle", "--no-cache"}, "extracting text from page 1"},
		{"links", []string{"doc", "links", "unloadable-1", "--page", "1", "--no-cache"}, "enumerating links on page 1"},
	}
	for _, mode := range errorModes {
		t.Run(mode.agent, func(t *testing.T) {
			t.Setenv("AGENT", mode.agent)
			ts, _ := setupCLITestEnv(t)
			defer ts.Close()
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					out, err := executeRoot(tc.args...)
					if err == nil || !strings.Contains(err.Error(), tc.wantText) {
						t.Fatalf("error = %v, want one containing %q; output: %q", err, tc.wantText, out)
					}
					if got := exitStatus(err); got != exitFailure {
						t.Errorf("exitStatus(%v) = %d, want %d", err, got, exitFailure)
					}
					if out != "" {
						t.Errorf("output = %q, want none", out)
					}
				})
			}
		})
	}
}
