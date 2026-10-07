package main

import (
	"cmp"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"

	cobrahelptree "github.com/alexgorbatchev/cobra-help-tree/v2"
	cloud "github.com/alexgorbatchev/go-remarkable-cloud"
	"github.com/alexgorbatchev/remarkable-cli/internal/agent"
	"github.com/spf13/cobra"
)

// Injected during build via -ldflags "-X main.version=1.0.0"
var version = "dev"

// pairingCodePage is the reMarkable page that issues the code auth pair takes.
const pairingCodePage = "my.remarkable.com/device/desktop/connect"

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remarkable",
		Short: "High-performance, generic CLI gateway for reMarkable Cloud Sync v3",
		// main reports every returned error, so Cobra must not print it as well.
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	// Strict contract: --version outputs ONLY the raw version string followed by a newline
	cmd.Version = version
	cmd.SetVersionTemplate("{{.Version}}\n")

	// Global flags
	cmd.PersistentFlags().StringVarP(&cfgFlag, "config", "c", "", "Path to credentials file (defaults to ~/.config/remarkable-cli/config.json or ~/.rmapi)")
	cmd.PersistentFlags().StringVar(&cacheDirFlag, "cache-dir", "", "Path to the cache of downloaded manifests, PDFs and strokes, safe to delete (defaults to ~/.cache/remarkable-cli)")
	cmd.PersistentFlags().BoolVar(&debugFlag, "debug", false, "Print outgoing reMarkable API requests for diagnostics")
	cmd.PersistentFlags().BoolVar(&noCacheFlag, "no-cache", false, "Disable local disk caching of blobs and manifests")

	// Domain subject command groups
	cmd.AddCommand(newAuthCmd())
	cmd.AddCommand(newDocCmd())
	cmd.AddCommand(newStrokeCmd())

	// Top-level convenience shortcut
	cmd.AddCommand(newStatusShortcutCmd())
	cmd.AddCommand(newSkillCmd())

	catalog := cobrahelptree.TechCatalog{
		"remarkable doc settings transfer": {
			Args: []cobrahelptree.ArgSpec{
				{Name: "<source-uuid>", Description: "UUID of the source document with initialized native pages"},
				{Name: "<destination-uuid>", Description: "UUID of the separate initialized destination document"},
			},
			MutatesDB: true,
		},
		"remarkable doc upload": {
			Args:      []cobrahelptree.ArgSpec{{Name: "<pdf>", Description: "Local PDF path"}},
			MutatesDB: true,
		},
		"remarkable doc upload-check": {
			Args: []cobrahelptree.ArgSpec{{Name: "<evidence>", Description: "JSON recovery evidence saved by doc upload"}},
		},
		"remarkable doc archive": {
			Args: []cobrahelptree.ArgSpec{
				{Name: "<id-or-name>", Description: "Document UUID, exact display name, or folder path"},
			},
		},
		"remarkable doc import": {
			Args: []cobrahelptree.ArgSpec{
				{Name: "<destination-uuid>", Description: "UUID of the initialized destination document"},
			},
			MutatesDB: true,
		},
		"remarkable auth pair": {
			Args: []cobrahelptree.ArgSpec{
				{Name: "<code>", Description: "8-character device registration code from " + pairingCodePage},
			},
			MutatesDB: true,
		},
		"remarkable doc search": {
			Args: []cobrahelptree.ArgSpec{
				{Name: "<id-or-name>", Description: "Document UUID or exact display name"},
				{Name: "<query>", Description: "Text string to search across document pages"},
			},
		},
		"remarkable doc links": {
			Args: []cobrahelptree.ArgSpec{
				{Name: "<id-or-name>", Description: "Document UUID or exact display name"},
			},
		},
		"remarkable doc inspect": {
			Args: []cobrahelptree.ArgSpec{
				{Name: "<id-or-name>", Description: "Document UUID or exact display name"},
			},
		},
		"remarkable doc cat": {
			Args: []cobrahelptree.ArgSpec{
				{Name: "<id-or-name>", Description: "Document UUID or exact display name"},
			},
		},
		"remarkable doc render": {
			Args: []cobrahelptree.ArgSpec{
				{Name: "<id-or-name>", Description: "Document UUID or exact display name"},
			},
		},
		"remarkable doc sync": {
			Args: []cobrahelptree.ArgSpec{
				{Name: "<id-or-name>", Description: "Document UUID or exact display name"},
			},
			MutatesDB: true,
		},
		"remarkable stroke inspect": {
			Args: []cobrahelptree.ArgSpec{
				{Name: "<file.rm>", Description: "Path to reMarkable v6 binary stroke file"},
			},
		},
		"remarkable stroke export": {
			Args: []cobrahelptree.ArgSpec{
				{Name: "<file.rm>", Description: "Path to reMarkable v6 binary stroke file"},
			},
		},
	}

	_ = cobrahelptree.SetupWithOptions(cmd, cobrahelptree.HelpOptions{
		Catalog: catalog,
		Tree: cobrahelptree.TreeOptions{
			HideGeneratedCommands: true,
		},
	})

	help := cmd.HelpFunc()
	cmd.SetHelpFunc(func(c *cobra.Command, args []string) {
		if agent.IsAgentMode() {
			if _, err := fmt.Fprintln(c.OutOrStdout(), "ALERT: Agents must read `AGENT=1 remarkable skill` before using this tool."); err != nil {
				c.PrintErrln(err)
				return
			}
		}
		help(c, args)
	})

	return cmd
}

