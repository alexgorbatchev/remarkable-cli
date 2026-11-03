package testcloud

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
	URL    string
	Hashes map[string]string
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

// New starts a mock sync v3 cloud server with protocol-valid blob hashes.
func New(t *testing.T, opts Options) *Server {
	t.Helper()
	version := opts.SchemaVersion
	if version == 0 {
		version = 4
	}
	payloads := fixturePayloads(opts, LoadFixtures(t))
	for path, data := range opts.ExtraFiles {
		payloads[strings.TrimPrefix(path, "/sync/v3/files/")] = data
	}
	payloads["root-hash"] = rootPayload(opts)
	catalog := &catalog{payloads: payloads, hashes: map[string]string{}, blobs: map[string][]byte{}}
	root := catalog.manifest("root-hash", true, version)
	for name := range payloads {
		catalog.file(name)
	}
	failurePath := ""
	if opts.Failure != nil {
		failurePath = opts.Failure.Path
		if name, ok := strings.CutPrefix(failurePath, "/sync/v3/files/"); ok {
			failurePath = "/sync/v3/files/" + catalog.hashes[name]
		}
	}
	var failedPathRequests atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if opts.Failure != nil && r.URL.Path == failurePath && int(failedPathRequests.Add(1)) > opts.Failure.After {
			if opts.Failure.Body != "" {
				_, _ = io.WriteString(w, opts.Failure.Body)
			} else {
				http.Error(w, "injected cloud failure", http.StatusBadGateway)
			}
			return
		}
		switch r.URL.Path {
		case "/token/v2/user", "/token/json/2/user/new":
			_, _ = io.WriteString(w, "mock-token")
		case "/token/json/2/device/new":
			_, _ = io.WriteString(w, "mock-device-token")
		case "/sync/v3/root":
			_, _ = fmt.Fprintf(w, `{"hash":%q,"generation":1,"schemaVersion":%d}`, root, version)
		default:
			data, ok := catalog.blobs[strings.TrimPrefix(r.URL.Path, "/sync/v3/files/")]
			if !ok {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write(data)
		}
	}))
	t.Cleanup(ts.Close)
	return &Server{Server: ts, URL: ts.URL, Hashes: catalog.hashes}
}
