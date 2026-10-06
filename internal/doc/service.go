package doc

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	cloud "github.com/alexgorbatchev/go-remarkable-cloud"
	render "github.com/alexgorbatchev/go-remarkable-render"
	"github.com/alexgorbatchev/go-rmscene"
	"github.com/alexgorbatchev/remarkable-cli/internal/agent"
)

// ItemSummary represents a user-facing document or folder.
type ItemSummary struct {
	ID       string
	Name     string
	Type     string
	Modified string
	Parent   string
}

// List returns a list of cloud items optionally filtered by folder, type, or name query.
func List(ctx context.Context, client *cloud.Client, folderID, docType, query string, limit int) ([]ItemSummary, error) {
	items, err := client.ListItems(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing cloud items: %w", err)
	}

	summaries := make([]ItemSummary, 0, len(items))
	query = strings.ToLower(strings.TrimSpace(query))

	for _, it := range items {
		if folderID != "" && it.Metadata.Parent != folderID {
			continue
		}
		if docType != "" && string(it.Metadata.Type) != docType {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(it.Metadata.VisibleName), query) {
			continue
		}
		summaries = append(summaries, ItemSummary{
			ID:       it.ID,
			Name:     it.Metadata.VisibleName,
			Type:     string(it.Metadata.Type),
			Modified: it.Metadata.LastModified,
			Parent:   it.Metadata.Parent,
		})
		if limit > 0 && len(summaries) >= limit {
			break
		}
	}
	return summaries, nil
}

// BuildTree constructs a hierarchical tree of cloud folders and documents.
func BuildTree(ctx context.Context, client *cloud.Client) (*agent.TreeNode, error) {
	items, err := client.ListItems(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetching items for tree: %w", err)
	}

	byParent := make(map[string][]*cloud.Item)
	byID := make(map[string]*cloud.Item)
	for _, it := range items {
		byID[it.ID] = it
		byParent[it.Metadata.Parent] = append(byParent[it.Metadata.Parent], it)
	}

	rootNode := &agent.TreeNode{Label: "/"}

	var addChildren func(node *agent.TreeNode, parentID string)
	addChildren = func(node *agent.TreeNode, parentID string) {
		for _, child := range byParent[parentID] {
			label := child.Metadata.VisibleName
			if child.IsCollection() {
				label += "/"
			}
			childNode := &agent.TreeNode{Label: label}
			node.Children = append(node.Children, childNode)
			if child.IsCollection() {
				addChildren(childNode, child.ID)
			}
		}
	}

	addChildren(rootNode, "")
	return rootNode, nil
}

// PageDetail holds page-level metadata and stroke presence.
type PageDetail struct {
	Index       int
	ID          string
	HasStrokes  bool
	StrokeBytes int64
}

// DocDetails holds detailed information about a document.
type DocDetails struct {
	ID           string
	Name         string
	Type         string
	LastModified string
	Pages        int
	FileType     string
	Pinned       bool
	Bookmarked   bool
	PageList     []PageDetail
}

// Inspect fetches detailed metadata for a document.
func Inspect(ctx context.Context, client *cloud.Client, idOrName string, includePages bool) (*DocDetails, error) {
	item, err := client.Resolve(ctx, idOrName)
	if err != nil {
		return nil, fmt.Errorf("resolving document %q: %w", idOrName, err)
	}

	details := &DocDetails{
		ID:           item.ID,
		Name:         item.Metadata.VisibleName,
		Type:         string(item.Metadata.Type),
		LastModified: item.Metadata.LastModified,
	}

	docContent, err := item.GetContent(ctx)
	if err == nil && docContent != nil {
		pageIDs := getPageIDs(docContent)
		details.Pages = len(pageIDs)
		details.FileType = docContent.FileType

		if includePages {
			manifest, _ := item.GetManifest(ctx)
			for i, pageID := range pageIDs {
				pd := PageDetail{
					Index: i,
					ID:    pageID,
				}
				if manifest != nil {
					if entry := manifest.FindSuffix(pageID + ".rm"); entry != nil {
						pd.HasStrokes = true
						pd.StrokeBytes = entry.Size
					}
				}
				details.PageList = append(details.PageList, pd)
			}
		}
	}

	return details, nil
}

