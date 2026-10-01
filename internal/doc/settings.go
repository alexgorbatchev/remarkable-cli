package doc

import (
	"context"
	"fmt"
	"strings"

	cloud "github.com/alexgorbatchev/go-remarkable-cloud"
)

// SettingsOptions keeps settings transfer separate from native stroke import.
type SettingsOptions struct {
	Mapping         []SettingsPageMap
	ReplaceViewport bool
}

// SettingsResult retains differences and commit identity even on failure.
// Verified means both transferred settings and all preservation checks passed.
type SettingsResult struct {
	State               cloud.UpdateState
	SourceID            string
	DestinationID       string
	SourceHash          string
	DestinationHash     string
	RootHash            string
	Generation          int64
	Uploaded            []string
	ViewportDifferences []ViewportDifference
}

// ValidateSettingsIdentities validates local inputs before credential lookup.
func ValidateSettingsIdentities(source, destination string) error {
	if !isUUID(source) || !isUUID(destination) {
		return fmt.Errorf("source and destination must be UUIDs")
	}
	if strings.EqualFold(source, destination) {
		return fmt.Errorf("source and destination must be separate documents")
	}
	return nil
}

// TransferSettings remaps structured tags and copies eight viewport fields from
// a fresh source revision. It writes destination content only and never retries
// an uncertain commit. Every native file is freshly verified on both documents.
func TransferSettings(ctx context.Context, client *cloud.Client, source, destination string, opts SettingsOptions) (*SettingsResult, error) {
	if err := ValidateSettingsIdentities(source, destination); err != nil {
		return nil, err
	}
	if err := validateSettingsMap(opts.Mapping); err != nil {
		return nil, err
	}
	root, manifest, err := settingsRoot(ctx, client)
	if err != nil {
		return nil, err
	}
	src, err := readSettingsSnapshot(ctx, client, manifest, source)
	if err != nil {
		return nil, fmt.Errorf("source snapshot: %w", err)
	}
	dst, err := readSettingsSnapshot(ctx, client, manifest, destination)
	if err != nil {
		return nil, fmt.Errorf("destination snapshot: %w", err)
	}
	result := &SettingsResult{State: cloud.UpdateStaged, SourceID: source, DestinationID: destination, SourceHash: src.hash, DestinationHash: dst.hash, Generation: root.Generation}
	sourceFields, err := settingsObject(src.files[source+".content"])
	if err != nil {
		return result, fmt.Errorf("source content: %w", err)
	}
	destinationFields, err := settingsObject(dst.files[destination+".content"])
	if err != nil {
		return result, fmt.Errorf("destination content: %w", err)
	}
	content, err := mergeSettings(sourceFields, destinationFields, opts, result)
	if err != nil {
		return result, err
	}
	if !sameSettingsJSON(content, dst.files[destination+".content"]) {
		update, err := client.UpdateDocumentFilesAtRoot(ctx, cloud.UpdateDocumentOptions{ID: destination, ExpectedHash: dst.hash, ExpectedRoot: *root, Files: []cloud.FileUpdate{{Name: destination + ".content", Data: content}}})
		result.State, result.Uploaded, result.RootHash = update.State, update.Uploaded, update.RootHash
		if err != nil {
			return result, fmt.Errorf("settings transfer state %s: %w", result.State, err)
		}
		// Library verification covers submitted content; preservation remains
		// pending until both complete document snapshots are freshly checked.
		result.State = cloud.UpdateCommitted
	} else {
		result.RootHash = root.Hash
		content = dst.files[destination+".content"]
	}
	if err := verifyTransferredSettings(ctx, client, src, dst, content, result); err != nil {
		return result, fmt.Errorf("settings preservation verification: %w", err)
	}
	result.State = cloud.UpdateVerified
	return result, nil
}

func verifyTransferredSettings(ctx context.Context, client *cloud.Client, src, dst *settingsSnapshot, content []byte, result *SettingsResult) error {
	root, manifest, err := settingsRoot(ctx, client)
	if err != nil {
		return err
	}
	if root.Hash != result.RootHash {
		return fmt.Errorf("cloud root changed after settings preflight/commit")
	}
	if result.State == cloud.UpdateStaged && root.Generation != result.Generation {
		return fmt.Errorf("cloud generation changed after settings preflight")
	}
	if _, err := verifySettingsSnapshot(ctx, client, manifest, src, nil); err != nil {
		return err
	}
	current, err := verifySettingsSnapshot(ctx, client, manifest, dst, content)
	if err != nil {
		return err
	}
	result.DestinationHash = current.hash
	end, err := client.GetRootState(ctx)
	if err != nil {
		return err
	}
	if *end != *root {
		return fmt.Errorf("cloud root changed during preservation verification")
	}
	return nil
}
