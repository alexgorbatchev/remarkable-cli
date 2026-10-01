package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	cloud "github.com/alexgorbatchev/go-remarkable-cloud"
	"github.com/alexgorbatchev/remarkable-cli/internal/doc"
)

func TestSettingsCommandValidation(t *testing.T) {
	t.Setenv("REMARKABLE_CONFIG", filepath.Join(t.TempDir(), "missing-credentials"))
	for _, args := range [][]string{
		{"doc", "settings", "transfer"},
		{"doc", "settings", "transfer", "source", "destination", "--mapping", "missing.json"},
		{"doc", "settings", "transfer", "33333333-3333-4333-8333-333333333333", "33333333-3333-4333-8333-333333333333", "--mapping", "missing.json"},
		{"doc", "settings", "transfer", "33333333-3333-4333-8333-333333333333", "44444444-4444-4444-8444-444444444444", "--mapping", "missing.json"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			out, err := executeRoot(args...)
			if err == nil || strings.Contains(err.Error(), "unknown command") || strings.Contains(err.Error(), "credentials") {
				t.Fatalf("local validation: %v output=%s", err, out)
			}
		})
	}
}

func TestSettingsOutput(t *testing.T) {
	for _, mode := range []string{"0", "1"} {
		for _, state := range []cloud.UpdateState{cloud.UpdateStaged, cloud.UpdateCommitUnknown, cloud.UpdateCommitted, cloud.UpdateVerified} {
			t.Run(fmt.Sprintf("agent=%s/%s", mode, state), func(t *testing.T) {
				t.Setenv("AGENT", mode)
				result := &doc.SettingsResult{State: state, SourceID: "source", DestinationID: "destination", SourceHash: "source-hash", DestinationHash: "destination-hash", RootHash: "root-hash", Generation: 7, Uploaded: []string{"destination.content"}, ViewportDifferences: []doc.ViewportDifference{
					{Field: "zoomMode", Destination: json.RawMessage(`"bestFit"`), Source: json.RawMessage(`"customFit"`)},
					{Field: "viewBackgroundFilter", Destination: json.RawMessage(`"off"`)},
					{Field: "customZoomCenterX", Source: json.RawMessage(`0`)},
				}}
				var out bytes.Buffer
				if err := printSettingsResult(&out, result); err != nil {
					t.Fatal(err)
				}
				for _, value := range []string{string(state), "source-hash", "destination-hash", "root-hash", "destination.content", "zoomMode", "bestFit", "customFit", "viewBackgroundFilter", "absent", "customZoomCenterX"} {
					if !strings.Contains(out.String(), value) {
						t.Errorf("missing %s: %s", value, out.String())
					}
				}
				if mode == "1" && (!strings.Contains(out.String(), "state: "+string(state)+"\n") || !strings.Contains(out.String(), "FIELD\tDESTINATION PRESENT\tDESTINATION VALUE\tSOURCE PRESENT\tSOURCE VALUE\n") || !strings.Contains(out.String(), "viewBackgroundFilter\ttrue\t\"off\"\tfalse\tabsent\n")) {
					t.Fatalf("agent differences: %s", out.String())
				}
			})
		}
	}
	if err := printSettingsResult(settingsFailWriter{}, &doc.SettingsResult{State: cloud.UpdateVerified}); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("output failure lost: %v", err)
	}
}

type settingsFailWriter struct{}

func (settingsFailWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestSettingsMappingValidationBeforeCredentials(t *testing.T) {
	t.Setenv("REMARKABLE_CONFIG", filepath.Join(t.TempDir(), "missing-credentials"))
	path := filepath.Join(t.TempDir(), "mapping.json")
	if err := os.WriteFile(path, []byte(`[{"source_page":0,"destination_page":1},{"source_page":1,"destination_page":1}]`), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := executeRoot("doc", "settings", "transfer", "33333333-3333-4333-8333-333333333333", "44444444-4444-4444-8444-444444444444", "--mapping", path)
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("mapping validation: %v", err)
	}
}

func TestSettingsCommandConflictAndReplacement(t *testing.T) {
	const source = "33333333-3333-4333-8333-333333333333"
	const destination = "44444444-4444-4444-8444-444444444444"
	for _, mode := range []string{"0", "1"} {
		for _, replace := range []bool{false, true} {
			t.Run(fmt.Sprintf("agent=%s/replace=%t", mode, replace), func(t *testing.T) {
				t.Setenv("AGENT", mode)
				blobs := make(map[string][]byte)
				store := func(data []byte) string {
					sum := sha256.Sum256(data)
					hash := hex.EncodeToString(sum[:])
					blobs[hash] = data
					return hash
				}
				manifest := func(id, zoom string) string {
					files := map[string][]byte{id + ".content": []byte(fmt.Sprintf(`{"fileType":"pdf","pages":["page"],"pageCount":1,"zoomMode":"%s","tags":[],"pageTags":[]}`, zoom)), id + ".metadata": []byte(`{"type":"DocumentType"}`), id + ".pdf": {0, 255, 7}}
					var names []string
					for name := range files {
						names = append(names, name)
					}
					sort.Strings(names)
					h := sha256.New()
					raw := []byte("3\n")
					for _, name := range names {
						hash := store(files[name])
						digest, err := hex.DecodeString(hash)
						if err != nil {
							t.Fatal(err)
						}
						h.Write(digest)
						raw = fmt.Appendf(raw, "%s:0:%s:0:%d\n", hash, name, len(files[name]))
					}
					hash := hex.EncodeToString(h.Sum(nil))
					blobs[hash] = raw
					return hash
				}
				srcHash, dstHash := manifest(source, "fitToHeight"), manifest(destination, "bestFit")
				root := cloud.RootState{Hash: store(fmt.Appendf(nil, "4\n0:.:2:100\n%s:0:%s:3:100\n%s:0:%s:3:100\n", srcHash, source, dstHash, destination)), Generation: 5, SchemaVersion: 4}
				puts := 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/sync/v3/root" {
						if r.Method == http.MethodPut {
							var update cloud.RootState
							if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
								t.Error(err)
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
						puts++
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
					if _, err := w.Write(data); err != nil {
						t.Error(err)
					}
				}))
				t.Cleanup(server.Close)
				t.Setenv("REMARKABLE_HOST", server.URL)
				dir := t.TempDir()
				config, mapping := filepath.Join(dir, "config"), filepath.Join(dir, "map.json")
				if err := os.WriteFile(config, []byte("usertoken: private-settings-test-token\n"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(mapping, []byte(`[]`), 0600); err != nil {
					t.Fatal(err)
				}
				args := []string{"doc", "settings", "transfer", source, destination, "--mapping", mapping, "--config", config, "--no-cache"}
				if replace {
					args = append(args, "--replace-viewport")
				}
				out, err := executeRoot(args...)
				if (err == nil) != replace {
					t.Fatalf("command outcome: %v %s", err, out)
				}
				if !strings.Contains(out, "bestFit") || !strings.Contains(out, "fitToHeight") || !strings.Contains(out, "zoomMode") {
					t.Fatalf("conflict differences not printed: %s", out)
				}
				if strings.Contains(out, "private-settings-test-token") || (err != nil && strings.Contains(err.Error(), "private-settings-test-token")) {
					t.Fatal("credentials leaked")
				}
				if replace && (!strings.Contains(out, "verified") || puts != 3) {
					t.Fatalf("replacement progress: %s writes=%d", out, puts)
				}
				if !replace && puts != 0 {
					t.Fatalf("conflict wrote %d blobs", puts)
				}
			})
		}
	}
}
