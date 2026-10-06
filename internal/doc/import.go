package doc

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	cloud "github.com/alexgorbatchev/go-remarkable-cloud"
	"github.com/alexgorbatchev/go-rmscene"
)

// PageImport maps a local native stroke file to a zero-based destination page.
type PageImport struct {
	Source string
	Page   int
}

// ImportedPage records the native page association selected during preflight.
type ImportedPage struct {
	Page   int
	PageID string
	Source string
}

// ImportResult includes partial progress even when transfer or verification fails.
type ImportResult struct {
	State    cloud.UpdateState
	Uploaded []string
	Pages    []ImportedPage
}

// LoadImportMapping reads a strict JSON array; relative source paths use its directory.
func LoadImportMapping(path string) ([]PageImport, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading mapping: %w", err)
	}
	var rows []struct {
		Source string `json:"source"`
		Page   *int   `json:"page"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&rows); err != nil {
		return nil, fmt.Errorf("decoding mapping: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("mapping must contain one JSON array")
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("mapping must contain at least one page")
	}
	mapping := make([]PageImport, 0, len(rows))
	for i, row := range rows {
		if row.Source == "" || row.Page == nil || *row.Page < 0 {
			return nil, fmt.Errorf("mapping row %d requires source and nonnegative page", i)
		}
		source := row.Source
		if !filepath.IsAbs(source) {
			source = filepath.Join(filepath.Dir(path), source)
		}
		mapping = append(mapping, PageImport{Source: source, Page: *row.Page})
	}
	return mapping, nil
}

// ValidateImportDestination requires the destination document UUID.
func ValidateImportDestination(id string) error {
	if !isUUID(id) {
		return fmt.Errorf("destination must be a UUID")
	}
	return nil
}

// ImportStrokes preserves native bytes and existing PDF/content files. All page
// mappings and destination handwriting are checked before any cloud upload.
func ImportStrokes(ctx context.Context, client *cloud.Client, id string, mapping []PageImport) (*ImportResult, error) {
	if err := ValidateImportDestination(id); err != nil {
		return nil, err
	}
	if len(mapping) == 0 {
		return nil, fmt.Errorf("at least one page mapping is required")
	}
	item, err := client.ResolveByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("resolving destination: %w", err)
	}
	if !item.IsDocument() {
		return nil, fmt.Errorf("destination %s is not a document", id)
	}
	manifest, err := item.GetManifest(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetching destination manifest: %w", err)
	}
	content, err := item.GetContent(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetching destination page structure: %w", err)
	}
	pageIDs, err := importPageIDs(content)
	if err != nil {
		return nil, err
	}
	result, files, err := preflightImport(ctx, client, id, manifest, pageIDs, mapping)
	if err != nil {
		return nil, err
	}
	metadata, err := importMetadata(ctx, client, manifest, id)
	if err != nil {
		return nil, err
	}
	files = append(files, metadata)
	update, err := client.UpdateDocumentFiles(ctx, id, item.Hash, files)
	result.State = update.State
	result.Uploaded = update.Uploaded
	if err != nil {
		return result, fmt.Errorf("import state %s: %w", result.State, err)
	}
	return result, nil
}

func isUUID(id string) bool {
	if len(id) != 36 || id[8] != '-' || id[13] != '-' || id[18] != '-' || id[23] != '-' {
		return false
	}
	decoded, err := hex.DecodeString(strings.ReplaceAll(id, "-", ""))
	return err == nil && len(decoded) == 16
}

func importPageIDs(content *cloud.DocumentContent) ([]string, error) {
	ids := content.Pages
	if len(content.CPages.Pages) > 0 {
		ids = make([]string, 0, len(content.CPages.Pages))
		for i, page := range content.CPages.Pages {
			if page.Deleted != nil && page.Deleted.Value != 0 {
				return nil, fmt.Errorf("destination page %d is deleted; page structure is incompatible", i)
			}
			ids = append(ids, page.ID)
		}
		if len(content.Pages) > 0 && !equalPageIDs(ids, content.Pages) {
			return nil, fmt.Errorf("destination page structures disagree")
		}
	}
	if len(ids) == 0 || (content.PageCount > 0 && content.PageCount != len(ids)) {
		return nil, fmt.Errorf("destination has no complete initialized native page structure")
	}
	seen := make(map[string]bool)
	for i, id := range ids {
		if id == "" || strings.ContainsAny(id, "/\\:\r\n") || id == "." || id == ".." || seen[id] {
			return nil, fmt.Errorf("destination page %d has missing, duplicate, or invalid native ID", i)
		}
		seen[id] = true
	}
	return ids, nil
}

func equalPageIDs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func preflightImport(ctx context.Context, client *cloud.Client, id string, manifest *cloud.Manifest, pageIDs []string, mapping []PageImport) (*ImportResult, []cloud.FileUpdate, error) {
	result := &ImportResult{}
	var files []cloud.FileUpdate
	var failures []error
	seen := make(map[int]bool)
	for _, row := range mapping {
		if row.Page < 0 || row.Page >= len(pageIDs) || seen[row.Page] {
			failures = append(failures, fmt.Errorf("page %d has duplicate or out-of-range mapping", row.Page))
			continue
		}
		seen[row.Page] = true
		data, err := os.ReadFile(row.Source)
		if err == nil {
			_, err = nativeBlocks(data)
		}
		if err != nil {
			failures = append(failures, fmt.Errorf("page %d source %s: %w", row.Page, row.Source, err))
			continue
		}
		pageID := pageIDs[row.Page]
		name := id + "/" + pageID + ".rm"
		if err := checkEmptyPage(ctx, client, manifest, name); err != nil {
			failures = append(failures, fmt.Errorf("page %d (%s): %w", row.Page, pageID, err))
			continue
		}
		files = append(files, cloud.FileUpdate{Name: name, Data: data})
		result.Pages = append(result.Pages, ImportedPage{Page: row.Page, PageID: pageID, Source: row.Source})
	}
	if len(failures) > 0 {
		return nil, nil, errors.Join(failures...)
	}
	return result, files, nil
}

func nativeBlocks(data []byte) ([]rmscene.Block, error) {
	if err := rmscene.ValidateHeaderBytes(data); err != nil {
		return nil, err
	}
	r := bytes.NewReader(data[rmscene.HeaderLength:])
	stream := rmscene.NewDataStream(r)
	var blocks []rmscene.Block
	for r.Len() > 0 {
		block, err := rmscene.ReadBlock(stream)
		if err != nil {
			return nil, fmt.Errorf("invalid native block: %w", err)
		}
		blocks = append(blocks, block)
	}
	return blocks, nil
}

func checkEmptyPage(ctx context.Context, client *cloud.Client, manifest *cloud.Manifest, name string) error {
	pageFile := path.Base(name)
	for _, candidate := range manifest.Entries {
		if (candidate.ID == pageFile || strings.HasSuffix(candidate.ID, "/"+pageFile)) && candidate.ID != name {
			return fmt.Errorf("incompatible native page association %s", candidate.ID)
		}
	}
	entry := manifest.Find(name)
	if entry == nil {
		return nil
	}
	data, err := client.GetBlobFresh(ctx, entry.Hash, name)
	if err != nil {
		return fmt.Errorf("reading existing native page: %w", err)
	}
	blocks, err := nativeBlocks(data)
	if err != nil {
		return fmt.Errorf("existing native page is incompatible: %w", err)
	}
	for _, block := range blocks {
		switch b := block.(type) {
		case *rmscene.MigrationInfoBlock, *rmscene.SceneTreeBlock, *rmscene.TreeNodeBlock, *rmscene.SceneGroupItemBlock, *rmscene.AuthorIdsBlock, *rmscene.SceneInfoBlock:
		case *rmscene.PageInfoBlock:
			if b.TextCharsCount != 0 || b.TextLinesCount != 0 {
				return fmt.Errorf("destination contains existing text")
			}
		default:
			return fmt.Errorf("destination contains handwriting, annotations, or unsupported block 0x%02x", block.BlockType())
		}
	}
	return nil
}

func importMetadata(ctx context.Context, client *cloud.Client, manifest *cloud.Manifest, id string) (cloud.FileUpdate, error) {
	entry := manifest.Find(id + ".metadata")
	if entry == nil {
		return cloud.FileUpdate{}, fmt.Errorf("destination metadata is missing")
	}
	data, err := client.GetBlobFresh(ctx, entry.Hash, entry.ID)
	if err != nil {
		return cloud.FileUpdate{}, err
	}
	var metadata map[string]json.RawMessage
	if err := json.Unmarshal(data, &metadata); err != nil {
		return cloud.FileUpdate{}, fmt.Errorf("decoding destination metadata: %w", err)
	}
	if metadata == nil {
		return cloud.FileUpdate{}, fmt.Errorf("destination metadata is not an object")
	}
	modified, err := json.Marshal(strconv.FormatInt(time.Now().UnixMilli(), 10))
	if err != nil {
		return cloud.FileUpdate{}, err
	}
	metadata["lastModified"] = modified
	data, err = json.Marshal(metadata)
	if err != nil {
		return cloud.FileUpdate{}, err
	}
	return cloud.FileUpdate{Name: entry.ID, Data: data}, nil
}
