package agent

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/olekukonko/tablewriter"
	"golang.org/x/term"
)

// IsAgentMode checks if the environment variable AGENT is set to 1, true, or yes.
func IsAgentMode() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("AGENT")))
	return v == "1" || v == "true" || v == "yes"
}

// TerminalWidth returns the width of stdout terminal or 80 if unavailable.
func TerminalWidth() int {
	if term.IsTerminal(int(os.Stdout.Fd())) {
		if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
			return w
		}
	}
	return 80
}

// PrintSeparator outputs a full-width divider line in human mode, and is omitted in agent mode.
func PrintSeparator(w io.Writer) {
	if IsAgentMode() {
		return
	}
	cols := TerminalWidth()
	if cols > 120 {
		cols = 120
	}
	fmt.Fprintln(w, strings.Repeat("-", cols))
}

// PrintStatus prints a status line with strict text tags ([OK], [INFO], [WARN], [ERROR]).
// In agent mode it prints compact prefixes (OK:, INFO:, WARN:, ERR:).
// Strict ban on emojis in both modes.
func PrintStatus(w io.Writer, level, message string) {
	upper := strings.ToUpper(strings.TrimSpace(level))
	if IsAgentMode() {
		prefix := upper
		switch upper {
		case "ERROR":
			prefix = "ERR"
		case "WARNING":
			prefix = "WARN"
		}
		fmt.Fprintf(w, "%s: %s\n", prefix, message)
		return
	}

	tag := fmt.Sprintf("[%s]", upper)
	fmt.Fprintf(w, "%-7s %s\n", tag, message)
}

// PrintTable formats tabular data as an ASCII table in human mode, or flat TSV in agent mode.
func PrintTable(w io.Writer, headers []string, rows [][]string) {
	if IsAgentMode() {
		// Flat, compact, token-conservative TSV output
		if len(headers) > 0 {
			fmt.Fprintln(w, strings.Join(headers, "\t"))
		}
		for _, row := range rows {
			fmt.Fprintln(w, strings.Join(row, "\t"))
		}
		return
	}

	table := tablewriter.NewTable(w, tablewriter.WithHeader(headers))
	_ = table.Bulk(rows)
	_ = table.Render()
}

// KeyValuePair represents an ordered key-value entry.
type KeyValuePair struct {
	Key   string
	Value string
}

// PrintKeyValues formats a sequence of key-values cleanly.
func PrintKeyValues(w io.Writer, pairs []KeyValuePair) {
	if IsAgentMode() {
		for _, p := range pairs {
			fmt.Fprintf(w, "%s: %s\n", p.Key, p.Value)
		}
		return
	}

	maxKeyLen := 0
	for _, p := range pairs {
		if len(p.Key) > maxKeyLen {
			maxKeyLen = len(p.Key)
		}
	}
	format := fmt.Sprintf("%%-%ds  %%s\n", maxKeyLen)
	for _, p := range pairs {
		fmt.Fprintf(w, format, p.Key+":", p.Value)
	}
}

// TreeNode represents a node in a hierarchical tree.
type TreeNode struct {
	Label    string
	Children []*TreeNode
}

// PrintTree prints a tree using box glyphs in human mode or indented bullets in agent mode.
func PrintTree(w io.Writer, root *TreeNode) {
	if root == nil {
		return
	}
	if IsAgentMode() {
		printTreeAgent(w, root, 0)
		return
	}
	fmt.Fprintln(w, root.Label)
	printTreeHuman(w, root.Children, "")
}

func printTreeAgent(w io.Writer, node *TreeNode, depth int) {
	indent := strings.Repeat("  ", depth)
	fmt.Fprintf(w, "%s* %s\n", indent, node.Label)
	for _, child := range node.Children {
		printTreeAgent(w, child, depth+1)
	}
}

func printTreeHuman(w io.Writer, nodes []*TreeNode, prefix string) {
	for i, node := range nodes {
		isLast := i == len(nodes)-1
		branch := "├── "
		nextPrefix := prefix + "│   "
		if isLast {
			branch = "└── "
			nextPrefix = prefix + "    "
		}
		fmt.Fprintf(w, "%s%s%s\n", prefix, branch, node.Label)
		if len(node.Children) > 0 {
			printTreeHuman(w, node.Children, nextPrefix)
		}
	}
}
