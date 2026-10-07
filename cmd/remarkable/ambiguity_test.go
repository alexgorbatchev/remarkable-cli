package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	cloud "github.com/alexgorbatchev/go-remarkable-cloud"
	"github.com/alexgorbatchev/remarkable-cli/internal/agent"
)

// folderItem is one item the folder cloud serves.
type folderItem struct {
	id, name, parent string
	folder           bool
}

// folderLibrary holds documents that share names across folders: "Plan" at
// the root, in two folders, directly inside a trashed folder, and in a live
// folder inside that trashed folder; two root folders named "Work"; two
// documents named "Draft" in one folder; and one document whose name is
// unique.
var folderLibrary = []folderItem{
	{id: "projects", name: "Projects", folder: true},
	{id: "archive", name: "Archive", folder: true},
	{id: "old", name: "Old", parent: "trash", folder: true},
	{id: "old-sub", name: "Sub", parent: "old", folder: true},
	{id: "work-b", name: "Work", folder: true},
	{id: "work-a", name: "Work", folder: true},
	{id: "plan-sub", name: "Plan", parent: "old-sub"},
	{id: "plan-projects", name: "Plan", parent: "projects"},
	{id: "plan-trashed", name: "Plan", parent: "old"},
	{id: "plan-root", name: "Plan"},
	{id: "plan-archive", name: "Plan", parent: "archive"},
	{id: "notes-a", name: "Notes", parent: "work-a"},
	{id: "notes-b", name: "Notes", parent: "work-b"},
	{id: "unique", name: "Unique Doc", parent: "projects"},
	{id: "dup", name: "Dup", folder: true},
	{id: "draft-2", name: "Draft", parent: "dup"},
	{id: "draft-1", name: "Draft", parent: "dup"},
}

// newFolderCloud serves items as a schema v3 cloud and returns its URL and a
// credentials file for it.
func newFolderCloud(t *testing.T, items []folderItem) (string, string) {
	t.Helper()
	files := map[string]string{"root": `{"hash":"root-hash","generation":1,"schemaVersion":3}`}
	var index strings.Builder
	for _, it := range items {
		fmt.Fprintf(&index, "%s-schema:%s:0:100\n", it.id, it.id)
		kind := cloud.ItemTypeDocument
		manifest := fmt.Sprintf("%[1]s-meta:%[1]s.metadata:0:50\n%[1]s-content:%[1]s.content:0:50\n", it.id)
		if it.folder {
			kind = cloud.ItemTypeCollection
			manifest = fmt.Sprintf("%[1]s-meta:%[1]s.metadata:0:50\n", it.id)
		}
		files["files/"+it.id+"-schema"] = manifest
		files["files/"+it.id+"-meta"] = fmt.Sprintf(`{"visibleName":%q,"type":%q,"parent":%q}`, it.name, kind, it.parent)
		files["files/"+it.id+"-content"] = `{"fileType":"notebook","pageCount":0}`
	}
	files["files/root-hash"] = index.String()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token/v2/user", "/token/json/2/user/new":
			w.Write([]byte("mock-token"))
			return
		}
		body, ok := files[strings.TrimPrefix(r.URL.Path, "/sync/v3/")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(body))
	}))
	t.Cleanup(ts.Close)
	return ts.URL, writeCredentials(t, pairedCredentials)
}

// candidateTable renders the candidate rows main prints after an ambiguity in
// the current output mode.
func candidateTable(rows [][]string) string {
	var b bytes.Buffer
	agent.PrintTable(&b, []string{"ID", "FOLDER", "REACHABLE"}, rows)
	return b.String()
}

