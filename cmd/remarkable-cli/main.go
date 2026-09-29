package main

import (
	"fmt"
	"os"

	cobrahelptree "github.com/alexgorbatchev/cobra-help-tree/v2"
	"github.com/spf13/cobra"
)

var version = "0.2.0"

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remarkable-cli",
		Short: "High-performance, generic CLI gateway for reMarkable Cloud Sync v3",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	// Strict contract: --version outputs ONLY the raw version string followed by a newline
	cmd.Version = version
	cmd.SetVersionTemplate("{{.Version}}\n")

	// Global flags
	cmd.PersistentFlags().StringVarP(&cfgFlag, "config", "c", "", "Path to credentials file (defaults to ~/.config/remarkable-cli/config.json or ~/.rmapi)")
	cmd.PersistentFlags().StringVar(&cacheDirFlag, "cache-dir", "", "Path to template cache directory (defaults to ~/.cache/remarkable-cli)")
	cmd.PersistentFlags().BoolVar(&debugFlag, "debug", false, "Print outgoing reMarkable API requests for diagnostics")
	cmd.PersistentFlags().BoolVar(&noCacheFlag, "no-cache", false, "Disable local disk caching of blobs and manifests")

	// Domain subject command groups
	cmd.AddCommand(newAuthCmd())
	cmd.AddCommand(newDocCmd())
	cmd.AddCommand(newStrokeCmd())

	// Top-level convenience shortcut
	cmd.AddCommand(newStatusShortcutCmd())

	catalog := cobrahelptree.TechCatalog{
		"remarkable-cli auth pair": {
			Args: []cobrahelptree.ArgSpec{
				{Name: "<code>", Description: "8-character device registration code from my.remarkable.com/device/desktop/connect"},
			},
			MutatesDB: true,
		},
		"remarkable-cli doc search": {
			Args: []cobrahelptree.ArgSpec{
				{Name: "<id-or-name>", Description: "Document UUID or exact display name"},
				{Name: "<query>", Description: "Text string to search across document pages"},
			},
		},
		"remarkable-cli doc links": {
			Args: []cobrahelptree.ArgSpec{
				{Name: "<id-or-name>", Description: "Document UUID or exact display name"},
			},
		},
		"remarkable-cli doc inspect": {
			Args: []cobrahelptree.ArgSpec{
				{Name: "<id-or-name>", Description: "Document UUID or exact display name"},
			},
		},
		"remarkable-cli doc cat": {
			Args: []cobrahelptree.ArgSpec{
				{Name: "<id-or-name>", Description: "Document UUID or exact display name"},
			},
		},
		"remarkable-cli doc render": {
			Args: []cobrahelptree.ArgSpec{
				{Name: "<id-or-name>", Description: "Document UUID or exact display name"},
			},
		},
		"remarkable-cli doc sync": {
			Args: []cobrahelptree.ArgSpec{
				{Name: "<id-or-name>", Description: "Document UUID or exact display name"},
			},
			MutatesDB: true,
		},
		"remarkable-cli stroke inspect": {
			Args: []cobrahelptree.ArgSpec{
				{Name: "<file.rm>", Description: "Path to reMarkable v6 binary stroke file"},
			},
		},
		"remarkable-cli stroke export": {
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

	return cmd
}

func main() {
	cmd := newRootCmd()
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
