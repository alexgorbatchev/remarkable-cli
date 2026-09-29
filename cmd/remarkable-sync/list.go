package main

import (
	"fmt"
	"path/filepath"

	"github.com/alexgorbatchev/remarkable-sync/internal/agent"
	"github.com/alexgorbatchev/remarkable-sync/internal/sync"
	"github.com/spf13/cobra"
)

func newListCommand() *cobra.Command {
	var outputDir string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List captured planner days in the data directory",
		Long:  "Scans the data directory and lists all captured daily planner dates.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if outputDir == "" {
				outputDir = filepath.Join("modules", "remarkable", "data")
			}

			dates, err := sync.CapturedDates(outputDir)
			if err != nil {
				return fmt.Errorf("read captured dates: %w", err)
			}

			if agent.IsAgentMode() {
				for _, d := range dates {
					fmt.Fprintln(cmd.OutOrStdout(), d)
				}
				return nil
			}

			if len(dates) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "[INFO] No planner days captured yet in %s/\n", outputDir)
				return nil
			}

			fmt.Fprintf(cmd.OutOrStdout(), "%d day(s) captured in %s/\n", len(dates), outputDir)
			for idx, d := range dates {
				branch := "├─"
				if idx == len(dates)-1 {
					branch = "╰─"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "  %s %s\n", branch, d)
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&outputDir, "output-dir", "o", "modules/remarkable/data", "Output directory holding page images")
	return cmd
}
