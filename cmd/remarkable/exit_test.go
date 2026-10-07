package main

import (
	"context"
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
		{"invalid flag", []string{"stroke", "inspect", "--bogus", "x.rm"}, nil, exitFailure, ""},
		{"missing local file", []string{"stroke", "inspect", filepath.Join(t.TempDir(), "missing.rm")}, nil, exitMissing, ""},
		{"missing document", []string{"doc", "render", "no such doc", "--no-cache", "-o", png}, docEnv, exitMissing, ""},
		{"page beyond native pages", []string{"doc", "render", "doc-1", "--page", "5", "--no-cache", "-o", png}, docEnv, exitMissing, ""},
		{"missing credentials", []string{"doc", "list", "--no-cache"}, nil, exitUnauthorized, ""},
		{"rejected credentials", []string{"doc", "list", "--no-cache"}, map[string]string{"REMARKABLE_HOST": rejecting.URL, "REMARKABLE_CONFIG": paired}, exitUnauthorized, ""},
		{"server error", []string{"doc", "list", "--no-cache"}, map[string]string{"REMARKABLE_HOST": unavailable.URL, "REMARKABLE_CONFIG": paired}, exitCloudUnavailable, ""},
		{"closed port", []string{"doc", "list", "--no-cache"}, map[string]string{"REMARKABLE_CONFIG": paired}, exitCloudUnavailable, ""},
		{"dropped connection", []string{"doc", "list", "--no-cache"}, map[string]string{"REMARKABLE_HOST": dropping.URL, "REMARKABLE_CONFIG": paired}, exitCloudUnavailable, ""},
		{"malformed cloud address", []string{"doc", "list", "--no-cache"}, map[string]string{"REMARKABLE_HOST": "http://bad host", "REMARKABLE_CONFIG": paired}, exitFailure, ""},
		{"staged write exits by cause", importArgs(staged), importEnv(staged), exitCloudUnavailable, "staged"},
		{"import sent root commit", importArgs(commitUnknown), importEnv(commitUnknown), exitCommitAttempted, "commit-unknown"},
		{"settings transfer sent root commit", settingsArgs, settingsEnv, exitCommitAttempted, "commit-unknown"},
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
		if got := exitStatus(err); got != exitMissing {
			t.Errorf("exitStatus(%v) = %d, want %d", err, got, exitMissing)
		}
	})
	t.Run("upload sent root commit", func(t *testing.T) {
		// The import cloud serves any blob, so it also accepts a new upload.
		fixture := newImportCloud(t, rejectRootCommit(http.StatusInternalServerError))
		t.Setenv("REMARKABLE_HOST", fixture.url)
		t.Setenv("REMARKABLE_CONFIG", fixture.credentials)
		evidence := filepath.Join(t.TempDir(), "upload.json")
		out, err := executeRoot("doc", "upload", "../../internal/doc/testdata/linked_pages.pdf", "--title", "Planner", "--evidence", evidence, "--no-cache")
		if got := exitStatus(err); got != exitCommitAttempted {
			t.Errorf("exitStatus(%v) = %d, want %d", err, got, exitCommitAttempted)
		}
		if !strings.Contains(out, "commit-unknown") {
			t.Errorf("output = %q, want progress state commit-unknown", out)
		}
	})
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
		{"unclassified failure", errors.New("document \"x\" has no background PDF"), exitFailure},
		{"ambiguous name", fmt.Errorf("resolving document %q: %w", "Plan", &cloud.AmbiguousNameError{Query: "Plan", Name: "Plan"}), exitFailure},
		{"generation conflict", &cloud.StatusError{Op: "commit root", StatusCode: http.StatusConflict, Err: cloud.ErrGenerationConflict}, exitFailure},
		{"other rejected request", &cloud.StatusError{Op: "get root state", StatusCode: http.StatusNotFound}, exitFailure},
		{"invalid cloud address", fmt.Errorf("get root state: %w", parseErr), exitFailure},
		{"missing item", fmt.Errorf("resolving document %q: %w", "x", fmt.Errorf("%w: name x", cloud.ErrItemNotFound)), exitMissing},
		{"missing blob", &cloud.StatusError{Op: "get blob", StatusCode: http.StatusNotFound, Err: cloud.ErrItemNotFound}, exitMissing},
		{"native page beyond document", fmt.Errorf("%w: 9 (document has 2 pages)", doc.ErrPageOutOfBounds), exitMissing},
		{"PDF page beyond document", fmt.Errorf("%w: page index 9", render.ErrPageOutOfBounds), exitMissing},
		{"missing local file", missingFile, exitMissing},
		{"missing credentials file", fmt.Errorf("%w at /x: run 'remarkable auth pair <code>' first", errCredentialsNotFound), exitUnauthorized},
		{"rejected credentials", fmt.Errorf("get root state: %w", rejected), exitUnauthorized},
		{"server error", fmt.Errorf("get root state: %w", unavailable), exitCloudUnavailable},
		{"request timeout status", &cloud.StatusError{Op: "get root state", StatusCode: http.StatusRequestTimeout}, exitCloudUnavailable},
		{"rate limited", &cloud.StatusError{Op: "get root state", StatusCode: http.StatusTooManyRequests}, exitCloudUnavailable},
		{"operation deadline", fmt.Errorf("get root state: %w", context.DeadlineExceeded), exitCloudUnavailable},
		{"TLS handshake timeout", tlsHandshakeTimeout(t), exitCloudUnavailable},
		{"connection closed early", urlError(io.ErrUnexpectedEOF), exitCloudUnavailable},
		{"renewal transport failure", fmt.Errorf("auth renewal failed after 401: %w", &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}), exitCloudUnavailable},
		{"credentials outrank cloud failure", errors.Join(rejected, unavailable), exitUnauthorized},
		{"cloud failure outranks missing input", errors.Join(missingFile, unavailable), exitCloudUnavailable},
		{"sent root commit outranks its cause", afterCommit(cloud.UpdateCommitUnknown, unavailable), exitCommitAttempted},
		{"unverified commit outranks its cause", afterCommit(cloud.UpdateCommitted, rejected), exitCommitAttempted},
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
