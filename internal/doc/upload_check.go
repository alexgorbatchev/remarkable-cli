package doc

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	cloud "github.com/alexgorbatchev/go-remarkable-cloud"
)

// ReadUploadEvidence validates local recovery identity before authentication.
func ReadUploadEvidence(path string) (*UploadEvidence, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading upload evidence: %w", err)
	}
	var evidence UploadEvidence
	if err := json.Unmarshal(data, &evidence); err != nil {
		return nil, fmt.Errorf("decoding upload evidence: %w", err)
	}
	if evidence.Version != 1 || !isUUID(evidence.Result.ID) || evidence.Pages < 1 || len(evidence.Files) != 4 || !uploadHashValid(evidence.Result.DocumentHash) {
		return nil, fmt.Errorf("upload evidence has incomplete or invalid recovery identity")
	}
	seen := make(map[string]bool)
	for _, file := range evidence.Files {
		suffix, found := strings.CutPrefix(file.Name, evidence.Result.ID)
		if !found || seen[suffix] || file.Size < 0 || !uploadHashValid(file.SHA256) || (suffix != ".pdf" && suffix != ".metadata" && suffix != ".content" && suffix != ".pagedata") {
			return nil, fmt.Errorf("invalid upload evidence file %q", file.Name)
		}
		seen[suffix] = true
	}
	return &evidence, nil
}

func uploadHashValid(hash string) bool {
	data, err := hex.DecodeString(hash)
	return err == nil && len(data) == 32
}

// CheckUpload performs only fresh cloud reads. It neither retries creation nor
// overwrites the original evidence; a changed document needs manual inspection.
func CheckUpload(ctx context.Context, client *cloud.Client, path string) (*UploadEvidence, error) {
	evidence, err := ReadUploadEvidence(path)
	if err != nil {
		return nil, err
	}
	root, err := client.GetRootState(ctx)
	if err != nil {
		return evidence, err
	}
	manifest, err := freshUploadManifest(ctx, client, root.Hash, "root.docSchema")
	if err != nil {
		return evidence, err
	}
	entry := manifest.Find(evidence.Result.ID)
	if entry == nil {
		return evidence, fmt.Errorf("upload UUID %s is absent from the current root snapshot; retain evidence and inspect before retrying", evidence.Result.ID)
	}
	if entry.Hash != evidence.Result.DocumentHash {
		return evidence, fmt.Errorf("upload UUID %s has a changed document hash; inspect it before retrying", entry.ID)
	}
	files, err := freshUploadManifest(ctx, client, entry.Hash, entry.ID+".docSchema")
	if err != nil {
		return evidence, err
	}
	if len(files.Entries) != len(evidence.Files) {
		return evidence, fmt.Errorf("upload recovery file set differs")
	}
	for _, file := range evidence.Files {
		entry := files.Find(file.Name)
		if entry == nil || entry.Hash != file.SHA256 || entry.Size != file.Size {
			return evidence, fmt.Errorf("upload recovery file association differs: %s", file.Name)
		}
		data, err := client.GetBlobFresh(ctx, entry.Hash, entry.ID)
		if err != nil {
			return evidence, err
		}
		if archiveHash(data) != file.SHA256 || int64(len(data)) != file.Size {
			return evidence, fmt.Errorf("fresh upload recovery bytes differ: %s", file.Name)
		}
	}
	current, err := client.GetRootState(ctx)
	if err != nil {
		return evidence, err
	}
	if *current != *root {
		return evidence, fmt.Errorf("cloud root changed during recovery; run upload-check again")
	}
	evidence.Result.State = cloud.UpdateVerified
	return evidence, nil
}
