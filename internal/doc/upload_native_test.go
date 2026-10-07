package doc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	cloud "github.com/alexgorbatchev/go-remarkable-cloud"
)

// tabletPageOrderFixture is the idx sequence of an 816-page PDF initialized by
// reMarkable software; the 523-, 589-, and 731-page planners it was compared
// with use the same sequence's leading values.
func tabletPageOrderFixture(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile("testdata/tablet_page_order.txt")
	if err != nil {
		t.Fatal(err)
	}
	return strings.Fields(string(data))
}

func TestNativePageOrderMatchesTabletInitialization(t *testing.T) {
	tablet := tabletPageOrderFixture(t)
	for _, pages := range []int{1, 2, 325, 326, 327, 589, 638, 639, 640, len(tablet)} {
		t.Run(fmt.Sprint(pages), func(t *testing.T) {
			order, err := nativePageOrder(pages)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(order, tablet[:pages]) {
				t.Fatalf("page order differs from tablet initialization:\n got %v\nwant %v", order, tablet[:pages])
			}
		})
	}
	for _, pages := range []int{0, len(tablet) + 1} {
		if _, err := nativePageOrder(pages); err == nil || !strings.Contains(err.Error(), "816") {
			t.Fatalf("nativePageOrder(%d) = %v, want an unsupported page count error", pages, err)
		}
	}
}

// addDocument stores a complete document under the cloud's file-hash manifest
// scheme and adds it to the current root.
func (s *uploadServer) addDocument(id string, files map[string][]byte) {
	docHash := settingsManifest(s.blobs, files)
	root := fmt.Appendf(bytes.Clone(s.blobs[s.root.Hash]), "%s:0:%s:%d:100\n", docHash, id, len(files))
	s.root.Hash = archiveTestHash(root)
	s.blobs[s.root.Hash] = root
}

