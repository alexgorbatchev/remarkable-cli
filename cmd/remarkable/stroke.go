package main

import (
	"fmt"
	"maps"
	"os"
	"slices"

	"github.com/alexgorbatchev/go-rmscene"
	"github.com/alexgorbatchev/remarkable-cli/internal/agent"
	"github.com/alexgorbatchev/remarkable-cli/internal/stroke"
	"github.com/spf13/cobra"
)

func newStrokeCmd() *cobra.Command {
	strokeCmd := &cobra.Command{
		Use:   "stroke",
		Short: "Inspect and convert reMarkable v6 binary stroke (.rm) files",
	}

	var (
		strokeOutputFlag string
		strokeWidthFlag  float64
		strokeHeightFlag float64
	)

	strokeInspectCmd := &cobra.Command{
		Use:   "inspect <file.rm>",
		Short: "Inspect structure, tools, and color palette of a .rm file",
		Args:  cobra.MatchAll(cobra.ExactArgs(1), nonEmptyPathArg),
		RunE: func(cmd *cobra.Command, args []string) error {
			rmBytes, err := stroke.ReadFile(args[0])
			if err != nil {
				return err
			}

			stats, err := stroke.Inspect(rmBytes)
			if err != nil {
				return err
			}

			pairs := []agent.KeyValuePair{
				{Key: "File", Value: args[0]},
				{Key: "File Size", Value: fmt.Sprintf("%d bytes", len(rmBytes))},
				{Key: "Total Blocks", Value: fmt.Sprintf("%d", stats.TotalBlocks)},
				{Key: "Lines", Value: fmt.Sprintf("%d", stats.Lines)},
				{Key: "Points", Value: fmt.Sprintf("%d", stats.Points)},
			}

			for _, tool := range slices.Sorted(maps.Keys(stats.Tools)) {
				pairs = append(pairs, agent.KeyValuePair{
					Key:   "Tool: " + tool,
					Value: fmt.Sprintf("%d lines", stats.Tools[tool]),
				})
			}
			for _, color := range slices.Sorted(maps.Keys(stats.Colors)) {
				pairs = append(pairs, agent.KeyValuePair{
					Key:   "Color: " + color,
					Value: fmt.Sprintf("%d lines", stats.Colors[color]),
				})
			}

			return agent.PrintKeyValues(cmd.OutOrStdout(), pairs)
		},
	}

	strokeExportCmd := &cobra.Command{
		Use:   "export <file.rm>",
		Short: "Convert binary .rm file directly into a standalone layered SVG",
		Args:  cobra.MatchAll(cobra.ExactArgs(1), nonEmptyPathArg),
		RunE: func(cmd *cobra.Command, args []string) error {
			rmBytes, err := stroke.ReadFile(args[0])
			if err != nil {
				return err
			}

			svgStr, err := stroke.ExportSVG(rmBytes, strokeWidthFlag, strokeHeightFlag)
			if err != nil {
				return err
			}

			if strokeOutputFlag != "" {
				if err := os.WriteFile(strokeOutputFlag, []byte(svgStr), 0644); err != nil {
					return fmt.Errorf("writing SVG output to %s: %w", strokeOutputFlag, err)
				}
				return agent.PrintStatus(cmd.OutOrStdout(), "ok", fmt.Sprintf("Exported SVG to %s", strokeOutputFlag))
			}

			if _, err := fmt.Fprint(cmd.OutOrStdout(), svgStr); err != nil {
				return fmt.Errorf("writing SVG output: %w", err)
			}
			return nil
		},
	}
	strokeExportCmd.Flags().StringVarP(&strokeOutputFlag, "output", "o", "", "Destination file for SVG (defaults to stdout)")
	strokeExportCmd.Flags().Float64Var(&strokeWidthFlag, "width", 0, fmt.Sprintf("Canvas width in points (default: %g)", rmscene.DefaultWidthPt))
	strokeExportCmd.Flags().Float64Var(&strokeHeightFlag, "height", 0, fmt.Sprintf("Canvas height in points (default: %g)", rmscene.DefaultHeightPt))

	strokeCmd.AddCommand(strokeInspectCmd)
	strokeCmd.AddCommand(strokeExportCmd)
	return strokeCmd
}
