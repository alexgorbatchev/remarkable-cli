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
		Use:   "remarkable-sync",
		Short: "High-performance CLI gateway and planner synchronizer for reMarkable Cloud Sync v3",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	// Strict contract: --version outputs ONLY the raw version string followed by a newline
	cmd.Version = version
	cmd.SetVersionTemplate("{{.Version}}\n")

	// Global flags
	cmd.PersistentFlags().StringVarP(&cfgFlag, "config", "c", "", "Path to credentials file (defaults to ~/.config/remarkable-sync/config.json or ~/.rmapi)")
	cmd.PersistentFlags().StringVar(&cacheDirFlag, "cache-dir", "", "Path to template cache directory (defaults to ~/.cache/remarkable-sync)")

	// Domain subject command groups
	cmd.AddCommand(newAuthCmd())
	cmd.AddCommand(newDocCmd())
	cmd.AddCommand(newStrokeCmd())

	// Top-level convenience shortcut
	cmd.AddCommand(newStatusShortcutCmd())

	catalog := cobrahelptree.TechCatalog{
		"remarkable-sync auth pair": {
			Args: []cobrahelptree.ArgSpec{
				{Name: "<code>", Description: "8-character device registration code from my.remarkable.com/device/desktop/connect"},
			},
			MutatesDB: true,
		},
		"remarkable-sync doc inspect": {
			Args: []cobrahelptree.ArgSpec{
				{Name: "<id-or-name>", Description: "Document UUID or exact display name"},
			},
		},
		"remarkable-sync doc cat": {
			Args: []cobrahelptree.ArgSpec{
				{Name: "<id-or-name>", Description: "Document UUID or exact display name"},
			},
		},
		"remarkable-sync doc render": {
			Args: []cobrahelptree.ArgSpec{
				{Name: "<id-or-name>", Description: "Document UUID or exact display name"},
			},
		},
		"remarkable-sync doc sync": {
			Args: []cobrahelptree.ArgSpec{
				{Name: "<id-or-name>", Description: "Document UUID or exact display name"},
			},
			MutatesDB: true,
		},
		"remarkable-sync stroke inspect": {
			Args: []cobrahelptree.ArgSpec{
				{Name: "<file.rm>", Description: "Path to reMarkable v6 binary stroke file"},
			},
		},
		"remarkable-sync stroke export": {
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
