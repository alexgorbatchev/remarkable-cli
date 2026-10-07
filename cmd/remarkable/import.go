package main

import (
	"context"
	"fmt"
	"time"

	"github.com/alexgorbatchev/remarkable-cli/internal/agent"
	"github.com/alexgorbatchev/remarkable-cli/internal/doc"
	"github.com/spf13/cobra"
)

func newDocImportCmd() *cobra.Command {
	var mappingPath string
	cmd := &cobra.Command{
		Use:   "import <destination-uuid>",
		Short: "Import native handwriting into empty pages of an existing document",
		Args:  cobra.MatchAll(cobra.ExactArgs(1), importDestinationArg),
		RunE: func(cmd *cobra.Command, args []string) error {
			mapping, err := doc.LoadImportMapping(mappingPath)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Minute)
			defer cancel()
			client, err := newCloudClient(ctx)
			if err != nil {
				return err
			}
			result, err := doc.ImportStrokes(ctx, client, args[0], mapping)
			if result == nil {
				return err
			}
			agent.PrintKeyValues(cmd.OutOrStdout(), []agent.KeyValuePair{{Key: "state", Value: string(result.State)}})
			for _, name := range result.Uploaded {
				agent.PrintKeyValues(cmd.OutOrStdout(), []agent.KeyValuePair{{Key: "uploaded", Value: name}})
			}
			rows := make([][]string, 0, len(result.Pages))
			for _, page := range result.Pages {
				rows = append(rows, []string{fmt.Sprint(page.Page), page.PageID, page.Source, string(result.State)})
			}
			agent.PrintTable(cmd.OutOrStdout(), []string{"PAGE", "PAGE ID", "SOURCE", "STATE"}, rows)
			return afterCommit(result.State, err)
		},
	}
	cmd.Flags().Var(checkedString(&mappingPath, "", validString(requireNonEmptyPath)), "mapping", "Required JSON file mapping native stroke paths to 0-based destination pages")
	if err := cmd.MarkFlagRequired("mapping"); err != nil {
		panic(err)
	} // Constructor invariant: the flag was just registered.
	return cmd
}
