package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cloud "github.com/alexgorbatchev/go-remarkable-cloud"
	"github.com/alexgorbatchev/remarkable-cli/internal/doc"
)

func TestArchiveCommandValidation(t *testing.T) {
	for _, args := range [][]string{{"doc", "archive"}, {"doc", "archive", "Source"}, {"doc", "archive", "Source", "extra", "-o", "backup.zip"}, {"doc", "archive", "Source", "-o", ""}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			out, err := executeRoot(args...)
			if err == nil {
				t.Fatal("expected argument/output validation error")
			}
			if strings.Contains(err.Error(), "unknown command") {
				t.Fatalf("archive command missing: %v; %s", err, out)
			}
			if strings.Contains(err.Error(), "credentials") {
				t.Fatalf("credentials accessed before local validation: %v", err)
			}
		})
	}
}

func TestArchiveCommandEvidenceOutput(t *testing.T) {
	for _, mode := range []string{"0", "1"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("AGENT", mode)
			var out bytes.Buffer
			snapshot := &doc.ArchiveSnapshot{DocumentID: "source-id", DocumentHash: "document-hash", Root: cloud.RootState{Hash: "root-hash", Generation: 7}, ManifestSHA256: "manifest-digest", Files: []doc.ArchiveFile{{Name: "source.metadata", Hash: "file-hash", SHA256: "file-digest", Size: 17}}}
			if err := printArchiveEvidence(&out, "backup.zip", snapshot); err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{"backup.zip", "source-id", "document-hash", "root-hash", "7", "manifest-digest", "source.metadata", "file-hash", "file-digest", "17"} {
				if !strings.Contains(out.String(), want) {
					t.Errorf("output missing %q: %s", want, &out)
				}
			}
			if mode == "1" && (!strings.Contains(out.String(), "state: verified\n") || !strings.Contains(out.String(), "NAME\tHASH\tSHA256\tBYTES\n")) {
				t.Fatalf("invalid agent output: %s", &out)
			}
			closed, err := os.Create(filepath.Join(t.TempDir(), "closed-output"))
			if err != nil {
				t.Fatal(err)
			}
			if err := closed.Close(); err != nil {
				t.Fatal(err)
			}
			if err := printArchiveEvidence(closed, "backup.zip", snapshot); !errors.Is(err, os.ErrClosed) {
				t.Fatalf("lost output error: %v", err)
			}
		})
	}
}