// silenceUsageOnRun stops cmd and its descendants from printing the usage
// screen for errors raised after their RunE starts. Cobra validates flags,
// positional arguments, required flags and flag groups before calling RunE, so
// those invocation errors keep the usage screen, while runtime failures, such
// as an unreachable cloud, report only their error.
func silenceUsageOnRun(cmd *cobra.Command) {
	if run := cmd.RunE; run != nil {
		cmd.RunE = func(c *cobra.Command, args []string) error {
			c.SilenceUsage = true
			return run(c, args)
		}
	}
	for _, child := range cmd.Commands() {
		silenceUsageOnRun(child)
	}
}

// prepareRoot adds Cobra's generated help and completion commands, which Cobra
// would otherwise add inside Execute, so that silenceUsageOnRun reaches every
// runnable command. Cobra's completion commands capture root's output writer
// when they are created, so callers configure output before preparing.
func prepareRoot(root *cobra.Command) {
	root.InitDefaultHelpCmd()
	root.InitDefaultCompletionCmd()
	silenceUsageOnRun(root)
}

// execute prepares root and runs the command its arguments select.
func execute(root *cobra.Command) error {
	prepareRoot(root)
	return root.Execute()
}

// errorReport returns the one-line message main prints for err. A rejected
// cloud request's error ends with the server's reason exactly as sent, which
// can end in a newline, so trailing whitespace is dropped before the line is
// terminated. When credentials are missing or rejected, it appends how to pair
// again.
func errorReport(err error) string {
	message := strings.TrimRightFunc(err.Error(), unicode.IsSpace)
	if errors.Is(err, cloud.ErrUnauthorized) {
		message += ": run 'remarkable auth pair <code>' with a new code from https://" + pairingCodePage
	}
	return message
}

// candidateHeaders label the table of items an ambiguous name matches.
var candidateHeaders = []string{"ID", "FOLDER", "REACHABLE"}

// candidateRows lists the items an ambiguous name matches in the library's
// order: reachable items first, then by folder path, then by ID. FOLDER is the
// absolute folder path of a reachable item, "/" at the root. An unreachable
// item has no path from the root, because an ancestor is trashed, deleted,
// missing from the listing, a document, or part of a parent loop, so FOLDER
// holds only the live folders below that ancestor, or "-" when there are none.
func candidateRows(e *cloud.AmbiguousNameError) [][]string {
	rows := make([][]string, 0, len(e.Candidates))
	for _, c := range e.Candidates {
		folder, reachable := "/"+c.FolderPath, "yes"
		if c.Unreachable {
			folder, reachable = cmp.Or(c.FolderPath, "-"), "no"
		}
		rows = append(rows, []string{c.Item.ID, folder, reachable})
	}
	return rows
}

// reportError writes main's report of err to w: the error line and, when a
// name is ambiguous, the table of every item it matches, so the caller can
// pass one item's ID.
func reportError(w io.Writer, err error) {
	agent.PrintStatus(w, "error", errorReport(err))
	var ambiguous *cloud.AmbiguousNameError
	if errors.As(err, &ambiguous) {
		agent.PrintTable(w, candidateHeaders, candidateRows(ambiguous))
	}
}

func main() {
	if err := execute(newRootCmd()); err != nil {
		reportError(os.Stderr, err)
		os.Exit(exitStatus(err))
	}
}
