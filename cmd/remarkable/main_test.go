package main

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/alexgorbatchev/go-rmscene"
)

func executeRoot(args ...string) (string, error) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(args)

	err := execute(cmd)
	return buf.String(), err
}

func executeRootOutAndErr(args ...string) (string, string, error) {
	cmd := newRootCmd()
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs(args)

	err := execute(cmd)
	return stdout.String(), stderr.String(), err
}

func setupCLITestEnv(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	return setupCLITestEnvWithFailure(t, "")
}

// cliFixtures holds the document files the mock cloud serves.
type cliFixtures struct {
	pdf      []byte // doc-1 background PDF with links
	strokes  []byte // doc-1 page-1 .rm strokes
	blankPDF []byte // blank-1 PDF: one page with an empty /Annots array, so no links
	// unloadablePDF is the unloadable-1 PDF: its page tree counts two pages
	// but holds one, so PDFium cannot load page 1; page 0 reads "needle here".
	unloadablePDF []byte
	uriLinksPDF   []byte // urilinks-1 PDF built by uriLinksPDF
}

func loadCLIFixtures(t *testing.T) cliFixtures {
	t.Helper()
	var f cliFixtures
	for name, dst := range map[string]*[]byte{
		"linked_pages.pdf":        &f.pdf,
		"oct1_notes_strokes.rm":   &f.strokes,
		"oct1_notes_template.pdf": &f.blankPDF,
		"unloadable_page.pdf":     &f.unloadablePDF,
	} {
		data, err := os.ReadFile(filepath.Join("../../internal/doc/testdata", name))
		if err != nil {
			t.Fatalf("reading fixture %s: %v", name, err)
		}
		*dst = data
	}
	f.uriLinksPDF = uriLinksPDF()
	return f
}

// pairedCredentials holds a device token and a session token.
const pairedCredentials = "devicetoken: dev\nusertoken: usr\n"

// writeCredentials writes content to a new credentials file and returns its path.
func writeCredentials(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("writing credentials: %v", err)
	}
	return path
}

func setupCLITestEnvWithFailure(t *testing.T, failedPath string) (*httptest.Server, string) {
	t.Helper()
	return setupCLITestEnvFailing(t, failedPath, 0)
}

