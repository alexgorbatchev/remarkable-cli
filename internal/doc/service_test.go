package doc

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	cloud "github.com/alexgorbatchev/go-remarkable-cloud"
	"github.com/alexgorbatchev/remarkable-cli/internal/testcloud"
)

func setupMockServer(t *testing.T) (*httptest.Server, *cloud.Client) {
	t.Helper()
	return setupMockServerFailing(t, mockFailure{})
}

func setupMockServerWithFailure(t *testing.T, failedPath string) (*httptest.Server, *cloud.Client) {
	t.Helper()
	return setupMockServerFailing(t, mockFailure{path: failedPath})
}

// mockFailure selects a request the mock cloud answers wrongly.
type mockFailure struct {
	// path is the request path that fails; empty means no request fails.
	path string
	// after is how many requests to path succeed before the failures begin.
	after int
	// body, when set, is served with status 200 in place of the real blob;
	// otherwise the request fails with HTTP 502.
	body string
}

func setupMockServerFailing(t *testing.T, failure mockFailure) (*httptest.Server, *cloud.Client) {
	t.Helper()

	var fail *testcloud.Failure
	if failure.path != "" {
		fail = &testcloud.Failure{
			Path:  failure.path,
			After: failure.after,
			Body:  failure.body,
		}
	}
	ts := testcloud.New(t, testcloud.Options{
		Preset:  testcloud.PresetDoc,
		Failure: fail,
	})
	return ts.Server, ts.NewClient(t)
}

func TestSyncDocumentRejectsUnsupportedFormats(t *testing.T) {
	for _, format := range []string{"jpg", "PNG", "../escape"} {
		t.Run(format, func(t *testing.T) {
			ts, client := setupMockServer(t)
			defer ts.Close()
			outputDir := t.TempDir()
			results, err := SyncDocument(context.Background(), client, "doc-1", SyncDocOptions{
				OutputDir: outputDir,
				Format:    format,
			})
			if err == nil || !strings.Contains(err.Error(), "unsupported format") {
				t.Fatalf("expected unsupported format error, got %v; results: %+v", err, results)
			}
			if len(results) != 0 {
				t.Errorf("invalid format returned page results: %+v", results)
			}
			entries, err := os.ReadDir(outputDir)
			if err != nil || len(entries) != 0 {
				t.Fatalf("invalid format created output: %v, %v", entries, err)
			}
		})
	}
}

func TestSyncDocumentPDFDownloadFailure(t *testing.T) {
	for _, format := range []string{"png", "svg", "rm"} {
		t.Run(format, func(t *testing.T) {
			ts, client := setupMockServerWithFailure(t, "/sync/v3/files/pdf-hash")
			defer ts.Close()
			outputDir := t.TempDir()
			results, err := SyncDocument(context.Background(), client, "doc-1", SyncDocOptions{
				OutputDir: outputDir,
				Format:    format,
			})
			if format == "png" {
				if err == nil || !strings.Contains(err.Error(), "downloading background PDF") || !strings.Contains(err.Error(), "502") {
					t.Fatalf("expected contextual PDF download error, got %v; results: %+v", err, results)
				}
				if len(results) != 0 {
					t.Errorf("PDF download failure returned page results: %+v", results)
				}
				entries, err := os.ReadDir(outputDir)
				if err != nil || len(entries) != 0 {
					t.Fatalf("PDF download failure created output: %v, %v", entries, err)
				}
				return
			}
			if err != nil || len(results) != 1 || results[0].State != "written" {
				t.Fatalf("stroke-only sync needs no PDF: %v; results: %+v", err, results)
			}
			data, err := os.ReadFile(results[0].Path)
			if err != nil || len(data) == 0 {
				t.Fatalf("stroke-only sync wrote no data: %v", err)
			}
		})
	}
}

