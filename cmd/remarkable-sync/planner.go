package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/alexgorbatchev/remarkable-sync/internal/agent"
	"github.com/alexgorbatchev/remarkable-sync/internal/config"
	"github.com/alexgorbatchev/remarkable-sync/internal/planner"
	"github.com/spf13/cobra"
)

func runPlannerList(outputDir string) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		dates, err := planner.CapturedDates(outputDir)
		if err != nil {
			return fmt.Errorf("reading planner captures: %w", err)
		}

		if agent.IsAgentMode() {
			for _, d := range dates {
				fmt.Fprintln(cmd.OutOrStdout(), d)
			}
			return nil
		}

		if len(dates) == 0 {
			agent.PrintStatus(cmd.OutOrStdout(), "info", fmt.Sprintf("No planner captures found in %s", outputDir))
			return nil
		}

		agent.PrintStatus(cmd.OutOrStdout(), "info", fmt.Sprintf("Found %d captured planner dates in %s", len(dates), outputDir))
		agent.PrintSeparator(cmd.OutOrStdout())
		for _, d := range dates {
			fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", d)
		}
		return nil
	}
}

func newPlannerCmd() *cobra.Command {
	plannerCmd := &cobra.Command{
		Use:     "planner",
		Aliases: []string{"day"},
		Short:   "Synchronize and inspect reMarkable daily planner pages",
	}

	var (
		plannerOutputFlag string
		plannerDocFlag    string
		plannerDPIFlag    int
		plannerForceFlag  bool
	)

	plannerListCmd := &cobra.Command{
		Use:   "list",
		Short: "List all captured planner dates present in output directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPlannerList(plannerOutputFlag)(cmd, args)
		},
	}
	plannerListCmd.Flags().StringVarP(&plannerOutputFlag, "output-dir", "o", "modules/remarkable/data", "Output directory holding page images")

	plannerSyncCmd := &cobra.Command{
		Use:   "sync [dates...]",
		Short: "Sync daily planner pages directly from reMarkable Cloud in seconds",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 2*time.Minute)
			defer cancel()

			client, err := newCloudClient(ctx)
			if err != nil {
				agent.PrintStatus(cmd.OutOrStdout(), "error", fmt.Sprintf("Authentication failed: %v", err))
				return err
			}

			opts := planner.SyncOptions{
				Dates:      args,
				OutputDir:  plannerOutputFlag,
				DPI:        plannerDPIFlag,
				DocName:    plannerDocFlag,
				CacheDir:   config.ResolveCacheDir(cacheDirFlag),
				Force:      plannerForceFlag,
				ConfigFile: config.ResolveConfigPath(cfgFlag),
			}

			results, err := planner.SyncPlanner(ctx, client, opts)
			if err != nil {
				agent.PrintStatus(cmd.OutOrStdout(), "error", fmt.Sprintf("Planner sync failed: %v", err))
				return err
			}

			if agent.IsAgentMode() {
				for _, r := range results {
					switch r.State {
					case "written":
						fmt.Fprintf(cmd.OutOrStdout(), "written: %s\n", r.Path)
					case "skipped":
						fmt.Fprintf(cmd.OutOrStdout(), "skipped: %s\n", r.Path)
					case "absent":
						fmt.Fprintf(cmd.OutOrStdout(), "absent: %s\n", r.Date)
					}
				}
				return nil
			}

			writtenCount := 0
			skippedCount := 0
			for _, r := range results {
				switch r.State {
				case "written":
					writtenCount++
					agent.PrintStatus(cmd.OutOrStdout(), "ok", fmt.Sprintf("Captured %s (%s)", r.Path, r.Kind))
				case "skipped":
					skippedCount++
					agent.PrintStatus(cmd.OutOrStdout(), "info", fmt.Sprintf("Skipped existing %s", r.Path))
				case "absent":
					agent.PrintStatus(cmd.OutOrStdout(), "warn", fmt.Sprintf("Date %s not found in planner document", r.Date))
				}
			}

			agent.PrintSeparator(cmd.OutOrStdout())
			agent.PrintStatus(cmd.OutOrStdout(), "ok", fmt.Sprintf("Sync complete: %d written, %d skipped", writtenCount, skippedCount))
			return nil
		},
	}
	plannerSyncCmd.Flags().StringVarP(&plannerOutputFlag, "output-dir", "o", "modules/remarkable/data", "Output directory holding page images")
	plannerSyncCmd.Flags().StringVar(&plannerDocFlag, "doc", "", "Name or UUID of planner document (defaults to '<year> - Daily')")
	plannerSyncCmd.Flags().IntVar(&plannerDPIFlag, "dpi", 200, "Rendering resolution DPI")
	plannerSyncCmd.Flags().BoolVarP(&plannerForceFlag, "force", "f", false, "Force re-rendering even if day already exists")

	plannerInspectCmd := &cobra.Command{
		Use:   "inspect <date>",
		Short: "Inspect planner captures for a specific YYYY-MM-DD date",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			date := args[0]
			dayPath := filepath.Join(plannerOutputFlag, fmt.Sprintf("%s-day.png", date))
			notesPath := filepath.Join(plannerOutputFlag, fmt.Sprintf("%s-notes.png", date))

			dayInfo, dayErr := os.Stat(dayPath)
			notesInfo, notesErr := os.Stat(notesPath)

			pairs := []agent.KeyValuePair{
				{Key: "Date", Value: date},
				{Key: "Day Page", Value: formatFileInfo(dayPath, dayInfo, dayErr)},
				{Key: "Notes Page", Value: formatFileInfo(notesPath, notesInfo, notesErr)},
			}

			agent.PrintKeyValues(cmd.OutOrStdout(), pairs)
			return nil
		},
	}
	plannerInspectCmd.Flags().StringVarP(&plannerOutputFlag, "output-dir", "o", "modules/remarkable/data", "Output directory holding page images")

	plannerCmd.AddCommand(plannerListCmd)
	plannerCmd.AddCommand(plannerSyncCmd)
	plannerCmd.AddCommand(plannerInspectCmd)
	return plannerCmd
}

func formatFileInfo(path string, info os.FileInfo, err error) string {
	if err != nil {
		return "not captured"
	}
	return fmt.Sprintf("present (%d bytes, %s)", info.Size(), info.ModTime().Format("2006-01-02 15:04"))
}

func newListShortcutCmd() *cobra.Command {
	var outputDir string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all captured planner dates present in output directory (alias for 'planner list')",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPlannerList(outputDir)(cmd, args)
		},
	}
	cmd.Flags().StringVarP(&outputDir, "output-dir", "o", "modules/remarkable/data", "Output directory holding page images")
	return cmd
}
