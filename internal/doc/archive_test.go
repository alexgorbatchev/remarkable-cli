package doc

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	cloud "github.com/alexgorbatchev/go-remarkable-cloud"
)

const archiveDocID = "22222222-2222-4222-8222-222222222222"

func archiveTestHash(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func archiveFixture(t *testing.T, schema, failure, output string) (*cloud.Client, map[string][]byte, []byte) {
	t.Helper()
	content := []byte(` {"pages":["first","inserted"],"redirectionPageMap":[0,-1],"tags":[{"name":"keep"}],"viewport":{"x":17}} `)
	if schema == "cPages" {
		content = []byte(` {"cPages":{"pages":[{"id":"first","redir":{"value":0}},{"id":"deleted","deleted":{"value":1}},{"id":"inserted","redir":{"value":-1}}],"original":{"pages":[{"id":"first"}]}},"tags":[{"name":"keep"}],"viewport":{"x":17}} `)
	}
	files := map[string][]byte{
		archiveDocID + ".content":     content,
		archiveDocID + ".metadata":    []byte(` {"visibleName":"Archive source","type":"DocumentType","version":7,"lastModified":"123","unknown":[1,2]} `),
		archiveDocID + ".pdf":         {0, 255, 1, 2, 3},
		archiveDocID + ".pagedata":    []byte("Blank\nCustom\n"),
		archiveDocID + "/first.rm":    []byte("native ink\x00\xff"),
		archiveDocID + "/inserted.rm": metadataOnlyRM(t),
		archiveDocID + "/deleted.rm":  []byte("deleted native ink"),
		archiveDocID + "/unknown.bin": {255, 0, 19, 200},
	}
	if failure == "unsafe" {
		files["../escape"] = []byte("unsafe")
	}
	if failure == "folder" {
		files[archiveDocID+".metadata"] = []byte(`{"visibleName":"Archive source","type":"CollectionType"}`)
	}
	var names []string
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	manifest := []byte("3\n")
	hasher := sha256.New()
	blobs := make(map[string][]byte)
	for _, name := range names {
		data := files[name]
		hash := archiveTestHash(data)
		rawHash, err := hex.DecodeString(hash)
		if err != nil {
			t.Fatal(err)
		}
		hasher.Write(rawHash)
		size := len(data)
		if failure == "size" && strings.HasSuffix(name, "unknown.bin") {
			size++
		}
		manifest = fmt.Appendf(manifest, "%s:0:%s:0:%d\n", hash, name, size)
		blobs[hash] = data
	}
	docHash := hex.EncodeToString(hasher.Sum(nil))
	blobs[docHash] = manifest
	rootData := fmt.Appendf(nil, "4\n0:.:1:100\n%s:0:%s:8:100\n", docHash, archiveDocID)
	root := cloud.RootState{Hash: archiveTestHash(rootData), Generation: 7, SchemaVersion: 4}
	blobs[root.Hash] = rootData
	var mu sync.Mutex
	var rootReads, schemaReads int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Method != http.MethodGet {
			t.Errorf("export attempted %s", r.Method)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path == "/sync/v3/root" {
			rootReads++
			if failure == "resolution-change" && rootReads == 2 {
				changed := fmt.Appendf(nil, "4\n0:.:1:100\n%s:0:%s:8:100\n", strings.Repeat("a", 64), archiveDocID)
				root.Hash = archiveTestHash(changed)
				blobs[root.Hash] = changed
			}
			if err := json.NewEncoder(w).Encode(root); err != nil {
				t.Error(err)
			}
			return
		}
		name := r.Header.Get("rm-filename")
		if name == archiveDocID+".docSchema" {
			schemaReads++
			if failure == "manifest" && schemaReads == 2 {
				if _, err := w.Write(bytes.Replace(manifest, []byte(archiveDocID+"/first.rm"), []byte(archiveDocID+"/changed.rm"), 1)); err != nil {
					t.Error(err)
				}
				return
			}
		}
		if strings.HasSuffix(name, "unknown.bin") {
			switch failure {
			case "changed":
				root.Generation++
			case "changed-hash":
				root.Hash = strings.Repeat("f", 64)
			case "missing":
				http.NotFound(w, r)
				return
			case "corrupt":
				w.Write([]byte("corrupted bytes"))
				return
			case "interrupted":
				w.Header().Set("Content-Length", "100")
				w.Write([]byte("partial"))
				return
			case "publish-race":
				if err := os.WriteFile(output, []byte("keep original"), 0600); err != nil {
					t.Error(err)
				}
			}
		}
		data, ok := blobs[strings.TrimPrefix(r.URL.Path, "/sync/v3/files/")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		if _, err := w.Write(data); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(server.Close)
	cache := t.TempDir()
	if failure == "cached" {
		if err := os.MkdirAll(filepath.Join(cache, "blobs"), 0700); err != nil {
			t.Fatal(err)
		}
		for name, data := range files {
			if strings.HasSuffix(name, ".metadata") {
				continue
			} // Resolution uses cached metadata; archive bytes must still be fresh.
			if err := os.WriteFile(filepath.Join(cache, "blobs", archiveTestHash(data)), []byte("corrupt cached bytes"), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	client, err := cloud.NewClient(cloud.WithStorageHost(server.URL), cloud.WithConfig(&cloud.Config{UserToken: "test"}), cloud.WithCacheDir(cache))
	if err != nil {
		t.Fatal(err)
	}
	return client, files, manifest
}

func readArchive(t *testing.T, path string) map[string][]byte {
	t.Helper()
	r, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := r.Close(); err != nil {
			t.Error(err)
		}
	}()
	entries := make(map[string][]byte)
	for _, file := range r.File {
		reader, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(reader)
		if err != nil {
			t.Fatal(err)
		}
		if err := reader.Close(); err != nil {
			t.Fatal(err)
		}
		if _, ok := entries[file.Name]; ok {
			t.Fatalf("duplicate ZIP name %s", file.Name)
		}
		entries[file.Name] = data
	}
	return entries
}

func TestArchiveDocumentPreservesSnapshot(t *testing.T) {
	for _, schema := range []string{"pages", "cPages", "cached"} {
		t.Run(schema, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "backup.zip")
			failure := ""
			if schema == "cached" {
				failure = "cached"
			}
			client, files, manifest := archiveFixture(t, schema, failure, path)
			result, err := ArchiveDocument(context.Background(), client, archiveDocID, path)
			if err != nil {
				t.Fatal(err)
			}
			entries := readArchive(t, path)
			if len(entries) != len(files)+2 {
				t.Fatalf("entries = %d, want %d", len(entries), len(files)+2)
			}
			for name, data := range files {
				if !bytes.Equal(entries["files/"+name], data) {
					t.Errorf("raw bytes changed for %s", name)
				}
			}
			if !bytes.Equal(entries["evidence/document.docSchema"], manifest) {
				t.Fatal("raw manifest changed")
			}
			var snapshot ArchiveSnapshot
			if err := json.Unmarshal(entries["evidence/snapshot.json"], &snapshot); err != nil {
				t.Fatal(err)
			}
			if snapshot.DocumentID != archiveDocID || snapshot.Root.Generation != 7 || snapshot.DocumentHash == "" || snapshot.ManifestSHA256 != archiveTestHash(manifest) {
				t.Fatalf("invalid identity evidence: %+v", snapshot)
			}
			if len(snapshot.Files) != len(files) {
				t.Fatal("missing file evidence")
			}
			for _, file := range snapshot.Files {
				data, ok := files[file.Name]
				if !ok || file.SHA256 != archiveTestHash(data) || file.Size != int64(len(data)) || file.Hash != file.SHA256 {
					t.Errorf("invalid file evidence: %+v", file)
				}
			}
			if result.DocumentHash != snapshot.DocumentHash || len(result.Files) != len(snapshot.Files) {
				t.Fatal("result disagrees with archive")
			}
			info, err := os.Stat(path)
			if err != nil || info.Mode().Perm() != 0600 {
				t.Fatalf("private archive permissions: %v %v", info, err)
			}
		})
	}
}

func TestArchiveDocumentFailureLeavesNoArchive(t *testing.T) {
	for _, test := range []struct{ scenario, want string }{
		{"changed", "cloud source changed"}, {"changed-hash", "cloud source changed"},
		{"resolution-change", "source changed during resolution"}, {"manifest", "manifest hash mismatch"},
		{"missing", "downloading native file"}, {"corrupt", "hash or size mismatch"}, {"size", "hash or size mismatch"},
		{"interrupted", "unexpected EOF"}, {"unsafe", "invalid native archive entry"},
		{"existing", "already exists"}, {"publish-race", "publishing archive without replacement"},
		{"canceled", "context canceled"}, {"empty-output", "output path is required"}, {"folder", "is not a document"},
	} {
		scenario := test.scenario
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			output := filepath.Join(dir, "backup.zip")
			client, _, _ := archiveFixture(t, "pages", scenario, output)
			if scenario == "existing" {
				if err := os.WriteFile(output, []byte("keep original"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if scenario == "canceled" {
				cancel()
			}
			if scenario == "empty-output" {
				output = ""
			}
			result, err := ArchiveDocument(ctx, client, archiveDocID, output)
			if err == nil || result != nil {
				t.Fatalf("expected failure without success evidence, got %v, %v", result, err)
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("wrong failure: %v, want %q", err, test.want)
			}
			paths, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "existing" || scenario == "publish-race" {
				data, err := os.ReadFile(output)
				if err != nil || string(data) != "keep original" || len(paths) != 1 {
					t.Fatal("existing output was altered")
				}
			} else if len(paths) != 0 {
				t.Fatalf("failed export left files: %v", paths)
			}
		})
	}
}