// TestReadFailuresFailInsteadOfReportingEmpty checks that a document whose
// content, manifest, or page text cannot be read fails with the cause instead
// of reporting no pages, unmarked pages, or no matches.
func TestReadFailuresFailInsteadOfReportingEmpty(t *testing.T) {
	inspect := func(pages bool) func(context.Context, *cloud.Client) (any, error) {
		return func(ctx context.Context, client *cloud.Client) (any, error) {
			return Inspect(ctx, client, "doc-1", pages)
		}
	}
	tests := []struct {
		name    string
		failure mockFailure
		call    func(context.Context, *cloud.Client) (any, error)
		// wantText is a part of the error message that names the failure.
		wantText string
		// wantStatus is the HTTP status the error must carry; 0 means none.
		wantStatus int
	}{
		{
			name:       "inspect content fetch",
			failure:    mockFailure{path: "/sync/v3/files/content-hash"},
			call:       inspect(false),
			wantText:   "fetching content schema",
			wantStatus: http.StatusBadGateway,
		},
		{
			name:     "inspect content parse",
			failure:  mockFailure{path: "/sync/v3/files/content-hash", body: "not json"},
			call:     inspect(false),
			wantText: "unmarshal content",
		},
		{
			// Resolving doc-1 reads its manifest once; --pages reads it again.
			name:       "inspect pages manifest fetch",
			failure:    mockFailure{path: "/sync/v3/files/doc-hash", after: 1},
			call:       inspect(true),
			wantText:   "fetching document manifest",
			wantStatus: http.StatusBadGateway,
		},
		{
			name: "search page text extraction",
			call: func(ctx context.Context, client *cloud.Client) (any, error) {
				return SearchDocument(ctx, client, "doc-unloadable", SearchQuery{Text: "needle"})
			},
			wantText: "extracting text from page 1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts, client := setupMockServerFailing(t, tt.failure)
			defer ts.Close()
			got, err := tt.call(context.Background(), client)
			if err == nil || !strings.Contains(err.Error(), tt.wantText) {
				t.Fatalf("got %+v, %v; want an error containing %q", got, err, tt.wantText)
			}
			if tt.wantStatus != 0 {
				var status *cloud.StatusError
				if !errors.As(err, &status) || status.StatusCode != tt.wantStatus {
					t.Errorf("error %v does not carry HTTP status %d", err, tt.wantStatus)
				}
			}
		})
	}
}

// TestInspectMissingContent checks that a document without a content file
// fails as missing, while a folder, which has no pages, reports none.
func TestInspectMissingContent(t *testing.T) {
	ts, client := setupMockServer(t)
	defer ts.Close()
	ctx := context.Background()

	details, err := Inspect(ctx, client, "doc-nocontent", true)
	if !errors.Is(err, cloud.ErrItemNotFound) || !strings.Contains(err.Error(), "fetching content schema") {
		t.Errorf("Inspect(doc-nocontent) = %+v, %v; want a missing content error", details, err)
	}

	// Contract guard: the folder has no content file, as sync v3 folders may not.
	folder, err := Inspect(ctx, client, "folder-1", true)
	if err != nil || folder.Type != "CollectionType" || folder.Pages != 0 || len(folder.PageList) != 0 {
		t.Errorf("Inspect(folder-1) = %+v, %v; want a folder with no pages", folder, err)
	}
}

func TestDocService_ListAndTree(t *testing.T) {
	ts, client := setupMockServer(t)
	defer ts.Close()

	ctx := context.Background()

	// 1. List with various filters
	items, err := List(ctx, client, "", "", "", 10)
	if err != nil || len(items) != 8 {
		t.Fatalf("List all failed: %v (got %d)", err, len(items))
	}

	// List with limit=1 to trigger limit break
	limited, _ := List(ctx, client, "", "", "", 1)
	if len(limited) != 1 {
		t.Errorf("expected 1 item with limit=1, got %d", len(limited))
	}

	// Filter by parent folder
	itemsInFolder, err := List(ctx, client, "folder-1", "", "", 10)
	if err != nil || len(itemsInFolder) != 1 || itemsInFolder[0].ID != "doc-1" {
		t.Fatalf("List folder-1 failed: %v", err)
	}

	// Filter by type
	foldersOnly, err := List(ctx, client, "", "CollectionType", "", 10)
	if err != nil || len(foldersOnly) != 1 || foldersOnly[0].ID != "folder-1" {
		t.Fatalf("List CollectionType failed: %v", err)
	}

	// Filter by query
	queryMatch, err := List(ctx, client, "", "", "Document", 10)
	if err != nil || len(queryMatch) != 1 || queryMatch[0].ID != "doc-1" {
		t.Fatalf("List query failed: %v", err)
	}
	if queryMatch[0].Name != "My Document" || queryMatch[0].Modified != "2026-09-29T10:00:00Z" {
		t.Fatalf("List lost document metadata: %+v", queryMatch[0])
	}
	byTitle, err := Inspect(ctx, client, "My Document", false)
	if err != nil || byTitle.ID != "doc-1" || byTitle.Name != "My Document" {
		t.Fatalf("Inspect by title failed: %v, %+v", err, byTitle)
	}

	// 2. BuildTree
	tree, err := BuildTree(ctx, client)
	if err != nil || tree == nil || len(tree.Children) == 0 {
		t.Fatalf("BuildTree failed: %v", err)
	}

	// 3. Inspect (with and without pages)
	details, err := Inspect(ctx, client, "doc-1", false)
	if err != nil || details.Pages != 1 || details.Name != "My Document" {
		t.Fatalf("Inspect basic failed: %v, %+v", err, details)
	}

	detailsWithPages, err := Inspect(ctx, client, "doc-1", true)
	if err != nil || len(detailsWithPages.PageList) != 1 || !detailsWithPages.PageList[0].HasStrokes {
		t.Fatalf("Inspect with pages failed: %v, %+v", err, detailsWithPages)
	}
}