// SearchMatch represents a page matching a query.
type SearchMatch struct {
	PageIndex int
	Snippet   string
}

// SearchDocument searches text within a PDF-based document and returns matching page indices.
func SearchDocument(ctx context.Context, client *cloud.Client, idOrName, query string) ([]SearchMatch, error) {
	item, err := client.Resolve(ctx, idOrName)
	if err != nil {
		return nil, fmt.Errorf("resolving document %q: %w", idOrName, err)
	}

	manifest, err := item.GetManifest(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetching document manifest: %w", err)
	}

	pdfFile := manifest.Find(item.ID + ".pdf")
	if pdfFile == nil {
		pdfFile = manifest.FindSuffix(".pdf")
	}
	if pdfFile == nil {
		return nil, fmt.Errorf("document %q has no searchable PDF text", idOrName)
	}

	pdfBytes, err := client.GetBlob(ctx, pdfFile.Hash, item.ID+".pdf")
	if err != nil {
		return nil, fmt.Errorf("downloading template PDF: %w", err)
	}

	doc, cleanup, err := render.OpenDocumentFromBytes(pdfBytes)
	if err != nil {
		return nil, fmt.Errorf("parsing PDF: %w", err)
	}
	defer cleanup()

	queryLower := strings.ToLower(query)
	var matches []SearchMatch

	for i := 0; i < doc.NumPage(); i++ {
		text, err := doc.Text(i)
		if err != nil {
			continue
		}

		if idx := strings.Index(strings.ToLower(text), queryLower); idx != -1 {
			start := idx - 20
			if start < 0 {
				start = 0
			}
			end := idx + len(query) + 40
			if end > len(text) {
				end = len(text)
			}

			snippet := strings.ReplaceAll(text[start:end], "\n", " ")
			snippet = strings.Join(strings.Fields(snippet), " ")

			matches = append(matches, SearchMatch{
				PageIndex: i,
				Snippet:   snippet,
			})
		}
	}

	return matches, nil
}

// PageLink represents an internal or external hyperlink on a page.
type PageLink struct {
	Index      int
	TargetPage int
	URI        string
}

var pageLinkRe = regexp.MustCompile(`#page=(\d+)`)

// GetLinks extracts all hyperlinks from a specific document page.
func GetLinks(ctx context.Context, client *cloud.Client, idOrName string, pageIdx int) ([]PageLink, error) {
	item, err := client.Resolve(ctx, idOrName)
	if err != nil {
		return nil, fmt.Errorf("resolving document %q: %w", idOrName, err)
	}

	manifest, err := item.GetManifest(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetching document manifest: %w", err)
	}

	pdfFile := manifest.Find(item.ID + ".pdf")
	if pdfFile == nil {
		pdfFile = manifest.FindSuffix(".pdf")
	}
	if pdfFile == nil {
		return nil, fmt.Errorf("document %q has no PDF link annotations", idOrName)
	}

	pdfBytes, err := client.GetBlob(ctx, pdfFile.Hash, item.ID+".pdf")
	if err != nil {
		return nil, fmt.Errorf("downloading template PDF: %w", err)
	}

	doc, cleanup, err := render.OpenDocumentFromBytes(pdfBytes)
	if err != nil {
		return nil, fmt.Errorf("parsing PDF: %w", err)
	}
	defer cleanup()

	docLinks, err := doc.Links(pageIdx)
	if err != nil {
		return nil, err
	}

	links := make([]PageLink, 0, len(docLinks))
	for _, dl := range docLinks {
		links = append(links, PageLink{
			Index:      dl.Index,
			TargetPage: dl.TargetPage,
			URI:        dl.URI,
		})
	}
	return links, nil
}

