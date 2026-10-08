package doc

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode"

	cloud "github.com/alexgorbatchev/go-remarkable-cloud"
	render "github.com/alexgorbatchev/go-remarkable-render"
)

const nativePagesPending = "pending-tablet-initialization"

// UploadOptions requires an explicit title and a new recovery evidence path.
// InitializePages creates the native page structure of every PDF page with the
// document, instead of leaving it for the tablet to create when first opened.
type UploadOptions struct {
	Title           string
	Folder          string
	Evidence        string
	InitializePages bool
	OnProgress      func(*UploadEvidence) error
}

// validateUploadOptions rejects invalid options before UploadPDF reads the PDF or
// contacts the cloud.
func validateUploadOptions(opts UploadOptions) error {
	if err := ValidateUploadTitle(opts.Title); err != nil {
		return err
	}
	if err := ValidateUploadFolder(opts.Folder); err != nil {
		return err
	}
	return ValidateUploadEvidencePath(opts.Evidence)
}

// ValidateUploadTitle requires a nonblank title without control characters.
func ValidateUploadTitle(title string) error {
	if strings.TrimSpace(title) == "" || strings.IndexFunc(title, unicode.IsControl) >= 0 {
		return fmt.Errorf("upload title must be nonempty and contain no control characters")
	}
	return nil
}

// ValidateUploadFolder accepts a collection UUID, or empty for the root.
func ValidateUploadFolder(folder string) error {
	if folder != "" && !isUUID(folder) {
		return fmt.Errorf("upload folder must be a collection UUID")
	}
	return nil
}

// ValidateUploadEvidencePath requires a recovery evidence path.
func ValidateUploadEvidencePath(path string) error {
	if path == "" {
		return fmt.Errorf("upload evidence path is required")
	}
	return nil
}

// UploadPDF creates a separate document, preserving the original PDF bytes.
// The recorded root snapshot governs both preflight policy and the cloud commit.
func UploadPDF(ctx context.Context, client *cloud.Client, path string, opts UploadOptions) (*UploadEvidence, error) {
	if err := validateUploadOptions(opts); err != nil {
		return nil, err
	}
	pdf, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading upload PDF: %w", err)
	}
	document, cleanup, err := render.OpenDocumentFromBytes(pdf)
	if err != nil {
		return nil, fmt.Errorf("validating upload PDF: %w", err)
	}
	pages := document.NumPage()
	cleanup()
	if pages < 1 {
		return nil, fmt.Errorf("upload PDF has no pages")
	}
	evidence := &UploadEvidence{Version: 1, Title: opts.Title, Folder: opts.Folder, Pages: pages, NativePages: nativePagesPending}
	var author string
	if opts.InitializePages {
		// Page identity is fixed here, before preflight, and recorded in the
		// evidence written before staging; recovery never regenerates it.
		evidence.NativePages = NativePagesInitialized
		if evidence.PageIDs, author, err = newNativePageIDs(pages); err != nil {
			return nil, err
		}
	}
	root, err := uploadPreflight(ctx, client, opts)
	if err != nil {
		return nil, err
	}
	id := uploadUUID()
	files, err := uploadFiles(id, pdf, evidence, author)
	if err != nil {
		return nil, err
	}
	evidence.Result = cloud.CreateResult{ID: id, State: cloud.UpdateStaged, Generation: root.Generation}
	for _, file := range files {
		evidence.Files = append(evidence.Files, UploadFile{Name: file.Name, SHA256: archiveHash(file.Data), Size: int64(len(file.Data))})
	}
	if err := writeUploadEvidence(opts.Evidence, evidence, true); err != nil {
		return nil, err
	}
	result, createErr := client.CreateDocument(ctx, cloud.CreateDocumentOptions{ID: id, ExpectedRoot: *root, Files: files, OnProgress: func(progress cloud.CreateResult) error {
		evidence.Result = progress
		if err := writeUploadEvidence(opts.Evidence, evidence, false); err != nil {
			return err
		}
		if opts.OnProgress != nil {
			return opts.OnProgress(evidence)
		}
		return nil
	}})
	evidence.Result = *result
	// Persist definitive rejections too: the precommit callback intentionally
	// records uncertainty before the request, even when the server rejects it.
	persistErr := writeUploadEvidence(opts.Evidence, evidence, false)
	return evidence, errors.Join(createErr, persistErr)
}