func TestSearchDocument(t *testing.T) {
	ts, client := setupMockServer(t)
	defer ts.Close()
	ctx := context.Background()

	tests := []struct {
		name            string
		doc             string
		query           SearchQuery
		wantPageIndexes []int
		wantErr         error
		wantErrContains string
	}{
		{
			name:            "match query 2026",
			doc:             "doc-1",
			query:           SearchQuery{Text: "2026"},
			wantPageIndexes: []int{0, 1, 2, 3, 4, 5},
		},
		{
			name:            "match deeper text Priority",
			doc:             "doc-1",
			query:           SearchQuery{Text: "Priority"},
			wantPageIndexes: []int{1, 2, 3, 4, 5},
		},
		{
			name:            "match deeper text Notes",
			doc:             "doc-1",
			query:           SearchQuery{Text: "Notes"},
			wantPageIndexes: []int{1, 2, 3, 4, 5},
		},
		{
			name:    "blank query",
			doc:     "doc-1",
			query:   SearchQuery{Text: ""},
			wantErr: errBlankSearchQuery,
		},
		{
			name:    "whitespace query",
			doc:     "doc-1",
			query:   SearchQuery{Text: " \r\n\t"},
			wantErr: errBlankSearchQuery,
		},
		{
			name:            "doc with no PDF",
			doc:             "doc-nopdf",
			query:           SearchQuery{Text: "query"},
			wantErrContains: "no searchable PDF text",
		},
		{
			name:            "corrupt PDF",
			doc:             "doc-corrupt",
			query:           SearchQuery{Text: "query"},
			wantErrContains: "opening PDF",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches, err := SearchDocument(ctx, client, tt.doc, tt.query)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("SearchDocument(%q) error = %v, want %v", tt.query.Text, err, tt.wantErr)
				}
				return
			}
			if tt.wantErrContains != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrContains) {
					t.Fatalf("SearchDocument(%q) error = %v, want error containing %q", tt.query.Text, err, tt.wantErrContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("SearchDocument(%q) unexpected error: %v", tt.query.Text, err)
			}
			var gotIndexes []int
			for _, m := range matches {
				gotIndexes = append(gotIndexes, m.PageIndex)
			}
			if !slices.Equal(gotIndexes, tt.wantPageIndexes) {
				t.Fatalf("SearchDocument(%q) page indexes = %v, want %v", tt.query.Text, gotIndexes, tt.wantPageIndexes)
			}
		})
	}
}

