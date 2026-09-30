package main

import (
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
)

func TestImportCommandValidation(t *testing.T) {
	for _, args := range [][]string{{"doc", "import"}, {"doc", "import", "Destination"}, {"doc", "import", "11111111-1111-4111-8111-111111111111", "--mapping", "missing.json"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			out, err := executeRoot(args...)
			if err == nil {
				t.Fatal("expected argument/mapping error")
			}
			if strings.Contains(err.Error(), "unknown command") {
				t.Fatalf("import command missing: %v; %s", err, out)
			}
			if strings.Contains(err.Error(), "credentials") {
				t.Fatalf("credentials accessed before local validation: %v", err)
			}
		})
	}
}

func TestImportCommandOutput(t *testing.T) {
	for _, mode := range []string{"0", "1"} {
		for _, fail := range []bool{false, true} {
			t.Run(fmt.Sprintf("agent=%s/failure=%t", mode, fail), func(t *testing.T) {
				t.Setenv("AGENT", mode)
				id := "11111111-1111-4111-8111-111111111111"
				blobs := make(map[string][]byte)
				store := func(data []byte) string {
					h := sha256.Sum256(data)
					hash := hex.EncodeToString(h[:])
					blobs[hash] = data
					return hash
				}
				metaHash := store([]byte(`{"visibleName":"Destination","type":"DocumentType"}`))
				contentHash := store([]byte(`{"fileType":"pdf","pages":["native-page"],"pageCount":1}`))
				docHash := store([]byte(fmt.Sprintf("3\n%s:0:%s.metadata:0:100\n%s:0:%s.content:0:100\n", metaHash, id, contentHash, id)))
				root := cloud.RootState{Hash: store([]byte(fmt.Sprintf("4\n0:.:1:200\n%s:0:%s:2:200\n", docHash, id))), Generation: 1, SchemaVersion: 4}
				var mu sync.Mutex
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					mu.Lock()
					defer mu.Unlock()
					if r.URL.Path == "/sync/v3/root" {
						if r.Method == http.MethodPut {
							var update cloud.RootState
							if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
								t.Error(err)
							}
							root.Hash = update.Hash
							root.Generation++
						}
						json.NewEncoder(w).Encode(root)
						return
					}
					hash := strings.TrimPrefix(r.URL.Path, "/sync/v3/files/")
					if r.Method == http.MethodPut {
						if fail && strings.HasSuffix(r.Header.Get("rm-filename"), ".metadata") {
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
					w.Write(data)
				}))
				t.Cleanup(server.Close)
				t.Setenv("REMARKABLE_HOST", server.URL)
				dir := t.TempDir()
				configPath := filepath.Join(dir, "config")
				if err := os.WriteFile(configPath, []byte("usertoken: private-test-token\n"), 0600); err != nil {
					t.Fatal(err)
				}
				t.Setenv("REMARKABLE_CONFIG", configPath)
				ink, err := os.ReadFile("../../internal/doc/testdata/oct1_notes_strokes.rm")
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "page.rm"), ink, 0600); err != nil {
					t.Fatal(err)
				}
				mapping := filepath.Join(dir, "mapping.json")
				if err := os.WriteFile(mapping, []byte(`[{"source":"page.rm","page":0}]`), 0600); err != nil {
					t.Fatal(err)
				}
				out, err := executeRoot("doc", "import", id, "--mapping", mapping, "--no-cache")
				if (err != nil) != fail {
					t.Fatalf("unexpected result: %v; %s", err, out)
				}
				want := "verified"
				if fail {
					want = "staged"
				}
				if !strings.Contains(out, want) || !strings.Contains(out, "native-page") {
					t.Fatalf("missing progress: %s", out)
				}
				if mode == "1" && (!strings.Contains(out, "state: "+want+"\n") || !strings.Contains(out, "PAGE\tPAGE ID\tSOURCE\tSTATE\n")) {
					t.Fatalf("invalid agent output: %s", out)
				}
				if strings.Contains(out, "private-test-token") {
					t.Fatal("credentials exposed")
				}
			})
		}
	}
}
