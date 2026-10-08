package testcloud

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/alexgorbatchev/go-remarkable-cloud"
)

// Failure specifies an endpoint and request threshold where the mock server
// injects a failure or an alternative payload.
type Failure struct {
	Path  string
	After int
	Body  string // if empty, responds with HTTP 502 Bad Gateway
}

// Preset identifies a preconfigured document catalog.
type Preset string

const (
	PresetDoc Preset = "doc"
	PresetCLI Preset = "cli"
)

// Options configures a mock sync v3 server.
type Options struct {
	SchemaVersion int
	Preset        Preset
	Documents     []string // optional subset of documents to include in the root manifest
	Failure       *Failure
	ExtraFiles    map[string][]byte
}

// Fixtures holds binary test fixture contents loaded from internal/doc/testdata.
type Fixtures struct {
	PDF           []byte
	Strokes       []byte
	BlankPDF      []byte
	UnloadablePDF []byte
}

// LoadFixtures loads binary test fixtures from internal/doc/testdata.
func LoadFixtures(t *testing.T) Fixtures {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Join(filepath.Dir(file), "..", "doc", "testdata")

	var f Fixtures
	for name, dst := range map[string]*[]byte{
		"linked_pages.pdf":        &f.PDF,
		"oct1_notes_strokes.rm":   &f.Strokes,
		"oct1_notes_template.pdf": &f.BlankPDF,
		"unloadable_page.pdf":     &f.UnloadablePDF,
	} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("reading test fixture %s: %v", name, err)
		}
		*dst = data
	}
	return f
}


// Server wraps httptest.Server and exposes cloud client creation helpers.
type Server struct {
	*httptest.Server
	URL string
}

// NewClient returns a *cloud.Client connected to the mock server using paired test credentials.
func (s *Server) NewClient(t *testing.T) *cloud.Client {
	t.Helper()
	c, err := cloud.NewClient(
		cloud.WithConfig(&cloud.Config{DeviceToken: "dev", UserToken: "usr"}),
		cloud.WithEndpoints(&cloud.Endpoints{WebappHost: s.URL, StorageHost: s.URL}),
	)
	if err != nil {
		t.Fatalf("creating test cloud client: %v", err)
	}
	return c
}

