package testcloud

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/alexgorbatchev/go-remarkable-cloud"
)

func TestTestCloudPresetsAndFailure(t *testing.T) {
	t.Run("doc preset schema and documents", func(t *testing.T) {
		ts := New(t, Options{Preset: PresetDoc})
		client := ts.NewClient(t)

		item, err := client.Resolve(context.Background(), "doc-1")
		if err != nil {
			t.Fatalf("resolve doc-1: %v", err)
		}
		if item.Metadata.VisibleName != "My Document" {
			t.Errorf("doc-1 name = %q, want My Document", item.Metadata.VisibleName)
		}

		folder, err := client.Resolve(context.Background(), "folder-1")
		if err != nil {
			t.Fatalf("resolve folder-1: %v", err)
		}
		if folder.Metadata.VisibleName != "My Folder" {
			t.Errorf("folder-1 name = %q, want My Folder", folder.Metadata.VisibleName)
		}
	})

	t.Run("cli preset schema and documents", func(t *testing.T) {
		ts := New(t, Options{
			Preset: PresetCLI,
			ExtraFiles: map[string][]byte{
				"/sync/v3/files/custom-file": []byte("custom-content"),
			},
		})
		client := ts.NewClient(t)

		item, err := client.Resolve(context.Background(), "doc-1")
		if err != nil {
			t.Fatalf("resolve doc-1: %v", err)
		}
		if item.Metadata.VisibleName != "My Document" {
			t.Errorf("doc-1 name = %q, want My Document", item.Metadata.VisibleName)
		}

		resp, err := http.Get(ts.URL + "/sync/v3/files/custom-file")
		if err != nil {
			t.Fatalf("get custom file: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("read custom file: %v", err)
		}
		if string(body) != "custom-content" {
			t.Errorf("custom file body = %q, want custom-content", string(body))
		}
	})

	t.Run("injected failure", func(t *testing.T) {
		ts := New(t, Options{
			Preset: PresetDoc,
			Failure: &Failure{
				Path:  "/sync/v3/root",
				After: 0,
			},
		})
		client := ts.NewClient(t)

		_, err := client.Resolve(context.Background(), "doc-1")
		if err == nil {
			t.Fatal("expected error on injected failure")
		}
		var statusErr *cloud.StatusError
		if !errors.As(err, &statusErr) || statusErr.StatusCode != 502 {
			t.Errorf("err = %v, want StatusError with 502", err)
		}

	})
}
