package testcloud

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	cloud "github.com/alexgorbatchev/go-remarkable-cloud"
)

// LibraryItem describes metadata and optional content in a listing fixture.
type LibraryItem struct {
	ID       string
	Metadata cloud.ItemMetadata
	Content  []byte
}

// NewLibrary serves a content-addressed library in the supplied root order.
func NewLibrary(t *testing.T, items []LibraryItem) *httptest.Server {
	t.Helper()
	blobs := make(map[string][]byte)
	root := "4\n"
	for _, item := range items {
		metadata, err := json.Marshal(item.Metadata)
		if err != nil {
			t.Fatal(err)
		}
		metaHash := FileHash(metadata)
		blobs[metaHash] = metadata
		manifest := fmt.Sprintf("3\n%s:0:%s.metadata:0:%d\n", metaHash, item.ID, len(metadata))
		count := 1
		if item.Content != nil {
			contentHash := FileHash(item.Content)
			blobs[contentHash] = item.Content
			manifest += fmt.Sprintf("%s:0:%s.content:0:%d\n", contentHash, item.ID, len(item.Content))
			count++
		}
		hash := ManifestHash([]byte(manifest))
		blobs[hash] = []byte(manifest)
		root += fmt.Sprintf("%s:0:%s:%d:%d\n", hash, item.ID, count, len(manifest))
	}
	rootHash := FileHash([]byte(root))
	blobs[rootHash] = []byte(root)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sync/v3/root" {
			if err := json.NewEncoder(w).Encode(cloud.RootState{Hash: rootHash, Generation: 1, SchemaVersion: 4}); err != nil {
				t.Error(err)
			}
			return
		}
		if strings.HasPrefix(r.URL.Path, "/token/") {
			if _, err := w.Write([]byte("mock-token")); err != nil {
				t.Error(err)
			}
			return
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
	return server
}
