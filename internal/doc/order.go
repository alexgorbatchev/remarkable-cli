package doc

import (
	"cmp"
	"slices"
	"strings"

	cloud "github.com/alexgorbatchev/go-remarkable-cloud"
)

// folderPath is the chain of folders from the root down to an item's parent.
// reachable is false when the chain ends at an ID that is not a listed folder,
// such as a trashed folder, a document, or a folder cycle.
type folderPath struct {
	folders   []*cloud.Item
	reachable bool
}

// sortItems orders items by folder path, then visible name, then ID, so cloud
// listings print identically on every run regardless of the order the cloud
// library returns them in. Items whose parent chain never reaches the root
// follow all others, ordered by parent ID, then visible name, then ID.
//
// Paths compare folder by folder from the root, each folder by name then ID,
// so a folder's contents precede its subfolders' contents, and two folders
// with the same name never interleave. Names and IDs compare byte-wise.
func sortItems(items []*cloud.Item) {
	paths := folderPaths(items)
	slices.SortFunc(items, func(a, b *cloud.Item) int {
		pa, pb := paths[a.Metadata.Parent], paths[b.Metadata.Parent]
		if pa.reachable != pb.reachable {
			if pa.reachable {
				return -1
			}
			return 1
		}
		var byPath int
		if pa.reachable {
			byPath = slices.CompareFunc(pa.folders, pb.folders, compareSiblings)
		} else {
			byPath = strings.Compare(a.Metadata.Parent, b.Metadata.Parent)
		}
		return cmp.Or(byPath, compareSiblings(a, b))
	})
}

// compareSiblings orders items that share a parent by visible name, then ID.
func compareSiblings(a, b *cloud.Item) int {
	return cmp.Or(
		strings.Compare(a.Metadata.VisibleName, b.Metadata.VisibleName),
		strings.Compare(a.ID, b.ID),
	)
}

// folderPaths resolves the folder path of every parent ID that items refer to.
func folderPaths(items []*cloud.Item) map[string]folderPath {
	folders := make(map[string]*cloud.Item)
	for _, it := range items {
		if it.IsCollection() {
			folders[it.ID] = it
		}
	}

	paths := map[string]folderPath{"": {reachable: true}}
	resolving := make(map[string]bool)
	var resolve func(id string) folderPath
	resolve = func(id string) folderPath {
		if p, ok := paths[id]; ok {
			return p
		}
		folder, ok := folders[id]
		if !ok || resolving[id] {
			return folderPath{}
		}
		resolving[id] = true
		parent := resolve(folder.Metadata.Parent)
		delete(resolving, id)

		p := folderPath{}
		if parent.reachable {
			p = folderPath{folders: append(slices.Clone(parent.folders), folder), reachable: true}
		}
		paths[id] = p
		return p
	}

	for _, it := range items {
		resolve(it.Metadata.Parent)
	}
	return paths
}
