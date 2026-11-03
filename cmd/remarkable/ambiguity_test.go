package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	cloud "github.com/alexgorbatchev/go-remarkable-cloud"
	"github.com/alexgorbatchev/remarkable-cli/internal/agent"
	"github.com/alexgorbatchev/remarkable-cli/internal/testcloud"
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

// newFolderCloud serves content-addressed items and returns its URL and a
// credentials file for it.
func newFolderCloud(t *testing.T, items []folderItem) (string, string) {
	t.Helper()
	var library []testcloud.LibraryItem
	for _, it := range items {
		kind := cloud.ItemTypeDocument
		var content []byte
		if it.folder {
			kind = cloud.ItemTypeCollection
		} else {
			content = []byte(`{"fileType":"notebook","pageCount":0}`)
		}
		library = append(library, testcloud.LibraryItem{ID: it.id, Metadata: cloud.ItemMetadata{VisibleName: it.name, Type: kind, Parent: it.parent}, Content: content})
	}
	ts := testcloud.NewLibrary(t, library)

	return ts.URL, writeCredentials(t, pairedCredentials)
}

// candidateTable renders the candidate rows main prints after an ambiguity in
// the current output mode.
func candidateTable(rows [][]string) string {
	var b bytes.Buffer
	_ = agent.PrintTable(&b, []string{"ID", "FOLDER", "REACHABLE"}, rows)
	return b.String()
}

func TestMainListsEveryCandidateForAmbiguousName(t *testing.T) {
	planRows := [][]string{
		{"plan-root", "/", "yes"},
		{"plan-archive", "/Archive", "yes"},
		{"plan-projects", "/Projects", "yes"},
	}
	const planAgentTable = "ID\tFOLDER\tREACHABLE\n" +
		"plan-root\t/\tyes\n" +
		"plan-archive\t/Archive\tyes\n" +
		"plan-projects\t/Projects\tyes\n"
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
		{"inspect", []string{"doc", "inspect", "Plan"}, `resolving document "Plan": ambiguous item name: "Plan" matches 3 items`, planRows},
		{"search", []string{"doc", "search", "Plan", "query"}, `resolving document "Plan": ambiguous item name: "Plan" matches 3 items`, planRows},
		{"links", []string{"doc", "links", "Plan"}, `resolving document "Plan": ambiguous item name: "Plan" matches 3 items`, planRows},
		{"cat", []string{"doc", "cat", "Plan"}, `resolving document "Plan": ambiguous item name: "Plan" matches 3 items`, planRows},
		{"render", []string{"doc", "render", "Plan", "-o", filepath.Join(out, "page.png")}, `resolving document "Plan": ambiguous item name: "Plan" matches 3 items`, planRows},
		{"sync", []string{"doc", "sync", "Plan", "-o", out}, `resolving document "Plan": ambiguous item name: "Plan" matches 3 items`, planRows},
		{"archive", []string{"doc", "archive", "Plan", "-o", filepath.Join(out, "plan.zip")}, `resolving archive source: ambiguous item name: "Plan" matches 3 items`, planRows},
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