func TestGetLinks(t *testing.T) {
	ts, client := setupMockServer(t)
	defer ts.Close()
	ctx := context.Background()

	tests := []struct {
		name            string
		doc             string
		page            int
		wantLinks       bool
		wantErr         error
		wantErrContains string
	}{
		{
			name:      "doc-1 page 0 has links",
			doc:       "doc-1",
			page:      0,
			wantLinks: true,
		},
		{
			name:            "doc-nopdf has no background PDF",
			doc:             "doc-nopdf",
			page:            0,
			wantErrContains: `document "doc-nopdf" has no background PDF`,
		},
		{
			name:            "doc-corrupt opening PDF error",
			doc:             "doc-corrupt",
			page:            0,
			wantErrContains: "opening PDF document",
		},
		{
			name:            "out of bounds page 999",
			doc:             "doc-1",
			page:            999,
			wantErrContains: "page index out of bounds",
		},
		{
			name:            "negative page -1",
			doc:             "doc-1",
			page:            -1,
			wantErrContains: "page index out of bounds",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			links, err := GetLinks(ctx, client, tt.doc, tt.page)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("GetLinks error = %v, want %v", err, tt.wantErr)
				}
			}
			if tt.wantErrContains != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrContains) {
					t.Fatalf("GetLinks error = %v, want error containing %q", err, tt.wantErrContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("GetLinks unexpected error: %v", err)
			}
			if tt.wantLinks && len(links) == 0 {
				t.Fatal("GetLinks returned 0 links, want at least 1")
			}
			if tt.wantLinks && links[0].TargetPage < 0 {
				t.Errorf("expected positive target page, got %d", links[0].TargetPage)
			}
		})
	}
}

func TestCat(t *testing.T) {
	ts, client := setupMockServer(t)
	defer ts.Close()
	ctx := context.Background()

	tests := []struct {
		name            string
		doc             string
		page            int
		format          string
		wantMinLen      int
		wantContains    string
		wantErr         error
		wantErrContains string
	}{
		{
			name:       "doc-1 pdf",
			doc:        "doc-1",
			page:       0,
			format:     "pdf",
			wantMinLen: 1,
		},
		{
			name:       "doc-suffix pdf suffix fallback",
			doc:        "doc-suffix",
			page:       0,
			format:     "pdf",
			wantMinLen: 1,
		},
		{
			name:       "doc-1 text",
			doc:        "doc-1",
			page:       0,
			format:     "text",
			wantMinLen: 1,
		},
		{
			name:       "doc-1 rm",
			doc:        "doc-1",
			page:       0,
			format:     "rm",
			wantMinLen: 1,
		},
		{
			name:       "doc-suffix rm suffix fallback",
			doc:        "doc-suffix",
			page:       0,
			format:     "rm",
			wantMinLen: 1,
		},
		{
			name:         "doc-1 svg",
			doc:          "doc-1",
			page:         0,
			format:       "svg",
			wantContains: "<svg",
		},
		{
			name:    "doc-1 out of bounds",
			doc:     "doc-1",
			page:    999,
			format:  "rm",
			wantErr: ErrPageOutOfBounds,
		},
		{
			name:    "doc-1 negative page",
			doc:     "doc-1",
			page:    -1,
			format:  "rm",
			wantErr: ErrPageOutOfBounds,
		},
		{
			name:            "doc-1 unsupported format",
			doc:             "doc-1",
			page:            0,
			format:          "unsupported-format",
			wantErrContains: "unsupported format",
		},
		{
			name:            "doc-nopdf pdf",
			doc:             "doc-nopdf",
			page:            0,
			format:          "pdf",
			wantErrContains: "no background PDF",
		},
		{
			name:            "doc-nopdf text",
			doc:             "doc-nopdf",
			page:            0,
			format:          "text",
			wantErrContains: "no PDF text",
		},
		{
			name:            "doc-nostroke rm",
			doc:             "doc-nostroke",
			page:            0,
			format:          "rm",
			wantErrContains: "no stroke data found",
		},
		{
			name:            "doc-nostroke svg",
			doc:             "doc-nostroke",
			page:            0,
			format:          "svg",
			wantErrContains: "no stroke data found",
		},
		{
			name:            "doc-nocontent rm",
			doc:             "doc-nocontent",
			page:            0,
			format:          "rm",
			wantErr:         cloud.ErrItemNotFound,
			wantErrContains: "fetching content schema",
		},
		{
			name:            "doc-corrupt text",
			doc:             "doc-corrupt",
			page:            0,
			format:          "text",
			wantErrContains: "opening PDF",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := Cat(ctx, client, tt.doc, tt.page, tt.format, &buf)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Cat error = %v, want %v", err, tt.wantErr)
				}
			}
			if tt.wantErrContains != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrContains) {
					t.Fatalf("Cat error = %v, want error containing %q", err, tt.wantErrContains)
				}
				return
			}
			if tt.wantErr == nil && tt.wantErrContains == "" && err != nil {
				t.Fatalf("Cat unexpected error: %v", err)
			}
			if tt.wantMinLen > 0 && buf.Len() < tt.wantMinLen {
				t.Fatalf("Cat output len = %d, want >= %d", buf.Len(), tt.wantMinLen)
			}
			if tt.wantContains != "" && !strings.Contains(buf.String(), tt.wantContains) {
				t.Fatalf("Cat output = %q, want substring %q", buf.String(), tt.wantContains)
			}
		})
	}
}

