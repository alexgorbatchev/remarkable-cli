package main

import (
	"github.com/spf13/cobra"
)

func newDayCommand() *cobra.Command {
	dayCmd := &cobra.Command{
		Use:   "day",
		Short: "Capture, sync, and inspect daily planner pages",
		Long:  "Commands for syncing and inspecting reMarkable daily planner pages.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	dayCmd.AddCommand(newSyncCommand())
	dayCmd.AddCommand(newListCommand())

	return dayCmd
}
