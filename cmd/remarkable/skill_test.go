package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const skillAlert = "ALERT: Agents must read `AGENT=1 remarkable skill` before using this tool.\n"

const (
	// maxSkillLines keeps SKILL.md under the 500-line body limit for skills.
	maxSkillLines = 499
	// maxReferenceLines keeps every reference at or below the length beyond
	// which a reference needs its own table of contents, so one read shows its
	// whole scope. Split a topic rather than exceed it.
	maxReferenceLines = 100
)

// lineCount counts the lines of a printed skill document.
func lineCount(s string) int {
	return strings.Count(strings.TrimSpace(s), "\n") + 1
}

// referenceDocument is one reference file in the repository.
type referenceDocument struct {
	topic   string
	content string
}

// referenceDocuments reads the reference files the binary must embed, in topic
// order. Read them before changing into a directory without repository files.
func referenceDocuments(t *testing.T) []referenceDocument {
	t.Helper()
	entries, err := os.ReadDir("references")
	if err != nil {
		t.Fatal(err)
	}
	var docs []referenceDocument
	for _, entry := range entries {
		topic, ok := strings.CutSuffix(entry.Name(), ".md")
		if !ok || entry.IsDir() {
			t.Fatalf("references/%s is not a Markdown reference", entry.Name())
		}
		content, err := os.ReadFile(filepath.Join("references", entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		docs = append(docs, referenceDocument{topic: topic, content: string(content)})
	}
	if len(docs) == 0 {
		t.Fatal("references/ holds no reference documents")
	}
	return docs
}

// isolateFromRepository runs the rest of the test without credentials or
// repository files, as an installed binary runs.
func isolateFromRepository(t *testing.T) {
	t.Helper()
	t.Setenv("REMARKABLE_CONFIG", filepath.Join(t.TempDir(), "missing"))
	t.Chdir(t.TempDir())
}

func TestSkillCommand(t *testing.T) {
	want, err := os.ReadFile("SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"0", "1"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("AGENT", mode)
			isolateFromRepository(t)
			out, err := executeRoot("skill")
			if err != nil {
				t.Fatalf("skill failed: %v", err)
			}
			if out != string(want) {
				t.Fatal("skill output must match SKILL.md byte for byte")
			}
			if lines := lineCount(out); lines > maxSkillLines {
				t.Fatalf("embedded skill has %d lines; limit is %d", lines, maxSkillLines)
			}
		})
	}
}

func TestSkillReferenceCommand(t *testing.T) {
	docs := referenceDocuments(t)
	for _, mode := range []string{"0", "1"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("AGENT", mode)
			isolateFromRepository(t)
			for _, doc := range docs {
				out, err := executeRoot("skill", "reference", "cat", doc.topic)
				if err != nil {
					t.Fatalf("skill reference cat %s failed: %v", doc.topic, err)
				}
				if out != doc.content {
					t.Errorf("skill reference cat %s must match references/%s.md byte for byte", doc.topic, doc.topic)
				}
				if lines := lineCount(out); lines > maxReferenceLines {
					t.Errorf("reference %s has %d lines; limit is %d", doc.topic, lines, maxReferenceLines)
				}
			}
		})
	}
}

func TestSkillReferenceList(t *testing.T) {
	docs := referenceDocuments(t)
	var want strings.Builder
	want.WriteString("TOPIC\tCOMMAND\n")
	for _, doc := range docs {
		for _, usage := range sectionHeadings(doc.content) {
			fmt.Fprintf(&want, "%s\t%s\n", doc.topic, usage)
		}
	}
	isolateFromRepository(t)

	t.Setenv("AGENT", "1")
	out, err := executeRoot("skill", "reference", "list")
	if err != nil {
		t.Fatalf("skill reference list failed: %v", err)
	}
	if out != want.String() {
		t.Fatalf("agent listing:\n%s\nwant one TSV row per documented command:\n%s", out, want.String())
	}

	t.Setenv("AGENT", "0")
	out, err = executeRoot("skill", "reference", "list")
	if err != nil {
		t.Fatalf("skill reference list failed: %v", err)
	}
	if strings.Contains(out, "\t") || !strings.Contains(out, "TOPIC") {
		t.Fatalf("human listing must be a table, got:\n%s", out)
	}
	for _, doc := range docs {
		if !strings.Contains(out, doc.topic) {
			t.Errorf("human listing is missing topic %s:\n%s", doc.topic, out)
		}
	}
}

// indexRowPrefix starts each row of the SKILL.md reference index, which
// continues with the topic, the closing backtick, and a cell that lists the
// topic's commands before its first ": ".
const indexRowPrefix = "| `AGENT=1 remarkable skill reference cat "

// indexCommand shortens a section heading's usage to the form the index
// lists: without the binary name, except for the root, and without arguments.
func indexCommand(usage string) string {
	words := strings.Fields(usage)
	if len(words) > 1 {
		words = words[1:]
	}
	words = slices.DeleteFunc(words, func(w string) bool {
		return strings.HasPrefix(w, "<") || strings.HasPrefix(w, "[")
	})
	return strings.Join(words, " ")
}

