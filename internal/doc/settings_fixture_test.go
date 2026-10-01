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
	"sort"
	"strings"
	"sync"
	"testing"

	cloud "github.com/alexgorbatchev/go-remarkable-cloud"
)

const settingsSourceID = "33333333-3333-4333-8333-333333333333"
const settingsDestinationID = "44444444-4444-4444-8444-444444444444"

type settingsCloud struct {
	client                              *cloud.Client
	blobs                               map[string][]byte
	root                                cloud.RootState
	originalSource, originalDestination map[string][]byte
	puts, commits                       int
}

func settingsManifest(blobs map[string][]byte, files map[string][]byte) string {
	var names []string
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	h := sha256.New()
	raw := []byte("3\n")
	for _, name := range names {
		data := files[name]
		digest := sha256.Sum256(data)
		hash := hex.EncodeToString(digest[:])
		blobs[hash] = bytes.Clone(data)
		h.Write(digest[:])
		raw = fmt.Appendf(raw, "%s:0:%s:0:%d\n", hash, name, len(data))
	}
	hash := hex.EncodeToString(h.Sum(nil))
	blobs[hash] = raw
	return hash
}

func settingsContent(schema, prefix string, source bool) []byte {
	pages := fmt.Sprintf(`"pages":["%s0","%s1"],"redirectionPageMap":[0,-1]`, prefix, prefix)
	if schema == "cPages" {
		pages = fmt.Sprintf(`"cPages":{"original":{"timestamp":"1:1","value":2},"pages":[{"id":"%s0","idx":{"timestamp":"1:2","value":"ba"},"redir":{"timestamp":"1:2","value":0}},{"id":"%s1","idx":{"timestamp":"1:3","value":"bb"},"redir":{"timestamp":"1:3","value":-1}}],"uuids":[{"first":"native-author","second":1}]}`, prefix, prefix)
	}
	settings := `"tags":[],"pageTags":[{"name":"keep-unmapped","pageId":"d0","timestamp":12}],"customZoomFuture":{"number":9007199254740993},"zoomMode":"customFit","viewBackgroundFilter":"off","customZoomCenterX":0,"customZoomCenterY":936,"customZoomOrientation":"portrait","customZoomPageHeight":1872,"customZoomPageWidth":1404,"customZoomScale":9`
	if source {
		settings = `"tags":[{"name":"document-tag","timestamp":1790713023807,"extra":{"keep":true}}],"pageTags":[{"name":"meeting","pageId":"s0","timestamp":1790713023807,"extra":{"value":9007199254740993}}],"zoomMode":"customFit","viewBackgroundFilter":"off","customZoomCenterX":0,"customZoomCenterY":936,"customZoomOrientation":"portrait","customZoomPageHeight":1872,"customZoomPageWidth":1404,"customZoomScale":9`
	}
	return []byte(fmt.Sprintf(`{"fileType":"pdf","formatVersion":2,"pageCount":2,%s,%s,"unknown":{"large":9007199254740993,"text":"keep"}}`, pages, settings))
}

