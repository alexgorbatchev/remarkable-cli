package main

import (
	"strings"
	"testing"

	"github.com/alexgorbatchev/remarkable-cli/internal/testcloud"
)

func TestTreeReportsUnreachableItems(t *testing.T) {
	for _, mode := range []string{"0", "1"} {
		t.Run("AGENT="+mode, func(t *testing.T) {
			t.Setenv("AGENT", mode)
			server := testcloud.New(t, testcloud.Options{
				Preset: testcloud.PresetCLI, Documents: []string{"doc-1"},
				ExtraFiles: map[string][]byte{
					"/sync/v3/files/meta-hash": []byte(`{"visibleName":"Orphan","type":"DocumentType","parent":"missing"}`),
				},
			})
			defer server.Close()
			t.Setenv("REMARKABLE_HOST", server.URL)
			out, err := executeRoot("doc", "tree", "--no-cache", "--config", writeCredentials(t, pairedCredentials))
			if err != nil {
				t.Fatal(err)
			}
			prefix := "[WARN]  "
			if mode == "1" {
				prefix = "WARN: "
			}
			if !strings.Contains(out, prefix+"Items omitted: 1;") || !strings.Contains(out, "use 'doc list'") {
				t.Fatalf("missing omission report: %q", out)
			}
		})
	}
}

func TestStatusRequiresAuthSubject(t *testing.T) {
	_, err := executeRoot("status")
	if err == nil || !strings.Contains(err.Error(), `unknown command "status"`) {
		t.Fatalf("top-level status: %v", err)
	}
	if _, err := executeRoot("auth", "status", "--help"); err != nil {
		t.Fatal(err)
	}
}