// New starts a mock sync v3 cloud server according to opts.
func New(t *testing.T, opts Options) *Server {
	t.Helper()

	f := LoadFixtures(t)

	schemaVer := opts.SchemaVersion
	if schemaVer == 0 {
		if opts.Preset == PresetCLI {
			schemaVer = 3
		} else {
			schemaVer = 4
		}
	}

	var failedPathRequests atomic.Int32

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if opts.Failure != nil && r.URL.Path == opts.Failure.Path {
			if int(failedPathRequests.Add(1)) > opts.Failure.After {
				if opts.Failure.Body != "" {
					_, _ = w.Write([]byte(opts.Failure.Body))
					return
				}
				http.Error(w, "injected cloud failure", http.StatusBadGateway)
				return
			}
		}

		if data, ok := opts.ExtraFiles[r.URL.Path]; ok {
			_, _ = w.Write(data)
			return
		}

		switch r.URL.Path {
		case "/token/v2/user", "/token/json/2/user/new":
			_, _ = w.Write([]byte("mock-token"))
		case "/token/json/2/device/new":
			_, _ = w.Write([]byte("mock-device-token"))

		case "/sync/v3/root":
			_, _ = fmt.Fprintf(w, `{"hash":"root-hash","generation":1,"schemaVersion":%d}`, schemaVer)

		case "/sync/v3/files/root-hash":
			serveRootManifest(w, opts, schemaVer)

		// PresetDoc documents
		case "/sync/v3/files/folder-hash":
			_, _ = w.Write([]byte("folder-meta-hash:folder-1.metadata:0:50\n"))
		case "/sync/v3/files/folder-meta-hash":
			_, _ = w.Write([]byte(`{"visibleName":"My Folder","type":"CollectionType"}`))

		case "/sync/v3/files/doc-hash":
			if opts.Preset == PresetCLI {
				_, _ = w.Write([]byte("meta-hash:doc-1.metadata:0:50\ncontent-hash:doc-1.content:0:100\npdf-hash:doc-1.pdf:0:1000\nstroke-hash:doc-1/page-1.rm:0:200\n"))
			} else {
				_, _ = w.Write([]byte("meta-hash:doc-1.metadata:0:50\ncontent-hash:doc-1.content:0:100\npdf-hash:doc-1.pdf:0:1000\nstroke-hash:doc-1/page-uuid-1.rm:0:200\n"))
			}
		case "/sync/v3/files/meta-hash":
			if opts.Preset == PresetCLI {
				_, _ = w.Write([]byte(`{"visibleName":"My Document","type":"DocumentType","lastModified":"2026-09-29T10:00:00Z"}`))
			} else {
				_, _ = w.Write([]byte(`{"visibleName":"My Document","type":"DocumentType","lastModified":"2026-09-29T10:00:00Z","parent":"folder-1"}`))
			}
		case "/sync/v3/files/content-hash":
			if opts.Preset == PresetCLI {
				_, _ = w.Write([]byte(`{"fileType":"pdf","pageCount":1,"pages":["page-1"]}`))
			} else {
				_, _ = w.Write([]byte(`{"fileType":"pdf","pageCount":1,"pages":["page-uuid-1"]}`))
			}
		case "/sync/v3/files/pdf-hash":
			_, _ = w.Write(f.PDF)
		case "/sync/v3/files/stroke-hash":
			_, _ = w.Write(f.Strokes)

		case "/sync/v3/files/nopdf-hash":
			_, _ = w.Write([]byte("nopdf-meta:doc-nopdf.metadata:0:50\nnopdf-content:doc-nopdf.content:0:50\n"))
		case "/sync/v3/files/nopdf-meta":
			_, _ = w.Write([]byte(`{"visibleName":"Empty Doc","type":"DocumentType"}`))
		case "/sync/v3/files/nopdf-content":
			_, _ = w.Write([]byte(`{"fileType":"notebook","pageCount":1,"cPages":{"pages":[{"id":"p1"}]}}`))

		case "/sync/v3/files/nostroke-hash":
			_, _ = w.Write([]byte("nostroke-meta:doc-nostroke.metadata:0:50\nnostroke-content:doc-nostroke.content:0:50\npdf-hash:doc-nostroke.pdf:0:1000\n"))
		case "/sync/v3/files/nostroke-meta":
			_, _ = w.Write([]byte(`{"visibleName":"No Stroke Doc","type":"DocumentType"}`))
		case "/sync/v3/files/nostroke-content":
			_, _ = w.Write([]byte(`{"fileType":"pdf","pageCount":1,"pages":["page-nostroke-1"]}`))

		case "/sync/v3/files/suffix-hash":
			_, _ = w.Write([]byte("suffix-meta:doc-suffix.metadata:0:50\nsuffix-content:doc-suffix.content:0:50\npdf-hash:other_name.pdf:0:1000\nstroke-hash:random_page.rm:0:200\n"))
		case "/sync/v3/files/suffix-meta":
			_, _ = w.Write([]byte(`{"visibleName":"Suffix Doc","type":"DocumentType"}`))
		case "/sync/v3/files/suffix-content":
			_, _ = w.Write([]byte(`{"fileType":"pdf","pageCount":1,"pages":["random_page"]}`))

		case "/sync/v3/files/nocontent-hash":
			if opts.Preset == PresetCLI {
				_, _ = w.Write([]byte("nocontent-meta:nocontent-1.metadata:0:50\n"))
			} else {
				_, _ = w.Write([]byte("nocontent-meta:doc-nocontent.metadata:0:50\npdf-hash:doc-nocontent.pdf:0:1000\n"))
			}
		case "/sync/v3/files/nocontent-meta":
			if opts.Preset == PresetCLI {
				_, _ = w.Write([]byte(`{"visibleName":"No Content Document","type":"DocumentType"}`))
			} else {
				_, _ = w.Write([]byte(`{"visibleName":"No Content Doc","type":"DocumentType"}`))
			}

		case "/sync/v3/files/corrupt-hash":
			_, _ = w.Write([]byte("corrupt-meta:doc-corrupt.metadata:0:50\ncorrupt-pdf:doc-corrupt.pdf:0:50\n"))
		case "/sync/v3/files/corrupt-meta":
			_, _ = w.Write([]byte(`{"visibleName":"Corrupt Doc","type":"DocumentType"}`))
		case "/sync/v3/files/corrupt-pdf":
			_, _ = w.Write([]byte("not-a-valid-pdf"))

		case "/sync/v3/files/unloadable-hash":
			if opts.Preset == PresetCLI {
				_, _ = w.Write([]byte("unloadable-meta:unloadable-1.metadata:0:50\nunloadable-content:unloadable-1.content:0:50\nunloadable-pdf:unloadable-1.pdf:0:500\n"))
			} else {
				_, _ = w.Write([]byte("unloadable-meta:doc-unloadable.metadata:0:50\nunloadable-content:doc-unloadable.content:0:50\nunloadable-pdf:doc-unloadable.pdf:0:500\n"))
			}
		case "/sync/v3/files/unloadable-meta":
			if opts.Preset == PresetCLI {
				_, _ = w.Write([]byte(`{"visibleName":"Unloadable Page PDF","type":"DocumentType"}`))
			} else {
				_, _ = w.Write([]byte(`{"visibleName":"Unloadable Page Doc","type":"DocumentType"}`))
			}
		case "/sync/v3/files/unloadable-content":
			_, _ = w.Write([]byte(`{"fileType":"pdf","pageCount":2,"pages":["unloadable-page-1","unloadable-page-2"]}`))
		case "/sync/v3/files/unloadable-pdf":
			_, _ = w.Write(f.UnloadablePDF)

		// PresetCLI additional documents
		case "/sync/v3/files/blank-hash":
			_, _ = w.Write([]byte("blank-meta:blank-1.metadata:0:50\nblank-content:blank-1.content:0:50\nblank-pdf:blank-1.pdf:0:1000\n"))
		case "/sync/v3/files/blank-meta":
			_, _ = w.Write([]byte(`{"visibleName":"Unlinked PDF","type":"DocumentType"}`))
		case "/sync/v3/files/blank-content":
			_, _ = w.Write([]byte(`{"fileType":"pdf","pageCount":1,"pages":["blank-page-1"]}`))
		case "/sync/v3/files/blank-pdf":
			_, _ = w.Write(f.BlankPDF)

		case "/sync/v3/files/notebook-hash":
			_, _ = w.Write([]byte("notebook-meta:notebook-1.metadata:0:50\nnotebook-content:notebook-1.content:0:50\n"))
		case "/sync/v3/files/notebook-meta":
			_, _ = w.Write([]byte(`{"visibleName":"Handwritten Notebook","type":"DocumentType"}`))
		case "/sync/v3/files/notebook-content":
			_, _ = w.Write([]byte(`{"fileType":"notebook","pageCount":1,"cPages":{"pages":[{"id":"notebook-page-1"}]}}`))

		case "/sync/v3/files/urilinks-hash":
			_, _ = w.Write([]byte("urilinks-meta:urilinks-1.metadata:0:50\nurilinks-content:urilinks-1.content:0:50\nurilinks-pdf:urilinks-1.pdf:0:1100\n"))
		case "/sync/v3/files/urilinks-meta":
			_, _ = w.Write([]byte(`{"visibleName":"URI Links PDF","type":"DocumentType"}`))
		case "/sync/v3/files/urilinks-content":
			_, _ = w.Write([]byte(`{"fileType":"pdf","pageCount":2,"pages":["urilinks-page-1","urilinks-page-2"]}`))

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	t.Cleanup(ts.Close)

	return &Server{
		Server: ts,
		URL:    ts.URL,
	}
}

func serveRootManifest(w http.ResponseWriter, opts Options, schemaVer int) {
	type entry struct {
		id   string
		line string
	}

	var all []entry
	if opts.Preset == PresetCLI {
		all = []entry{
			{"doc-1", "doc-hash:doc-1:0:100\n"},
			{"notebook-1", "notebook-hash:notebook-1:0:100\n"},
			{"blank-1", "blank-hash:blank-1:0:100\n"},
			{"unloadable-1", "unloadable-hash:unloadable-1:0:100\n"},
			{"nocontent-1", "nocontent-hash:nocontent-1:0:100\n"},
			{"urilinks-1", "urilinks-hash:urilinks-1:0:100\n"},
		}
	} else {
		all = []entry{
			{"folder-1", "folder-hash:folder-1:0:10\n"},
			{"doc-1", "doc-hash:doc-1:0:100\n"},
			{"doc-nopdf", "nopdf-hash:doc-nopdf:0:100\n"},
			{"doc-nostroke", "nostroke-hash:doc-nostroke:0:100\n"},
			{"doc-suffix", "suffix-hash:doc-suffix:0:100\n"},
			{"doc-nocontent", "nocontent-hash:doc-nocontent:0:100\n"},
			{"doc-corrupt", "corrupt-hash:doc-corrupt:0:100\n"},
			{"doc-unloadable", "unloadable-hash:doc-unloadable:0:100\n"},
		}
	}

	if schemaVer >= 4 {
		_, _ = fmt.Fprintf(w, "%d\n0:.:75:211191896\n", schemaVer)
	}

	for _, e := range all {
		if len(opts.Documents) == 0 || slices.Contains(opts.Documents, e.id) {
			_, _ = w.Write([]byte(e.line))
		}
	}
}
