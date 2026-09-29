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
	if docContent, err := client.GetDocumentContent(ctx, item.Hash, item.ID); err == nil && docContent != nil {
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

	manifest, err := client.GetManifest(ctx, item.Hash, item.ID)
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
		docContent, err := client.GetDocumentContent(ctx, item.Hash, item.ID)
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

	manifest, err := client.GetManifest(ctx, item.Hash, item.ID)
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
	docContent, err := client.GetDocumentContent(ctx, item.Hash, item.ID)
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
