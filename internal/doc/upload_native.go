package doc

import (
	"encoding/json"
	"fmt"
	"strings"

	cloud "github.com/alexgorbatchev/go-remarkable-cloud"
)

// NativePagesInitialized is the native_pages state of an upload that created
// the native page structure of every PDF page with the document.
const NativePagesInitialized = "initialized"

// nativePageTemplate is the template every page of a tablet-initialized PDF
// records, both in .content and in .pagedata.
const nativePageTemplate = "Blank"

// Native page registers are last-writer-wins values stamped "author:counter".
// Every tablet-initialized PDF records the original page count at counter 1
// and each page's order, PDF redirection, and template at counter 2, all from
// the author that created the page structure.
const (
	nativeAuthorNumber      = 1
	nativeOriginalTimestamp = "1:1"
	nativePageTimestamp     = "1:2"
)

// tabletPageOrder reproduces the idx values reMarkable software assigns when it
// initializes a PDF: runs of two-letter values from "ba", each later run behind
// a longer prefix and introduced by one marker value. PDFs of 523, 589, 731,
// and 816 pages initialized by the tablet and the Android app all use leading
// values of this one sequence (testdata/tablet_page_order.txt). The markers,
// prefixes, and counts are fitted to that observed data, not derived from a
// known rule: the third run's count stops at the last observed value, and the
// tablet's values beyond it are unknown. Extend a run or add one only with new
// tablet-produced evidence; longer PDFs are rejected rather than extrapolated.
var tabletPageOrder = []struct {
	marker string
	prefix string
	count  int
}{
	{prefix: "", count: 325},                      // "ba" .. "nm"
	{marker: "nna", prefix: "nn", count: 312},     // "nnba" .. "nnmz"
	{marker: "nnnaa", prefix: "nnna", count: 177}, // "nnnaba" .. "nnnahu"
}

// maxInitializedPages is the longest page order observed in tablet output.
var maxInitializedPages = func() int {
	total := 0
	for _, run := range tabletPageOrder {
		if run.marker != "" {
			total++
		}
		total += run.count
	}
	return total
}()

// nativePageOrder returns the tablet's idx values for the first pages PDF pages.
func nativePageOrder(pages int) ([]string, error) {
	if err := validateInitializedPageCount(pages); err != nil {
		return nil, err
	}
	const letters = "abcdefghijklmnopqrstuvwxyz"
	order := make([]string, 0, maxInitializedPages)
	for _, run := range tabletPageOrder {
		if run.marker != "" {
			order = append(order, run.marker)
		}
		// Each run counts in base 26 from 26 ("ba"), leaving one-letter values free.
		for value := len(letters); value < len(letters)+run.count; value++ {
			order = append(order, run.prefix+string(letters[value/len(letters)])+string(letters[value%len(letters)]))
		}
	}
	return order[:pages], nil
}

func validateInitializedPageCount(pages int) error {
	if pages < 1 || pages > maxInitializedPages {
		return fmt.Errorf("--initialize-pages supports PDFs of 1 to %d pages, the longest native page order verified from tablet-initialized documents; this PDF has %d", maxInitializedPages, pages)
	}
	return nil
}

type nativeRegister[T any] struct {
	Timestamp string `json:"timestamp"`
	Value     T      `json:"value"`
}

type nativePage struct {
	ID       string                 `json:"id"`
	Idx      nativeRegister[string] `json:"idx"`
	Redir    nativeRegister[int]    `json:"redir"`
	Template nativeRegister[string] `json:"template"`
}

type nativeAuthor struct {
	UUID   string `json:"first"`
	Number int    `json:"second"`
}

// nativePageStructure is the cPages object of a never-opened PDF: no
// lastOpened register, and no per-page scroll or modification registers,
// which the tablet adds when the document is opened and edited.
type nativePageStructure struct {
	Original nativeRegister[int] `json:"original"`
	Pages    []nativePage        `json:"pages"`
	Authors  []nativeAuthor      `json:"uuids"`
}