// setupCLITestEnvFailing starts the mock cloud with every request to
// failedPath after the first okRequests failing with HTTP 502.
func setupCLITestEnvFailing(t *testing.T, failedPath string, okRequests int) (*httptest.Server, string) {
	t.Helper()

	fixtures := loadCLIFixtures(t)

	var failedPathRequests atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == failedPath && int(failedPathRequests.Add(1)) > okRequests {
			http.Error(w, "injected cloud failure", http.StatusBadGateway)
			return
		}
		switch r.URL.Path {
		case "/token/v2/user", "/token/json/2/user/new":
			w.Write([]byte("mock-token"))
		case "/token/json/2/device/new":
			w.Write([]byte("mock-device-token"))
		case "/sync/v3/root":
			w.Write([]byte(`{"hash":"root-hash","generation":1,"schemaVersion":3}`))
		case "/sync/v3/files/root-hash":
			w.Write([]byte("doc-hash:doc-1:0:100\nnotebook-hash:notebook-1:0:100\nblank-hash:blank-1:0:100\nunloadable-hash:unloadable-1:0:100\nnocontent-hash:nocontent-1:0:100\nurilinks-hash:urilinks-1:0:100\n"))
		case "/sync/v3/files/urilinks-hash":
			w.Write([]byte("urilinks-meta:urilinks-1.metadata:0:50\nurilinks-content:urilinks-1.content:0:50\nurilinks-pdf:urilinks-1.pdf:0:1100\n"))
		case "/sync/v3/files/urilinks-meta":
			w.Write([]byte(`{"visibleName":"URI Links PDF","type":"DocumentType"}`))
		case "/sync/v3/files/urilinks-content":
			w.Write([]byte(`{"fileType":"pdf","pageCount":2,"pages":["urilinks-page-1","urilinks-page-2"]}`))
		case "/sync/v3/files/urilinks-pdf":
			w.Write(fixtures.uriLinksPDF)
		case "/sync/v3/files/nocontent-hash":
			// A document that lists no content file.
			w.Write([]byte("nocontent-meta:nocontent-1.metadata:0:50\n"))
		case "/sync/v3/files/nocontent-meta":
			w.Write([]byte(`{"visibleName":"No Content Document","type":"DocumentType"}`))
		case "/sync/v3/files/unloadable-hash":
			w.Write([]byte("unloadable-meta:unloadable-1.metadata:0:50\nunloadable-content:unloadable-1.content:0:50\nunloadable-pdf:unloadable-1.pdf:0:500\n"))
		case "/sync/v3/files/unloadable-meta":
			w.Write([]byte(`{"visibleName":"Unloadable Page PDF","type":"DocumentType"}`))
		case "/sync/v3/files/unloadable-content":
			w.Write([]byte(`{"fileType":"pdf","pageCount":2,"pages":["unloadable-page-1","unloadable-page-2"]}`))
		case "/sync/v3/files/unloadable-pdf":
			w.Write(fixtures.unloadablePDF)
		case "/sync/v3/files/blank-hash":
			w.Write([]byte("blank-meta:blank-1.metadata:0:50\nblank-content:blank-1.content:0:50\nblank-pdf:blank-1.pdf:0:1000\n"))
		case "/sync/v3/files/blank-meta":
			w.Write([]byte(`{"visibleName":"Unlinked PDF","type":"DocumentType"}`))
		case "/sync/v3/files/blank-content":
			w.Write([]byte(`{"fileType":"pdf","pageCount":1,"pages":["blank-page-1"]}`))
		case "/sync/v3/files/blank-pdf":
			w.Write(fixtures.blankPDF)
		case "/sync/v3/files/notebook-hash":
			w.Write([]byte("notebook-meta:notebook-1.metadata:0:50\nnotebook-content:notebook-1.content:0:50\n"))
		case "/sync/v3/files/notebook-meta":
			w.Write([]byte(`{"visibleName":"Handwritten Notebook","type":"DocumentType"}`))
		case "/sync/v3/files/notebook-content":
			w.Write([]byte(`{"fileType":"notebook","pageCount":1,"cPages":{"pages":[{"id":"notebook-page-1"}]}}`))
		case "/sync/v3/files/doc-hash":
			w.Write([]byte("meta-hash:doc-1.metadata:0:50\ncontent-hash:doc-1.content:0:100\npdf-hash:doc-1.pdf:0:1000\nstroke-hash:doc-1/page-1.rm:0:200\n"))
		case "/sync/v3/files/meta-hash":
			w.Write([]byte(`{"visibleName":"My Document","type":"DocumentType","lastModified":"2026-09-29T10:00:00Z"}`))
		case "/sync/v3/files/content-hash":
			w.Write([]byte(`{"fileType":"pdf","pageCount":1,"pages":["page-1"]}`))
		case "/sync/v3/files/pdf-hash":
			w.Write(fixtures.pdf)
		case "/sync/v3/files/stroke-hash":
			w.Write(fixtures.strokes)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	configPath := writeCredentials(t, pairedCredentials)

	t.Setenv("REMARKABLE_HOST", ts.URL)
	t.Setenv("REMARKABLE_CONFIG", configPath)
	// The mock's fixed blob hashes must never reach, or be served from, the
	// developer's real blob cache.
	t.Setenv("REMARKABLE_CACHE_DIR", t.TempDir())

	return ts, configPath
}

func TestAuthStatusListingFailure(t *testing.T) {
	for _, mode := range []string{"0", "1"} {
		for _, args := range [][]string{{"auth", "status"}, {"status"}} {
			t.Run(mode+"/"+strings.Join(args, " "), func(t *testing.T) {
				t.Setenv("AGENT", mode)
				ts, _ := setupCLITestEnvWithFailure(t, "/sync/v3/files/root-hash")
				defer ts.Close()
				out, err := executeRoot(append(args, "--no-cache")...)
				if err == nil || !strings.Contains(err.Error(), "listing cloud items") || !strings.Contains(err.Error(), "502") {
					t.Fatalf("expected contextual item listing error, got %v; output: %s", err, out)
				}
				if strings.Contains(out, "status: connected") || strings.Contains(out, "[OK]") || strings.Contains(out, "items: 0") {
					t.Fatalf("failed item listing reported success: %s", out)
				}
			})
		}
	}
}

func TestAuthStatusRootFailure(t *testing.T) {
	for _, mode := range []string{"0", "1"} {
		t.Run("agent="+mode, func(t *testing.T) {
			t.Setenv("AGENT", mode)
			ts, _ := setupCLITestEnvWithFailure(t, "/sync/v3/root")
			defer ts.Close()
			stdout, _, err := executeRootOutAndErr("auth", "status")
			if err == nil {
				t.Fatalf("auth status must fail when root fails")
			}
			if stdout != "" {
				t.Fatalf("failed auth status must produce empty stdout, got: %q", stdout)
			}
		})
	}
}


func TestDocSyncCloudFailures(t *testing.T) {
	for _, mode := range []string{"0", "1"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("AGENT", mode)
			ts, _ := setupCLITestEnvWithFailure(t, "/sync/v3/files/pdf-hash")
			defer ts.Close()
			for _, format := range []string{"jpg", "png"} {
				stdout, _, err := executeRootOutAndErr("doc", "sync", "doc-1", "--format", format, "--no-cache", "-o", t.TempDir())
				if err == nil {
					t.Fatalf("sync %s must fail", format)
				}
				if format == "png" && stdout != "" {
					t.Fatalf("failed sync must produce empty stdout, got: %q", stdout)
				}
			}
		})
	}
}

