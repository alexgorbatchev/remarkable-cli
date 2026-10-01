package main

import (
	"bytes"
	"encoding/json"
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

func TestUploadCheckRejectsMalformedEvidenceBeforeAuthentication(t *testing.T) {
	const id = "11111111-1111-4111-8111-111111111111"
	const otherID = "22222222-2222-4222-8222-222222222222"
	for _, prefix := range []string{"", otherID} {
		for _, suffix := range []string{".pdf", ".metadata", ".content", ".pagedata"} {
			t.Run(prefix+suffix, func(t *testing.T) {
				evidence := doc.UploadEvidence{Version: 1, Pages: 2, Result: cloud.CreateResult{ID: id, DocumentHash: strings.Repeat("a", 64)}}
				for _, name := range []string{".pdf", ".metadata", ".content", ".pagedata"} {
					file := doc.UploadFile{Name: id + name, SHA256: strings.Repeat("b", 64), Size: 1}
					if name == suffix {
						file.Name = prefix + name
					}
					evidence.Files = append(evidence.Files, file)
				}
				data, err := json.Marshal(evidence)
				if err != nil {
					t.Fatal(err)
				}
				dir := t.TempDir()
				path := filepath.Join(dir, "upload.json")
				if err := os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
				out, err := executeRoot("doc", "upload-check", path, "--config", filepath.Join(dir, "missing-credentials"))
				if err == nil || !strings.Contains(err.Error(), "invalid upload evidence file") {
					t.Fatalf("malformed attachment reached authentication or was accepted: %v: %s", err, out)
				}
			})
		}
	}
	for _, data := range []string{"not JSON", `{}`} {
		t.Run(data, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "upload.json")
			if err := os.WriteFile(path, []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			out, err := executeRoot("doc", "upload-check", path, "--config", filepath.Join(dir, "missing-credentials"))
			if err == nil || strings.Contains(err.Error(), "credentials") {
				t.Fatalf("malformed evidence reached authentication: %v: %s", err, out)
			}
		})
	}
}
