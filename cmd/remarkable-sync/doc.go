package main

import (
	"context"
	"fmt"
	"time"

	"github.com/alexgorbatchev/remarkable-sync/internal/agent"
	"github.com/alexgorbatchev/remarkable-sync/internal/doc"
	"github.com/spf13/cobra"
)

func newDocCmd() *cobra.Command {
	docCmd := &cobra.Command{
		Use:   "doc",
		Short: "Inspect, browse, and export documents and notebooks",
	}

	var (
		docFolderFlag string
		docTypeFlag   string
		docLimitFlag  int

		catPageFlag   int
		catFormatFlag string

		renderPageFlag   int
		renderDPIFlag    int
		renderOutputFlag string

		syncOutputDirFlag string
		syncFormatFlag    string
		syncDPIFlag       int
		syncForceFlag     bool
	)

	docListCmd := &cobra.Command{
		Use:   "list",
		Short: "List documents and folders in reMarkable Cloud",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 25*time.Second)
			defer cancel()

			client, err := newCloudClient(ctx)
			if err != nil {
				return err
			}

			items, err := doc.List(ctx, client, docFolderFlag, docTypeFlag, docLimitFlag)
			if err != nil {
				return err
			}

			headers := []string{"ID", "NAME", "TYPE", "MODIFIED"}
			rows := make([][]string, 0, len(items))
			for _, it := range items {
				rows = append(rows, []string{
					it.ID,
					it.Name,
					it.Type,
					it.Modified,
				})
			}

			agent.PrintTable(cmd.OutOrStdout(), headers, rows)
			return nil
		},
	}
	docListCmd.Flags().StringVar(&docFolderFlag, "folder", "", "Filter items by parent folder ID")
	docListCmd.Flags().StringVar(&docTypeFlag, "type", "", "Filter items by type (DocumentType, CollectionType)")
	docListCmd.Flags().IntVar(&docLimitFlag, "limit", 0, "Maximum number of items to return")

	docTreeCmd := &cobra.Command{
		Use:   "tree",
		Short: "Display virtual folder and document hierarchy as a tree",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()

			client, err := newCloudClient(ctx)
			if err != nil {
				return err
			}

			tree, err := doc.BuildTree(ctx, client)
			if err != nil {
				return err
			}

			agent.PrintTree(cmd.OutOrStdout(), tree)
			return nil
		},
	}

	docInspectCmd := &cobra.Command{
		Use:   "inspect <id-or-name>",
		Short: "Display detailed document metadata and page structure",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 20*time.Second)
			defer cancel()

			client, err := newCloudClient(ctx)
			if err != nil {
				return err
			}

			details, err := doc.Inspect(ctx, client, args[0])
			if err != nil {
				return err
			}

			pairs := []agent.KeyValuePair{
				{Key: "ID", Value: details.ID},
				{Key: "Name", Value: details.Name},
				{Key: "Type", Value: details.Type},
				{Key: "Format", Value: details.FileType},
				{Key: "Pages", Value: fmt.Sprintf("%d", details.Pages)},
				{Key: "Modified", Value: details.LastModified},
			}

			agent.PrintKeyValues(cmd.OutOrStdout(), pairs)
			return nil
		},
	}

	docCatCmd := &cobra.Command{
		Use:   "cat <id-or-name>",
		Short: "Stream document page content (pdf, rm strokes, or svg) to stdout",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()

			client, err := newCloudClient(ctx)
			if err != nil {
				return err
			}

			return doc.Cat(ctx, client, args[0], catPageFlag, catFormatFlag, cmd.OutOrStdout())
		},
	}
	docCatCmd.Flags().IntVar(&catPageFlag, "page", 0, "Page index to extract (0-based)")
	docCatCmd.Flags().StringVar(&catFormatFlag, "format", "svg", "Output format (pdf, rm, svg)")

	docRenderCmd := &cobra.Command{
		Use:   "render <id-or-name>",
		Short: "Render document page with strokes to a high-resolution PNG",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if renderOutputFlag == "" {
				return fmt.Errorf("output path required: specify with -o or --output")
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), 45*time.Second)
			defer cancel()

			client, err := newCloudClient(ctx)
			if err != nil {
				return err
			}

			err = doc.RenderPage(ctx, client, args[0], renderPageFlag, renderDPIFlag, renderOutputFlag)
			if err != nil {
				return err
			}

			agent.PrintStatus(cmd.OutOrStdout(), "ok", fmt.Sprintf("Rendered page %d to %s", renderPageFlag, renderOutputFlag))
			return nil
		},
	}
	docRenderCmd.Flags().IntVar(&renderPageFlag, "page", 0, "Page index to render (0-based)")
	docRenderCmd.Flags().IntVar(&renderDPIFlag, "dpi", 200, "Rendering resolution DPI")
	docRenderCmd.Flags().StringVarP(&renderOutputFlag, "output", "o", "", "Destination path for PNG image")
	_ = docRenderCmd.MarkFlagRequired("output")

	docSyncCmd := &cobra.Command{
		Use:   "sync <id-or-name>",
		Short: "Synchronize document pages (png, svg, or rm) to a local directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Minute)
			defer cancel()

			client, err := newCloudClient(ctx)
			if err != nil {
				return err
			}

			opts := doc.SyncDocOptions{
				OutputDir: syncOutputDirFlag,
				Format:    syncFormatFlag,
				DPI:       syncDPIFlag,
				Force:     syncForceFlag,
			}

			results, err := doc.SyncDocument(ctx, client, args[0], opts)
			if err != nil {
				agent.PrintStatus(cmd.OutOrStdout(), "error", fmt.Sprintf("Document sync failed: %v", err))
				return err
			}

			if agent.IsAgentMode() {
				for _, r := range results {
					fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", r.State, r.Path)
				}
				return nil
			}

			written := 0
			skipped := 0
			for _, r := range results {
				if r.State == "written" {
					written++
					agent.PrintStatus(cmd.OutOrStdout(), "ok", fmt.Sprintf("Synced %s", r.Path))
				} else {
					skipped++
					agent.PrintStatus(cmd.OutOrStdout(), "info", fmt.Sprintf("Skipped unchanged %s", r.Path))
				}
			}
			agent.PrintSeparator(cmd.OutOrStdout())
			agent.PrintStatus(cmd.OutOrStdout(), "ok", fmt.Sprintf("Sync complete: %d written, %d skipped", written, skipped))
			return nil
		},
	}
	docSyncCmd.Flags().StringVarP(&syncOutputDirFlag, "output-dir", "o", ".", "Output directory for exported pages")
	docSyncCmd.Flags().StringVar(&syncFormatFlag, "format", "png", "Page format (png, svg, rm)")
	docSyncCmd.Flags().IntVar(&syncDPIFlag, "dpi", 200, "Rendering resolution DPI for PNGs")
	docSyncCmd.Flags().BoolVarP(&syncForceFlag, "force", "f", false, "Force re-export even if page already exists")

	docCmd.AddCommand(docListCmd)
	docCmd.AddCommand(docTreeCmd)
	docCmd.AddCommand(docInspectCmd)
	docCmd.AddCommand(docCatCmd)
	docCmd.AddCommand(docRenderCmd)
	docCmd.AddCommand(docSyncCmd)
	return docCmd
}
