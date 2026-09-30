package doc

import (
	"bytes"
	"context"
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

	pdfData, err := os.ReadFile("testdata/linked_pages.pdf")
	if err != nil {
		t.Fatalf("reading pdf test fixture: %v", err)
	}
	rmData, err := os.ReadFile("testdata/oct1_notes_strokes.rm")
	if err != nil {
		t.Fatalf("reading rm test fixture: %v", err)
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token/v2/user":
			w.Write([]byte("mock-token"))
		case "/sync/v3/root":
			w.Write([]byte(`{"hash":"root-hash","generation":1,"schemaVersion":3}`))
		case "/sync/v3/files/root-hash":
			// 6 items: folder, doc with strokes, doc without strokes, suffix doc, nocontent doc, corrupt doc
			w.Write([]byte("folder-hash:folder-1:0:10\ndoc-hash:doc-1:0:100\nnopdf-hash:doc-nopdf:0:100\nnostroke-hash:doc-nostroke:0:100\nsuffix-hash:doc-suffix:0:100\nnocontent-hash:doc-nocontent:0:100\ncorrupt-hash:doc-corrupt:0:100\n"))
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
	matches, err := SearchDocument(ctx, client, "doc-1", "Sep 28")
	if err != nil || len(matches) == 0 {
		t.Fatalf("SearchDocument failed: %v, matches: %+v", err, matches)
	}
	if matches[0].PageIndex != 0 {
		t.Errorf("expected page 0 match, got %d", matches[0].PageIndex)
	}

	// Search matching deeper text (exercising start > 0)
	_, _ = SearchDocument(ctx, client, "doc-1", "Priority")
	_, _ = SearchDocument(ctx, client, "doc-1", "Notes")

	// Search on document with no PDF
	_, errNoPDFSearch := SearchDocument(ctx, client, "doc-nopdf", "query")
	if errNoPDFSearch == nil {
		t.Error("expected error searching doc with no PDF")
	}

	// Search on corrupt PDF
	_, errCorrupt := SearchDocument(ctx, client, "doc-corrupt", "query")
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

	// GetLinks on doc with no PDF
	_, errNoPDFLinks := GetLinks(ctx, client, "doc-nopdf", 0)
	if errNoPDFLinks == nil {
		t.Error("expected error getting links on doc with no PDF")
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
	if _, err := SearchDocument(ctx, client, "nonexistent", "query"); err == nil {
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
