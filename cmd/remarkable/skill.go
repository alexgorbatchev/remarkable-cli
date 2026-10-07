package main

import (
	"bytes"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"

	"github.com/alexgorbatchev/remarkable-cli/internal/agent"
	"github.com/spf13/cobra"
)

//go:embed SKILL.md
var skillContent string

// referenceFiles holds the skill's reference documents. A file's name without
// its .md extension is the topic `skill reference cat` prints it for.
//
//go:embed references/*.md
var referenceFiles embed.FS

// skillReference is one embedded reference document.
type skillReference struct {
	topic   string
	content string
}

// skillReferences lists the embedded references in topic order.
var skillReferences = loadSkillReferences()

// loadSkillReferences reads the references embedded at build time. A read can
// fail only if the embed directive and this code disagree, which is a build
// defect rather than a runtime condition, so it panics.
func loadSkillReferences() []skillReference {
	// fs.Glob returns names in lexical order.
	names, err := fs.Glob(referenceFiles, "references/*.md")
	if err != nil {
		panic(fmt.Sprintf("listing embedded skill references: %v", err))
	}
	references := make([]skillReference, 0, len(names))
	for _, name := range names {
		content, err := referenceFiles.ReadFile(name)
		if err != nil {
			panic(fmt.Sprintf("reading embedded skill reference %s: %v", name, err))
		}
		topic := strings.TrimSuffix(path.Base(name), ".md")
		references = append(references, skillReference{topic: topic, content: string(content)})
	}
	return references
}

// referenceTopics returns every reference topic in topic order.
func referenceTopics() []string {
	topics := make([]string, 0, len(skillReferences))
	for _, r := range skillReferences {
		topics = append(topics, r.topic)
	}
	return topics
}

// commandHeadingPrefix starts every command section heading in the skill and
// its references, such as "## `remarkable doc list`".
const commandHeadingPrefix = "## `"

// documentedCommands returns the command usages that content's section
// headings name, in document order.
func documentedCommands(content string) []string {
	var usages []string
	for line := range strings.Lines(content) {
		heading, ok := strings.CutPrefix(strings.TrimSuffix(line, "\n"), commandHeadingPrefix)
		if !ok {
			continue
		}
		if usage, ok := strings.CutSuffix(heading, "`"); ok {
			usages = append(usages, usage)
		}
	}
	return usages
}

func newSkillCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skill",
		Short: "Print the embedded SKILL.md usage guide for agents",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := io.WriteString(cmd.OutOrStdout(), skillContent); err != nil {
				return fmt.Errorf("printing skill: %w", err)
			}
			return nil
		},
	}
	cmd.AddCommand(newSkillReferenceCmd())
	return cmd
}

func newSkillReferenceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reference",
		Short: "Print the skill's per-command reference documents",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List reference topics and the commands each documents",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return printReferenceList(cmd.OutOrStdout())
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:       "cat <topic>",
		Short:     "Print one embedded reference document verbatim",
		Args:      cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		ValidArgs: referenceTopics(),
		RunE: func(cmd *cobra.Command, args []string) error {
			return printReference(cmd.OutOrStdout(), args[0])
		},
	})
	return cmd
}

// printReferenceList writes one row per documented command with the topic of
// the reference that holds it.
func printReferenceList(w io.Writer) error {
	var rows [][]string
	for _, r := range skillReferences {
		for _, usage := range documentedCommands(r.content) {
			rows = append(rows, []string{r.topic, usage})
		}
	}
	// The formatting primitives write to memory so a failed output write is
	// returned to the command instead of being lost.
	var buf bytes.Buffer
	agent.PrintTable(&buf, []string{"TOPIC", "COMMAND"}, rows)
	if _, err := io.Copy(w, &buf); err != nil {
		return fmt.Errorf("printing skill reference list: %w", err)
	}
	return nil
}

// printReference writes the reference for topic, which the command's argument
// validator has already checked against the embedded topics.
func printReference(w io.Writer, topic string) error {
	for _, r := range skillReferences {
		if r.topic != topic {
			continue
		}
		if _, err := io.WriteString(w, r.content); err != nil {
			return fmt.Errorf("printing skill reference %s: %w", topic, err)
		}
		return nil
	}
	return fmt.Errorf("unknown skill reference topic %q", topic)
}
