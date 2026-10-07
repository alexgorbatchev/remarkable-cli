package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	cloud "github.com/alexgorbatchev/go-remarkable-cloud"
	render "github.com/alexgorbatchev/go-remarkable-render"
	"github.com/alexgorbatchev/remarkable-cli/internal/doc"
)

// TestMainExitStatus runs main against real local failures and real HTTP
// servers and checks the exit status each class of failure reports.
func TestMainExitStatus(t *testing.T) {
	docCloud, docCredentials := setupCLITestEnv(t)
	defer docCloud.Close()
	rejecting := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "invalid Authorization header", http.StatusUnauthorized)
	}))
	defer rejecting.Close()
	unavailable := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
	}))
	defer unavailable.Close()
	dropping := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, _, err := http.NewResponseController(w).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		conn.Close() // The client sees the connection end before any response.
	}))
	defer dropping.Close()
	truncating := newTruncatingServer(t)
	staged := newImportCloud(t, rejectMetadataUpload(http.StatusServiceUnavailable))
	commitUnknown := newImportCloud(t, rejectRootCommit(http.StatusInternalServerError))
	settingsUnknown := newSettingsCloud(t, rejectRootCommit(http.StatusInternalServerError))
	paired := writeCredentials(t, pairedCredentials)
	png := filepath.Join(t.TempDir(), "page.png")
	docEnv := map[string]string{"REMARKABLE_HOST": docCloud.URL, "REMARKABLE_CONFIG": docCredentials}
	importArgs := func(c importCloud) []string {
		return []string{"doc", "import", c.id, "--mapping", c.mapping, "--no-cache"}
	}
	importEnv := func(c importCloud) map[string]string {
		return map[string]string{"REMARKABLE_HOST": c.url, "REMARKABLE_CONFIG": c.credentials}
	}
	settingsArgs := []string{"doc", "settings", "transfer", settingsUnknown.source, settingsUnknown.destination, "--mapping", settingsUnknown.mapping, "--replace-viewport", "--no-cache"}
	settingsEnv := map[string]string{"REMARKABLE_HOST": settingsUnknown.url, "REMARKABLE_CONFIG": settingsUnknown.credentials}

	cases := []struct {
		name string
		args []string
		// env overrides the isolated defaults: missing credentials and a
		// closed cloud port.
		env  map[string]string
		want int
		// state is the progress state a cloud write must report on stdout.
		state string
	}{
		{"invalid flag", []string{"stroke", "inspect", "--bogus", "x.rm"}, nil, 1, ""},
		{"missing local file", []string{"stroke", "inspect", filepath.Join(t.TempDir(), "missing.rm")}, nil, 3, ""},
		{"missing document", []string{"doc", "render", "no such doc", "--no-cache", "-o", png}, docEnv, 3, ""},
		{"page beyond native pages", []string{"doc", "render", "doc-1", "--page", "5", "--no-cache", "-o", png}, docEnv, 3, ""},
		{"missing credentials", []string{"doc", "list", "--no-cache"}, nil, 4, ""},
		{"rejected credentials", []string{"doc", "list", "--no-cache"}, map[string]string{"REMARKABLE_HOST": rejecting.URL, "REMARKABLE_CONFIG": paired}, 4, ""},
		{"server error", []string{"doc", "list", "--no-cache"}, map[string]string{"REMARKABLE_HOST": unavailable.URL, "REMARKABLE_CONFIG": paired}, 5, ""},
		{"closed port", []string{"doc", "list", "--no-cache"}, map[string]string{"REMARKABLE_CONFIG": paired}, 5, ""},
		{"dropped connection", []string{"doc", "list", "--no-cache"}, map[string]string{"REMARKABLE_HOST": dropping.URL, "REMARKABLE_CONFIG": paired}, 5, ""},
		{"truncated response body", []string{"doc", "list", "--no-cache"}, map[string]string{"REMARKABLE_HOST": truncating.URL, "REMARKABLE_CONFIG": paired}, 5, ""},
		{"malformed cloud address", []string{"doc", "list", "--no-cache"}, map[string]string{"REMARKABLE_HOST": "http://bad host", "REMARKABLE_CONFIG": paired}, 1, ""},
		{"staged write exits by cause", importArgs(staged), importEnv(staged), 5, "staged"},
		{"import sent root commit", importArgs(commitUnknown), importEnv(commitUnknown), 6, "commit-unknown"},
		{"settings transfer sent root commit", settingsArgs, settingsEnv, 6, "commit-unknown"},
	}
	for _, mode := range errorModes {
		t.Run(mode.agent, func(t *testing.T) {
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					env := map[string]string{"AGENT": mode.agent}
					for key, value := range tc.env {
						env[key] = value
					}
					got := runMainProcess(t, mainRun{args: tc.args, env: env})
					if got.exitCode != tc.want {
						t.Errorf("exit status = %d, want %d; stderr: %s", got.exitCode, tc.want, got.stderr)
					}
					if !strings.HasPrefix(got.stderr, mode.prefix) && !strings.Contains(got.stderr, "\n"+mode.prefix) {
						t.Errorf("stderr = %q, want an error report prefixed %q", got.stderr, mode.prefix)
					}
					if tc.state != "" && !strings.Contains(got.stdout, tc.state) {
						t.Errorf("stdout = %q, want progress state %q", got.stdout, tc.state)
					}
				})
			}
		})
	}
}