// TestSkillIndexNamesEveryReference checks the SKILL.md index against the
// embedded references: one row per topic, with its exact printing command,
// listing exactly the commands that the topic's section headings document.
func TestSkillIndexNamesEveryReference(t *testing.T) {
	want := map[string][]string{}
	for _, doc := range referenceDocuments(t) {
		var commands []string
		for _, usage := range sectionHeadings(doc.content) {
			commands = append(commands, indexCommand(usage))
		}
		want[doc.topic] = commands
	}
	out, err := executeRoot("skill")
	if err != nil {
		t.Fatal(err)
	}
	backticked := regexp.MustCompile("`([^`]+)`")
	indexed := map[string]bool{}
	for line := range strings.Lines(out) {
		rest, ok := strings.CutPrefix(line, indexRowPrefix)
		if !ok {
			continue
		}
		topic, cells, ok := strings.Cut(rest, "` | ")
		if !ok {
			t.Errorf("malformed index row: %s", line)
			continue
		}
		commands, ok := want[topic]
		if !ok {
			t.Errorf("index row prints topic %q, which is not an embedded reference", topic)
			continue
		}
		if indexed[topic] {
			t.Errorf("index lists topic %s more than once", topic)
		}
		indexed[topic] = true
		listed, _, _ := strings.Cut(cells, ": ")
		var got []string
		for _, m := range backticked.FindAllStringSubmatch(listed, -1) {
			got = append(got, m[1])
		}
		if !slices.Equal(got, commands) {
			t.Errorf("index row for %s lists %q; its sections document %q", topic, got, commands)
		}
	}
	for topic := range want {
		if !indexed[topic] {
			t.Errorf("skill index must name reference %s with %s%s`", topic, indexRowPrefix[2:], topic)
		}
	}
}

func TestPrintReferenceRejectsUnknownTopic(t *testing.T) {
	err := printReference(io.Discard, "no-such-topic")
	if err == nil || err.Error() != `unknown skill reference topic "no-such-topic"` {
		t.Fatalf("expected unknown topic error, got %v", err)
	}
}

func TestSkillCommandRejectsArguments(t *testing.T) {
	topic := referenceDocuments(t)[0].topic
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"skill", "extra"}, `unknown command "extra" for "remarkable skill"`},
		{[]string{"skill", "reference", "list", "extra"}, `unknown command "extra" for "remarkable skill reference list"`},
		{[]string{"skill", "reference", "cat"}, "accepts 1 arg(s), received 0"},
		{[]string{"skill", "reference", "cat", topic, "extra"}, "accepts 1 arg(s), received 2"},
		{[]string{"skill", "reference", "cat", "no-such-topic"}, `invalid argument "no-such-topic" for "remarkable skill reference cat"`},
	}
	for _, tc := range cases {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			out, err := executeRoot(tc.args...)
			if err == nil || err.Error() != tc.want {
				t.Fatalf("expected Cobra argument rejection %q, got %v: %s", tc.want, err, out)
			}
		})
	}
}

func commandTree(t *testing.T) *cobra.Command {
	t.Helper()
	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	prepareRoot(cmd)
	return cmd
}

func walkCommands(cmd *cobra.Command, visit func(*cobra.Command)) {
	if cmd.Hidden {
		return
	}
	visit(cmd)
	for _, child := range cmd.Commands() {
		walkCommands(child, visit)
	}
}

func TestHelpSkillAlert(t *testing.T) {
	for _, mode := range []string{"0", "1", "true", "yes"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("AGENT", mode)
			walkCommands(commandTree(t), func(cmd *cobra.Command) {
				args := strings.Fields(cmd.CommandPath())[1:]
				out, err := executeRoot(append(args, "--help")...)
				if err != nil {
					t.Fatalf("%s help: %v", cmd.CommandPath(), err)
				}
				if mode == "0" {
					if strings.Contains(out, skillAlert) {
						t.Errorf("%s human help contains agent alert", cmd.CommandPath())
					}
					return
				}
				if !strings.HasPrefix(out, skillAlert) || strings.Count(out, skillAlert) != 1 {
					t.Errorf("%s help must start with one skill alert", cmd.CommandPath())
				}
				if !strings.Contains(out, "command: "+cmd.CommandPath()+"\n") {
					t.Errorf("%s lost its agent help content: %s", cmd.CommandPath(), out)
				}
			})
			out, err := executeRoot()
			if err != nil || (mode != "0" && !strings.HasPrefix(out, skillAlert)) {
				t.Fatalf("bare invocation help: %v: %s", err, out)
			}
			out, err = executeRoot("help", "doc", "render")
			if err != nil || (mode != "0" && !strings.HasPrefix(out, skillAlert)) {
				t.Fatalf("explicit help command: %v: %s", err, out)
			}
			out, err = executeRoot("--version")
			if err != nil || out != version+"\n" {
				t.Fatalf("version output changed: %v: %q", err, out)
			}
		})
	}
}