func TestStrokeExportHelpMatchesCanvas(t *testing.T) {
	for _, mode := range []string{"0", "1"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("AGENT", mode)
			t.Setenv("COLUMNS", "200")
			path := filepath.Join(t.TempDir(), "empty.rm")
			if err := os.WriteFile(path, []byte(rmscene.HeaderV6), 0600); err != nil {
				t.Fatal(err)
			}
			out, err := executeRoot("stroke", "export", path)
			if err != nil {
				t.Fatal(err)
			}
			var svg struct {
				Width  string `xml:"width,attr"`
				Height string `xml:"height,attr"`
			}
			if err := xml.Unmarshal([]byte(out), &svg); err != nil {
				t.Fatal(err)
			}
			help, err := executeRoot("stroke", "export", "--help")
			if err != nil {
				t.Fatal(err)
			}
			for _, dimension := range []string{svg.Width, svg.Height} {
				if dimension == "" || !strings.Contains(help, "default: "+dimension+")") {
					t.Errorf("help omits actual exported canvas dimension %q: %s", dimension, help)
				}
			}
		})
	}
}

func TestRootCommand_HelpAndVersion(t *testing.T) {
	t.Setenv("AGENT", "0")
	out, err := executeRoot("--help")
	if err != nil {
		t.Fatalf("help failed: %v", err)
	}
	if !strings.Contains(out, "remarkable") {
		t.Fatalf("expected remarkable in help, got: %s", out)
	}

	outVer, errVer := executeRoot("--version")
	if errVer != nil {
		t.Fatalf("version failed: %v", errVer)
	}
	if outVer != version+"\n" {
		t.Errorf("expected version '%s\\n', got %q", version, outVer)
	}
}

func TestMainInvocation(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Args = []string{"remarkable-cli", "--version"}
	main()
}

