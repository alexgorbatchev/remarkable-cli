package doc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	cloud "github.com/alexgorbatchev/go-remarkable-cloud"
	render "github.com/alexgorbatchev/go-remarkable-render"
)

const uploadFolderID = "11111111-1111-4111-8111-111111111111"
const uploadParentFolderID = "44444444-4444-4444-8444-444444444444"

type uploadServer struct {
	mu       sync.Mutex
	root     cloud.RootState
	blobs    map[string][]byte
	writes   int
	commits  int
	failure  string
	evidence string
	// checked records evidence paths whose upload commit was checked; later
	// commits, such as an import into the uploaded document, are not uploads.
	checked map[string]bool
	t       *testing.T
}

func newUploadServer(t *testing.T, failure, evidence string) (*uploadServer, *cloud.Client) {
	t.Helper()
	s := &uploadServer{t: t, blobs: make(map[string][]byte), failure: failure, evidence: evidence, checked: make(map[string]bool)}
	root := []byte("4\n0:.:1:0\n")
	for _, id := range []string{uploadFolderID, archiveDocID, uploadParentFolderID} {
		metadata := cloud.ItemMetadata{Type: cloud.ItemTypeDocument, VisibleName: "Source"}
		if id == uploadParentFolderID {
			metadata.Type = cloud.ItemTypeCollection
			metadata.VisibleName = "ParentFolder"
			if failure == "trashed-ancestor-folder" {
				metadata.Parent = "trash"
			}
		}
		if id == uploadFolderID {
			metadata.Type = cloud.ItemTypeCollection
			metadata.VisibleName = "Folder"
			if failure == "trashed-ancestor-folder" {
				metadata.Parent = uploadParentFolderID
			}
		}
		if failure == "collision" && id == archiveDocID {
			metadata.VisibleName = "Planner"
			metadata.Parent = uploadFolderID
		}
		if failure == "deleted-folder" && id == uploadFolderID {
			metadata.Deleted = true
		}
		if failure == "document-folder" && id == uploadFolderID {
			metadata.Type = cloud.ItemTypeDocument
		}
		data, err := json.Marshal(metadata)
		if err != nil {
			t.Fatal(err)
		}
		hash := archiveTestHash(data)
		s.blobs[hash] = data
		manifest := fmt.Appendf(nil, "3\n%s:0:%s.metadata:0:%d\n", hash, id, len(data))
		docHash := archiveTestHash(manifest)
		s.blobs[docHash] = manifest
		root = fmt.Appendf(root, "%s:0:%s:1:%d\n", docHash, id, len(data))
	}
	s.root = cloud.RootState{Hash: archiveTestHash(root), Generation: 7, SchemaVersion: 4}
	s.blobs[s.root.Hash] = root
	server := httptest.NewServer(http.HandlerFunc(s.serveHTTP))
	t.Cleanup(server.Close)
	c, err := cloud.NewClient(cloud.WithStorageHost(server.URL), cloud.WithConfig(&cloud.Config{UserToken: "test"}), cloud.WithCacheDir(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	return s, c
}

func (s *uploadServer) serveHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r.URL.Path == "/sync/v3/root" {
		if r.Method == http.MethodPut {
			s.writes++
			s.commits++
			if !s.checked[s.evidence] {
				s.checked[s.evidence] = true
				data, err := os.ReadFile(s.evidence)
				if err != nil {
					s.t.Error(err)
				}
				var evidence UploadEvidence
				if err := json.Unmarshal(data, &evidence); err != nil {
					s.t.Error(err)
				}
				if evidence.Result.State != cloud.UpdateCommitUnknown || evidence.Result.ID == "" || evidence.Result.DocumentHash == "" {
					s.t.Error("commit sent before durable recovery identity")
				}
				if evidence.NativePages == NativePagesInitialized && len(evidence.PageIDs) != evidence.Pages {
					s.t.Error("commit sent before durable native page identity")
				}
			}
			if s.failure == "conflict" {
				w.WriteHeader(http.StatusPreconditionFailed)
				return
			}
			var update cloud.RootState
			if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
				s.t.Error(err)
			}
			if update.Generation != s.root.Generation {
				s.t.Error("unchecked generation")
			}
			s.root.Hash = update.Hash
			s.root.Generation++
			if s.failure == "unknown" {
				fmt.Fprint(w, "invalid JSON")
				return
			}
		}
		if err := json.NewEncoder(w).Encode(s.root); err != nil {
			s.t.Error(err)
		}
		return
	}
	hash := strings.TrimPrefix(r.URL.Path, "/sync/v3/files/")
	if r.Method == http.MethodPut {
		s.writes++
		data, err := io.ReadAll(r.Body)
		if err != nil {
			s.t.Error(err)
		}
		s.blobs[hash] = data
		return
	}
	data, ok := s.blobs[hash]
	if !ok {
		http.NotFound(w, r)
		return
	}
	if s.commits > 0 && strings.HasSuffix(r.Header.Get("rm-filename"), ".pdf") && s.failure == "corrupt" {
		data = []byte("corrupt PDF")
	}
	if _, err := w.Write(data); err != nil {
		s.t.Error(err)
	}
}