func TestRenderPage(t *testing.T) {
	ts, client := setupMockServer(t)
	defer ts.Close()
	ctx := context.Background()

	tmpDir := t.TempDir()

	tests := []struct {
		name            string
		doc             string
		page            int
		dpi             int
		wantErr         error
		wantErrContains string
	}{
		{
			name: "doc-1 default DPI",
			doc:  "doc-1",
			page: 0,
			dpi:  0,
		},
		{
			name: "doc-1 150 DPI",
			doc:  "doc-1",
			page: 0,
			dpi:  150,
		},
		{
			name: "doc-suffix 200 DPI suffix match",
			doc:  "doc-suffix",
			page: 0,
			dpi:  200,
		},
		{
			name: "doc-nostroke 200 DPI without strokes",
			doc:  "doc-nostroke",
			page: 0,
			dpi:  200,
		},
		{
			name:            "doc-nopdf no background template",
			doc:             "doc-nopdf",
			page:            0,
			dpi:             200,
			wantErrContains: "no PDF stationery template",
		},
		{
			name:            "doc-corrupt missing content schema",
			doc:             "doc-corrupt",
			page:            0,
			dpi:             200,
			wantErr:         cloud.ErrItemNotFound,
			wantErrContains: "fetching content schema",
		},
		{
			name:            "doc-nocontent missing content schema",
			doc:             "doc-nocontent",
			page:            0,
			dpi:             200,
			wantErr:         cloud.ErrItemNotFound,
			wantErrContains: "fetching content schema",
		},
		{
			name:    "doc-1 out of bounds page 999",
			doc:     "doc-1",
			page:    999,
			dpi:     200,
			wantErr: ErrPageOutOfBounds,
		},
		{
			name:    "doc-1 negative page -1",
			doc:     "doc-1",
			page:    -1,
			dpi:     200,
			wantErr: ErrPageOutOfBounds,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outPath := filepath.Join(tmpDir, tt.name+".png")
			err := RenderPage(ctx, client, tt.doc, tt.page, tt.dpi, outPath)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("RenderPage error = %v, want %v", err, tt.wantErr)
				}
			}
			if tt.wantErrContains != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrContains) {
					t.Fatalf("RenderPage error = %v, want error containing %q", err, tt.wantErrContains)
				}
				return
			}
			if tt.wantErr == nil && tt.wantErrContains == "" {
				if err != nil {
					t.Fatalf("RenderPage unexpected error: %v", err)
				}
				info, err := os.Stat(outPath)
				if err != nil || info.Size() == 0 {
					t.Fatalf("expected non-empty output PNG at %s (err: %v)", outPath, err)
				}
			}
		})
	}
}