func settingsFixture(t *testing.T, sourceSchema, destinationSchema, failure string, mutate ...func([]byte, []byte) ([]byte, []byte)) *settingsCloud {
	t.Helper()
	f := &settingsCloud{blobs: make(map[string][]byte)}
	sourceContent := settingsContent(sourceSchema, "s", true)
	destinationContent := settingsContent(destinationSchema, "d", false)
	switch failure {
	case "uninitialized":
		destinationContent = []byte(`{"fileType":"pdf","pageCount":2,"pages":null}`)
	case "native-id-duplicate":
		destinationContent = bytes.Replace(destinationContent, []byte(`"id":"d1"`), []byte(`"id":"d0"`), 1)
	case "page-count":
		destinationContent = bytes.Replace(destinationContent, []byte(`"pageCount":2`), []byte(`"pageCount":3`), 1)
	case "deleted":
		destinationContent = bytes.Replace(destinationContent, []byte(`"id":"d1"`), []byte(`"id":"d1","deleted":{"value":1}`), 1)
	case "disagree":
		destinationContent = bytes.Replace(destinationContent, []byte(`"pageCount":2`), []byte(`"pageCount":2,"pages":["different","d1"]`), 1)
	case "unknown-page-tag":
		sourceContent = bytes.Replace(sourceContent, []byte(`"pageId":"s0"`), []byte(`"pageId":"missing"`), 1)
	case "tag-conflict":
		destinationContent = bytes.Replace(destinationContent, []byte(`"pageId":"d0"`), []byte(`"pageId":"d1"`), 1)
	case "document-tag-conflict":
		destinationContent = bytes.Replace(destinationContent, []byte(`"tags":[]`), []byte(`"tags":[{"name":"different","timestamp":99}]`), 1)
	case "malformed-tags":
		sourceContent = bytes.Replace(sourceContent, []byte(`"timestamp":1790713023807`), []byte(`"timestamp":"invalid"`), 1)
	case "null-tags":
		sourceContent = bytes.Replace(sourceContent, []byte(`"pageTags":[{"name":"meeting","pageId":"s0","timestamp":1790713023807,"extra":{"value":9007199254740993}}]`), []byte(`"pageTags":null`), 1)
	case "null-zoom":
		sourceContent = bytes.Replace(sourceContent, []byte(`"customZoomCenterX":0`), []byte(`"customZoomCenterX":null`), 1)
	case "unknown-zoom":
		sourceContent = bytes.Replace(sourceContent, []byte(`"zoomMode":"customFit"`), []byte(`"zoomMode":"futureUnknown"`), 1)
	case "duplicate-key":
		sourceContent = bytes.Replace(sourceContent, []byte(`"pageCount":2`), []byte(`"pageCount":2,"pageCount":1`), 1)
	case "viewport-conflict":
		destinationContent = bytes.Replace(destinationContent, []byte(`"zoomMode":"customFit"`), []byte(`"zoomMode":"bestFit"`), 1)
	case "viewport-delete":
		sourceContent = bytes.Replace(sourceContent, []byte(`,"viewBackgroundFilter":"off"`), nil, 1)
	case "viewport-add":
		destinationContent = bytes.Replace(destinationContent, []byte(`,"viewBackgroundFilter":"off"`), nil, 1)
	case "no-page-tags":
		sourceContent = bytes.Replace(sourceContent, []byte(`"pageTags":[{"name":"meeting","pageId":"s0","timestamp":1790713023807,"extra":{"value":9007199254740993}}]`), []byte(`"pageTags":[]`), 1)
	}
	for _, change := range mutate {
		sourceContent, destinationContent = change(sourceContent, destinationContent)
	}
	f.originalSource = map[string][]byte{settingsSourceID + ".content": sourceContent, settingsSourceID + ".metadata": []byte(`{"type":"DocumentType","visibleName":"Source"}`), settingsSourceID + ".pdf": {0, 255, 1, 3}, settingsSourceID + "/s0.rm": {0, 255, 9, 7}}
	f.originalDestination = map[string][]byte{settingsDestinationID + ".content": destinationContent, settingsDestinationID + ".metadata": []byte(`{"type":"DocumentType","visibleName":"Destination","unchanged":true}`), settingsDestinationID + ".pdf": {1, 255, 2, 4}, settingsDestinationID + "/d1.rm": {0, 255, 5, 2}, settingsDestinationID + "/extra.bin": {7, 255, 0}}
	if failure == "folder" {
		f.originalSource[settingsSourceID+".metadata"] = []byte(`{"type":"CollectionType"}`)
	}
	srcHash := settingsManifest(f.blobs, f.originalSource)
	dstHash := settingsManifest(f.blobs, f.originalDestination)
	setRoot := func() {
		raw := fmt.Appendf(nil, "4\n0:.:2:100\n%s:0:%s:4:100\n%s:0:%s:5:100\n", srcHash, settingsSourceID, dstHash, settingsDestinationID)
		f.root.Hash = archiveHash(raw)
		f.blobs[f.root.Hash] = raw
	}
	f.root = cloud.RootState{Generation: 3, SchemaVersion: 4}
	setRoot()
	var mu sync.Mutex
	changed := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.URL.Path == "/sync/v3/root" {
			if r.Method == http.MethodPut {
				f.commits++
				if failure == "generation-conflict" {
					w.WriteHeader(http.StatusPreconditionFailed)
					return
				}
				var update cloud.RootState
				if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
					t.Error(err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				if update.Generation != f.root.Generation {
					w.WriteHeader(http.StatusPreconditionFailed)
					return
				}
				f.root.Hash = update.Hash
				f.root.Generation++
				if failure == "commit-unknown" {
					w.WriteHeader(http.StatusBadGateway)
					return
				}
			}
			if err := json.NewEncoder(w).Encode(f.root); err != nil {
				t.Error(err)
			}
			return
		}
		name := r.Header.Get("rm-filename")
		hash := strings.TrimPrefix(r.URL.Path, "/sync/v3/files/")
		if r.Method == http.MethodPut {
			f.puts++
			if failure == "stage-failure" {
				w.WriteHeader(http.StatusBadGateway)
				return
			}
			data, err := io.ReadAll(r.Body)
			if err != nil {
				t.Error(err)
			}
			f.blobs[hash] = data
			return
		}
		if !changed && strings.HasSuffix(name, ".pdf") && strings.HasPrefix(name, settingsSourceID) && f.commits == 0 {
			switch failure {
			case "root-change":
				f.root.Generation++
				changed = true
			case "source-change":
				srcHash = strings.Repeat("a", 64)
				setRoot()
				f.root.Generation++
				changed = true
			case "destination-change":
				dstHash = strings.Repeat("b", 64)
				setRoot()
				f.root.Generation++
				changed = true
			}
		}
		if !changed && f.commits > 0 && name == "root.docSchema" {
			switch failure {
			case "source-change-after", "destination-change-after":
				data := bytes.Clone(f.blobs[hash])
				old := srcHash
				if failure == "destination-change-after" {
					old = dstHash
				}
				if failure == "destination-change-after" {
					m, err := cloud.ParseManifest(hash, bytes.NewReader(data))
					if err != nil {
						t.Error(err)
					}
					old = m.Find(settingsDestinationID).Hash
				}
				data = bytes.Replace(data, []byte(old), []byte(strings.Repeat("c", 64)), 1)
				f.root.Hash = archiveHash(data)
				f.blobs[f.root.Hash] = data
				f.root.Generation++
				changed = true
				if _, err := w.Write(data); err != nil {
					t.Error(err)
				}
				return
			}
		}
		corrupt := (failure == "corrupt-source" && name == settingsSourceID+".pdf") || (failure == "corrupt-destination" && name == settingsDestinationID+".pdf") || (failure == "stage-corrupt" && f.puts > 0 && f.commits == 0 && name == settingsDestinationID+".content") || (failure == "content-corrupt" && f.commits > 0 && name == settingsDestinationID+".content") || (failure == "pdf-corrupt" && f.commits > 0 && name == settingsDestinationID+".pdf") || (failure == "stroke-corrupt" && f.commits > 0 && name == settingsDestinationID+"/d1.rm") || (failure == "source-corrupt-after" && f.commits > 0 && name == settingsSourceID+"/s0.rm")
		if failure == "verify-root-change" && f.commits > 0 && name == settingsDestinationID+"/extra.bin" {
			f.root.Generation++
		}
		data, ok := f.blobs[hash]
		if !ok {
			http.NotFound(w, r)
			return
		}
		if corrupt {
			data = []byte("corrupt")
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
		for hash := range f.blobs {
			if err := os.WriteFile(filepath.Join(cache, "blobs", hash), []byte("corrupt cached bytes"), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	client, err := cloud.NewClient(cloud.WithStorageHost(server.URL), cloud.WithConfig(&cloud.Config{UserToken: "test"}), cloud.WithCacheDir(cache))
	if err != nil {
		t.Fatal(err)
	}
	f.client = client
	return f
}

func (f *settingsCloud) file(t *testing.T, id, suffix string) []byte {
	t.Helper()
	ctx := context.Background()
	raw, err := f.client.GetBlobFresh(ctx, f.root.Hash, "root.docSchema")
	if err != nil {
		t.Fatal(err)
	}
	root, err := cloud.ParseManifest(f.root.Hash, bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	docHash := root.Find(id).Hash
	raw, err = f.client.GetBlobFresh(ctx, docHash, id+".docSchema")
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := cloud.ParseManifest(docHash, bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	entry := manifest.Find(id + suffix)
	if entry == nil {
		t.Fatalf("missing file %s%s", id, suffix)
	}
	data, err := f.client.GetBlobFresh(ctx, entry.Hash, entry.ID)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func (f *settingsCloud) assertPreserved(t *testing.T) {
	t.Helper()
	for name, data := range f.originalSource {
		if !bytes.Equal(f.file(t, settingsSourceID, strings.TrimPrefix(name, settingsSourceID)), data) {
			t.Errorf("source changed: %s", name)
		}
	}
	for name, data := range f.originalDestination {
		if strings.HasSuffix(name, ".content") {
			continue
		}
		if !bytes.Equal(f.file(t, settingsDestinationID, strings.TrimPrefix(name, settingsDestinationID)), data) {
			t.Errorf("destination changed: %s", name)
		}
	}
}
