package doc

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cloud "github.com/alexgorbatchev/go-remarkable-cloud"
	"github.com/alexgorbatchev/remarkable-cli/internal/testcloud"
)

func TestSyncRejectsUnsafeDocumentNames(t *testing.T) {
	for _, name := range []string{"..", " .. ", ".", " . ", "\x00", "   "} {
		t.Run(fmt.Sprintf("%q", name), func(t *testing.T) {
			metadata, err := json.Marshal(cloud.ItemMetadata{VisibleName: name, Type: "DocumentType"})
			if err != nil {
				t.Fatal(err)
			}
			server := testcloud.New(t, testcloud.Options{Preset: testcloud.PresetDoc, ExtraFiles: map[string][]byte{
				"/sync/v3/files/meta-hash": metadata,
			}})
			base := t.TempDir()
			results, err := SyncDocument(t.Context(), server.NewClient(t), "doc-1", SyncDocOptions{OutputDir: filepath.Join(base, "out"), Format: "rm", Force: true})
			if err == nil || !strings.Contains(err.Error(), "doc-1") || !strings.Contains(err.Error(), "invalid output directory name") || len(results) != 0 {
				t.Fatalf("results=%+v, err=%v", results, err)
			}
			entries, err := os.ReadDir(base)
			if err != nil || len(entries) != 0 {
				t.Fatalf("unsafe name wrote output: %v, %v", entries, err)
			}
		})
	}
}