// catFormats lists the formats Cat writes, in the order errors present them.
var catFormats = []string{"pdf", "text", "rm", "svg"}

// ParseCatFormat returns the Cat format named by s, ignoring case and
// surrounding whitespace.
func ParseCatFormat(s string) (string, error) {
	format := strings.ToLower(strings.TrimSpace(s))
	if !slices.Contains(catFormats, format) {
		return "", fmt.Errorf("unsupported format %q (choose from: %s)", format, strings.Join(catFormats, ", "))
	}
	return format, nil
}

// Cat streams document page contents (pdf, rm, svg, or text) directly to w.
func Cat(ctx context.Context, client *cloud.Client, idOrName string, pageIdx int, format string, w io.Writer) error {
	format, err := ParseCatFormat(format)
	if err != nil {
		return err
	}

	item, err := client.Resolve(ctx, idOrName)
	if err != nil {
		return fmt.Errorf("resolving document %q: %w", idOrName, err)
	}

	manifest, err := item.GetManifest(ctx)
	if err != nil {
		return fmt.Errorf("fetching document manifest: %w", err)
	}

	switch format {
	case "pdf":
		pdfFile := manifest.Find(item.ID + ".pdf")
		if pdfFile == nil {
			pdfFile = manifest.FindSuffix(".pdf")
		}
		if pdfFile == nil {
			return fmt.Errorf("document %q has no background PDF", idOrName)
		}
		data, err := client.GetBlob(ctx, pdfFile.Hash, item.ID+".pdf")
		if err != nil {
			return fmt.Errorf("downloading PDF blob: %w", err)
		}
		_, err = w.Write(data)
		return err

	case "text":
		pdfFile := manifest.Find(item.ID + ".pdf")
		if pdfFile == nil {
			pdfFile = manifest.FindSuffix(".pdf")
		}
		if pdfFile == nil {
			return fmt.Errorf("document %q has no PDF text to extract", idOrName)
		}
		pdfBytes, err := client.GetBlob(ctx, pdfFile.Hash, item.ID+".pdf")
		if err != nil {
			return fmt.Errorf("downloading PDF blob: %w", err)
		}
		doc, cleanup, err := render.OpenDocumentFromBytes(pdfBytes)
		if err != nil {
			return fmt.Errorf("opening PDF: %w", err)
		}
		defer cleanup()

		text, err := doc.Text(pageIdx)
		if err != nil {
			return fmt.Errorf("extracting text: %w", err)
		}
		_, err = io.WriteString(w, text)
		return err

	default: // ParseCatFormat leaves only "rm" and "svg".
		docContent, err := item.GetContent(ctx)
		if err != nil {
			return fmt.Errorf("fetching content schema: %w", err)
		}
		pageIDs := getPageIDs(docContent)
		if pageIdx < 0 || pageIdx >= len(pageIDs) {
			return fmt.Errorf("page index %d out of bounds (document has %d pages)", pageIdx, len(pageIDs))
		}
		pageID := pageIDs[pageIdx]
		rmName := fmt.Sprintf("%s/%s.rm", item.ID, pageID)
		fileEntry := manifest.Find(rmName)
		if fileEntry == nil {
			fileEntry = manifest.FindSuffix(pageID + ".rm")
		}
		if fileEntry == nil {
			return fmt.Errorf("no stroke data found for page %d (%s)", pageIdx, pageID)
		}
		rmBytes, err := client.GetBlob(ctx, fileEntry.Hash, rmName)
		if err != nil {
			return fmt.Errorf("downloading stroke blob: %w", err)
		}

		if format == "rm" {
			_, err = w.Write(rmBytes)
			return err
		}

		svgStr, err := rmscene.RenderRMToSVG(rmBytes)
		if err != nil {
			return fmt.Errorf("converting strokes to SVG: %w", err)
		}
		_, err = io.WriteString(w, svgStr)
		return err
	}
}

