package doc

import (
	"bytes"
	"context"
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	cloud "github.com/alexgorbatchev/go-remarkable-cloud"
	"github.com/alexgorbatchev/remarkable-cli/internal/agent"
)

type listingFixtureItem struct {
	id, name, itemType, parent string
}

// orderFixture is a library whose IDs deliberately disagree with its names, so
// an order that sorts by ID before name, or that ignores folder identity, fails.
var orderFixture = []listingFixtureItem{
	{"d-agenda", "Agenda", "DocumentType", ""},
	{"f-3", "Alpha", "CollectionType", ""},
	{"f-2", "Beta", "CollectionType", ""},
	{"f-1", "Beta", "CollectionType", ""},
	{"d-twin-9", "Twin", "DocumentType", ""},
	{"d-twin-8", "Twin", "DocumentType", ""},
	{"d-zed", "Zed", "DocumentType", ""},
	{"d-lower", "alpha notes", "DocumentType", ""},
	{"f-inner", "Inner", "CollectionType", "f-3"},
	{"d-note", "Note", "DocumentType", "f-3"},
	{"d-deep", "Deep", "DocumentType", "f-inner"},
	{"d-zulu", "Zulu", "DocumentType", "f-1"},
	{"d-same", "Same", "DocumentType", "f-2"},
	// The cloud library omits trashed items, so d-lost's parent is missing.
	{"f-trashed", "Old", "CollectionType", "trash"},
	{"d-lost", "Lost", "DocumentType", "f-trashed"},
	// Folders that are each other's parent never reach the root.
	{"f-loop-a", "Loop A", "CollectionType", "f-loop-b"},
	{"f-loop-b", "Loop B", "CollectionType", "f-loop-a"},
	// A document is not a folder, so its children never reach the root.
	{"d-attached", "Attached", "DocumentType", "d-zed"},
}

// wantListOrder is orderFixture by folder path, then name, then ID, followed by
// the items whose parent chain never reaches the root, ordered by parent ID.
var wantListOrder = []string{
	"d-agenda", "f-3", "f-1", "f-2", "d-twin-8", "d-twin-9", "d-zed", "d-lower",
	"f-inner", "d-note",
	"d-deep",
	"d-zulu",
	"d-same",
	"d-attached", "f-loop-b", "f-loop-a", "d-lost",
}

// newShuffledListingClient serves orderFixture with root manifest entries in a
// seed-specific order. The cloud library fetches item manifests concurrently,
// so the order it returns also varies between calls with the same seed.
func newShuffledListingClient(t *testing.T, seed uint64) *cloud.Client {
	t.Helper()

	items := slices.Clone(orderFixture)
	rand.New(rand.NewPCG(seed, seed)).Shuffle(len(items), func(i, j int) {
		items[i], items[j] = items[j], items[i]
	})

	var root strings.Builder
	files := make(map[string]string)
	for _, it := range items {
		fmt.Fprintf(&root, "%s-schema:%s:0:1\n", it.id, it.id)
		files[it.id+"-schema"] = fmt.Sprintf("%s-meta:%s.metadata:0:1\n", it.id, it.id)
		files[it.id+"-meta"] = fmt.Sprintf(`{"visibleName":%q,"type":%q,"parent":%q}`, it.name, it.itemType, it.parent)
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sync/v3/root":
			w.Write([]byte(`{"hash":"root-hash","generation":1,"schemaVersion":3}`))
			return
		case "/sync/v3/files/root-hash":
			w.Write([]byte(root.String()))
			return
		}
		body, ok := files[strings.TrimPrefix(r.URL.Path, "/sync/v3/files/")]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Write([]byte(body))
	}))
	t.Cleanup(ts.Close)

	client, err := cloud.NewClient(
		cloud.WithConfig(&cloud.Config{DeviceToken: "dev", UserToken: "usr"}),
		cloud.WithEndpoints(&cloud.Endpoints{WebappHost: ts.URL, StorageHost: ts.URL}),
	)
	if err != nil {
		t.Fatalf("creating cloud client: %v", err)
	}
	return client
}

var listingSeeds = []uint64{1, 2, 3, 4, 5, 6, 7, 8}

func summaryIDs(items []ItemSummary) []string {
	ids := make([]string, 0, len(items))
	for _, it := range items {
		ids = append(ids, it.ID)
	}
	return ids
}

func TestListOrderIsDeterministic(t *testing.T) {
	tests := []struct {
		name            string
		folder, docType string
		limit           int
		want            []string
	}{
		{"all items", "", "", 0, wantListOrder},
		{"limit takes the first items", "", "", 3, []string{"d-agenda", "f-3", "f-1"}},
		{"limit applies after filters", "", "DocumentType", 4, []string{"d-agenda", "d-twin-8", "d-twin-9", "d-zed"}},
		{"folder contents", "f-3", "", 0, []string{"f-inner", "d-note"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, seed := range listingSeeds {
				client := newShuffledListingClient(t, seed)
				for call := range 2 {
					items, err := List(context.Background(), client, tt.folder, tt.docType, "", tt.limit)
					if err != nil {
						t.Fatalf("seed %d call %d: List failed: %v", seed, call, err)
					}
					if got := summaryIDs(items); !slices.Equal(got, tt.want) {
						t.Fatalf("seed %d call %d: List order\n got: %v\nwant: %v", seed, call, got, tt.want)
					}
				}
			}
		})
	}
}

func TestBuildTreeOrderIsDeterministic(t *testing.T) {
	tests := []struct {
		mode string
		want string
	}{
		{"0", strings.Join([]string{
			"/",
			"├── Agenda",
			"├── Alpha/",
			"│   ├── Inner/",
			"│   │   └── Deep",
			"│   └── Note",
			"├── Beta/",
			"│   └── Zulu",
			"├── Beta/",
			"│   └── Same",
			"├── Twin",
			"├── Twin",
			"├── Zed",
			"└── alpha notes",
			"",
		}, "\n")},
		{"1", strings.Join([]string{
			"* /",
			"  * Agenda",
			"  * Alpha/",
			"    * Inner/",
			"      * Deep",
			"    * Note",
			"  * Beta/",
			"    * Zulu",
			"  * Beta/",
			"    * Same",
			"  * Twin",
			"  * Twin",
			"  * Zed",
			"  * alpha notes",
			"",
		}, "\n")},
	}
	for _, tt := range tests {
		t.Run("AGENT="+tt.mode, func(t *testing.T) {
			t.Setenv("AGENT", tt.mode)
			for _, seed := range listingSeeds {
				client := newShuffledListingClient(t, seed)
				for call := range 2 {
					tree, err := BuildTree(context.Background(), client)
					if err != nil {
						t.Fatalf("seed %d call %d: BuildTree failed: %v", seed, call, err)
					}
					var out bytes.Buffer
					agent.PrintTree(&out, tree)
					if out.String() != tt.want {
						t.Fatalf("seed %d call %d: tree output\n got:\n%s\nwant:\n%s", seed, call, out.String(), tt.want)
					}
				}
			}
		})
	}
}