// uploadUUID returns a random version 4 UUID.
func uploadUUID() string {
	var id [16]byte
	_, _ = rand.Read(id[:]) // crypto/rand.Read never returns an error (Go 1.24+); it crashes the program on failure
	id[6] = (id[6] & 0x0f) | 0x40
	id[8] = (id[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", id[:4], id[4:6], id[6:8], id[8:10], id[10:])
}

func freshUploadManifest(ctx context.Context, client *cloud.Client, hash, name string) (*cloud.Manifest, error) {
	data, err := client.GetBlobFresh(ctx, hash, name)
	if err != nil {
		return nil, err
	}
	return cloud.ParseManifest(hash, bytes.NewReader(data))
}

func uploadPreflight(ctx context.Context, client *cloud.Client, opts UploadOptions) (*cloud.RootState, error) {
	root, err := client.GetRootState(ctx)
	if err != nil {
		return nil, err
	}
	manifest, err := freshUploadManifest(ctx, client, root.Hash, "root.docSchema")
	if err != nil {
		return nil, fmt.Errorf("reading upload root snapshot: %w", err)
	}
	allMeta := make(map[string]cloud.ItemMetadata)
	for _, entry := range manifest.Entries {
		if entry.ID == "." {
			continue
		}
		files, err := freshUploadManifest(ctx, client, entry.Hash, entry.ID+".docSchema")
		if err != nil {
			return nil, fmt.Errorf("reading upload preflight item %s: %w", entry.ID, err)
		}
		meta := files.Find(entry.ID + ".metadata")
		if meta == nil {
			return nil, fmt.Errorf("upload preflight item %s has no metadata", entry.ID)
		}
		data, err := client.GetBlobFresh(ctx, meta.Hash, meta.ID)
		if err != nil {
			return nil, err
		}
		if archiveHash(data) != meta.Hash {
			return nil, fmt.Errorf("upload preflight metadata checksum mismatch for %s", entry.ID)
		}
		var metadata cloud.ItemMetadata
		if err := json.Unmarshal(data, &metadata); err != nil {
			return nil, fmt.Errorf("reading upload metadata: %w", err)
		}
		allMeta[entry.ID] = metadata
		if !metadata.Deleted && metadata.Parent == opts.Folder && metadata.VisibleName == opts.Title {
			return nil, fmt.Errorf("title %q already exists in destination folder (%s); choose a distinct title", opts.Title, entry.ID)
		}
	}
	if opts.Folder != "" {
		folderItem, ok := allMeta[opts.Folder]
		if !ok {
			return nil, fmt.Errorf("destination folder %s not found: %w", opts.Folder, cloud.ErrItemNotFound)
		}
		if folderItem.Type != cloud.ItemTypeCollection {
			return nil, fmt.Errorf("destination folder %s is not a collection", opts.Folder)
		}
		visited := make(map[string]bool)
		for cur := opts.Folder; cur != ""; {
			if visited[cur] {
				return nil, fmt.Errorf("destination folder %s has a cycle in its parent chain", opts.Folder)
			}
			visited[cur] = true
			item, ok := allMeta[cur]
			if !ok {
				return nil, fmt.Errorf("destination folder %s ancestor %s not found: %w", opts.Folder, cur, cloud.ErrItemNotFound)
			}
			if item.Deleted || item.Parent == "trash" {
				return nil, fmt.Errorf("destination folder %s is deleted or in trash", opts.Folder)
			}
			if item.Type != cloud.ItemTypeCollection {
				return nil, fmt.Errorf("destination folder %s ancestor %s is not a collection", opts.Folder, cur)
			}
			cur = item.Parent
		}
	}
	return root, nil
}

func uploadFiles(id string, pdf []byte, evidence *UploadEvidence, author string) ([]cloud.FileUpdate, error) {
	pages := evidence.Pages
	now := fmt.Sprint(time.Now().UnixMilli())
	metadata := map[string]any{"type": cloud.ItemTypeDocument, "visibleName": evidence.Title, "parent": evidence.Folder, "deleted": false, "pinned": false, "createdTime": now, "lastModified": now, "lastOpened": "0", "lastOpenedPage": 0}
	// The ordinary PDF content schema represents unopened native pages as null.
	// Page counts describe the actual PDF; without InitializePages, tablet sync
	// owns native initialization.
	content := map[string]any{"fileType": "pdf", "formatVersion": 1, "pageCount": pages, "originalPageCount": pages, "pages": nil, "sizeInBytes": fmt.Sprint(len(pdf)), "coverPageNumber": -1, "documentMetadata": map[string]any{}, "extraMetadata": map[string]any{}, "fontName": "", "lineHeight": -1, "margins": 125, "orientation": "portrait", "textAlignment": "justify", "textScale": 1, "zoomMode": "bestFit", "tags": []any{}, "pageTags": []any{}}
	initialize := evidence.NativePages == NativePagesInitialized
	if initialize {
		if err := initializeContent(content, evidence.PageIDs, author); err != nil {
			return nil, err
		}
	}
	files := []cloud.FileUpdate{{Name: id + ".pdf", Data: pdf}, {Name: id + ".pagedata", Data: uploadPageData(pages, initialize)}}
	for _, file := range []struct {
		name  string
		value any
	}{{"metadata", metadata}, {"content", content}} {
		data, err := json.Marshal(file.value)
		if err != nil {
			return nil, fmt.Errorf("encoding upload %s: %w", file.name, err)
		}
		files = append(files, cloud.FileUpdate{Name: id + "." + file.name, Data: data})
	}
	return files, nil
}
