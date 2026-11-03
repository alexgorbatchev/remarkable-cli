package testcloud

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// catalog resolves named fixture references to protocol-addressed blobs before
// starting the server. Names select injected failures; requests use hashes only.
type catalog struct {
	payloads map[string][]byte
	hashes   map[string]string
	blobs    map[string][]byte
}

func (c *catalog) file(name string) string {
	if hash, ok := c.hashes[name]; ok {
		return hash
	}
	data := c.payloads[name]
	hash := fmt.Sprintf("%x", sha256.Sum256(data))
	c.hashes[name], c.blobs[hash] = hash, data
	return hash
}

func (c *catalog) manifest(name string, root bool, version int) string {
	if hash, ok := c.hashes[name]; ok {
		return hash
	}
	lines := strings.Split(strings.TrimSpace(string(c.payloads[name])), "\n")
	type entry struct{ name, id string }
	entries := make([]entry, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		fields := strings.Split(line, ":")
		entries = append(entries, entry{fields[0], fields[1]})
	}
	slices.SortFunc(entries, func(a, b entry) int { return strings.Compare(a.id, b.id) })
	var wire strings.Builder
	fmt.Fprintf(&wire, "%d\n", version)
	var digest []byte
	for _, e := range entries {
		var hash string
		subfiles := 0
		if root {
			hash = c.manifest(e.name, false, 3)
			subfiles = strings.Count(strings.TrimSpace(string(c.payloads[e.name])), "\n") + 1
		} else {
			hash = c.file(e.name)
		}
		fmt.Fprintf(&wire, "%s:0:%s:%d:%s\n", hash, e.id, subfiles, strconv.Itoa(len(c.blobs[hash])))
		decoded, err := hex.DecodeString(hash)
		if err != nil {
			panic(err)
		}
		digest = append(digest, decoded...)
	}
	data := []byte(wire.String())
	if root && version >= 4 {
		digest = data
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(digest))
	c.hashes[name], c.blobs[hash] = hash, data
	return hash
}

func rootPayload(opts Options) []byte {
	entries := [][2]string{{"folder-hash", "folder-1"}, {"doc-hash", "doc-1"}, {"nopdf-hash", "doc-nopdf"}, {"nostroke-hash", "doc-nostroke"}, {"suffix-hash", "doc-suffix"}, {"nocontent-hash", "doc-nocontent"}, {"corrupt-hash", "doc-corrupt"}, {"unloadable-hash", "doc-unloadable"}}
	if opts.Preset == PresetCLI {
		entries = [][2]string{{"doc-hash", "doc-1"}, {"notebook-hash", "notebook-1"}, {"blank-hash", "blank-1"}, {"unloadable-hash", "unloadable-1"}, {"nocontent-hash", "nocontent-1"}, {"urilinks-hash", "urilinks-1"}}
	}
	var result strings.Builder
	for _, e := range entries {
		if len(opts.Documents) == 0 || slices.Contains(opts.Documents, e[1]) {
			fmt.Fprintf(&result, "%s:%s\n", e[0], e[1])
		}
	}
	return []byte(result.String())
}