func TestStrokeCommands(t *testing.T) {
	tmpDir := t.TempDir()
	strokePath := filepath.Join(tmpDir, "test.rm")
	_ = os.WriteFile(strokePath, []byte(rmscene.HeaderV6), 0644)

	// 1. Stroke inspect
	t.Setenv("AGENT", "0")
	out, err := executeRoot("stroke", "inspect", strokePath)
	if err != nil {
		t.Fatalf("stroke inspect failed: %v", err)
	}
	if !strings.Contains(out, "Total Blocks:") {
		t.Errorf("expected Total Blocks in inspect output, got %s", out)
	}

	// Stroke inspect with real strokes
	_, _ = executeRoot("stroke", "inspect", "../../internal/doc/testdata/oct1_notes_strokes.rm")

	// 2. Stroke export with and without -o
	outSvgPath := filepath.Join(tmpDir, "exported.svg")
	_, err = executeRoot("stroke", "export", strokePath, "-o", outSvgPath)
	if err != nil {
		t.Fatalf("stroke export failed: %v", err)
	}
	content, err := os.ReadFile(outSvgPath)
	if err != nil || !strings.Contains(string(content), "<svg") {
		t.Fatalf("expected valid SVG file at %s", outSvgPath)
	}

	// Export to stdout
	outSvgStdout, err := executeRoot("stroke", "export", strokePath)
	if err != nil || !strings.Contains(outSvgStdout, "<svg") {
		t.Fatalf("stroke export stdout failed: %v", err)
	}

	// Stroke errors on nonexistent file
	_, errInspectNone := executeRoot("stroke", "inspect", "/nonexistent/stroke.rm")
	if errInspectNone == nil {
		t.Error("expected error on nonexistent stroke inspect")
	}
	_, errExportNone := executeRoot("stroke", "export", "/nonexistent/stroke.rm")
	if errExportNone == nil {
		t.Error("expected error on nonexistent stroke export")
	}
	_, _ = executeRoot("stroke", "export", strokePath, "-o", "/dev/null/cannot/write.svg")
}

func TestStrokeInspect_DeterministicOrder(t *testing.T) {
	t.Setenv("AGENT", "1")
	rmPath := filepath.Join("..", "..", "internal", "stroke", "testdata", "oct1_notes_strokes.rm")

	wantOrder := []string{
		"File: ../../internal/stroke/testdata/oct1_notes_strokes.rm",
		"File Size: 201969 bytes",
		"Total Blocks: 409",
		"Lines: 393",
		"Points: 12457",
		"Tool: Ballpoint: 154 lines",
		"Tool: Calligraphy: 31 lines",
		"Tool: Fineliner: 51 lines",
		"Tool: Highlighter: 45 lines",
		"Tool: Marker: 16 lines",
		"Tool: Mechanical Pencil: 40 lines",
		"Tool: Paintbrush: 29 lines",
		"Tool: Pencil: 24 lines",
		"Tool: Shader: 3 lines",
		"Color: Black: 321 lines",
		"Color: Blue: 6 lines",
		"Color: Gray: 18 lines",
		"Color: Highlight: 48 lines",
	}

	var firstOutput string
	for i := range 20 {
		out, err := executeRoot("stroke", "inspect", rmPath)
		if err != nil {
			t.Fatalf("run %d: stroke inspect failed: %v", i, err)
		}
		if i == 0 {
			firstOutput = out
			lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
			if !slices.Equal(lines, wantOrder) {
				t.Fatalf("unexpected line order:\ngot:\n%s\nwant:\n%s", strings.Join(lines, "\n"), strings.Join(wantOrder, "\n"))
			}
		} else if out != firstOutput {
			t.Fatalf("run %d: nondeterministic output:\ngot:\n%s\nwant:\n%s", i, out, firstOutput)
		}
	}
}

