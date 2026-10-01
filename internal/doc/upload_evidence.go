package doc

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	cloud "github.com/alexgorbatchev/go-remarkable-cloud"
)

// UploadFile records the exact intended native attachment bytes.
type UploadFile struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// UploadEvidence contains no credentials and survives an uncertain commit.
type UploadEvidence struct {
	Version     int                `json:"version"`
	Title       string             `json:"title"`
	Folder      string             `json:"folder"`
	Pages       int                `json:"pages"`
	NativePages string             `json:"native_pages"`
	Result      cloud.CreateResult `json:"result"`
	Files       []UploadFile       `json:"files"`
}

func writeUploadEvidence(path string, evidence *UploadEvidence, exclusive bool) error {
	data, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding upload evidence: %w", err)
	}
	data = append(data, '\n')
	var file *os.File
	if exclusive {
		file, err = os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	} else {
		file, err = os.CreateTemp(filepath.Dir(path), ".remarkable-upload-*.json")
	}
	if err != nil {
		return fmt.Errorf("creating upload evidence: %w", err)
	}
	defer func() {
		_ = file.Close() // Successful writes check Close below.
		if !exclusive {
			_ = os.Remove(file.Name())
		} // Best-effort temporary cleanup.
	}()
	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("writing upload evidence: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("syncing upload evidence: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("closing upload evidence: %w", err)
	}
	if !exclusive {
		if err := os.Rename(file.Name(), path); err != nil {
			return fmt.Errorf("publishing upload evidence: %w", err)
		}
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return fmt.Errorf("opening evidence directory: %w", err)
	}
	defer func() { _ = dir.Close() }() // Directory close is best-effort after Sync.
	if err := dir.Sync(); err != nil {
		return fmt.Errorf("syncing evidence directory: %w", err)
	}
	return nil
}
