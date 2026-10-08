package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
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
		{"doc", "upload", "planner.pdf", "--title", "Planner", "--evidence", "upload.json", "--initialize-pages=maybe"},
		{"doc", "upload", "", "--title", "Planner", "--evidence", "upload.json"},
		{"doc", "upload-check"},
		{"doc", "upload-check", ""},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			out, err := executeRoot(args...)
			if err == nil || strings.Contains(err.Error(), "unknown command") || strings.Contains(err.Error(), "credentials") {
				t.Fatalf("expected local command validation, got %v: %s", err, out)
			}
		})
	}
}

// TestUploadCommandInitializesPages runs doc upload, then doc inspect --pages
// and doc import against the new document with no tablet step between them.
func TestUploadCommandInitializesPages(t *testing.T) {
	for _, mode := range []string{"0", "1"} {
		for _, initialize := range []bool{false, true} {
			t.Run(fmt.Sprintf("agent=%s/initialize=%t", mode, initialize), func(t *testing.T) {
				t.Setenv("AGENT", mode)
				fixture := newImportCloud(t, func(*http.Request) int { return 0 })
				t.Setenv("REMARKABLE_HOST", fixture.url)
				t.Setenv("REMARKABLE_CONFIG", fixture.credentials)
				evidencePath := filepath.Join(t.TempDir(), "upload.json")
				args := []string{"doc", "upload", "../../internal/doc/testdata/linked_pages.pdf", "--title", "Planner", "--evidence", evidencePath, "--no-cache"}
				native := "pending-tablet-initialization"
				if initialize {
					args = append(args, "--initialize-pages")
					native = "initialized"
				}
				out, err := executeRoot(args...)
				if err != nil {
					t.Fatalf("upload: %v; %s", err, out)
				}
				evidence, err := doc.ReadUploadEvidence(evidencePath)
				if err != nil {
					t.Fatal(err)
				}
				if evidence.Result.State != cloud.UpdateVerified || evidence.NativePages != native || len(evidence.PageIDs) != map[bool]int{false: 0, true: evidence.Pages}[initialize] {
					t.Fatalf("wrong upload evidence: %+v", evidence)
				}
				if !strings.Contains(out, native) || mode == "1" && !strings.Contains(out, "native_pages: "+native+"\n") {
					t.Fatalf("output does not report native_pages %s: %s", native, out)
				}
				inspected, err := executeRoot("doc", "inspect", evidence.Result.ID, "--pages", "--no-cache")
				if err != nil {
					t.Fatalf("inspect --pages: %v; %s", err, inspected)
				}
				for _, id := range evidence.PageIDs {
					if !strings.Contains(inspected, id) {
						t.Fatalf("inspect --pages lacks native page %s: %s", id, inspected)
					}
				}
				imported, err := executeRoot("doc", "import", evidence.Result.ID, "--mapping", fixture.mapping, "--no-cache")
				if initialize {
					if err != nil || !strings.Contains(imported, "verified") || !strings.Contains(imported, evidence.PageIDs[0]) {
						t.Fatalf("import immediately after initialized upload: %v; %s", err, imported)
					}
					return
				}
				if err == nil || !strings.Contains(err.Error(), "no complete initialized native page structure") {
					t.Fatalf("import into pending pages = %v; %s", err, imported)
				}
			})
		}
	}
}

func TestUploadEvidenceOutput(t *testing.T) {
	for _, native := range []string{"pending-tablet-initialization", "initialized"} {
		for _, mode := range []string{"0", "1"} {
			t.Run(native+"/"+mode, func(t *testing.T) {
				testUploadEvidenceOutput(t, native, mode)
			})
		}
	}
}