func TestAuthCommands(t *testing.T) {
	ts, configPath := setupCLITestEnv(t)
	defer ts.Close()

	// 1. Auth status in human mode
	t.Setenv("AGENT", "0")
	out, err := executeRoot("auth", "status", "--debug")
	if err != nil || !strings.Contains(out, "[OK]") {
		t.Fatalf("auth status human failed: %v, out: %s", err, out)
	}

	// 2. Auth status in agent mode
	t.Setenv("AGENT", "1")
	outAgent, errAgent := executeRoot("status", "--no-cache")
	if errAgent != nil || !strings.Contains(outAgent, "status: connected") {
		t.Fatalf("auth status agent failed: %v, out: %s", errAgent, outAgent)
	}

	// 3. Auth token
	outToken, errToken := executeRoot("auth", "token")
	if errToken != nil || !strings.Contains(outToken, "mock-token") {
		t.Fatalf("auth token failed: %v, out: %s", errToken, outToken)
	}

	// 4. Auth pair validation
	_, errShort := executeRoot("auth", "pair", "short")
	if errShort == nil {
		t.Fatal("expected error on short pairing code")
	}

	// Auth pair success
	outPair, errPair := executeRoot("auth", "pair", "12345678")
	if errPair != nil || !strings.Contains(outPair, "paired") {
		t.Fatalf("auth pair failed: %v, out: %s", errPair, outPair)
	}

	// Auth pair save error
	_, _ = executeRoot("auth", "pair", "12345678", "-c", "/dev/null/cannot/write/config.json")

	// Missing config file error
	t.Setenv("REMARKABLE_CONFIG", "/nonexistent/config.json")
	_, errMissing := executeRoot("auth", "status")
	if errMissing == nil {
		t.Error("expected error on missing config in auth status")
	}
	_, errMissingToken := executeRoot("auth", "token")
	if errMissingToken == nil {
		t.Error("expected error on missing config in auth token")
	}

	// Network failure with debugTransport
	t.Setenv("REMARKABLE_CONFIG", configPath)
	t.Setenv("REMARKABLE_HOST", "http://127.0.0.1:1")
	_, _ = executeRoot("auth", "status", "--debug")
}

// TestDocSearchMatching runs doc search against the fixture PDF, whose text
// layer separates lines with "\r\n". Page 0 is a year calendar holding "1" as a
// day number, page 1 starts "Jan 1", and pages 3 to 5 hold "1" only inside
// longer numbers such as "12"; page 1 continues "Thu 8\r\nThursday".
func TestDocSearchMatching(t *testing.T) {
	ts, _ := setupCLITestEnv(t)
	defer ts.Close()
	t.Setenv("AGENT", "1")

	tests := []struct {
		name      string
		args      []string
		wantPages []string
	}{
		{"substring", []string{"1"}, []string{"0", "1", "3", "4", "5"}},
		{"whole word", []string{"1", "--word"}, []string{"0", "1"}},
		{"query across line break", []string{"Thu 8 Thursday"}, []string{"1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := append([]string{"doc", "search", "doc-1", "--no-cache"}, tt.args...)
			out, err := executeRoot(args...)
			if err != nil {
				t.Fatalf("doc search %v failed: %v, out: %s", tt.args, err, out)
			}
			var pages []string
			for line := range strings.Lines(out) {
				page, _, _ := strings.Cut(line, "\t")
				pages = append(pages, page)
			}
			if !slices.Equal(pages, tt.wantPages) {
				t.Errorf("doc search %v matched pages %v, want %v; out: %s", tt.args, pages, tt.wantPages, out)
			}
		})
	}
}

func TestDocSearch_HumanModeEscaping(t *testing.T) {
	ts, _ := setupCLITestEnv(t)
	defer ts.Close()
	t.Setenv("AGENT", "0")

	out, err := executeRoot("doc", "search", "urilinks-1", "Ctrl", "--no-cache")
	if err != nil {
		t.Fatalf("doc search failed: %v, out: %s", err, out)
	}

	// Must contain single-escaped ESC (\x1b) and single-escaped backslash (\\)
	if !strings.Contains(out, `Ctrl\x1b[31m`) {
		t.Errorf("doc search human output missing single-escaped ESC: %q", out)
	}
	if strings.Contains(out, `Ctrl\\x1b[31m`) {
		t.Errorf("doc search human output double-escaped ESC: %q", out)
	}
	if !strings.Contains(out, `a\\b`) {
		t.Errorf("doc search human output missing escaped backslash: %q", out)
	}
	if strings.Contains(out, `a\\\\b`) {
		t.Errorf("doc search human output double-escaped backslash: %q", out)
	}
}