func TestSyncDocument(t *testing.T) {
	ts, client := setupMockServer(t)
	defer ts.Close()
	ctx := context.Background()

	tmpDir := t.TempDir()
	syncDir := filepath.Join(tmpDir, "synced")

	tests := []struct {
		name            string
		doc             string
		opts            SyncDocOptions
		wantCount       int
		wantState       string
		wantErr         error
		wantErrContains string
	}{
		{
			name: "doc-1 png written",
			doc:  "doc-1",
			opts: SyncDocOptions{
				OutputDir: syncDir,
				Format:    "png",
				Force:     false,
				Pages:     []int{0},
			},
			wantCount: 1,
			wantState: "written",
		},
		{
			name: "doc-1 png skipped when existing without force",
			doc:  "doc-1",
			opts: SyncDocOptions{
				OutputDir: syncDir,
				Format:    "png",
				Force:     false,
			},
			wantCount: 1,
			wantState: "skipped",
		},
		{
			name: "doc-1 svg written",
			doc:  "doc-1",
			opts: SyncDocOptions{
				OutputDir: syncDir,
				Format:    "svg",
				Force:     true,
			},
			wantCount: 1,
			wantState: "written",
		},
		{
			name: "doc-1 rm written",
			doc:  "doc-1",
			opts: SyncDocOptions{
				OutputDir: syncDir,
				Format:    "rm",
				Force:     true,
			},
			wantCount: 1,
			wantState: "written",
		},
		{
			name: "doc-nostroke png written",
			doc:  "doc-nostroke",
			opts: SyncDocOptions{
				OutputDir: syncDir,
				Format:    "png",
				Force:     true,
			},
			wantCount: 1,
			wantState: "written",
		},
		{
			name: "doc-nostroke svg written",
			doc:  "doc-nostroke",
			opts: SyncDocOptions{
				OutputDir: syncDir,
				Format:    "svg",
				Force:     true,
			},
			wantCount: 1,
			wantState: "written",
		},
		{
			name: "doc-nostroke rm written",
			doc:  "doc-nostroke",
			opts: SyncDocOptions{
				OutputDir: syncDir,
				Format:    "rm",
				Force:     true,
			},
			wantCount: 1,
			wantState: "written",
		},
		{
			name: "doc-corrupt missing content schema",
			doc:  "doc-corrupt",
			opts: SyncDocOptions{
				OutputDir: syncDir,
				Format:    "png",
				Force:     true,
			},
			wantErr:         cloud.ErrItemNotFound,
			wantErrContains: "fetching content schema",
		},
		{
			name: "doc-1 oob pages filtered",
			doc:  "doc-1",
			opts: SyncDocOptions{
				OutputDir: syncDir,
				Format:    "png",
				Pages:     []int{-1, 0, 999},
				Force:     true,
			},
			wantCount: 1,
			wantState: "written",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := SyncDocument(ctx, client, tt.doc, tt.opts)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("SyncDocument error = %v, want %v", err, tt.wantErr)
				}
			}
			if tt.wantErrContains != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrContains) {
					t.Fatalf("SyncDocument error = %v, want error containing %q", err, tt.wantErrContains)
				}
				return
			}
			if tt.wantErr == nil && tt.wantErrContains == "" && err != nil {
				t.Fatalf("SyncDocument unexpected error: %v", err)
			}
			if len(results) != tt.wantCount {
				t.Fatalf("SyncDocument results count = %d, want %d", len(results), tt.wantCount)
			}
			if tt.wantCount > 0 && results[0].State != tt.wantState {
				t.Fatalf("SyncDocument result[0].State = %q, want %q", results[0].State, tt.wantState)
			}
		})
	}

	t.Run("default output directory isolates writes to temp dir", func(t *testing.T) {
		tempDir := t.TempDir()
		t.Chdir(tempDir)

		results, err := SyncDocument(ctx, client, "doc-1", SyncDocOptions{
			OutputDir: "",
			Format:    "",
			Force:     true,
		})
		if err != nil || len(results) != 1 {
			t.Fatalf("SyncDocument default failed: %v, results=%+v", err, results)
		}
		expectedPath := filepath.Join(tempDir, "My Document", "page-000.png")
		if _, err := os.Stat(expectedPath); err != nil {
			t.Fatalf("expected written page at %s, err: %v", expectedPath, err)
		}
	})
}

func TestGetPageIDs_Unit(t *testing.T) {
	// 1. dc.Pages non-empty
	dc1 := &cloud.DocumentContent{Pages: []string{"a", "b"}}
	ids1 := getPageIDs(dc1)
	if len(ids1) != 2 || ids1[0] != "a" {
		t.Errorf("expected [a, b], got %v", ids1)
	}

	// 2. dc.CPages.Pages
	dc2 := &cloud.DocumentContent{
		CPages: cloud.CPages{
			Pages: []cloud.CPage{
				{ID: "p1"},
				{ID: ""},
				{ID: "p2"},
			},
		},
	}
	ids2 := getPageIDs(dc2)
	if len(ids2) != 2 || ids2[0] != "p1" || ids2[1] != "p2" {
		t.Errorf("expected [p1, p2], got %v", ids2)
	}
}