func TestMainListsEveryCandidateForAmbiguousName(t *testing.T) {
	planRows := [][]string{
		{"plan-root", "/", "yes"},
		{"plan-archive", "/Archive", "yes"},
		{"plan-projects", "/Projects", "yes"},
		{"plan-trashed", "-", "no"},
		{"plan-sub", "Sub", "no"},
	}
	const planAgentTable = "ID\tFOLDER\tREACHABLE\n" +
		"plan-root\t/\tyes\n" +
		"plan-archive\t/Archive\tyes\n" +
		"plan-projects\t/Projects\tyes\n" +
		"plan-trashed\t-\tno\n" +
		"plan-sub\tSub\tno\n"
	workRows := [][]string{
		{"work-a", "/", "yes"},
		{"work-b", "/", "yes"},
	}
	out := t.TempDir()
	cases := []struct {
		name    string
		args    []string
		message string
		rows    [][]string
	}{
		{"inspect", []string{"doc", "inspect", "Plan"}, `resolving document "Plan": ambiguous item name: "Plan" matches 5 items`, planRows},
		{"search", []string{"doc", "search", "Plan", "query"}, `resolving document "Plan": ambiguous item name: "Plan" matches 5 items`, planRows},
		{"links", []string{"doc", "links", "Plan"}, `resolving document "Plan": ambiguous item name: "Plan" matches 5 items`, planRows},
		{"cat", []string{"doc", "cat", "Plan"}, `resolving document "Plan": ambiguous item name: "Plan" matches 5 items`, planRows},
		{"render", []string{"doc", "render", "Plan", "-o", filepath.Join(out, "page.png")}, `resolving document "Plan": ambiguous item name: "Plan" matches 5 items`, planRows},
		{"sync", []string{"doc", "sync", "Plan", "-o", out}, `resolving document "Plan": ambiguous item name: "Plan" matches 5 items`, planRows},
		{"archive", []string{"doc", "archive", "Plan", "-o", filepath.Join(out, "plan.zip")}, `resolving archive source: ambiguous item name: "Plan" matches 5 items`, planRows},
		{"intermediate path segment", []string{"doc", "inspect", "Work/Notes"}, `resolving document "Work/Notes": ambiguous item name: "Work" in "Work/Notes" matches 2 items`, workRows},
		{"final path segment", []string{"doc", "inspect", "Dup/Draft"}, `resolving document "Dup/Draft": ambiguous item name: "Draft" in "Dup/Draft" matches 2 items`, [][]string{
			{"draft-1", "/Dup", "yes"},
			{"draft-2", "/Dup", "yes"},
		}},
	}
	for _, mode := range errorModes {
		t.Run(mode.agent, func(t *testing.T) {
			t.Setenv("AGENT", mode.agent)
			url, config := newFolderCloud(t, folderLibrary)
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					got := runMainProcess(t, mainRun{
						args: append(tc.args, "--no-cache"),
						env:  map[string]string{"AGENT": mode.agent, "REMARKABLE_HOST": url, "REMARKABLE_CONFIG": config},
					})
					table := candidateTable(tc.rows)
					if mode.agent == "1" && tc.name == "inspect" && table != planAgentTable {
						t.Fatalf("agent candidate table = %q, want %q", table, planAgentTable)
					}
					want := mode.prefix + tc.message + "\n" + table
					if got.exitCode != 1 {
						t.Errorf("exit status = %d, want %d", got.exitCode, 1)
					}
					if got.stderr != want {
						t.Errorf("stderr = %q, want the error once followed by every candidate: %q", got.stderr, want)
					}
					// doc sync also echoes its failure to stdout.
					if tc.name != "sync" && got.stdout != "" {
						t.Errorf("stdout = %q, want empty", got.stdout)
					}
				})
			}
		})
	}
}

func TestMainResolvesUniqueNamesAndPaths(t *testing.T) {
	for _, mode := range errorModes {
		t.Run(mode.agent, func(t *testing.T) {
			url, config := newFolderCloud(t, folderLibrary)
			for _, tc := range []struct{ query, id string }{
				{"Unique Doc", "unique"},
				{"Projects/Plan", "plan-projects"},
				{"Archive/Plan", "plan-archive"},
				{"plan-trashed", "plan-trashed"},
			} {
				t.Run(tc.query, func(t *testing.T) {
					got := runMainProcess(t, mainRun{
						args: []string{"doc", "inspect", tc.query, "--no-cache"},
						env:  map[string]string{"AGENT": mode.agent, "REMARKABLE_HOST": url, "REMARKABLE_CONFIG": config},
					})
					if got.exitCode != 0 || got.stderr != "" {
						t.Fatalf("exit status %d, stderr %q; want success", got.exitCode, got.stderr)
					}
					if !strings.Contains(got.stdout, tc.id) {
						t.Errorf("stdout = %q, want document %s", got.stdout, tc.id)
					}
				})
			}
		})
	}
}
