package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/alexgorbatchev/remarkable-cli/internal/agent"
	"github.com/alexgorbatchev/remarkable-cli/internal/doc"
	"github.com/spf13/cobra"
)

func printImportResult(w io.Writer, result *doc.ImportResult) error {
	var buf bytes.Buffer
	if err := agent.PrintKeyValues(&buf, []agent.KeyValuePair{{Key: "state", Value: string(result.State)}}); err != nil {
		return err
	}
	for _, name := range result.Uploaded {
		if err := agent.PrintKeyValues(&buf, []agent.KeyValuePair{{Key: "uploaded", Value: name}}); err != nil {
			return err
		}
	}
	rows := make([][]string, 0, len(result.Pages))
	for _, page := range result.Pages {
		rows = append(rows, []string{fmt.Sprint(page.Page), page.PageID, page.Source, string(result.State)})
	}
	if len(rows) > 0 {
		if err := agent.PrintTable(&buf, []string{"PAGE", "PAGE ID", "SOURCE", "STATE"}, rows); err != nil {
			return err
		}
	}
	if _, err := io.Copy(w, &buf); err != nil {
		return fmt.Errorf("writing import results: %w", err)
	}
	return nil
}

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
			return afterCommit(result.State, errors.Join(err, printImportResult(cmd.OutOrStdout(), result)))
		},
	}
	cmd.Flags().Var(checkedString(&mappingPath, "", validString(requireNonEmptyPath)), "mapping", "Required JSON file mapping native stroke paths to 0-based destination pages")
	if err := cmd.MarkFlagRequired("mapping"); err != nil {
		panic(err)
	} // Constructor invariant: the flag was just registered.
	return cmd
}