// newNativePageStructure assigns page i of the PDF to native page pageIDs[i],
// in PDF order, with author recorded as the writer of every register.
func newNativePageStructure(pageIDs []string, author string) (*nativePageStructure, error) {
	order, err := nativePageOrder(len(pageIDs))
	if err != nil {
		return nil, err
	}
	structure := &nativePageStructure{
		Original: nativeRegister[int]{Timestamp: nativeOriginalTimestamp, Value: len(pageIDs)},
		Pages:    make([]nativePage, 0, len(pageIDs)),
		Authors:  []nativeAuthor{{UUID: author, Number: nativeAuthorNumber}},
	}
	for i, id := range pageIDs {
		structure.Pages = append(structure.Pages, nativePage{
			ID:       id,
			Idx:      nativeRegister[string]{Timestamp: nativePageTimestamp, Value: order[i]},
			Redir:    nativeRegister[int]{Timestamp: nativePageTimestamp, Value: i},
			Template: nativeRegister[string]{Timestamp: nativePageTimestamp, Value: nativePageTemplate},
		})
	}
	return structure, nil
}

// newNativePageIDs generates the document's native page IDs and the author
// UUID that the uploaded page structure records for its registers.
func newNativePageIDs(pages int) (pageIDs []string, author string, err error) {
	if err := validateInitializedPageCount(pages); err != nil {
		return nil, "", err
	}
	pageIDs = make([]string, 0, pages)
	for range pages {
		id, err := uploadUUID("native page")
		if err != nil {
			return nil, "", err
		}
		pageIDs = append(pageIDs, id)
	}
	author, err = uploadUUID("native page author")
	if err != nil {
		return nil, "", err
	}
	return pageIDs, author, nil
}

// initializeContent converts the ordinary format 1 PDF content into the format
// 2 schema the tablet writes when it initializes a PDF: cPages replaces the
// legacy pages list, and cPages.original replaces originalPageCount.
func initializeContent(content map[string]any, pageIDs []string, author string) error {
	structure, err := newNativePageStructure(pageIDs, author)
	if err != nil {
		return err
	}
	content["formatVersion"] = 2
	content["cPages"] = structure
	delete(content, "pages")
	delete(content, "originalPageCount")
	return nil
}

func uploadPageData(pages int, initialize bool) []byte {
	if initialize {
		return []byte(strings.Repeat(nativePageTemplate+"\n", pages))
	}
	return []byte(strings.Repeat("\n", pages))
}

// validateEvidencePageIDs checks the recorded native state before any recovery
// read: initialized evidence names one unique page UUID per PDF page.
func validateEvidencePageIDs(evidence *UploadEvidence) error {
	switch evidence.NativePages {
	case nativePagesPending:
		if len(evidence.PageIDs) != 0 {
			return fmt.Errorf("upload evidence records page IDs for %s pages", nativePagesPending)
		}
		return nil
	case NativePagesInitialized:
	default:
		return fmt.Errorf("upload evidence has unknown native_pages %q", evidence.NativePages)
	}
	if len(evidence.PageIDs) != evidence.Pages {
		return fmt.Errorf("upload evidence records %d native page IDs for %d pages", len(evidence.PageIDs), evidence.Pages)
	}
	seen := map[string]bool{evidence.Result.ID: true}
	for i, id := range evidence.PageIDs {
		if !isUUID(id) || seen[id] {
			return fmt.Errorf("upload evidence native page %d has a missing, duplicate, or invalid UUID", i)
		}
		seen[id] = true
	}
	return nil
}

// checkUploadedPageIdentity compares freshly downloaded content with the page
// IDs recovery evidence recorded before the commit.
func checkUploadedPageIdentity(evidence *UploadEvidence, data []byte) error {
	var content cloud.DocumentContent
	if err := json.Unmarshal(data, &content); err != nil {
		return fmt.Errorf("decoding uploaded content: %w", err)
	}
	pages := content.CPages.Pages
	if len(content.Pages) != 0 || content.PageCount != evidence.Pages || len(pages) != len(evidence.PageIDs) {
		return fmt.Errorf("upload recovery native page identity differs: %d native pages for %d recorded page IDs", len(pages), len(evidence.PageIDs))
	}
	for i, page := range pages {
		if page.ID != evidence.PageIDs[i] || page.Redir == nil || page.Redir.Value != i || page.Deleted != nil {
			return fmt.Errorf("upload recovery native page identity differs at page %d", i)
		}
	}
	return nil
}
