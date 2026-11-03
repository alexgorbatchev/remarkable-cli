package doc

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"testing"

	cloud "github.com/alexgorbatchev/go-remarkable-cloud"
	"github.com/alexgorbatchev/remarkable-cli/internal/testcloud"
)

func documentReads(output string) map[string]func(context.Context, *cloud.Client, string) error {
	return map[string]func(context.Context, *cloud.Client, string) error{
		"search": func(ctx context.Context, c *cloud.Client, id string) error {
			_, err := SearchDocument(ctx, c, id, SearchQuery{Text: "October"})
			return err
		},
		"links": func(ctx context.Context, c *cloud.Client, id string) error {
			_, err := GetLinks(ctx, c, id, 0)
			return err
		},
		"pdf": func(ctx context.Context, c *cloud.Client, id string) error {
			return Cat(ctx, c, id, 0, "pdf", io.Discard)
		},
		"text": func(ctx context.Context, c *cloud.Client, id string) error {
			return Cat(ctx, c, id, 0, "text", io.Discard)
		},
		"render": func(ctx context.Context, c *cloud.Client, id string) error {
			return RenderPage(ctx, c, id, 0, 72, filepath.Join(output, "page.png"))
		},
		"sync": func(ctx context.Context, c *cloud.Client, id string) error {
			_, err := SyncDocument(ctx, c, id, SyncDocOptions{OutputDir: output, Format: "png", DPI: 72})
			return err
		},
	}
}

func TestDocumentReadsSharePDFAndManifestPolicy(t *testing.T) {
	for name, read := range documentReads(t.TempDir()) {
		t.Run(name, func(t *testing.T) {
			missing := testcloud.New(t, testcloud.Options{Preset: testcloud.PresetDoc, Documents: []string{"doc-nopdf"}})
			if err := read(t.Context(), missing.NewClient(t), "doc-nopdf"); !errors.Is(err, ErrNoBackgroundPDF) {
				t.Fatalf("missing PDF = %v", err)
			}
			// No disk cache: a second manifest request fails, so a successful read proves
			// resolution and attachment lookup share the immutable document snapshot.
			snapshot := testcloud.New(t, testcloud.Options{Preset: testcloud.PresetDoc, Documents: []string{"doc-1"}, Failure: &testcloud.Failure{Path: "/sync/v3/files/doc-hash", After: 1}})
			if err := read(t.Context(), snapshot.NewClient(t), "doc-1"); err != nil {
				t.Fatal(err)
			}
		})
	}
}