// sectionHeadings returns the command usages that content's command section
// headings, such as "## `remarkable doc list`", name, in document order.
func sectionHeadings(content string) []string {
	var usages []string
	for line := range strings.Lines(content) {
		if usage, ok := strings.CutPrefix(strings.TrimSuffix(line, "\n"), commandHeadingPrefix); ok {
			usages = append(usages, strings.TrimSuffix(usage, "`"))
		}
	}
	return usages
}

// commandSection is the text under one command heading and the printed skill
// document that holds it.
type commandSection struct {
	document string
	text     string
}

// commandSections maps each command usage documented in docs, keyed by the
// document name that prints it, to its section. A section runs from its
// heading to the next "## " heading. A usage documented twice is an error.
func commandSections(t *testing.T, docs map[string]string) map[string]commandSection {
	t.Helper()
	sections := map[string]commandSection{}
	for _, name := range slices.Sorted(maps.Keys(docs)) {
		var usage string
		var text strings.Builder
		flush := func() {
			if usage == "" {
				return
			}
			if prior, ok := sections[usage]; ok {
				t.Errorf("command %s is documented in both %s and %s", usage, prior.document, name)
			}
			sections[usage] = commandSection{document: name, text: text.String()}
			usage = ""
			text.Reset()
		}
		for line := range strings.Lines(docs[name]) {
			if strings.HasPrefix(line, "## ") {
				flush()
				if heading, ok := strings.CutPrefix(strings.TrimSuffix(line, "\n"), commandHeadingPrefix); ok {
					usage = strings.TrimSuffix(heading, "`")
				}
				continue
			}
			text.WriteString(line)
		}
		flush()
	}
	return sections
}

// printedSkillDocuments returns SKILL.md and every reference as the binary
// prints them, keyed by the name that identifies each in test failures.
func printedSkillDocuments(t *testing.T) map[string]string {
	t.Helper()
	docs := map[string]string{}
	out, err := executeRoot("skill")
	if err != nil {
		t.Fatal(err)
	}
	docs["SKILL.md"] = out
	for _, doc := range referenceDocuments(t) {
		out, err := executeRoot("skill", "reference", "cat", doc.topic)
		if err != nil {
			t.Fatalf("skill reference cat %s: %v", doc.topic, err)
		}
		docs["references/"+doc.topic+".md"] = out
	}
	return docs
}

// TestSkillDocumentsCommandInterface checks SKILL.md and its references,
// together, against the live command tree: every command has exactly one
// section, every section names a live command, and each section lists every
// flag the command accepts with its shorthand, type, and default.
func TestSkillDocumentsCommandInterface(t *testing.T) {
	sections := commandSections(t, printedSkillDocuments(t))
	live := map[string]bool{}
	walkCommands(commandTree(t), func(cmd *cobra.Command) {
		cmd.InitDefaultHelpFlag()
		cmd.InitDefaultVersionFlag()
		usage := cmd.Use
		if cmd.Parent() != nil {
			usage = cmd.Parent().CommandPath() + " " + usage
		}
		live[usage] = true
		section, ok := sections[usage]
		if !ok {
			t.Errorf("skill and references are missing command %s", usage)
			return
		}
		checkFlags := func(f *pflag.Flag) {
			if f.Hidden {
				return
			}
			if f.Name == "help" && cmd.Parent() != nil {
				return // The shared help flag is documented once in the root section.
			}
			shorthand := "—"
			if f.Shorthand != "" {
				shorthand = "`-" + f.Shorthand + "`"
			}
			def := f.DefValue
			if def == "" {
				def = `""`
			}
			row := fmt.Sprintf("| `--%s` | %s | `%s` | `%s` |", f.Name, shorthand, f.Value.Type(), def)
			if !strings.Contains(section.text, row) {
				t.Errorf("%s section in %s is missing flag/type/default: %s", usage, section.document, row)
			}
		}
		cmd.LocalNonPersistentFlags().VisitAll(checkFlags)
		cmd.PersistentFlags().VisitAll(checkFlags)
	})
	for usage, section := range sections {
		if !live[usage] {
			t.Errorf("%s documents %s, which is not a command", section.document, usage)
		}
	}
}

func TestSkillOutputErrors(t *testing.T) {
	topic := referenceDocuments(t)[0].topic
	for _, args := range [][]string{
		{"skill"},
		{"skill", "reference", "list"},
		{"skill", "reference", "cat", topic},
		{"--help"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Setenv("AGENT", "1")
			f, err := os.Create(filepath.Join(t.TempDir(), "closed"))
			if err != nil {
				t.Fatal(err)
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}
			cmd := newRootCmd()
			stderr := new(bytes.Buffer)
			cmd.SetOut(f)
			cmd.SetErr(stderr)
			cmd.SetArgs(args)
			err = execute(cmd)
			if args[0] == "skill" {
				if !errors.Is(err, os.ErrClosed) {
					t.Fatalf("expected closed output error, got %v", err)
				}
			} else if !strings.Contains(stderr.String(), "file already closed") {
				t.Fatalf("expected help output error on stderr, got %q", stderr.String())
			}
		})
	}
}