// RenderPage renders a specific document page with strokes to a PNG file.
func RenderPage(ctx context.Context, client *cloud.Client, idOrName string, pageIdx int, dpi int, outputPath string) error {
	item, err := client.Resolve(ctx, idOrName)
	if err != nil {
		return fmt.Errorf("resolving document %q: %w", idOrName, err)
	}

	manifest, err := item.GetManifest(ctx)
	if err != nil {
		return fmt.Errorf("fetching document manifest: %w", err)
	}

	pdfFile := manifest.Find(item.ID + ".pdf")
	if pdfFile == nil {
		pdfFile = manifest.FindSuffix(".pdf")
	}
	if pdfFile == nil {
		return fmt.Errorf("document %q has no PDF stationery template", idOrName)
	}
	pdfBytes, err := client.GetBlob(ctx, pdfFile.Hash, item.ID+".pdf")
	if err != nil {
		return fmt.Errorf("downloading template PDF: %w", err)
	}

	docContent, err := item.GetContent(ctx)
	if err != nil {
		return fmt.Errorf("fetching content schema: %w", err)
	}
	pageIDs := getPageIDs(docContent)
	if pageIdx < 0 || pageIdx >= len(pageIDs) {
		return fmt.Errorf("page index %d out of bounds (document has %d pages)", pageIdx, len(pageIDs))
	}
	pageID := pageIDs[pageIdx]
	rmName := fmt.Sprintf("%s/%s.rm", item.ID, pageID)

	var rmBytes []byte
	fileEntry := manifest.Find(rmName)
	if fileEntry == nil {
		fileEntry = manifest.FindSuffix(pageID + ".rm")
	}
	if fileEntry != nil {
		rmBytes, err = client.GetBlob(ctx, fileEntry.Hash, rmName)
		if err != nil {
			return fmt.Errorf("downloading stroke blob: %w", err)
		}
	}

	if dpi <= 0 {
		dpi = render.DefaultDPI
	}
	pngBytes, err := render.RenderPlannerPage(pdfBytes, pageIdx, rmBytes, dpi)
	if err != nil {
		return fmt.Errorf("rendering page: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}
	return os.WriteFile(outputPath, pngBytes, 0644)
}

// SyncDocOptions configures document page synchronization.
type SyncDocOptions struct {
	OutputDir string
	Format    string // "png", "svg", "rm"
	DPI       int
	Force     bool
	Pages     []int // optional specific page indices (empty means all pages)
}

// SyncPageResult tracks the state of an individual synced page.
type SyncPageResult struct {
	PageIndex int
	PageID    string
	Path      string
	State     string // "written", "skipped"
}

// syncFormats lists the page formats SyncDocument writes; the first is the default.
var syncFormats = []string{"png", "svg", "rm"}

// ParseSyncFormat returns the SyncDocument page format named by s; empty selects
// the default PNG.
func ParseSyncFormat(s string) (string, error) {
	if s == "" {
		return syncFormats[0], nil
	}
	if !slices.Contains(syncFormats, s) {
		return "", fmt.Errorf("unsupported format %q (choose from: %s)", s, strings.Join(syncFormats, ", "))
	}
	return s, nil
}

// SyncDocument synchronizes pages of any document to local disk.
func SyncDocument(ctx context.Context, client *cloud.Client, idOrName string, opts SyncDocOptions) ([]SyncPageResult, error) {
	if opts.OutputDir == "" {
		opts.OutputDir = "."
	}
	format, err := ParseSyncFormat(opts.Format)
	if err != nil {
		return nil, err
	}
	opts.Format = format
	if opts.DPI <= 0 {
		opts.DPI = render.DefaultDPI
	}

	item, err := client.Resolve(ctx, idOrName)
	if err != nil {
		return nil, fmt.Errorf("resolving document %q: %w", idOrName, err)
	}

	docContent, err := item.GetContent(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetching content schema: %w", err)
	}

	manifest, err := item.GetManifest(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetching document manifest: %w", err)
	}

	var pdfBytes []byte
	if opts.Format == "png" {
		pdfFile := manifest.Find(item.ID + ".pdf")
		if pdfFile == nil {
			pdfFile = manifest.FindSuffix(".pdf")
		}
		if pdfFile != nil {
			pdfBytes, err = client.GetBlob(ctx, pdfFile.Hash, item.ID+".pdf")
			if err != nil {
				return nil, fmt.Errorf("downloading background PDF: %w", err)
			}
		}
	}

	pageIDs := getPageIDs(docContent)
	pageIndices := opts.Pages
	if len(pageIndices) == 0 {
		for i := 0; i < len(pageIDs); i++ {
			pageIndices = append(pageIndices, i)
		}
	}

	docDir := filepath.Join(opts.OutputDir, sanitizeFilename(item.Metadata.VisibleName))
	var results []SyncPageResult

	for _, pageIdx := range pageIndices {
		if pageIdx < 0 || pageIdx >= len(pageIDs) {
			continue
		}
		pageID := pageIDs[pageIdx]
		filename := fmt.Sprintf("page-%03d.%s", pageIdx, opts.Format)
		outPath := filepath.Join(docDir, filename)

		if !opts.Force {
			if _, err := os.Stat(outPath); err == nil {
				results = append(results, SyncPageResult{
					PageIndex: pageIdx,
					PageID:    pageID,
					Path:      outPath,
					State:     "skipped",
				})
				continue
			}
		}

		rmName := fmt.Sprintf("%s/%s.rm", item.ID, pageID)
		var rmBytes []byte
		entry := manifest.Find(rmName)
		if entry == nil {
			entry = manifest.FindSuffix(pageID + ".rm")
		}
		if entry != nil {
			rmBytes, err = client.GetBlob(ctx, entry.Hash, rmName)
			if err != nil {
				return results, fmt.Errorf("downloading strokes for page %d: %w", pageIdx, err)
			}
		}

		if err := os.MkdirAll(docDir, 0755); err != nil {
			return results, fmt.Errorf("creating document output directory: %w", err)
		}

		switch opts.Format {
		case "png":
			pngBytes, err := render.RenderPlannerPage(pdfBytes, pageIdx, rmBytes, opts.DPI)
			if err != nil {
				return results, fmt.Errorf("rendering page %d: %w", pageIdx, err)
			}
			if err := os.WriteFile(outPath, pngBytes, 0644); err != nil {
				return results, fmt.Errorf("writing %s: %w", outPath, err)
			}
		case "svg":
			svgStr, err := rmscene.RenderRMToSVG(rmBytes)
			if err != nil {
				return results, fmt.Errorf("converting page %d to SVG: %w", pageIdx, err)
			}
			if err := os.WriteFile(outPath, []byte(svgStr), 0644); err != nil {
				return results, fmt.Errorf("writing %s: %w", outPath, err)
			}
		case "rm":
			if err := os.WriteFile(outPath, rmBytes, 0644); err != nil {
				return results, fmt.Errorf("writing %s: %w", outPath, err)
			}
		}

		results = append(results, SyncPageResult{
			PageIndex: pageIdx,
			PageID:    pageID,
			Path:      outPath,
			State:     "written",
		})
	}

	return results, nil
}

func getPageIDs(dc *cloud.DocumentContent) []string {
	if len(dc.Pages) > 0 {
		return dc.Pages
	}
	var ids []string
	for _, p := range dc.CPages.Pages {
		if p.ID != "" {
			ids = append(ids, p.ID)
		}
	}
	return ids
}

func sanitizeFilename(s string) string {
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, "\\", "-")
	return strings.TrimSpace(s)
}
