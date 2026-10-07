package doc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	cloud "github.com/alexgorbatchev/go-remarkable-cloud"
)

type settingsSnapshot struct {
	id       string
	hash     string
	manifest *cloud.Manifest
	files    map[string][]byte
}

func settingsRoot(ctx context.Context, client *cloud.Client) (*cloud.RootState, *cloud.Manifest, error) {
	root, err := client.GetRootState(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("reading settings root: %w", err)
	}
	data, err := client.GetBlobFresh(ctx, root.Hash, "root.docSchema")
	if err != nil {
		return nil, nil, fmt.Errorf("reading fresh root manifest: %w", err)
	}
	if archiveHash(data) != root.Hash {
		return nil, nil, fmt.Errorf("root manifest hash mismatch")
	}
	manifest, err := cloud.ParseManifest(root.Hash, bytes.NewReader(data))
	if err != nil {
		return nil, nil, err
	}
	seen := make(map[string]bool)
	for _, entry := range manifest.Entries {
		if seen[entry.ID] {
			return nil, nil, fmt.Errorf("duplicate root entry %s", entry.ID)
		}
		seen[entry.ID] = true
	}
	return root, manifest, nil
}

func readSettingsSnapshot(ctx context.Context, client *cloud.Client, root *cloud.Manifest, id string) (*settingsSnapshot, error) {
	entry := root.Find(id)
	if entry == nil {
		return nil, fmt.Errorf("%w: document %s is missing from root", cloud.ErrItemNotFound, id)
	}
	data, err := client.GetBlobFresh(ctx, entry.Hash, id+".docSchema")
	if err != nil {
		return nil, err
	}
	manifest, err := cloud.ParseManifest(entry.Hash, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if err := validateArchiveManifest(manifest); err != nil {
		return nil, err
	}
	snapshot := &settingsSnapshot{id: id, hash: entry.Hash, manifest: manifest, files: make(map[string][]byte)}
	for _, file := range manifest.Entries {
		data, err := client.GetBlobFresh(ctx, file.Hash, file.ID)
		if err != nil {
			return nil, fmt.Errorf("reading fresh %s: %w", file.ID, err)
		}
		if archiveHash(data) != file.Hash || int64(len(data)) != file.Size {
			return nil, fmt.Errorf("file %s hash or size mismatch", file.ID)
		}
		snapshot.files[file.ID] = data
	}
	content, metadata := snapshot.files[id+".content"], snapshot.files[id+".metadata"]
	if content == nil || metadata == nil {
		return nil, fmt.Errorf("document %s needs content and metadata", id)
	}
	fields, err := settingsObject(metadata)
	if err != nil {
		return nil, fmt.Errorf("invalid document metadata: %w", err)
	}
	var kind string
	if err := json.Unmarshal(fields["type"], &kind); err != nil || kind != string(cloud.ItemTypeDocument) {
		return nil, fmt.Errorf("%s is not a document", id)
	}
	return snapshot, nil
}

func verifySettingsSnapshot(ctx context.Context, client *cloud.Client, root *cloud.Manifest, before *settingsSnapshot, content []byte) (*settingsSnapshot, error) {
	if content == nil {
		entry := root.Find(before.id)
		if entry == nil || entry.Hash != before.hash {
			return nil, fmt.Errorf("source document changed during transfer")
		}
	}
	current, err := readSettingsSnapshot(ctx, client, root, before.id)
	if err != nil {
		return nil, err
	}
	if len(current.manifest.Entries) != len(before.manifest.Entries) {
		return nil, fmt.Errorf("document %s file set changed", before.id)
	}
	for _, entry := range before.manifest.Entries {
		wanted := before.files[entry.ID]
		if content != nil && entry.ID == before.id+".content" {
			wanted = content
		}
		actual := current.manifest.Find(entry.ID)
		if actual == nil || actual.Hash != archiveHash(wanted) || actual.Size != int64(len(wanted)) || actual.Type != entry.Type || actual.Subfiles != entry.Subfiles || !bytes.Equal(current.files[entry.ID], wanted) {
			return nil, fmt.Errorf("document %s preservation verification failed for %s", before.id, entry.ID)
		}
	}
	return current, nil
}
