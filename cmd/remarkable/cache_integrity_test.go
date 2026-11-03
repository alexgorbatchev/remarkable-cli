package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestCloudHashCannotEscapeCLICache(t *testing.T) {
	base := t.TempDir()
	outside := filepath.Join(base, "outside")
	if err := os.WriteFile(outside, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	var blobRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sync/v3/root" {
			if err := json.NewEncoder(w).Encode(map[string]any{"hash": "../../outside", "generation": 1, "schemaVersion": 4}); err != nil {
				t.Error(err)
			}
			return
		}
		blobRequests.Add(1)
		http.Error(w, "unexpected blob request", http.StatusInternalServerError)
	}))
	defer server.Close()
	t.Setenv("REMARKABLE_HOST", server.URL)
	t.Setenv("REMARKABLE_CONFIG", writeCredentials(t, pairedCredentials))
	cache := filepath.Join(base, "cache")
	out, err := executeRoot("doc", "list", "--cache-dir", cache)
	if err == nil || !strings.Contains(err.Error(), "SHA-256 hash") || out != "" || blobRequests.Load() != 0 {
		t.Fatalf("output %q, error %v, blob requests %d", out, err, blobRequests.Load())
	}
	data, err := os.ReadFile(outside)
	if err != nil || string(data) != "preserve" {
		t.Fatalf("outside file %q, %v", data, err)
	}
	if _, err := os.Stat(cache); !os.IsNotExist(err) {
		t.Fatalf("invalid hash touched cache: %v", err)
	}
}
