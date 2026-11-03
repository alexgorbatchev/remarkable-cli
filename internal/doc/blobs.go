package doc

import (
	"context"
	"errors"
	"fmt"

	cloud "github.com/alexgorbatchev/go-remarkable-cloud"
)

// ErrNoBackgroundPDF identifies a document without a PDF attachment.
var ErrNoBackgroundPDF = errors.New("document has no background PDF")

func backgroundPDF(ctx context.Context, client *cloud.Client, item *cloud.Item, manifest *cloud.Manifest) ([]byte, error) {
	entry := manifest.Find(item.ID + ".pdf")
	if entry == nil {
		entry = manifest.FindSuffix(".pdf")
	}
	if entry == nil {
		return nil, fmt.Errorf("%w: %s", ErrNoBackgroundPDF, item.ID)
	}
	data, err := client.GetBlob(ctx, entry.Hash, entry.ID)
	if err != nil {
		return nil, fmt.Errorf("downloading background PDF: %w", err)
	}
	return data, nil
}

// strokeBlob distinguishes an absent native attachment from a failed download.
func strokeBlob(ctx context.Context, client *cloud.Client, item *cloud.Item, manifest *cloud.Manifest, pageID string) ([]byte, bool, error) {
	entry := manifest.Find(item.ID + "/" + pageID + ".rm")
	if entry == nil {
		entry = manifest.FindSuffix(pageID + ".rm")
	}
	if entry == nil {
		return nil, false, nil
	}
	data, err := client.GetBlob(ctx, entry.Hash, entry.ID)
	if err != nil {
		return nil, true, fmt.Errorf("downloading stroke blob: %w", err)
	}
	return data, true, nil
}
