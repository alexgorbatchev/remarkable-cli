package doc

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	cloud "github.com/alexgorbatchev/go-remarkable-cloud"
)

const (
	archiveFormatVersion = 1
	archiveManifestPath  = "evidence/document.docSchema"
	archiveSnapshotPath  = "evidence/snapshot.json"
)

// ArchiveFile records both the cloud address and the downloaded byte digest.
type ArchiveFile struct {
	Name   string `json:"name"`
	Hash   string `json:"hash"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// ArchiveSnapshot identifies a source revision and every preserved native file.
type ArchiveSnapshot struct {
	FormatVersion  int             `json:"format_version"`
	DocumentID     string          `json:"document_id"`
	DocumentHash   string          `json:"document_hash"`
	Root           cloud.RootState `json:"root"`
	ManifestSHA256 string          `json:"manifest_sha256"`
	Files          []ArchiveFile   `json:"files"`
}

// ArchiveDocument publishes a complete ZIP only after fresh downloads and an
// unchanged cloud root confirm one consistent source revision. It never writes
// cloud data or replaces an existing output path.
func ArchiveDocument(ctx context.Context, client *cloud.Client, query, output string) (*ArchiveSnapshot, error) {
	if output == "" {
		return nil, fmt.Errorf("archive output path is required")
	}
	if _, err := os.Lstat(output); err == nil {
		return nil, fmt.Errorf("archive output already exists: %s", output)
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("checking archive output: %w", err)
	}
	item, err := client.Resolve(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("resolving archive source: %w", err)
	}
	if !item.IsDocument() {
		return nil, fmt.Errorf("archive source %s is not a document", item.ID)
	}
	root, manifest, raw, err := archiveSource(ctx, client, item)
	if err != nil {
		return nil, err
	}
	snapshot := &ArchiveSnapshot{FormatVersion: archiveFormatVersion, DocumentID: item.ID, DocumentHash: item.Hash, Root: *root, ManifestSHA256: archiveHash(raw)}
	tmp, err := os.CreateTemp(filepath.Dir(output), ".remarkable-archive-*.zip")
	if err != nil {
		return nil, fmt.Errorf("creating temporary archive: %w", err)
	}
	defer func() {
		_ = tmp.Close()           // Best-effort cleanup; the successful path checks Close explicitly.
		_ = os.Remove(tmp.Name()) // Best-effort removal of unpublished or linked temporary data.
	}()
	if err := writeArchive(ctx, client, tmp, manifest, raw, snapshot); err != nil {
		return nil, err
	}
	current, err := client.GetRootState(ctx)
	if err != nil {
		return nil, fmt.Errorf("checking archive source revision: %w", err)
	}
	if *current != *root {
		return nil, fmt.Errorf("cloud source changed during archive export; retry from a fresh snapshot")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := tmp.Sync(); err != nil {
		return nil, fmt.Errorf("syncing archive: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return nil, fmt.Errorf("closing archive: %w", err)
	}
	// A hard link atomically publishes complete bytes and refuses replacement,
	// including when another process creates the destination after preflight.
	if err := os.Link(tmp.Name(), output); err != nil {
		return nil, fmt.Errorf("publishing archive without replacement: %w", err)
	}
	return snapshot, nil
}

func archiveSource(ctx context.Context, client *cloud.Client, item *cloud.Item) (*cloud.RootState, *cloud.Manifest, []byte, error) {
	root, err := client.GetRootState(ctx)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("reading archive root: %w", err)
	}
	rootData, err := client.GetBlobFresh(ctx, root.Hash, "root.docSchema")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("reading fresh archive root manifest: %w", err)
	}
	rootManifest, err := cloud.ParseManifest(root.Hash, bytes.NewReader(rootData))
	if err != nil {
		return nil, nil, nil, err
	}
	entry := rootManifest.Find(item.ID)
	if entry == nil || entry.Hash != item.Hash {
		return nil, nil, nil, fmt.Errorf("archive source changed during resolution or has no document manifest")
	}
	raw, err := client.GetBlobFresh(ctx, item.Hash, item.ID+".docSchema")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("reading fresh document manifest: %w", err)
	}
	manifest, err := cloud.ParseManifest(item.Hash, bytes.NewReader(raw))
	if err != nil {
		return nil, nil, nil, err
	}
	if err := validateArchiveManifest(manifest); err != nil {
		return nil, nil, nil, err
	}
	if manifest.Find(item.ID+".content") == nil || manifest.Find(item.ID+".metadata") == nil {
		return nil, nil, nil, fmt.Errorf("archive source has no complete content and metadata entries")
	}
	return root, manifest, raw, nil
}

func validateArchiveManifest(manifest *cloud.Manifest) error {
	entries := append([]cloud.SchemaEntry(nil), manifest.Entries...)
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	hasher := sha256.New()
	for i, entry := range entries {
		if !fs.ValidPath(entry.ID) || entry.ID == "." || strings.ContainsAny(entry.ID, "\\\x00\r\n:") || entry.Size < 0 {
			return fmt.Errorf("invalid native archive entry %q", entry.ID)
		}
		if i > 0 && entries[i-1].ID == entry.ID {
			return fmt.Errorf("duplicate native archive entry %q", entry.ID)
		}
		hash, err := hex.DecodeString(entry.Hash)
		if err != nil || len(hash) != sha256.Size {
			return fmt.Errorf("invalid native archive hash for %q", entry.ID)
		}
		hasher.Write(hash)
	}
	// Cloud v3 document addresses hash ordered binary file hashes, rather than
	// the textual manifest; raw manifest SHA-256 is recorded separately.
	if hex.EncodeToString(hasher.Sum(nil)) != manifest.Hash {
		return fmt.Errorf("native document manifest hash mismatch")
	}
	return nil
}

func writeArchive(ctx context.Context, client *cloud.Client, output *os.File, manifest *cloud.Manifest, raw []byte, snapshot *ArchiveSnapshot) error {
	w := zip.NewWriter(output)
	defer func() { _ = w.Close() }() // Flush failures on the success path are checked below.
	for _, entry := range manifest.Entries {
		data, err := client.GetBlobFresh(ctx, entry.Hash, entry.ID)
		if err != nil {
			return fmt.Errorf("downloading native file %q: %w", entry.ID, err)
		}
		digest := archiveHash(data)
		if digest != entry.Hash || int64(len(data)) != entry.Size {
			return fmt.Errorf("native file %q hash or size mismatch", entry.ID)
		}
		if err := writeArchiveEntry(w, "files/"+entry.ID, data); err != nil {
			return err
		}
		snapshot.Files = append(snapshot.Files, ArchiveFile{Name: entry.ID, Hash: entry.Hash, SHA256: digest, Size: int64(len(data))})
	}
	if err := writeArchiveEntry(w, archiveManifestPath, raw); err != nil {
		return err
	}
	evidence, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding archive evidence: %w", err)
	}
	if err := writeArchiveEntry(w, archiveSnapshotPath, append(evidence, '\n')); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("finishing ZIP archive: %w", err)
	}
	return nil
}

func writeArchiveEntry(w *zip.Writer, name string, data []byte) error {
	entry, err := w.Create(name)
	if err != nil {
		return fmt.Errorf("creating archive entry %q: %w", name, err)
	}
	if _, err := entry.Write(data); err != nil {
		return fmt.Errorf("writing archive entry %q: %w", name, err)
	}
	return nil
}

func archiveHash(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
