package doc

import (
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
	"strings"
	"sync"
	"testing"

	cloud "github.com/alexgorbatchev/go-remarkable-cloud"
	"github.com/alexgorbatchev/go-rmscene"
)

const importDocID = "11111111-1111-4111-8111-111111111111"

func importHash(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func importFixture(t *testing.T, failure string, existing []byte) (*cloud.Client, *map[string][]byte, *int) {
	t.Helper()
	content := []byte(`{"fileType":"pdf","pageCount":2,"cPages":{"pages":[{"id":"first"},{"id":"future"}]}}`)
	if failure == "structure" {
		content = []byte(`{"fileType":"pdf","pageCount":2,"originalPageCount":2}`)
	}
	meta := []byte(`{"visibleName":"Destination","type":"DocumentType","custom":"keep","lastModified":"1"}`)
	pdf := []byte("unchanged PDF background")
	files := map[string][]byte{importDocID + ".content": content, importDocID + ".metadata": meta, importDocID + ".pdf": pdf}
	if existing != nil {
		name := importDocID + "/future.rm"
		if failure == "alias" {
			name = "future.rm"
		}
		files[name] = existing
	}
	blobs := make(map[string][]byte)
	manifest := "3\n"
	for name, data := range files {
		hash := importHash(data)
		blobs[hash] = data
		manifest += fmt.Sprintf("%s:0:%s:0:%d\n", hash, name, len(data))
	}
	docHash := importHash([]byte(manifest))
	blobs[docHash] = []byte(manifest)
	rootData := []byte(fmt.Sprintf("4\n0:.:1:100\n%s:0:%s:3:100\n", docHash, importDocID))
	root := cloud.RootState{Hash: importHash(rootData), Generation: 1, SchemaVersion: 4}
	blobs[root.Hash] = rootData
	puts := new(int)
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.URL.Path == "/sync/v3/root" {
			if r.Method == http.MethodPut {
				var update struct {
					Hash       string `json:"hash"`
					Generation int64  `json:"generation"`
				}
				if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
					t.Error(err)
				}
				if update.Generation != root.Generation {
					w.WriteHeader(http.StatusPreconditionFailed)
					return
				}
				root.Hash = update.Hash
				root.Generation++
			}
			if err := json.NewEncoder(w).Encode(root); err != nil {
				t.Error(err)
			}
			return
		}
		hash := strings.TrimPrefix(r.URL.Path, "/sync/v3/files/")
		if r.Method == http.MethodPut {
			*puts++
			if failure == "write" || (failure == "partial" && strings.HasSuffix(r.Header.Get("rm-filename"), ".metadata")) {
				w.WriteHeader(http.StatusBadGateway)
				return
			}
			data, err := io.ReadAll(r.Body)
			if err != nil {
				t.Error(err)
			}
			blobs[hash] = data
			return
		}
		data, ok := blobs[hash]
		if !ok {
			http.NotFound(w, r)
			return
		}
		if failure == "verify" && strings.HasSuffix(r.Header.Get("rm-filename"), ".rm") && *puts > 0 {
			data = []byte("corrupt")
		}
		if _, err := w.Write(data); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(server.Close)
	client, err := cloud.NewClient(cloud.WithStorageHost(server.URL), cloud.WithConfig(&cloud.Config{UserToken: "test"}), cloud.WithCacheDir(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	return client, &blobs, puts
}

func metadataOnlyRM(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := rmscene.NewDataWriter(&buf)
	if err := w.WriteHeader(); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteBlock(rmscene.BlockTypeMigrationInfo, 1, 1, func(w *rmscene.DataWriter) error {
		if err := w.WriteId(1, rmscene.CrdtId{Part1: 1, Part2: 1}); err != nil {
			return err
		}
		return w.WriteTaggedBool(2, true)
	}); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteBlock(rmscene.BlockTypeSceneGroup, 1, 1, func(w *rmscene.DataWriter) error {
		for i := 1; i <= 4; i++ {
			if err := w.WriteId(i, rmscene.CrdtId{Part1: 1, Part2: uint64(i)}); err != nil {
				return err
			}
		}
		return w.WriteTaggedInt(5, 0)
	}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestImportStrokes(t *testing.T) {
	ink, err := os.ReadFile("testdata/oct1_notes_strokes.rm")
	if err != nil {
		t.Fatal(err)
	}
	metadata := metadataOnlyRM(t)
	for _, scenario := range []string{"ink", "metadata", "conflict", "alias", "empty-existing", "structure", "write", "partial", "verify", "missing", "duplicate", "bounds", "truncated", "uuid"} {
		t.Run(scenario, func(t *testing.T) {
			var existing []byte
			if scenario == "conflict" || scenario == "alias" {
				existing = ink
			}
			if scenario == "empty-existing" {
				existing = metadata
			}
			client, blobs, puts := importFixture(t, scenario, existing)
			data := ink
			if scenario == "metadata" {
				data = metadata
			}
			if scenario == "truncated" {
				data = append(bytes.Clone(metadata), 1, 2)
			}
			path := filepath.Join(t.TempDir(), "native.rm")
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			mapping := []PageImport{{Source: path, Page: 1}}
			if scenario == "missing" {
				mapping[0].Source += ".missing"
			}
			if scenario == "bounds" {
				mapping[0].Page = 2
			}
			if scenario == "duplicate" {
				mapping = append(mapping, mapping[0])
			}
			id := importDocID
			if scenario == "uuid" {
				id = "Destination"
			}
			result, err := ImportStrokes(context.Background(), client, id, mapping)
			if scenario != "ink" && scenario != "metadata" && scenario != "empty-existing" {
				if err == nil {
					t.Fatal("expected failure")
				}
				if scenario != "write" && scenario != "partial" && scenario != "verify" && *puts != 0 {
					t.Fatal("preflight failure wrote cloud data")
				}
				if scenario == "conflict" && !strings.Contains(err.Error(), "page 1") {
					t.Fatalf("missing page conflict: %v", err)
				}
				if scenario == "partial" && (result == nil || result.State != "staged" || len(result.Uploaded) != 1) {
					t.Fatalf("lost partial transfer progress: %+v", result)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if result.State != "verified" || len(result.Pages) != 1 || result.Pages[0].PageID != "future" {
				t.Fatalf("wrong result: %+v", result)
			}
			if !bytes.Equal((*blobs)[importHash(data)], data) {
				t.Fatal("native bytes changed")
			}
			root, err := client.GetRootState(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			rootManifest, err := client.GetManifest(context.Background(), root.Hash, "root.docSchema")
			if err != nil {
				t.Fatal(err)
			}
			manifest, err := client.GetManifest(context.Background(), rootManifest.Find(importDocID).Hash, importDocID+".docSchema")
			if err != nil {
				t.Fatal(err)
			}
			page := manifest.Find(importDocID + "/future.rm")
			if page == nil || page.Hash != importHash(data) {
				t.Fatal("native page association lost")
			}
			contentBytes, err := client.GetBlobFresh(context.Background(), manifest.Find(importDocID+".content").Hash, importDocID+".content")
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(contentBytes, []byte(`"id":"future"`)) {
				t.Fatal("native page structure changed")
			}
			metadataBytes, err := client.GetBlobFresh(context.Background(), manifest.Find(importDocID+".metadata").Hash, importDocID+".metadata")
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(metadataBytes, []byte(`"custom":"keep"`)) {
				t.Fatal("unrelated metadata lost")
			}
			if !bytes.Equal((*blobs)[importHash([]byte("unchanged PDF background"))], []byte("unchanged PDF background")) {
				t.Fatal("background changed")
			}
			source, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(source, data) {
				t.Fatal("source changed")
			}
		})
	}
}

func TestLoadImportMapping(t *testing.T) {
	for _, input := range []string{`[{"source":"ink.rm","page":0}]`, `[{"source":"ink.rm"}]`, `[{"source":"ink.rm","page":-1}]`, `[{"source":"ink.rm","page":0,"other":true}]`, `[]`, `[{"source":"ink.rm","page":0}] null`} {
		t.Run(input, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "mapping.json")
			if err := os.WriteFile(path, []byte(input), 0600); err != nil {
				t.Fatal(err)
			}
			mapping, err := LoadImportMapping(path)
			if input != `[{"source":"ink.rm","page":0}]` {
				if err == nil {
					t.Fatal("accepted invalid mapping")
				}
				return
			}
			if err != nil || len(mapping) != 1 || mapping[0].Source != filepath.Join(filepath.Dir(path), "ink.rm") || mapping[0].Page != 0 {
				t.Fatalf("wrong mapping: %v, %+v", err, mapping)
			}
		})
	}
}
