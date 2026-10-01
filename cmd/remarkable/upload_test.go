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

func TestUploadCommandValidation(t *testing.T) {
	for _, args := range [][]string{
		{"doc", "upload"},
		{"doc", "upload", "planner.pdf"},
		{"doc", "upload", "planner.pdf", "--title", "Planner"},
		{"doc", "upload", "planner.pdf", "--title", " ", "--evidence", "upload.json"},
		{"doc", "upload", "planner.pdf", "--title", "Planner", "--folder", "bad", "--evidence", "upload.json"},
		{"doc", "upload-check"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			out, err := executeRoot(args...)
			if err == nil || strings.Contains(err.Error(), "unknown command") || strings.Contains(err.Error(), "credentials") {
				t.Fatalf("expected local command validation, got %v: %s", err, out)
			}
		})
	}
}

func TestUploadEvidenceOutput(t *testing.T) {
	for _, mode := range []string{"0", "1"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("AGENT", mode)
			var out bytes.Buffer
			evidence := &doc.UploadEvidence{Title: "Planner", Folder: "folder", Pages: 2, NativePages: "pending-tablet-initialization", Result: cloud.CreateResult{State: cloud.UpdateCommitUnknown, ID: "uuid", DocumentHash: "document-hash", RootHash: "root-hash", Generation: 7, Uploaded: []string{"uuid.pdf"}}}
			if err := printUploadEvidence(&out, "upload.json", evidence); err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{"commit-unknown", "uuid", "document-hash", "root-hash", "Planner", "folder", "2", "7", "pending-tablet-initialization", "upload.json", "uploaded: uuid.pdf", "tablet", "upload-check"} {
				if !strings.Contains(out.String(), want) {
					t.Errorf("missing %q: %s", want, &out)
				}
			}
			if mode == "1" && !strings.Contains(out.String(), "state: commit-unknown\n") {
				t.Fatalf("wrong agent output: %s", &out)
			}
			closed, err := os.Create(filepath.Join(t.TempDir(), "closed"))
			if err != nil {
				t.Fatal(err)
			}
			if err := closed.Close(); err != nil {
				t.Fatal(err)
			}
			if err := printUploadEvidence(closed, "upload.json", evidence); !errors.Is(err, os.ErrClosed) {
				t.Fatalf("lost output error: %v", err)
			}
		})
	}
}