func (s *uploadServer) freshFile(t *testing.T, c *cloud.Client, id, name string) []byte {
	t.Helper()
	ctx := context.Background()
	root, err := freshUploadManifest(ctx, c, s.root.Hash, "root.docSchema")
	if err != nil {
		t.Fatal(err)
	}
	entry := root.Find(id)
	if entry == nil {
		t.Fatalf("document %s is missing", id)
	}
	manifest, err := freshUploadManifest(ctx, c, entry.Hash, id+".docSchema")
	if err != nil {
		t.Fatal(err)
	}
	file := manifest.Find(name)
	if file == nil {
		t.Fatalf("file %s is missing", name)
	}
	data, err := c.GetBlobFresh(ctx, file.Hash, file.ID)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

type initializedContent struct {
	FormatVersion int             `json:"formatVersion"`
	PageCount     int             `json:"pageCount"`
	CPages        json.RawMessage `json:"cPages"`
	PageTags      []struct {
		Name   string `json:"name"`
		PageID string `json:"pageId"`
	} `json:"pageTags"`
	ZoomMode string `json:"zoomMode"`
}

type initializedCPages struct {
	Original struct {
		Timestamp string `json:"timestamp"`
		Value     int    `json:"value"`
	} `json:"original"`
	Pages []struct {
		ID  string `json:"id"`
		Idx struct {
			Timestamp string `json:"timestamp"`
			Value     string `json:"value"`
		} `json:"idx"`
		Redir struct {
			Timestamp string `json:"timestamp"`
			Value     int    `json:"value"`
		} `json:"redir"`
		Template struct {
			Timestamp string `json:"timestamp"`
			Value     string `json:"value"`
		} `json:"template"`
	} `json:"pages"`
	UUIDs []struct {
		First  string `json:"first"`
		Second int    `json:"second"`
	} `json:"uuids"`
}

func TestUploadInitializedPagesThenImportAndTransferSettings(t *testing.T) {
	dir := t.TempDir()
	evidencePath := filepath.Join(dir, "upload.json")
	s, c := newUploadServer(t, "", evidencePath)
	sourcePDF := []byte("source PDF bytes")
	sourceContent := settingsContent("cPages", "s", true)
	s.addDocument(settingsSourceID, map[string][]byte{settingsSourceID + ".content": sourceContent, settingsSourceID + ".metadata": []byte(`{"type":"DocumentType","visibleName":"Source"}`), settingsSourceID + ".pdf": sourcePDF})
	pdf, err := os.ReadFile("testdata/linked_pages.pdf")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	evidence, err := UploadPDF(ctx, c, "testdata/linked_pages.pdf", UploadOptions{Title: "Planner", Evidence: evidencePath, InitializePages: true})
	if err != nil || evidence.Result.State != cloud.UpdateVerified {
		t.Fatalf("initialized upload: %+v, %v", evidence, err)
	}
	if evidence.NativePages != "initialized" || evidence.Pages < 2 || len(evidence.PageIDs) != evidence.Pages {
		t.Fatalf("missing native page identity: %+v", evidence)
	}
	seen := make(map[string]bool)
	for _, id := range evidence.PageIDs {
		if !isUUID(id) || seen[id] || id == evidence.Result.ID {
			t.Fatalf("page IDs are not unique UUIDs: %v", evidence.PageIDs)
		}
		seen[id] = true
	}
	id := evidence.Result.ID
	if !bytes.Equal(s.freshFile(t, c, id, id+".pdf"), pdf) {
		t.Fatal("PDF bytes or links changed")
	}
	if got := s.freshFile(t, c, id, id+".pagedata"); string(got) != strings.Repeat("Blank\n", evidence.Pages) {
		t.Fatalf("pagedata = %q, want one Blank template per page", got)
	}
	rawContent := s.freshFile(t, c, id, id+".content")
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(rawContent, &fields); err != nil {
		t.Fatal(err)
	}
	for _, legacy := range []string{"pages", "originalPageCount"} {
		if _, ok := fields[legacy]; ok {
			t.Fatalf("initialized content keeps format 1 field %q: %s", legacy, rawContent)
		}
	}
	var content initializedContent
	if err := json.Unmarshal(rawContent, &content); err != nil {
		t.Fatal(err)
	}
	var cPages initializedCPages
	if err := json.Unmarshal(content.CPages, &cPages); err != nil {
		t.Fatal(err)
	}
	tablet := tabletPageOrderFixture(t)
	if content.FormatVersion != 2 || content.PageCount != evidence.Pages || cPages.Original.Value != evidence.Pages || cPages.Original.Timestamp != "1:1" || len(cPages.Pages) != evidence.Pages {
		t.Fatalf("wrong initialized document structure: %s", rawContent)
	}
	if len(cPages.UUIDs) != 1 || !isUUID(cPages.UUIDs[0].First) || cPages.UUIDs[0].Second != 1 || seen[cPages.UUIDs[0].First] {
		t.Fatalf("wrong native author table: %+v", cPages.UUIDs)
	}
	for i, page := range cPages.Pages {
		if page.ID != evidence.PageIDs[i] || page.Idx.Value != tablet[i] || page.Idx.Timestamp != "1:2" || page.Redir.Value != i || page.Redir.Timestamp != "1:2" || page.Template.Value != "Blank" || page.Template.Timestamp != "1:2" {
			t.Fatalf("page %d has wrong native association: %+v", i, page)
		}
	}
	if _, err := CheckUpload(ctx, c, evidencePath); err != nil {
		t.Fatalf("fresh recovery check of initialized upload: %v", err)
	}

	details, err := Inspect(ctx, c, id, true)
	if err != nil {
		t.Fatalf("inspect --pages immediately after upload: %v", err)
	}
	var inspected []string
	for _, page := range details.PageList {
		inspected = append(inspected, page.ID)
	}
	if details.Pages != evidence.Pages || !slices.Equal(inspected, evidence.PageIDs) {
		t.Fatalf("inspect pages = %d %v, want %v", details.Pages, inspected, evidence.PageIDs)
	}

	ink, err := os.ReadFile("testdata/oct1_notes_strokes.rm")
	if err != nil {
		t.Fatal(err)
	}
	imported, err := ImportStrokes(ctx, c, id, []PageImport{{Source: "testdata/oct1_notes_strokes.rm", Page: 1}})
	if err != nil || imported.State != cloud.UpdateVerified || len(imported.Pages) != 1 || imported.Pages[0].PageID != evidence.PageIDs[1] {
		t.Fatalf("import immediately after upload: %+v, %v", imported, err)
	}
	if !bytes.Equal(s.freshFile(t, c, id, id+"/"+evidence.PageIDs[1]+".rm"), ink) {
		t.Fatal("imported native bytes differ")
	}

	settings, err := TransferSettings(ctx, c, settingsSourceID, id, SettingsOptions{Mapping: []SettingsPageMap{{SourcePage: 0, DestinationPage: 1}}, ReplaceViewport: true})
	if err != nil || settings.State != cloud.UpdateVerified {
		t.Fatalf("settings transfer immediately after upload: %+v, %v", settings, err)
	}
	var transferred initializedContent
	if err := json.Unmarshal(s.freshFile(t, c, id, id+".content"), &transferred); err != nil {
		t.Fatal(err)
	}
	if len(transferred.PageTags) != 1 || transferred.PageTags[0].PageID != evidence.PageIDs[1] || transferred.ZoomMode != "customFit" || !bytes.Equal(transferred.CPages, content.CPages) {
		t.Fatalf("settings transfer lost page association or structure: %+v", transferred)
	}
	if !bytes.Equal(s.freshFile(t, c, id, id+".pdf"), pdf) || !bytes.Equal(s.freshFile(t, c, id, id+"/"+evidence.PageIDs[1]+".rm"), ink) || !bytes.Equal(s.freshFile(t, c, settingsSourceID, settingsSourceID+".content"), sourceContent) {
		t.Fatal("PDF, imported strokes, or source content changed")
	}
	after, err := Inspect(ctx, c, id, true)
	if err != nil {
		t.Fatal(err)
	}
	for i, page := range after.PageList {
		if page.ID != evidence.PageIDs[i] || page.HasStrokes != (i == 1) {
			t.Fatalf("page %d after import and transfer: %+v", i, page)
		}
	}
}

// pdfWithPages builds a minimal valid PDF with blank pages.
func pdfWithPages(pages int) []byte {
	var buf bytes.Buffer
	var offsets []int
	object := func(body string) {
		offsets = append(offsets, buf.Len())
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", len(offsets), body)
	}
	buf.WriteString("%PDF-1.4\n")
	object("<< /Type /Catalog /Pages 2 0 R >>")
	var kids strings.Builder
	for i := range pages {
		fmt.Fprintf(&kids, "%d 0 R ", i+3)
	}
	object(fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", kids.String(), pages))
	for range pages {
		object("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>")
	}
	start := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n0000000000 65535 f \n", len(offsets)+1)
	for _, offset := range offsets {
		fmt.Fprintf(&buf, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets)+1, start)
	return buf.Bytes()
}

func TestUploadInitializePagesRejectsUnsupportedPageCountBeforeWrites(t *testing.T) {
	dir := t.TempDir()
	evidencePath := filepath.Join(dir, "upload.json")
	s, c := newUploadServer(t, "", evidencePath)
	tablet := tabletPageOrderFixture(t)
	for _, pages := range []int{len(tablet), len(tablet) + 1} {
		t.Run(fmt.Sprint(pages), func(t *testing.T) {
			path := filepath.Join(dir, fmt.Sprintf("pages-%d.pdf", pages))
			if err := os.WriteFile(path, pdfWithPages(pages), 0600); err != nil {
				t.Fatal(err)
			}
			evidencePath := filepath.Join(dir, fmt.Sprintf("upload-%d.json", pages))
			s.evidence = evidencePath
			writes := s.writes
			evidence, err := UploadPDF(context.Background(), c, path, UploadOptions{Title: fmt.Sprintf("Planner %d", pages), Evidence: evidencePath, InitializePages: true})
			if pages == len(tablet) {
				if err != nil || len(evidence.PageIDs) != pages {
					t.Fatalf("largest supported initialized upload: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), "816") || evidence != nil {
				t.Fatalf("unsupported page count = %+v, %v", evidence, err)
			}
			if s.writes != writes {
				t.Fatalf("unsupported page count wrote cloud data: %d", s.writes-writes)
			}
			if _, err := os.Stat(evidencePath); !os.IsNotExist(err) {
				t.Fatalf("unsupported page count created evidence: %v", err)
			}
			if _, err := UploadPDF(context.Background(), c, path, UploadOptions{Title: "Ordinary large PDF", Evidence: evidencePath}); err != nil {
				t.Fatalf("ordinary upload of a large PDF: %v", err)
			}
		})
	}
}

func TestReadUploadEvidenceValidatesNativePageIdentity(t *testing.T) {
	const id = "11111111-1111-4111-8111-111111111111"
	const pageA = "22222222-2222-4222-8222-222222222222"
	const pageB = "33333333-3333-4333-8333-333333333333"
	for name, change := range map[string]func(*UploadEvidence){
		"valid initialized": func(*UploadEvidence) {},
		"valid pending":     func(e *UploadEvidence) { e.NativePages = nativePagesPending; e.PageIDs = nil },
		"unknown state":     func(e *UploadEvidence) { e.NativePages = "partial" },
		"pending with IDs":  func(e *UploadEvidence) { e.NativePages = nativePagesPending },
		"missing IDs":       func(e *UploadEvidence) { e.PageIDs = e.PageIDs[:1] },
		"duplicate IDs":     func(e *UploadEvidence) { e.PageIDs = []string{pageA, pageA} },
		"invalid ID":        func(e *UploadEvidence) { e.PageIDs = []string{pageA, "page"} },
		"document ID":       func(e *UploadEvidence) { e.PageIDs = []string{pageA, id} },
	} {
		t.Run(name, func(t *testing.T) {
			evidence := UploadEvidence{Version: 1, Pages: 2, NativePages: NativePagesInitialized, PageIDs: []string{pageA, pageB}, Result: cloud.CreateResult{ID: id, DocumentHash: strings.Repeat("a", 64)}}
			for _, suffix := range []string{".pdf", ".metadata", ".content", ".pagedata"} {
				evidence.Files = append(evidence.Files, UploadFile{Name: id + suffix, SHA256: strings.Repeat("b", 64), Size: 1})
			}
			change(&evidence)
			data, err := json.Marshal(evidence)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "upload.json")
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			_, err = ReadUploadEvidence(path)
			if strings.HasPrefix(name, "valid") != (err == nil) {
				t.Fatalf("ReadUploadEvidence = %v", err)
			}
		})
	}
}

func TestCheckUploadRejectsEvidencePageIDsDifferingFromCloud(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upload.json")
	_, c := newUploadServer(t, "", path)
	evidence, err := UploadPDF(context.Background(), c, "testdata/linked_pages.pdf", UploadOptions{Title: "Planner", Evidence: path, InitializePages: true})
	if err != nil {
		t.Fatal(err)
	}
	evidence.PageIDs[0], evidence.PageIDs[1] = evidence.PageIDs[1], evidence.PageIDs[0]
	if err := writeUploadEvidence(path, evidence, false); err != nil {
		t.Fatal(err)
	}
	if _, err := CheckUpload(context.Background(), c, path); err == nil || !strings.Contains(err.Error(), "native page identity") {
		t.Fatalf("recovery accepted page IDs that differ from the cloud: %v", err)
	}
}
