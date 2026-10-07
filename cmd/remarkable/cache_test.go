package main

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestCacheDirHoldsDownloadedBlobs guards the cache contract that the
// --cache-dir help, README and skill describe: cloud commands store every
// downloaded manifest and document file under blobs/<hash> and nothing else,
// reuse those files on later runs, refill a deleted cache, and --no-cache
// neither reads nor writes the cache.
func TestCacheDirHoldsDownloadedBlobs(t *testing.T) {
	pdfData, err := os.ReadFile("../../internal/doc/testdata/linked_pages.pdf")
	if err != nil {
		t.Fatalf("reading pdf fixture: %v", err)
	}
	rmData, err := os.ReadFile("../../internal/doc/testdata/oct1_notes_strokes.rm")
	if err != nil {
		t.Fatalf("reading rm fixture: %v", err)
	}
	cacheDir := filepath.Join(t.TempDir(), "cache")
	// catPage streams page 0 of doc-1 as the background PDF or the raw strokes.
	catPage := func(format string, flags ...string) error {
		args := append([]string{"doc", "cat", "doc-1", "--page", "0", "--format", format}, flags...)
		if out, err := executeRoot(args...); err != nil {
			return errors.Join(err, errors.New(out))
		}
		return nil
	}
	downloadPage := func() {
		t.Helper()
		for _, format := range []string{"pdf", "rm"} {
			if err := catPage(format, "--cache-dir", cacheDir); err != nil {
				t.Fatalf("doc cat --format %s with cache: %v", format, err)
			}
		}
	}
	assertCache := func() {
		t.Helper()
		top, err := os.ReadDir(cacheDir)
		if err != nil {
			t.Fatalf("reading cache directory: %v", err)
		}
		if len(top) != 1 || top[0].Name() != "blobs" || !top[0].IsDir() {
			t.Fatalf("cache directory must hold only blobs/, got %v", top)
		}
		entries, err := os.ReadDir(filepath.Join(cacheDir, "blobs"))
		if err != nil {
			t.Fatalf("reading blob cache: %v", err)
		}
		var names []string
		for _, entry := range entries {
			if !entry.Type().IsRegular() {
				t.Fatalf("blob cache holds non-file entry %s", entry.Name())
			}
			names = append(names, entry.Name())
		}
		// Every downloaded manifest, metadata, content, PDF and stroke file is
		// cached under its cloud hash.
		want := []string{"content-hash", "doc-hash", "meta-hash", "pdf-hash", "root-hash", "stroke-hash"}
		if !slices.Equal(names, want) {
			t.Fatalf("blob cache entries = %v, want %v", names, want)
		}
		for hash, data := range map[string][]byte{"pdf-hash": pdfData, "stroke-hash": rmData} {
			got, err := os.ReadFile(filepath.Join(cacheDir, "blobs", hash))
			if err != nil || !bytes.Equal(got, data) {
				t.Fatalf("cached %s differs from the downloaded bytes (err %v)", hash, err)
			}
		}
	}

	ts, _ := setupCLITestEnv(t)
	defer ts.Close()
	downloadPage()
	assertCache()

	if err := os.RemoveAll(cacheDir); err != nil {
		t.Fatal(err)
	}
	downloadPage()
	assertCache()

	failing, _ := setupCLITestEnvWithFailure(t, "/sync/v3/files/pdf-hash")
	defer failing.Close()
	if err := catPage("pdf", "--cache-dir", cacheDir); err != nil {
		t.Fatalf("doc cat must reuse the cached PDF: %v", err)
	}
	if err := catPage("pdf", "--cache-dir", cacheDir, "--no-cache"); err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("--no-cache must download the PDF instead of reading the cache, got %v", err)
	}
	unused := filepath.Join(t.TempDir(), "unused")
	// This run downloads the manifests and metadata before the PDF fails.
	if err := catPage("pdf", "--cache-dir", unused, "--no-cache"); err == nil {
		t.Fatal("--no-cache must download the failing PDF")
	}
	if _, err := os.Stat(unused); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("--no-cache wrote the cache directory %s (stat err %v)", unused, err)
	}
}
