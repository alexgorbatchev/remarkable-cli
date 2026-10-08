package agent

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

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

// DecodeChar decodes the first character of s, which must not be empty. It
// returns utf8.RuneError and size 1 for an invalid UTF-8 sequence, as
// utf8.DecodeRuneInString does. printable reports a valid character that
// unicode.IsPrint accepts: a letter, mark, number, punctuation character,
// symbol, or the ASCII space. Any other character, such as a control, a format
// character like a bidirectional override, a non-ASCII space, or a line or
// paragraph separator, could split a TSV field or a table row, reach the
// terminal as a control, or be invisible.
func DecodeChar(s string) (r rune, size int, printable bool) {
	r, size = utf8.DecodeRuneInString(s)
	valid := r != utf8.RuneError || size > 1
	return r, size, valid && unicode.IsPrint(r)
}

// PrintableText returns text with every character DecodeChar does not report
// printable, and every backslash, replaced by its Go escape sequence. The
// library collapses white space in link text and snippets, but other controls,
// such as ESC, and format characters remain. A rejected character takes the
// escape strconv.QuoteRune writes for it (\a, \b, \f, \n, \r, \t, or \v; \xNN
// for another ASCII control; otherwise \uNNNN, or \UNNNNNNNN above U+FFFF; hex
// digits in lowercase), a backslash becomes \\, and each byte of an invalid
// UTF-8 sequence becomes \xNN as in strconv.Quote. Every other character, the
// space, '%', and quotation marks included, is kept, so text that needs no
// escaping is returned unchanged, and each backslash in the result starts an
// escape that strconv.UnquoteChar with quote 0 decodes.
func PrintableText(text string) string {
	var b strings.Builder
	for i := 0; i < len(text); {
		r, size, printable := DecodeChar(text[i:])
		switch {
		case printable && r != '\\':
			b.WriteString(text[i : i+size])
		case r == utf8.RuneError && size == 1:
			fmt.Fprintf(&b, `\x%02x`, text[i])
		default:
			quoted := strconv.QuoteRune(r)
			// Drop the single quotation marks around the character literal.
			b.WriteString(quoted[1 : len(quoted)-1])
		}
		i += size
	}
	return b.String()
}

// PrintTable formats tabular data as an ASCII table in human mode, or flat TSV in agent mode.
func PrintTable(w io.Writer, headers []string, rows [][]string) {
	if IsAgentMode() {
		// Flat, compact, token-conservative TSV output
		if len(headers) > 0 {
			fmt.Fprintln(w, strings.Join(headers, "\t"))
		}
		for _, row := range rows {
			escapedRow := make([]string, len(row))
			for j, cell := range row {
				escapedRow[j] = PrintableText(cell)
			}
			fmt.Fprintln(w, strings.Join(escapedRow, "\t"))
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
	escaped := make([]KeyValuePair, len(pairs))
	for i, p := range pairs {
		escaped[i] = KeyValuePair{
			Key:   p.Key,
			Value: PrintableText(p.Value),
		}
	}

	if IsAgentMode() {
		for _, p := range escaped {
			fmt.Fprintf(w, "%s: %s\n", p.Key, p.Value)
		}
		return
	}

	maxKeyLen := 0
	for _, p := range escaped {
		if len(p.Key) > maxKeyLen {
			maxKeyLen = len(p.Key)
		}
	}
	format := fmt.Sprintf("%%-%ds  %%s\n", maxKeyLen)
	for _, p := range escaped {
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
	fmt.Fprintln(w, PrintableText(root.Label))
	printTreeHuman(w, root.Children, "")
}

func printTreeAgent(w io.Writer, node *TreeNode, depth int) {
	indent := strings.Repeat("  ", depth)
	fmt.Fprintf(w, "%s* %s\n", indent, PrintableText(node.Label))
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
		fmt.Fprintf(w, "%s%s%s\n", prefix, branch, PrintableText(node.Label))
		if len(node.Children) > 0 {
			printTreeHuman(w, node.Children, nextPrefix)
		}
	}
}
