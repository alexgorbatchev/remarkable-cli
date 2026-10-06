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
	"strings"
	"testing"

	cloud "github.com/alexgorbatchev/go-remarkable-cloud"
)

func setupMockServer(t *testing.T) (*httptest.Server, *cloud.Client) {
	t.Helper()
	return setupMockServerWithFailure(t, "")
}

func setupMockServerWithFailure(t *testing.T, failedPath string) (*httptest.Server, *cloud.Client) {
	t.Helper()

	pdfData, err := os.ReadFile("testdata/linked_pages.pdf")
	if err != nil {
		t.Fatalf("reading pdf test fixture: %v", err)
	}
	rmData, err := os.ReadFile("testdata/oct1_notes_strokes.rm")
	if err != nil {
		t.Fatalf("reading rm test fixture: %v", err)
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == failedPath {
			http.Error(w, "injected cloud failure", http.StatusBadGateway)
			return
		}
		switch r.URL.Path {
		case "/token/v2/user":
			w.Write([]byte("mock-token"))
		case "/sync/v3/root":
			w.Write([]byte(`{"hash":"root-hash","generation":1,"schemaVersion":4}`))
		case "/sync/v3/files/root-hash":
			// Seven items plus the schema v4 aggregate record.
			w.Write([]byte("4\n0:.:75:211191896\nfolder-hash:folder-1:0:10\ndoc-hash:doc-1:0:100\nnopdf-hash:doc-nopdf:0:100\nnostroke-hash:doc-nostroke:0:100\nsuffix-hash:doc-suffix:0:100\nnocontent-hash:doc-nocontent:0:100\ncorrupt-hash:doc-corrupt:0:100\n"))
		case "/sync/v3/files/nocontent-hash":
			w.Write([]byte("nocontent-meta:doc-nocontent.metadata:0:50\npdf-hash:doc-nocontent.pdf:0:1000\n"))
		case "/sync/v3/files/nocontent-meta":
			w.Write([]byte(`{"visibleName":"No Content Doc","type":"DocumentType"}`))
		case "/sync/v3/files/suffix-hash":
			w.Write([]byte("suffix-meta:doc-suffix.metadata:0:50\nsuffix-content:doc-suffix.content:0:50\npdf-hash:other_name.pdf:0:1000\nstroke-hash:random_page.rm:0:200\n"))
		case "/sync/v3/files/suffix-meta":
			w.Write([]byte(`{"visibleName":"Suffix Doc","type":"DocumentType"}`))
		case "/sync/v3/files/suffix-content":
			w.Write([]byte(`{"fileType":"pdf","pageCount":1,"pages":["random_page"]}`))
		case "/sync/v3/files/nostroke-hash":
			w.Write([]byte("nostroke-meta:doc-nostroke.metadata:0:50\nnostroke-content:doc-nostroke.content:0:50\npdf-hash:doc-nostroke.pdf:0:1000\n"))
		case "/sync/v3/files/nostroke-meta":
			w.Write([]byte(`{"visibleName":"No Stroke Doc","type":"DocumentType"}`))
		case "/sync/v3/files/nostroke-content":
			w.Write([]byte(`{"fileType":"pdf","pageCount":1,"pages":["page-nostroke-1"]}`))
		case "/sync/v3/files/folder-hash":
			w.Write([]byte("folder-meta-hash:folder-1.metadata:0:50\n"))
		case "/sync/v3/files/folder-meta-hash":
			w.Write([]byte(`{"visibleName":"My Folder","type":"CollectionType"}`))
		case "/sync/v3/files/doc-hash":
			w.Write([]byte("meta-hash:doc-1.metadata:0:50\ncontent-hash:doc-1.content:0:100\npdf-hash:doc-1.pdf:0:1000\nstroke-hash:doc-1/page-uuid-1.rm:0:200\n"))
		case "/sync/v3/files/meta-hash":
			w.Write([]byte(`{"visibleName":"My Document","type":"DocumentType","lastModified":"2026-09-29T10:00:00Z","parent":"folder-1"}`))
		case "/sync/v3/files/content-hash":
			w.Write([]byte(`{"fileType":"pdf","pageCount":1,"pages":["page-uuid-1"]}`))
		case "/sync/v3/files/nopdf-hash":
			w.Write([]byte("nopdf-meta:doc-nopdf.metadata:0:50\nnopdf-content:doc-nopdf.content:0:50\n"))
		case "/sync/v3/files/nopdf-meta":
			w.Write([]byte(`{"visibleName":"Empty Doc","type":"DocumentType"}`))
		case "/sync/v3/files/nopdf-content":
			w.Write([]byte(`{"fileType":"notebook","pageCount":1,"cPages":{"pages":[{"id":"p1"}]}}`))
		case "/sync/v3/files/corrupt-hash":
			w.Write([]byte("corrupt-meta:doc-corrupt.metadata:0:50\ncorrupt-pdf:doc-corrupt.pdf:0:50\n"))
		case "/sync/v3/files/corrupt-meta":
			w.Write([]byte(`{"visibleName":"Corrupt Doc","type":"DocumentType"}`))
		case "/sync/v3/files/corrupt-pdf":
			w.Write([]byte("not-a-valid-pdf"))
		case "/sync/v3/files/pdf-hash":
			w.Write(pdfData)
		case "/sync/v3/files/stroke-hash":
			w.Write(rmData)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	client, err := cloud.NewClient(
		cloud.WithConfig(&cloud.Config{DeviceToken: "dev", UserToken: "usr"}),
		cloud.WithEndpoints(&cloud.Endpoints{WebappHost: ts.URL, StorageHost: ts.URL}),
	)
	if err != nil {
		t.Fatalf("creating cloud client: %v", err)
	}

	return ts, client
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

func TestDocService_Comprehensive(t *testing.T) {
	ts, client := setupMockServer(t)
	defer ts.Close()

	ctx := context.Background()

	// 1. List with various filters
	items, err := List(ctx, client, "", "", "", 10)
	if err != nil || len(items) != 7 {
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

	// 4. SearchDocument
	matches, err := SearchDocument(ctx, client, "doc-1", SearchQuery{Text: "2026"})
	if err != nil || len(matches) == 0 {
		t.Fatalf("SearchDocument failed: %v, matches: %+v", err, matches)
	}
	if matches[0].PageIndex != 0 {
		t.Errorf("expected page 0 match, got %d", matches[0].PageIndex)
	}

	// Search matching deeper text (exercising start > 0)
	_, _ = SearchDocument(ctx, client, "doc-1", SearchQuery{Text: "Priority"})
	_, _ = SearchDocument(ctx, client, "doc-1", SearchQuery{Text: "Notes"})

	// A blank query would otherwise match every page of the searchable doc-1.
	for _, blank := range []string{"", " \r\n\t"} {
		blankMatches, err := SearchDocument(ctx, client, "doc-1", SearchQuery{Text: blank})
		if !errors.Is(err, errBlankSearchQuery) {
			t.Errorf("SearchDocument(%q) = %+v, %v; want blank-query error", blank, blankMatches, err)
		}
	}

	// Search on document with no PDF
	_, errNoPDFSearch := SearchDocument(ctx, client, "doc-nopdf", SearchQuery{Text: "query"})
	if errNoPDFSearch == nil {
		t.Error("expected error searching doc with no PDF")
	}

	// Search on corrupt PDF
	_, errCorrupt := SearchDocument(ctx, client, "doc-corrupt", SearchQuery{Text: "query"})
	if errCorrupt == nil {
		t.Error("expected error on corrupt PDF")
	}

	// 5. GetLinks
	links, err := GetLinks(ctx, client, "doc-1", 0)
	if err != nil || len(links) == 0 {
		t.Fatalf("GetLinks failed: %v, links: %+v", err, links)
	}
	if links[0].TargetPage < 0 {
		t.Errorf("expected positive target page, got %d", links[0].TargetPage)
	}

	// GetLinks on a notebook with no PDF names the missing PDF.
	noPDFLinks, errNoPDFLinks := GetLinks(ctx, client, "doc-nopdf", 0)
	if want := `document "doc-nopdf" has no background PDF`; errNoPDFLinks == nil || errNoPDFLinks.Error() != want {
		t.Errorf("GetLinks on notebook without PDF = %+v, %v; want error %q", noPDFLinks, errNoPDFLinks, want)
	}

	// Out of bounds links
	_, errOOB := GetLinks(ctx, client, "doc-1", 999)
	if errOOB == nil {
		t.Error("expected error for out of bounds links")
	}
	_, errNegLinks := GetLinks(ctx, client, "doc-1", -1)
	if errNegLinks == nil {
		t.Error("expected error for negative links")
	}

	// 6. Cat formats: pdf, text, rm, svg
	var bufPDF bytes.Buffer
	if err := Cat(ctx, client, "doc-1", 0, "pdf", &bufPDF); err != nil || bufPDF.Len() == 0 {
		t.Fatalf("Cat pdf failed: %v", err)
	}

	// Suffix matching in Cat
	_ = Cat(ctx, client, "doc-suffix", 0, "pdf", &bufPDF)

	var bufText bytes.Buffer
	if err := Cat(ctx, client, "doc-1", 0, "text", &bufText); err != nil || bufText.Len() == 0 {
		t.Fatalf("Cat text failed: %v", err)
	}

	var bufRM bytes.Buffer
	if err := Cat(ctx, client, "doc-1", 0, "rm", &bufRM); err != nil || bufRM.Len() == 0 {
		t.Fatalf("Cat rm failed: %v", err)
	}
	_ = Cat(ctx, client, "doc-suffix", 0, "rm", &bufRM)

	var bufSVG bytes.Buffer
	if err := Cat(ctx, client, "doc-1", 0, "svg", &bufSVG); err != nil || !strings.Contains(bufSVG.String(), "<svg") {
		t.Fatalf("Cat svg failed: %v", err)
	}

	// Cat errors
	if err := Cat(ctx, client, "doc-1", 999, "rm", &bufRM); err == nil {
		t.Error("expected error on out of bounds cat")
	}
	if err := Cat(ctx, client, "doc-1", -1, "rm", &bufRM); err == nil {
		t.Error("expected error on negative page cat")
	}
	if err := Cat(ctx, client, "doc-1", 0, "unsupported-format", &bufRM); err == nil {
		t.Error("expected error on unsupported format")
	}
	if err := Cat(ctx, client, "doc-nopdf", 0, "pdf", &bufPDF); err == nil {
		t.Error("expected error on doc with no pdf")
	}
	if err := Cat(ctx, client, "doc-nopdf", 0, "text", &bufText); err == nil {
		t.Error("expected error on doc with no pdf text")
	}
	if err := Cat(ctx, client, "doc-nostroke", 0, "rm", &bufRM); err == nil {
		t.Error("expected error on doc with no strokes")
	}
	_ = Cat(ctx, client, "doc-nostroke", 0, "svg", &bufSVG)
	_ = Cat(ctx, client, "doc-nocontent", 0, "rm", &bufRM)
	_ = Cat(ctx, client, "doc-corrupt", 0, "text", &bufText)
	_, _ = GetLinks(ctx, client, "doc-corrupt", 0)

	// 7. RenderPage (with PDF, without PDF, and without strokes)
	tmpDir := t.TempDir()
	outPNG := filepath.Join(tmpDir, "rendered.png")
	if err := RenderPage(ctx, client, "doc-1", 0, 0, outPNG); err != nil {
		t.Fatalf("RenderPage failed: %v", err)
	}
	if info, err := os.Stat(outPNG); err != nil || info.Size() == 0 {
		t.Fatalf("expected non-empty output PNG: %v", err)
	}

	// Render without strokes
	outNoStroke := filepath.Join(tmpDir, "rendered-nostroke.png")
	if err := RenderPage(ctx, client, "doc-nostroke", 0, 200, outNoStroke); err != nil {
		t.Fatalf("RenderPage nostroke failed: %v", err)
	}
	_ = RenderPage(ctx, client, "doc-1", 0, 150, outPNG)
	_ = RenderPage(ctx, client, "doc-suffix", 0, 200, outPNG)
	_ = RenderPage(ctx, client, "doc-corrupt", 0, 200, outPNG)
	_ = RenderPage(ctx, client, "doc-nocontent", 0, 200, outPNG)

	// Render without PDF background should return error
	outNoPDF := filepath.Join(tmpDir, "rendered-nopdf.png")
	if err := RenderPage(ctx, client, "doc-nopdf", 0, 200, outNoPDF); err == nil {
		t.Error("expected error rendering doc with no pdf background")
	}

	// RenderPage OOB and negative
	if err := RenderPage(ctx, client, "doc-1", 999, 200, outPNG); err == nil {
		t.Error("expected error on out of bounds render")
	}
	if err := RenderPage(ctx, client, "doc-1", -1, 200, outPNG); err == nil {
		t.Error("expected error on negative page render")
	}

	// 8. SyncDocument (png, svg, rm) with explicit pages and defaults
	syncDir := filepath.Join(tmpDir, "synced")
	resultsPNG, err := SyncDocument(ctx, client, "doc-1", SyncDocOptions{
		OutputDir: syncDir,
		Format:    "png",
		Force:     false,
		Pages:     []int{0},
	})
	if err != nil || len(resultsPNG) != 1 || resultsPNG[0].State != "written" {
		t.Fatalf("SyncDocument PNG failed: %v, %+v", err, resultsPNG)
	}

	// Second run should be skipped
	resultsSkipped, err := SyncDocument(ctx, client, "doc-1", SyncDocOptions{
		OutputDir: syncDir,
		Format:    "png",
		Force:     false,
	})
	if err != nil || len(resultsSkipped) != 1 || resultsSkipped[0].State != "skipped" {
		t.Fatalf("expected skipped page, got: %+v", resultsSkipped)
	}

	// Sync with default options (empty OutputDir, empty Format)
	resultsDefault, err := SyncDocument(ctx, client, "doc-1", SyncDocOptions{
		OutputDir: "",
		Format:    "",
		Force:     true,
	})
	if err != nil || len(resultsDefault) != 1 {
		t.Fatalf("SyncDocument default failed: %v", err)
	}
	_ = os.RemoveAll("My Document") // cleanup default output

	// Sync SVG
	resultsSVG, err := SyncDocument(ctx, client, "doc-1", SyncDocOptions{
		OutputDir: syncDir,
		Format:    "svg",
		Force:     true,
	})
	if err != nil || len(resultsSVG) != 1 {
		t.Fatalf("SyncDocument SVG failed: %v", err)
	}

	// Sync RM
	resultsRM, err := SyncDocument(ctx, client, "doc-1", SyncDocOptions{
		OutputDir: syncDir,
		Format:    "rm",
		Force:     true,
	})
	if err != nil || len(resultsRM) != 1 {
		t.Fatalf("SyncDocument RM failed: %v", err)
	}

	// Sync doc-nostroke and invalid page indices
	_, _ = SyncDocument(ctx, client, "doc-nostroke", SyncDocOptions{
		OutputDir: syncDir,
		Format:    "png",
		Force:     true,
	})
	_, _ = SyncDocument(ctx, client, "doc-nostroke", SyncDocOptions{
		OutputDir: syncDir,
		Format:    "svg",
		Force:     true,
	})
	_, _ = SyncDocument(ctx, client, "doc-nostroke", SyncDocOptions{
		OutputDir: syncDir,
		Format:    "rm",
		Force:     true,
	})
	_, _ = SyncDocument(ctx, client, "doc-corrupt", SyncDocOptions{
		OutputDir: syncDir,
		Format:    "png",
		Force:     true,
	})
	_, _ = SyncDocument(ctx, client, "doc-1", SyncDocOptions{
		OutputDir: syncDir,
		Format:    "png",
		Pages:     []int{-1, 0, 999},
		Force:     true,
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