// TestPDFCommandExitStatus classifies failures of commands that open a PDF in
// process: each child process would load the PDF engine again, which takes
// seconds under the race detector. TestMainExitStatus shows that main exits
// with exitStatus of the error a command returns.
func TestPDFCommandExitStatus(t *testing.T) {
	t.Run("page beyond PDF pages", func(t *testing.T) {
		ts, _ := setupCLITestEnv(t)
		defer ts.Close()
		err := inProcessError(t, "doc", "links", "doc-1", "--page", "99", "--no-cache")
		if got := exitStatus(err); got != 3 {
			t.Errorf("exitStatus(%v) = %d, want %d", err, got, 3)
		}
	})
	t.Run("upload sent root commit", func(t *testing.T) {
		// The import cloud serves any blob, so it also accepts a new upload.
		fixture := newImportCloud(t, rejectRootCommit(http.StatusInternalServerError))
		t.Setenv("REMARKABLE_HOST", fixture.url)
		t.Setenv("REMARKABLE_CONFIG", fixture.credentials)
		evidence := filepath.Join(t.TempDir(), "upload.json")
		out, err := executeRoot("doc", "upload", "../../internal/doc/testdata/linked_pages.pdf", "--title", "Planner", "--evidence", evidence, "--no-cache")
		if got := exitStatus(err); got != 6 {
			t.Errorf("exitStatus(%v) = %d, want %d", err, got, 6)
		}
		if !strings.Contains(out, "commit-unknown") {
			t.Errorf("output = %q, want progress state commit-unknown", out)
		}
	})
}

// newTruncatingServer starts a server whose every response declares a longer
// body than it sends before closing the connection.
func newTruncatingServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, buf, err := http.NewResponseController(w).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		buf.WriteString("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: 100\r\n\r\n{\"hash\":\"root-")
		if err := buf.Flush(); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

// truncatedBody returns the error reading a response body through the cloud
// HTTP client reports when the connection closes before the body ends.
func truncatedBody(t *testing.T) error {
	t.Helper()
	resp, err := newCloudHTTPClient(false).Get(newTruncatingServer(t).URL + "/sync/v3/root")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	_, err = io.ReadAll(resp.Body)
	if err == nil {
		t.Fatal("expected a truncated body")
	}
	return fmt.Errorf("decode root state: %w", err)
}

// tlsCertificateRequired returns the error an HTTPS request reports when the
// server rejects the handshake with a TLS alert because the client sent no
// certificate.
func tlsCertificateRequired(t *testing.T) error {
	t.Helper()
	server := httptest.NewUnstartedServer(http.NotFoundHandler())
	server.TLS = &tls.Config{ClientAuth: tls.RequireAnyClientCert}
	server.StartTLS()
	defer server.Close()
	_, err := server.Client().Get(server.URL)
	if err == nil {
		t.Fatal("expected the server to refuse the handshake")
	}
	return err
}

// tlsUnknownAuthority returns the error an HTTPS request reports when the
// server's certificate is not signed by a trusted authority.
func tlsUnknownAuthority(t *testing.T) error {
	t.Helper()
	server := httptest.NewTLSServer(http.NotFoundHandler())
	defer server.Close()
	_, err := (&http.Client{Transport: &http.Transport{}}).Get(server.URL)
	if err == nil {
		t.Fatal("expected certificate verification to fail")
	}
	return err
}

// dialError wraps a failed lookup of the cloud host the way net/http reports it.
func dialError(dns *net.DNSError) error {
	return urlError(&net.OpError{Op: "dial", Net: "tcp", Err: dns})
}

// tlsHandshakeTimeout returns the error an HTTPS request reports when the
// server accepts the connection but never completes the TLS handshake.
func tlsHandshakeTimeout(t *testing.T) error {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			defer conn.Close() // Hold the connection open without answering.
		}
	}()
	client := &http.Client{Transport: &http.Transport{TLSHandshakeTimeout: 50 * time.Millisecond}}
	_, err = client.Get("https://" + listener.Addr().String())
	if err == nil {
		t.Fatal("expected a TLS handshake timeout")
	}
	return err
}

