package main

import (
	"github.com/spf13/cobra"
)

var (
	version = "0.1.0"
)

func newRootCommand() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:          "remarkable-sync",
		Short:        "reMarkable Cloud daily planner sync and document export CLI",
		Long:         "remarkable-sync is a command-line tool for synchronizing and rendering reMarkable tablet daily planner notes over Cloud API.",
		Version:      version,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	rootCmd.SetVersionTemplate("{{.Version}}\n")

	// Subject group: day
	rootCmd.AddCommand(newDayCommand())

	// Top-level aliases for convenience and compatibility
	rootCmd.AddCommand(newSyncCommand())
	rootCmd.AddCommand(newListCommand())

	// Diagnostics & Auth
	rootCmd.AddCommand(newStatusCommand())
	rootCmd.AddCommand(newAuthCommand())

	setupHelp(rootCmd)

	return rootCmd
}