func testUploadEvidenceOutput(t *testing.T, native, mode string) {
	t.Setenv("AGENT", mode)
	var out bytes.Buffer
	evidence := &doc.UploadEvidence{Title: "Planner", Folder: "folder", Pages: 2, NativePages: native, Result: cloud.CreateResult{State: cloud.UpdateCommitUnknown, ID: "uuid", DocumentHash: "document-hash", RootHash: "root-hash", Generation: 7, Uploaded: []string{"uuid.pdf"}}}
	if err := printUploadEvidence(&out, "upload.json", evidence); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"commit-unknown", "uuid", "document-hash", "root-hash", "Planner", "folder", "2", "7", native, "upload.json", "uploaded: uuid.pdf", "upload-check"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q: %s", want, &out)
		}
	}
	nextStep := map[string][]string{"pending-tablet-initialization": {"Open on tablet, sync, then inspect --pages before doc import"}, "initialized": {"doc inspect --pages", "doc import", "doc settings transfer", "without opening the tablet"}}[native]
	for _, want := range nextStep {
		if !strings.Contains(out.String(), want) {
			t.Errorf("next step for %s lacks %q: %s", native, want, &out)
		}
	}
	if native == "initialized" && strings.Contains(out.String(), "Open on tablet") {
		t.Errorf("initialized pages still require a tablet step: %s", &out)
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
	for native, pageIDs := range map[string][]string{"initialized": {id}, "pending-tablet-initialization": {otherID}, "unknown": nil} {
		t.Run("native_pages "+native, func(t *testing.T) {
			evidence := doc.UploadEvidence{Version: 1, Pages: 2, NativePages: native, PageIDs: pageIDs, Result: cloud.CreateResult{ID: id, DocumentHash: strings.Repeat("a", 64)}}
			for _, name := range []string{".pdf", ".metadata", ".content", ".pagedata"} {
				evidence.Files = append(evidence.Files, doc.UploadFile{Name: id + name, SHA256: strings.Repeat("b", 64), Size: 1})
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
			if err == nil || !strings.Contains(err.Error(), "upload evidence") || strings.Contains(err.Error(), "credentials") {
				t.Fatalf("invalid native page evidence reached authentication or was accepted: %v: %s", err, out)
			}
		})
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

func TestUploadFolderErrors(t *testing.T) {
	fixture := newImportCloud(t, func(r *http.Request) int { return 0 })
	t.Setenv("REMARKABLE_HOST", fixture.url)

	pdfPath := "../../internal/doc/testdata/linked_pages.pdf"

	// 1. Missing folder UUID -> exit 3
	evidencePath := filepath.Join(t.TempDir(), "evidence1.json")
	missingFolderID := "22222222-2222-4222-8222-222222222222"
	_, err := executeRoot(
		"doc", "upload", pdfPath,
		"--title", "Test Upload Missing Folder",
		"--evidence", evidencePath,
		"--folder", missingFolderID,
		"--config", fixture.credentials,
	)
	if err == nil {
		t.Fatal("expected error for missing folder, got nil")
	}
	if exitStatus(err) != 3 {
		t.Errorf("exit status for missing folder = %d, want 3 (err: %v)", exitStatus(err), err)
	}

	// 2. Existing UUID of DocumentType (not collection) -> exit 1, naming collection
	evidencePath2 := filepath.Join(t.TempDir(), "evidence2.json")
	docID := "11111111-1111-4111-8111-111111111111" // from fixture, has type "DocumentType"
	_, err2 := executeRoot(
		"doc", "upload", pdfPath,
		"--title", "Test Upload Doc As Folder",
		"--evidence", evidencePath2,
		"--folder", docID,
		"--config", fixture.credentials,
	)
	if err2 == nil {
		t.Fatal("expected error for non-collection folder, got nil")
	}
	if exitStatus(err2) != 1 {
		t.Errorf("exit status for non-collection folder = %d, want 1 (err: %v)", exitStatus(err2), err2)
	}
	if !strings.Contains(err2.Error(), "not a collection") {
		t.Errorf("error message missing 'not a collection': %v", err2)
	}
}