func TestDocService_MissingItems(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	client, _ := cloud.NewClient(
		cloud.WithConfig(&cloud.Config{UserToken: "usr"}),
		cloud.WithEndpoints(&cloud.Endpoints{WebappHost: ts.URL, StorageHost: ts.URL}),
	)

	ctx := context.Background()
	if _, err := Inspect(ctx, client, "nonexistent", false); err == nil {
		t.Error("expected error for nonexistent inspect")
	}
	if _, err := SearchDocument(ctx, client, "nonexistent", SearchQuery{Text: "query"}); err == nil {
		t.Error("expected error for nonexistent search")
	}
	if _, err := GetLinks(ctx, client, "nonexistent", 0); err == nil {
		t.Error("expected error for nonexistent links")
	}
	if err := Cat(ctx, client, "nonexistent", 0, "pdf", io.Discard); err == nil {
		t.Error("expected error for nonexistent cat")
	}
	if err := RenderPage(ctx, client, "nonexistent", 0, 200, "/tmp/out.png"); err == nil {
		t.Error("expected error for nonexistent render")
	}
	if _, err := SyncDocument(ctx, client, "nonexistent", SyncDocOptions{}); err == nil {
		t.Error("expected error for nonexistent sync")
	}
}

func TestParseFormats(t *testing.T) {
	tests := []struct {
		name    string
		parse   func(string) (string, error)
		input   string
		want    string
		wantErr string
	}{
		{"cat ignores case and spaces", ParseCatFormat, " PDF ", "pdf", ""},
		{"cat text", ParseCatFormat, "text", "text", ""},
		{"cat rejects png", ParseCatFormat, "PNG", "", `unsupported format "png" (choose from: pdf, text, rm, svg)`},
		{"sync empty selects png", ParseSyncFormat, "", "png", ""},
		{"sync rm", ParseSyncFormat, "rm", "rm", ""},
		{"sync is exact", ParseSyncFormat, "SVG", "", `unsupported format "SVG" (choose from: png, svg, rm)`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.parse(tt.input)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("parse(%q) error = %v, want %q", tt.input, err, tt.wantErr)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("parse(%q) = %q, %v; want %q", tt.input, got, err, tt.want)
			}
		})
	}
}

func TestList_CaseFoldingQuery(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token/v2/user", "/token/json/2/user/new":
			w.Write([]byte("mock-token"))
		case "/sync/v3/root":
			w.Write([]byte(`{"hash":"root-hash","generation":1,"schemaVersion":3}`))
		case "/sync/v3/files/root-hash":
			w.Write([]byte("h1:d1:0:10\nh2:d2:0:10\nh3:d3:0:10\n"))
		case "/sync/v3/files/h1":
			w.Write([]byte("m1:d1.metadata:0:50\n"))
		case "/sync/v3/files/m1":
			w.Write([]byte(`{"visibleName":"Straße","type":"DocumentType"}`))
		case "/sync/v3/files/h2":
			w.Write([]byte("m2:d2.metadata:0:50\n"))
		case "/sync/v3/files/m2":
			w.Write([]byte(`{"visibleName":"κόσμος","type":"DocumentType"}`))
		case "/sync/v3/files/h3":
			w.Write([]byte("m3:d3.metadata:0:50\n"))
		case "/sync/v3/files/m3":
			w.Write([]byte(`{"visibleName":"ſtyle guide","type":"DocumentType"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	client, err := cloud.NewClient(
		cloud.WithConfig(&cloud.Config{DeviceToken: "dev", UserToken: "usr"}),
		cloud.WithEndpoints(&cloud.Endpoints{WebappHost: ts.URL, StorageHost: ts.URL}),
	)
	if err != nil {
		t.Fatalf("creating cloud client: %v", err)
	}

	cases := []struct {
		query  string
		wantID string
	}{
		{"strasse", "d1"},
		{"ΚΌΣΜΟΣ", "d2"},
		{"style", "d3"},
	}

	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			res, err := List(context.Background(), client, "", "", tc.query, 0)
			if err != nil {
				t.Fatalf("List(%q) failed: %v", tc.query, err)
			}
			if len(res) != 1 || res[0].ID != tc.wantID {
				t.Fatalf("List(%q) returned %+v, want %s", tc.query, res, tc.wantID)
			}
		})
	}
}

