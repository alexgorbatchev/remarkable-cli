package doc

import (
	"cmp"
	"slices"
	"strings"

	cloud "github.com/alexgorbatchev/go-remarkable-cloud"
	"golang.org/x/text/cases"
)

// folderPath is the chain of folders from the root down to an item's parent.
// reachable is false when the chain ends at an ID that is not a listed folder,
// such as a trashed folder, a document, or a folder cycle.
type folderPath struct {
	folders   []*cloud.Item
	reachable bool
}

// itemOrder compares cloud items in listing order.
type itemOrder struct {
	folded map[*cloud.Item]string
	paths  map[string]folderPath
}

// sortItems orders items by folder path, then name, then ID, so cloud listings
// print identically on every run regardless of the order the cloud library
// returns them in. Items whose parent chain never reaches the root follow all
// others, ordered by parent ID (bytes), then name, then ID.
//
// Paths compare folder by folder from the root, each folder by name then ID,
// so a folder's contents precede its subfolders' contents, and two folders
// with the same name never interleave.
//
// Names compare by their Unicode full case folding first, so case does not
// decide the order, as with the case-insensitive --query filter, and the
// result is closer to an alphabetical view such as the tablet's than raw
// bytes are. Names with equal foldings then compare by their UTF-8 bytes, and
// IDs by bytes, which keeps the order total and identical on every run.
func sortItems(items []*cloud.Item) {
	fold := cases.Fold()
	o := itemOrder{
		folded: make(map[*cloud.Item]string, len(items)),
		paths:  folderPaths(items),
	}
	for _, it := range items {
		o.folded[it] = fold.String(it.Metadata.VisibleName)
	}
	slices.SortFunc(items, o.compare)
}

func (o itemOrder) compare(a, b *cloud.Item) int {
	pa, pb := o.paths[a.Metadata.Parent], o.paths[b.Metadata.Parent]
	if pa.reachable != pb.reachable {
		if pa.reachable {
			return -1
		}
		return 1
	}
	var byPath int
	if pa.reachable {
		byPath = slices.CompareFunc(pa.folders, pb.folders, o.compareSiblings)
	} else {
		byPath = strings.Compare(a.Metadata.Parent, b.Metadata.Parent)
	}
	return cmp.Or(byPath, o.compareSiblings(a, b))
}

// compareSiblings orders items that share a parent by case-folded name, then
// name bytes, then ID.
func (o itemOrder) compareSiblings(a, b *cloud.Item) int {
	return cmp.Or(
		strings.Compare(o.folded[a], o.folded[b]),
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