func TestExitStatusClassifiesErrors(t *testing.T) {
	_, parseErr := http.NewRequest(http.MethodGet, "http://bad host/sync/v3/root", nil)
	if parseErr == nil {
		t.Fatal("expected an invalid cloud address to be rejected")
	}
	unavailable := &cloud.StatusError{Op: "get root state", StatusCode: http.StatusServiceUnavailable}
	rejected := &cloud.StatusError{Op: "get root state", StatusCode: http.StatusUnauthorized, Err: cloud.ErrUnauthorized}
	missingFile := fmt.Errorf("reading mapping: %w", &fs.PathError{Op: "open", Path: "m.json", Err: fs.ErrNotExist})
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"unclassified failure", errors.New("document \"x\" has no background PDF"), 1},
		{"ambiguous name", fmt.Errorf("resolving document %q: %w", "Plan", &cloud.AmbiguousNameError{Query: "Plan", Name: "Plan"}), 1},
		{"generation conflict", &cloud.StatusError{Op: "commit root", StatusCode: http.StatusConflict, Err: cloud.ErrGenerationConflict}, 1},
		{"other rejected request", &cloud.StatusError{Op: "get root state", StatusCode: http.StatusNotFound}, 1},
		{"invalid cloud address", fmt.Errorf("get root state: %w", parseErr), 1},
		{"missing item", fmt.Errorf("resolving document %q: %w", "x", fmt.Errorf("%w: name x", cloud.ErrItemNotFound)), 3},
		{"missing blob", &cloud.StatusError{Op: "get blob", StatusCode: http.StatusNotFound, Err: cloud.ErrItemNotFound}, 3},
		{"native page beyond document", fmt.Errorf("%w: 9 (document has 2 pages)", doc.ErrPageOutOfBounds), 3},
		{"PDF page beyond document", fmt.Errorf("%w: page index 9", render.ErrPageOutOfBounds), 3},
		{"missing local file", missingFile, 3},
		{"missing credentials file", fmt.Errorf("%w at /x: run 'remarkable auth pair <code>' first", errCredentialsNotFound), 4},
		{"rejected credentials", fmt.Errorf("get root state: %w", rejected), 4},
		{"server error", fmt.Errorf("get root state: %w", unavailable), 5},
		{"request timeout status", &cloud.StatusError{Op: "get root state", StatusCode: http.StatusRequestTimeout}, 5},
		{"rate limited", &cloud.StatusError{Op: "get root state", StatusCode: http.StatusTooManyRequests}, 5},
		{"operation deadline", fmt.Errorf("get root state: %w", context.DeadlineExceeded), 5},
		{"TLS handshake timeout", tlsHandshakeTimeout(t), 5},
		{"connection closed early", urlError(io.ErrUnexpectedEOF), 5},
		{"truncated response body", truncatedBody(t), 5},
		{"TLS handshake refused by server alert", tlsCertificateRequired(t), 1},
		{"TLS alert sent by client", urlError(&net.OpError{Op: "local error", Err: errors.New("tls: bad certificate")}), 1},
		{"untrusted server certificate", tlsUnknownAuthority(t), 1},
		{"unknown cloud host", dialError(&net.DNSError{Err: "no such host", Name: "cloud.invalid", IsNotFound: true}), 1},
		{"DNS lookup timeout", dialError(&net.DNSError{Err: "i/o timeout", Name: "cloud.example", IsTimeout: true}), 5},
		{"DNS server failure", dialError(&net.DNSError{Err: "server misbehaving", Name: "cloud.example", IsTemporary: true}), 5},
		{"renewal transport failure", fmt.Errorf("auth renewal failed after 401: %w", &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}), 5},
		{"credentials outrank cloud failure", errors.Join(rejected, unavailable), 4},
		{"cloud failure outranks missing input", errors.Join(missingFile, unavailable), 5},
		{"sent root commit outranks its cause", afterCommit(cloud.UpdateCommitUnknown, unavailable), 6},
		{"unverified commit outranks its cause", afterCommit(cloud.UpdateCommitted, rejected), 6},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := exitStatus(tc.err); got != tc.want {
				t.Errorf("exitStatus(%v) = %d, want %d", tc.err, got, tc.want)
			}
		})
	}
}

// urlError wraps err the way net/http reports a failed request.
func urlError(err error) error {
	return &url.Error{Op: "Get", URL: "http://cloud/sync/v3/root", Err: err}
}

func TestAfterCommitMarksOnlySentCommits(t *testing.T) {
	cause := errors.New("cause")
	for _, tc := range []struct {
		state  cloud.UpdateState
		marked bool
	}{
		{cloud.UpdateStaged, false},
		{cloud.UpdateCommitUnknown, true},
		{cloud.UpdateCommitted, true},
		{cloud.UpdateVerified, false},
	} {
		t.Run(string(tc.state), func(t *testing.T) {
			if afterCommit(tc.state, nil) != nil {
				t.Fatal("success must stay nil")
			}
			err := afterCommit(tc.state, cause)
			var attempted *commitAttemptedError
			if errors.As(err, &attempted) != tc.marked {
				t.Fatalf("afterCommit(%s) = %#v, marked = %t", tc.state, err, tc.marked)
			}
			if !errors.Is(err, cause) || err.Error() != cause.Error() {
				t.Fatalf("afterCommit(%s) changed the reported error: %v", tc.state, err)
			}
		})
	}
}