func TestDocCommands(t *testing.T) {
	ts, _ := setupCLITestEnv(t)
	defer ts.Close()

	// 1. Doc list human and agent
	t.Setenv("AGENT", "0")
	outList, err := executeRoot("doc", "list")
	if err != nil || !strings.Contains(outList, "My Document") {
		t.Fatalf("doc list failed: %v, out: %s", err, outList)
	}

	t.Setenv("AGENT", "1")
	outListAgent, _ := executeRoot("doc", "list", "--type", "DocumentType", "--limit", "5")
	if !strings.Contains(outListAgent, "doc-1") {
		t.Fatalf("doc list agent failed: %s", outListAgent)
	}

	// 2. Doc tree human and agent
	t.Setenv("AGENT", "0")
	outTree, err := executeRoot("doc", "tree")
	if err != nil || !strings.Contains(outTree, "My Document") {
		t.Fatalf("doc tree failed: %v, out: %s", err, outTree)
	}

	t.Setenv("AGENT", "1")
	outTreeAgent, _ := executeRoot("doc", "tree")
	if !strings.Contains(outTreeAgent, "* My Document") {
		t.Fatalf("doc tree agent failed: %s", outTreeAgent)
	}

	// 3. Doc inspect (with and without --pages)
	t.Setenv("AGENT", "0")
	outInspect, err := executeRoot("doc", "inspect", "doc-1", "--pages")
	if err != nil || !strings.Contains(outInspect, "My Document") || !strings.Contains(outInspect, "STROKES") {
		t.Fatalf("doc inspect failed: %v, out: %s", err, outInspect)
	}
	outInspectNoPages, err := executeRoot("doc", "inspect", "doc-1")
	if err != nil || !strings.Contains(outInspectNoPages, "My Document") {
		t.Fatalf("doc inspect without pages failed: %v", err)
	}

	// 4. Doc search human and agent
	outSearch, err := executeRoot("doc", "search", "doc-1", "2026")
	if err != nil || !strings.Contains(outSearch, "PAGE") {
		t.Fatalf("doc search human failed: %v, out: %s", err, outSearch)
	}

	t.Setenv("AGENT", "1")
	outSearchAgent, _ := executeRoot("doc", "search", "doc-1", "2026")
	if !strings.Contains(outSearchAgent, "0\t") {
		t.Fatalf("doc search agent failed: %s", outSearchAgent)
	}

	// Search empty
	t.Setenv("AGENT", "0")
	outSearchEmpty, _ := executeRoot("doc", "search", "doc-1", "NonexistentQuery")
	if !strings.Contains(outSearchEmpty, "No matches found") {
		t.Fatalf("expected no matches found, got: %s", outSearchEmpty)
	}

	// Links on a PDF page without links succeed: a notice in human mode, no rows for agents.
	outLinksEmpty, err := executeRoot("doc", "links", "blank-1", "--page", "0")
	if err != nil || !strings.Contains(outLinksEmpty, "No hyperlinks found on page 0") {
		t.Fatalf("doc links on unlinked page (human) = %v, out: %q; want no-hyperlinks notice", err, outLinksEmpty)
	}
	t.Setenv("AGENT", "1")
	outLinksEmptyAgent, err := executeRoot("doc", "links", "blank-1", "--page", "0")
	if err != nil || outLinksEmptyAgent != "" {
		t.Fatalf("doc links on unlinked page (agent) = %v, out: %q; want no output", err, outLinksEmptyAgent)
	}
	t.Setenv("AGENT", "0")

	// 5. Doc links human and agent
	outLinks, err := executeRoot("doc", "links", "doc-1", "--page", "0")
	if err != nil || !strings.Contains(outLinks, "TARGET PAGE") {
		t.Fatalf("doc links human failed: %v, out: %s", err, outLinks)
	}

	t.Setenv("AGENT", "1")
	outLinksAgent, _ := executeRoot("doc", "links", "doc-1", "--page", "0")
	if !strings.Contains(outLinksAgent, "#page=") {
		t.Fatalf("doc links agent failed: %s", outLinksAgent)
	}

	// 6. Doc cat formats
	t.Setenv("AGENT", "0")
	outCatText, err := executeRoot("doc", "cat", "doc-1", "--page", "0", "--format", "text")
	if err != nil || !strings.Contains(outCatText, "2026") {
		t.Fatalf("doc cat text failed: %v, out: %s", err, outCatText)
	}
	_, _ = executeRoot("doc", "cat", "doc-1", "--page", "0", "--format", "pdf")
	_, _ = executeRoot("doc", "cat", "doc-1", "--page", "0", "--format", "rm")
	_, _ = executeRoot("doc", "cat", "doc-1", "--page", "0", "--format", "svg")
	_, _ = executeRoot("doc", "cat", "nonexistent", "--page", "0")
	_, _ = executeRoot("doc", "links", "nonexistent")
	_, _ = executeRoot("doc", "inspect", "nonexistent")
	_, _ = executeRoot("doc", "render", "nonexistent", "-o", "/tmp/any.png")
	_, _ = executeRoot("doc", "sync", "nonexistent", "-o", "/tmp/any")

	// 7. Doc render
	tmpDir := t.TempDir()
	outPNG := filepath.Join(tmpDir, "rendered.png")
	outRender, err := executeRoot("doc", "render", "doc-1", "--page", "0", "-o", outPNG, "--debug")
	if err != nil || !strings.Contains(outRender, "[OK]") {
		t.Fatalf("doc render failed: %v, out: %s", err, outRender)
	}
	// Missing output flag error
	_, errNoOut := executeRoot("doc", "render", "doc-1", "--page", "0")
	if errNoOut == nil {
		t.Error("expected error when output flag is missing")
	}
	_, _ = executeRoot("doc", "render", "doc-1", "--page", "0", "-o", "/dev/null/cannot/write.png")

	// 8. Doc sync
	syncDir := filepath.Join(tmpDir, "synced")
	outSync, err := executeRoot("doc", "sync", "doc-1", "-o", syncDir)
	if err != nil || !strings.Contains(outSync, "[OK]") {
		t.Fatalf("doc sync failed: %v, out: %s", err, outSync)
	}

	t.Setenv("AGENT", "1")
	outSyncAgent, _ := executeRoot("doc", "sync", "doc-1", "-o", syncDir)
	if !strings.Contains(outSyncAgent, "skipped") {
		t.Fatalf("doc sync agent failed: %s", outSyncAgent)
	}
}

