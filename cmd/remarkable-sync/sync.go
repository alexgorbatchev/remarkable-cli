package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/alexgorbatchev/remarkable-sync/internal/agent"
	"github.com/alexgorbatchev/remarkable-sync/internal/sync"
	"github.com/spf13/cobra"
)

func newSyncCommand() *cobra.Command {
	var (
		outputDir  string
		dpi        int
		force      bool
		configFile string
		cacheDir   string
		docName    string
	)

	cmd := &cobra.Command{
		Use:   "sync [dates...]",
		Short: "Sync daily planner pages directly from reMarkable Cloud in seconds",
		Long:  "Fetches planner page stroke files from the reMarkable Cloud API and renders layered high-resolution PNGs.",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dateRegex := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
			var dates []string

			for _, arg := range args {
				if dateRegex.MatchString(arg) {
					dates = append(dates, arg)
				}
			}

			outputDir = sync.ResolveOutputDir(outputDir)

			if len(dates) == 0 {
				dates = sync.DefaultTargets(outputDir, time.Now())
			}

			if !agent.IsAgentMode() {
				fmt.Fprintf(cmd.OutOrStdout(), "Syncing planner day(s): %s\n", strings.Join(dates, ", "))
			}

			ctx := context.Background()
			results, err := sync.SyncPlanner(ctx, sync.SyncOptions{
				Dates:      dates,
				OutputDir:  outputDir,
				DPI:        dpi,
				DocName:    docName,
				ConfigFile: configFile,
				CacheDir:   cacheDir,
				Force:      force,
			})

			if err != nil {
				if errors.Is(err, sync.ErrCloudUnavailable) {
					if agent.IsAgentMode() {
						fmt.Fprintf(cmd.ErrOrStderr(), "ERR: remarkable unavailable: %v\n", err)
						fmt.Fprintln(cmd.ErrOrStderr(), "ERR: remarkable NOT synced; no planner pages were captured this run")
					} else {
						fmt.Fprintf(cmd.ErrOrStderr(), "[ERROR] reMarkable Cloud unavailable: %v\n", err)
					}
					os.Exit(3)
				}

				if agent.IsAgentMode() {
					fmt.Fprintf(cmd.ErrOrStderr(), "ERR: sync failed: %v\n", err)
				} else {
					fmt.Fprintf(cmd.ErrOrStderr(), "[ERROR] Sync failed: %v\n", err)
				}
				os.Exit(2)
			}

			if agent.IsAgentMode() {
				for _, r := range results {
					fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", r.State, r.Path)
				}
				return nil
			}

			for _, r := range results {
				label := fmt.Sprintf("[%s]", strings.ToUpper(r.State))
				switch r.State {
				case "written":
					fmt.Fprintf(cmd.OutOrStdout(), "  %-11s %s %s -> %s\n", label, r.Date, r.Kind, r.Path)
				case "skipped":
					fmt.Fprintf(cmd.OutOrStdout(), "  %-11s %s %s (already on disk)\n", label, r.Date, r.Kind)
				case "absent":
					fmt.Fprintf(cmd.OutOrStdout(), "  %-11s %s %s (not in planner)\n", label, r.Date, r.Kind)
				}
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Sync complete (%d items processed).\n", len(results))
			return nil
		},
	}

	cmd.Flags().StringVarP(&outputDir, "output-dir", "o", "modules/remarkable/data", "Output directory holding page images")
	cmd.Flags().IntVar(&dpi, "dpi", 200, "Rendering resolution DPI (default: 200)")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force re-rendering even if day already exists")
	cmd.Flags().StringVarP(&configFile, "config", "c", "", "Path to config file (defaults to ~/.rmapi)")
	cmd.Flags().StringVar(&cacheDir, "cache-dir", "", "Path to stationery template cache directory")
	cmd.Flags().StringVar(&docName, "doc", "", "Name or UUID of planner document (defaults to '<year> - Daily')")

	return cmd
}
