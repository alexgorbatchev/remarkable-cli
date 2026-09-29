package doc

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	cloud "github.com/alexgorbatchev/go-remarkable-cloud"
	render "github.com/alexgorbatchev/go-remarkable-render"
	"github.com/alexgorbatchev/go-rmscene"
	"github.com/alexgorbatchev/remarkable-sync/internal/agent"
)

// ItemSummary represents a user-facing document or folder.
type ItemSummary struct {
	ID       string
	Name     string
	Type     string
	Modified string
	Parent   string
}

// List returns a list of cloud items optionally filtered by folder or type.
func List(ctx context.Context, client *cloud.Client, folderID, docType string, limit int) ([]ItemSummary, error) {
	items, err := client.ListItems(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing cloud items: %w", err)
	}

	summaries := make([]ItemSummary, 0, len(items))
	for _, it := range items {
		if folderID != "" && it.Metadata.Parent != folderID {
			continue
		}
		if docType != "" && string(it.Metadata.Type) != docType {
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

	// Index by parent ID
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
}

// Inspect fetches detailed metadata for a document.
func Inspect(ctx context.Context, client *cloud.Client, idOrName string) (*DocDetails, error) {
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

	// Try fetching document content
	if docContent, err := item.GetContent(ctx); err == nil && docContent != nil {
		details.Pages = docContent.PageCount
		details.FileType = docContent.FileType
	}

	return details, nil
}

// Cat streams document page contents (pdf, rm, or svg) directly to w.
func Cat(ctx context.Context, client *cloud.Client, idOrName string, pageIdx int, format string, w io.Writer) error {
	item, err := client.Resolve(ctx, idOrName)
	if err != nil {
		return fmt.Errorf("resolving document %q: %w", idOrName, err)
	}

	manifest, err := item.GetManifest(ctx)
	if err != nil {
		return fmt.Errorf("fetching document manifest: %w", err)
	}

	format = strings.ToLower(strings.TrimSpace(format))
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

	case "rm", "svg":
		// Find stroke file for page index
		docContent, err := item.GetContent(ctx)
		if err != nil {
			return fmt.Errorf("fetching content schema: %w", err)
		}
		if pageIdx < 0 || pageIdx >= len(docContent.Pages) {
			return fmt.Errorf("page index %d out of bounds (document has %d pages)", pageIdx, len(docContent.Pages))
		}
		pageID := docContent.Pages[pageIdx]
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

		// Convert to SVG
		svgStr, err := rmscene.RenderRMToSVG(rmBytes)
		if err != nil {
			return fmt.Errorf("converting strokes to SVG: %w", err)
		}
		_, err = io.WriteString(w, svgStr)
		return err

	default:
		return fmt.Errorf("unsupported format %q (choose from: pdf, rm, svg)", format)
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

	// 1. Download PDF background
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

	// 2. Download page strokes
	docContent, err := item.GetContent(ctx)
	if err != nil {
		return fmt.Errorf("fetching content schema: %w", err)
	}
	if pageIdx < 0 || pageIdx >= len(docContent.Pages) {
		return fmt.Errorf("page index %d out of bounds (document has %d pages)", pageIdx, len(docContent.Pages))
	}
	pageID := docContent.Pages[pageIdx]
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

	// 3. Composite with render engine
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

// SyncDocument synchronizes pages of any document to local disk.
func SyncDocument(ctx context.Context, client *cloud.Client, idOrName string, opts SyncDocOptions) ([]SyncPageResult, error) {
	if opts.OutputDir == "" {
		opts.OutputDir = "."
	}
	if opts.Format == "" {
		opts.Format = "png"
	}
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
			pdfBytes, _ = client.GetBlob(ctx, pdfFile.Hash, item.ID+".pdf")
		}
	}

	// Determine which pages to sync
	pageIndices := opts.Pages
	if len(pageIndices) == 0 {
		for i := 0; i < len(docContent.Pages); i++ {
			pageIndices = append(pageIndices, i)
		}
	}

	docDir := filepath.Join(opts.OutputDir, sanitizeFilename(item.Metadata.VisibleName))
	var results []SyncPageResult

	for _, pageIdx := range pageIndices {
		if pageIdx < 0 || pageIdx >= len(docContent.Pages) {
			continue
		}
		pageID := docContent.Pages[pageIdx]
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

		// Download stroke blob
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

func sanitizeFilename(s string) string {
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, "\\", "-")
	return strings.TrimSpace(s)
}