func TestDocList_EscapedNames(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token/v2/user", "/token/json/2/user/new":
			w.Write([]byte("mock-token"))
		case "/sync/v3/root":
			w.Write([]byte(`{"hash":"root-hash","generation":1,"schemaVersion":3}`))
		case "/sync/v3/files/root-hash":
			w.Write([]byte("forged-hash:forged-1:0:100\n"))
		case "/sync/v3/files/forged-hash":
			w.Write([]byte("forged-meta:forged-1.metadata:0:50\n"))
		case "/sync/v3/files/forged-meta":
			w.Write([]byte(`{"visibleName":"Notes\tDocumentType\nfake-id\tInjected","type":"DocumentType","lastModified":"2026-09-29T10:00:00Z"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	configPath := writeCredentials(t, pairedCredentials)
	t.Setenv("REMARKABLE_HOST", ts.URL)
	t.Setenv("REMARKABLE_CONFIG", configPath)
	t.Setenv("AGENT", "1")

	out, err := executeRoot("doc", "list")
	if err != nil {
		t.Fatalf("doc list failed: %v", err)
	}

	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	// Expected: 1 header line + 1 document row = 2 lines
	if len(lines) != 2 {
		t.Fatalf("doc list produced %d lines, want 2: %q", len(lines), out)
	}
	headers := strings.Split(lines[0], "\t")
	row := strings.Split(lines[1], "\t")
	if len(row) != len(headers) {
		t.Fatalf("row has %d fields, want %d: %q", len(row), len(headers), lines[1])
	}
}

func TestDocSync_EscapedPaths(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token/v2/user", "/token/json/2/user/new":
			w.Write([]byte("mock-token"))
		case "/sync/v3/root":
			w.Write([]byte(`{"hash":"root-hash","generation":1,"schemaVersion":3}`))
		case "/sync/v3/files/root-hash":
			w.Write([]byte("doc-hash:doc-1:0:100\n"))
		case "/sync/v3/files/doc-hash":
			w.Write([]byte("meta-hash:doc-1.metadata:0:50\ncontent-hash:doc-1.content:0:100\nstroke-hash:doc-1/page-1.rm:0:200\n"))
		case "/sync/v3/files/meta-hash":
			w.Write([]byte(`{"visibleName":"My\nNotes\u001b[2J","type":"DocumentType","lastModified":"2026-09-29T10:00:00Z"}`))
		case "/sync/v3/files/content-hash":
			w.Write([]byte(`{"fileType":"notebook","cPages":{"pages":[{"id":"page-1"}]}}`))
		case "/sync/v3/files/stroke-hash":
			w.Write([]byte("fake-rm"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	configPath := writeCredentials(t, pairedCredentials)
	outDir := t.TempDir()

	for _, mode := range []struct {
		agent string
	}{
		{"0"},
		{"1"},
	} {
		t.Run("agent="+mode.agent, func(t *testing.T) {
			t.Setenv("REMARKABLE_HOST", ts.URL)
			t.Setenv("REMARKABLE_CONFIG", configPath)
			t.Setenv("REMARKABLE_CACHE_DIR", t.TempDir())
			t.Setenv("AGENT", mode.agent)

			out, err := executeRoot("doc", "sync", "doc-1", "-o", outDir, "--format", "rm", "--force")
			if err != nil {
				t.Fatalf("doc sync failed: %v, out: %s", err, out)
			}

			if strings.Contains(out, "\x1b") {
				t.Errorf("doc sync output contains raw ESC: %q", out)
			}
			if strings.Contains(out, "\nNotes") {
				t.Errorf("doc sync output contains unescaped newline splitting lines: %q", out)
			}
			if !strings.Contains(out, `\nNotes\x1b[2J`) {
				t.Errorf("doc sync output missing escaped path: %q", out)
			}
		})
	}
}

type failWriter struct{}

func (f failWriter) Write(p []byte) (int, error) {
	return 0, errors.New("simulated write failure")
}

func TestCommandWriteErrors(t *testing.T) {
	ts, _ := setupCLITestEnv(t)
	defer ts.Close()

	rmPath := filepath.Join("..", "..", "internal", "stroke", "testdata", "oct1_notes_strokes.rm")

	cases := []struct {
		name string
		args []string
	}{
		{"stroke inspect", []string{"stroke", "inspect", rmPath}},
		{"stroke export", []string{"stroke", "export", rmPath}},
		{"auth token", []string{"auth", "token"}},
		{"doc list", []string{"doc", "list"}},
	}

	for _, tc := range cases {
		for _, agentVal := range []string{"0", "1"} {
			t.Run(tc.name+"/agent="+agentVal, func(t *testing.T) {
				t.Setenv("AGENT", agentVal)
				cmd := newRootCmd()
				cmd.SetOut(failWriter{})
				cmd.SetErr(io.Discard)
				cmd.SetArgs(tc.args)
				err := execute(cmd)
				if err == nil {
					t.Fatalf("%s with failing stdout writer must return error, got nil", tc.name)
				}
			})
		}
	}
}



