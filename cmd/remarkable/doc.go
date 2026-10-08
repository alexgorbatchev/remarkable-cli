package main

import (
	"context"
	"fmt"
	"time"

	"github.com/alexgorbatchev/remarkable-cli/internal/agent"
	"github.com/alexgorbatchev/remarkable-cli/internal/doc"
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
		docQueryFlag  string
		docLimitFlag  int

		catPageFlag   int
		catFormatFlag string

		linksPageFlag int

		renderPageFlag   int
		renderDPIFlag    int
		renderOutputFlag string

		syncOutputDirFlag string
		syncFormatFlag    string
		syncDPIFlag       int
		syncForceFlag     bool

		inspectPagesFlag bool

		searchWordFlag bool
	)

	docListCmd := &cobra.Command{
		Use:   "list",
		Short: "List documents and folders in reMarkable Cloud",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 25*time.Second)
			defer cancel()

			client, err := newCloudClient(ctx)
			if err != nil {
				return err
			}

			items, err := doc.List(ctx, client, docFolderFlag, docTypeFlag, docQueryFlag, docLimitFlag)
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

			return agent.PrintTable(cmd.OutOrStdout(), headers, rows)
		},
	}
	docListCmd.Flags().StringVar(&docFolderFlag, "folder", "", "Filter items by parent folder ID")
	docListCmd.Flags().StringVar(&docTypeFlag, "type", "", "Filter items by type (DocumentType, CollectionType)")
	docListCmd.Flags().StringVarP(&docQueryFlag, "query", "q", "", "Filter items matching title substring")
	docListCmd.Flags().IntVar(&docLimitFlag, "limit", 0, "Maximum number of items to return")

	docTreeCmd := &cobra.Command{
		Use:   "tree",
		Short: "Display virtual folder and document hierarchy as a tree",
		Args:  cobra.NoArgs,
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

			return agent.PrintTree(cmd.OutOrStdout(), tree)
		},
	}

	docSearchCmd := &cobra.Command{
		Use:   "search <id-or-name> <query>",
		Short: "Search document text and return matching 0-based page indexes",
		Args:  cobra.MatchAll(cobra.ExactArgs(2), searchQueryArgs),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()

			client, err := newCloudClient(ctx)
			if err != nil {
				return err
			}

			matches, err := doc.SearchDocument(ctx, client, args[0], doc.SearchQuery{Text: args[1], WholeWord: searchWordFlag})
			if err != nil {
				return err
			}

			if agent.IsAgentMode() {
				for _, m := range matches {
					if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%d\t%s\n", m.PageIndex, m.Snippet); err != nil {
						return fmt.Errorf("writing search match: %w", err)
					}
				}
				return nil
			}

			if len(matches) == 0 {
				return agent.PrintStatus(cmd.OutOrStdout(), "info", fmt.Sprintf("No matches found for %q", args[1]))
			}

			headers := []string{"PAGE", "SNIPPET"}
			rows := make([][]string, 0, len(matches))
			for _, m := range matches {
				snippet, err := agent.UnescapeText(m.Snippet)
				if err != nil {
					snippet = m.Snippet
				}
				rows = append(rows, []string{
					fmt.Sprintf("%d", m.PageIndex),
					snippet,
				})
			}
			return agent.PrintTable(cmd.OutOrStdout(), headers, rows)
		},
	}

	docSearchCmd.Flags().BoolVar(&searchWordFlag, "word", false, "Match the query only as whole words")

	docLinksCmd := &cobra.Command{
		Use:   "links <id-or-name>",
		Short: "List internal and external hyperlinks on a document page",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()

			client, err := newCloudClient(ctx)
			if err != nil {
				return err
			}

			links, err := doc.GetLinks(ctx, client, args[0], linksPageFlag)
			if err != nil {
				return err
			}

			rows := make([][]string, len(links))
			for i, l := range links {
				rows[i] = linkFields(l)
			}

			if agent.IsAgentMode() {
				return agent.PrintTable(cmd.OutOrStdout(), nil, rows)
			}

			if len(links) == 0 {
				return agent.PrintStatus(cmd.OutOrStdout(), "info", fmt.Sprintf("No hyperlinks found on page %d", linksPageFlag))
			}

			headers := []string{"LINK #", "TARGET PAGE", "URI", "TEXT", "RECT"}
			return agent.PrintTable(cmd.OutOrStdout(), headers, rows)
		},
	}
	docLinksCmd.Flags().Var(pageIndex(&linksPageFlag, 0), "page", "Page index to inspect links from (0-based)")

	docInspectCmd := &cobra.Command{
		Use:   "inspect <id-or-name>",
		Short: "Display detailed document metadata and page structure",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 25*time.Second)
			defer cancel()

			client, err := newCloudClient(ctx)
			if err != nil {
				return err
			}

			details, err := doc.Inspect(ctx, client, args[0], inspectPagesFlag)
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

			if err := agent.PrintKeyValues(cmd.OutOrStdout(), pairs); err != nil {
				return err
			}

			if inspectPagesFlag && len(details.PageList) > 0 {
				if !agent.IsAgentMode() {
					if _, err := fmt.Fprintln(cmd.OutOrStdout()); err != nil {
						return fmt.Errorf("writing separator: %w", err)
					}
				}
				headers := []string{"PAGE", "PAGE ID", "STROKES"}
				rows := make([][]string, 0, len(details.PageList))
				for _, p := range details.PageList {
					strokeStr := "empty"
					if p.HasStrokes {
						strokeStr = fmt.Sprintf("present (%d bytes)", p.StrokeBytes)
					}
					rows = append(rows, []string{
						fmt.Sprintf("%d", p.Index),
						p.ID,
						strokeStr,
					})
				}
				return agent.PrintTable(cmd.OutOrStdout(), headers, rows)
			}
			return nil
		},
	}
	docInspectCmd.Flags().BoolVar(&inspectPagesFlag, "pages", false, "Include page-by-page stroke presence listing")

	docCatCmd := &cobra.Command{
		Use:   "cat <id-or-name>",
		Short: "Stream document page content (pdf, rm strokes, svg, or text) to stdout",
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
	docCatCmd.Flags().Var(pageIndex(&catPageFlag, 0), "page", "Page index to extract (0-based)")
	docCatCmd.Flags().Var(checkedString(&catFormatFlag, "svg", doc.ParseCatFormat), "format", "Output format (pdf, text, rm, svg)")

	docRenderCmd := &cobra.Command{
		Use:   "render <id-or-name>",
		Short: "Render document page with strokes to a high-resolution PNG",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
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

			return agent.PrintStatus(cmd.OutOrStdout(), "ok", fmt.Sprintf("Rendered page %d to %s", renderPageFlag, renderOutputFlag))
		},
	}
	docRenderCmd.Flags().Var(pageIndex(&renderPageFlag, 0), "page", "Page index to render (0-based)")
	docRenderCmd.Flags().IntVar(&renderDPIFlag, "dpi", 200, "Rendering resolution DPI")
	docRenderCmd.Flags().VarP(checkedString(&renderOutputFlag, "", validString(requireNonEmptyPath)), "output", "o", "Destination path for PNG image")
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
				return err
			}

			if agent.IsAgentMode() {
				for _, r := range results {
					if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", r.State, agent.PrintableText(r.Path)); err != nil {
						return fmt.Errorf("writing sync result: %w", err)
					}
				}
				return nil
			}

			written := 0
			skipped := 0
			for _, r := range results {
				if r.State == "written" {
					written++
					if err := agent.PrintStatus(cmd.OutOrStdout(), "ok", fmt.Sprintf("Synced %s", agent.PrintableText(r.Path))); err != nil {
						return err
					}
				} else {
					skipped++
					if err := agent.PrintStatus(cmd.OutOrStdout(), "info", fmt.Sprintf("Skipped unchanged %s", agent.PrintableText(r.Path))); err != nil {
						return err
					}
				}
			}
			if err := agent.PrintSeparator(cmd.OutOrStdout()); err != nil {
				return err
			}
			return agent.PrintStatus(cmd.OutOrStdout(), "ok", fmt.Sprintf("Sync complete: %d written, %d skipped", written, skipped))
		},
	}
	docSyncCmd.Flags().VarP(checkedString(&syncOutputDirFlag, ".", validString(requireNonEmptyPath)), "output-dir", "o", "Output directory for exported pages")
	docSyncCmd.Flags().Var(checkedString(&syncFormatFlag, "png", doc.ParseSyncFormat), "format", "Page format (png, svg, rm)")
	docSyncCmd.Flags().IntVar(&syncDPIFlag, "dpi", 200, "Rendering resolution DPI for PNGs")
	docSyncCmd.Flags().BoolVarP(&syncForceFlag, "force", "f", false, "Force re-export even if page already exists")

	docCmd.AddCommand(docListCmd)
	docCmd.AddCommand(docTreeCmd)
	docCmd.AddCommand(docSearchCmd)
	docCmd.AddCommand(docLinksCmd)
	docCmd.AddCommand(docInspectCmd)
	docCmd.AddCommand(docCatCmd)
	docCmd.AddCommand(docRenderCmd)
	docCmd.AddCommand(docSyncCmd)
	docCmd.AddCommand(newDocImportCmd())
	docCmd.AddCommand(newDocArchiveCmd())
	docCmd.AddCommand(newDocUploadCmd())
	docCmd.AddCommand(newDocUploadCheckCmd())
	docCmd.AddCommand(newDocSettingsCmd())
	return docCmd
}
