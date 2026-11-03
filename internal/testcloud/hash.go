package testcloud

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
)

// FileHash addresses ordinary file bytes and version-four root manifests.
func FileHash(data []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

// ManifestHash addresses a version-three manifest by its child hashes in ID
// order. Fixture manifests must contain valid five-field protocol entries.
func ManifestHash(data []byte) string {
	var entries [][]string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n")[1:] {
		entries = append(entries, strings.Split(line, ":"))
	}
	slices.SortFunc(entries, func(a, b []string) int { return strings.Compare(a[2], b[2]) })
	var digest []byte
	for _, entry := range entries {
		hash, err := hex.DecodeString(entry[0])
		if err != nil || len(hash) != sha256.Size {
			panic("invalid fixture hash: " + entry[0])
		}
		digest = append(digest, hash...)
	}
	return FileHash(digest)
}