func TestUploadPDF(t *testing.T) {
	for _, initialize := range []bool{false, true} {
		for _, failure := range []string{"", "collision", "deleted-folder", "trashed-ancestor-folder", "document-folder", "missing-folder", "invalid-pdf", "evidence-exists", "conflict", "unknown", "corrupt"} {
			t.Run(fmt.Sprintf("initialize=%t/%s", initialize, failure), func(t *testing.T) {
				testUploadPDF(t, initialize, failure)
			})
		}
	}
}

func testUploadPDF(t *testing.T, initialize bool, failure string) {
	wantNative := nativePagesPending
	if initialize {
		wantNative = NativePagesInitialized
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "source.pdf")
	pdf, err := os.ReadFile("testdata/linked_pages.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if failure == "invalid-pdf" {
		pdf = []byte("%PDF-1.7 invalid")
	}
	if err := os.WriteFile(path, pdf, 0600); err != nil {
		t.Fatal(err)
	}
	evidencePath := filepath.Join(dir, "upload.json")
	if failure == "evidence-exists" {
		if err := os.WriteFile(evidencePath, []byte("keep"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	s, c := newUploadServer(t, failure, evidencePath)
	original := s.root.Hash
	opts := UploadOptions{Title: "Planner", Folder: uploadFolderID, Evidence: evidencePath, InitializePages: initialize}
	if failure == "missing-folder" {
		opts.Folder = "33333333-3333-4333-8333-333333333333"
	}
	evidence, err := UploadPDF(context.Background(), c, path, opts)
	if failure != "" {
		if err == nil {
			t.Fatal("expected upload error")
		}
		wantError := map[string]string{"collision": "already exists", "deleted-folder": "deleted or in trash", "trashed-ancestor-folder": "deleted or in trash", "document-folder": "not a collection", "missing-folder": "not found", "invalid-pdf": "validating upload PDF", "evidence-exists": "creating upload evidence", "conflict": "generation", "unknown": "invalid", "corrupt": "bytes"}[failure]
		if !strings.Contains(err.Error(), wantError) {
			t.Fatalf("wrong failure: %v, want %q", err, wantError)
		}
		if failure == "missing-folder" && !errors.Is(err, cloud.ErrItemNotFound) {
			t.Fatalf("expected ErrItemNotFound for missing folder: %v", err)
		}
		switch failure {
		case "conflict", "unknown", "corrupt":
			if s.commits != 1 {
				t.Fatalf("expected one generation-checked commit request, got %d", s.commits)
			}
			if failure == "conflict" && !errors.Is(err, cloud.ErrGenerationConflict) {
				t.Fatalf("lost generation conflict: %v", err)
			}
			want := cloud.UpdateStaged
			if failure == "unknown" {
				want = cloud.UpdateCommitUnknown
			}
			if failure == "corrupt" {
				want = cloud.UpdateCommitted
			}
			if evidence == nil || evidence.Result.State != want || !isUUID(evidence.Result.ID) {
				t.Fatalf("missing recovery state: %+v, %v", evidence, err)
			}
			if failure == "unknown" {
				before := s.writes
				recovered, err := CheckUpload(context.Background(), c, evidencePath)
				if err != nil || recovered.Result.State != cloud.UpdateVerified || s.writes != before {
					t.Fatalf("read-only recovery failed: %+v, %v", recovered, err)
				}
				if recovered.NativePages != wantNative || !slices.Equal(recovered.PageIDs, evidence.PageIDs) {
					t.Fatalf("recovery changed native page identity: %v, want %v", recovered.PageIDs, evidence.PageIDs)
				}
			}
		default:
			if s.writes != 0 {
				t.Fatalf("failed preflight wrote cloud data: %d", s.writes)
			}
		}
		return
	}
	if err != nil || evidence.Result.State != cloud.UpdateVerified {
		t.Fatalf("upload: %+v, %v", evidence, err)
	}
	pdfDoc, cleanup, err := render.OpenDocumentFromBytes(pdf)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if pdfDoc.NumPage() < 2 || evidence.Pages != pdfDoc.NumPage() || evidence.NativePages != wantNative || len(evidence.PageIDs) != map[bool]int{false: 0, true: evidence.Pages}[initialize] {
		t.Fatalf("wrong page state: %+v", evidence)
	}
	if !bytes.Equal(s.blobs[archiveTestHash(pdf)], pdf) {
		t.Fatal("PDF bytes or links changed")
	}
	root, err := cloud.ParseManifest(s.root.Hash, bytes.NewReader(s.blobs[s.root.Hash]))
	if err != nil {
		t.Fatal(err)
	}
	old, err := cloud.ParseManifest(original, bytes.NewReader(s.blobs[original]))
	if err != nil {
		t.Fatal(err)
	}
	if root.Find(archiveDocID).Hash != old.Find(archiveDocID).Hash {
		t.Fatal("annotated source changed")
	}
	manifest, err := cloud.ParseManifest(evidence.Result.DocumentHash, bytes.NewReader(s.blobs[evidence.Result.DocumentHash]))
	if err != nil {
		t.Fatal(err)
	}
	var metadata cloud.ItemMetadata
	if err := json.Unmarshal(s.blobs[manifest.Find(evidence.Result.ID+".metadata").Hash], &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.Parent != opts.Folder || metadata.VisibleName != opts.Title || metadata.Type != cloud.ItemTypeDocument {
		t.Fatalf("wrong metadata: %+v", metadata)
	}
	var content map[string]json.RawMessage
	if err := json.Unmarshal(s.blobs[manifest.Find(evidence.Result.ID+".content").Hash], &content); err != nil {
		t.Fatal(err)
	}
	_, hasCPages := content["cPages"]
	_, hasPages := content["pages"]
	if hasCPages != initialize || string(content["pageCount"]) != fmt.Sprint(evidence.Pages) {
		t.Fatalf("wrong native page structure for initialize=%t: %s", initialize, content)
	}
	if !initialize && string(content["pages"]) != "null" || initialize && hasPages {
		t.Fatalf("wrong legacy page list for initialize=%t: %s", initialize, content)
	}
	before := s.writes
	if _, err := CheckUpload(context.Background(), c, evidencePath); err != nil || before != s.writes {
		t.Fatalf("fresh recovery: %v", err)
	}
	// Populate the real disk cache with the good PDF, then have storage
	// return corrupt bytes under its old hash to prove recovery bypasses it.
	if _, err := c.GetBlob(context.Background(), archiveTestHash(pdf), evidence.Result.ID+".pdf"); err != nil {
		t.Fatal(err)
	}
	s.failure = "corrupt"
	if _, err := CheckUpload(context.Background(), c, evidencePath); err == nil {
		t.Fatal("recovery accepted corrupted bytes from fresh download")
	}
	s.failure = ""
	s.blobs[s.root.Hash] = bytes.ReplaceAll(s.blobs[s.root.Hash], []byte(evidence.Result.DocumentHash), []byte(strings.Repeat("a", 64)))
	if _, err := CheckUpload(context.Background(), c, evidencePath); err == nil || !strings.Contains(err.Error(), "changed document hash") {
		t.Fatalf("recovery accepted changed identity: %v", err)
	}
}

func TestUploadProgressFailurePreventsCommit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upload.json")
	s, c := newUploadServer(t, "", path)
	evidence, err := UploadPDF(context.Background(), c, "testdata/linked_pages.pdf", UploadOptions{Title: "Planner", Evidence: path, OnProgress: func(evidence *UploadEvidence) error {
		if evidence.Result.State == cloud.UpdateCommitUnknown {
			return fmt.Errorf("closed stdout")
		}
		return nil
	}})
	if err == nil || !strings.Contains(err.Error(), "closed stdout") || s.commits != 0 || evidence.Result.State != cloud.UpdateStaged {
		t.Fatalf("committed despite output failure: %+v, %v, commits %d", evidence, err, s.commits)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var saved UploadEvidence
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Result.State != cloud.UpdateStaged {
		t.Fatalf("uncertain evidence after unsent commit: %+v", saved)
	}
}

func TestCheckUploadReportsUncommittedDocumentMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upload.json")
	_, c := newUploadServer(t, "conflict", path)
	if _, err := UploadPDF(context.Background(), c, "testdata/linked_pages.pdf", UploadOptions{Title: "Planner", Evidence: path}); !errors.Is(err, cloud.ErrGenerationConflict) {
		t.Fatalf("upload = %v, want a rejected commit", err)
	}
	if _, err := CheckUpload(context.Background(), c, path); !errors.Is(err, cloud.ErrItemNotFound) || !strings.Contains(err.Error(), "absent from the current root snapshot") {
		t.Fatalf("check of an uncommitted upload = %v, want ErrItemNotFound", err)
	}
}

func TestUploadRootFolderAndEvidencePreservation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "upload.json")
	s, c := newUploadServer(t, "", path)
	evidence, err := UploadPDF(context.Background(), c, "testdata/linked_pages.pdf", UploadOptions{Title: "Planner", Evidence: path})
	if err != nil || evidence.Folder != "" {
		t.Fatalf("root upload failed: %+v, %v", evidence, err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	writes := s.writes
	if _, err := UploadPDF(context.Background(), c, "testdata/linked_pages.pdf", UploadOptions{Title: "Other title", Evidence: path}); err == nil {
		t.Fatal("replaced existing evidence")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) || s.writes != writes {
		t.Fatal("existing evidence or cloud changed")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("evidence permissions = %o", info.Mode().Perm())
	}
}
