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

func newDocSettingsCmd() *cobra.Command {
	group := &cobra.Command{Use: "settings", Short: "Manage document tags and view settings", Args: cobra.NoArgs}
	var mappingPath string
	var replaceViewport bool
	transfer := &cobra.Command{Use: "transfer <source-uuid> <destination-uuid>", Short: "Copy mapped tags and view settings to a separate document", Args: cobra.MatchAll(cobra.ExactArgs(2), settingsIdentityArgs), RunE: func(cmd *cobra.Command, args []string) error {
		mapping, err := doc.LoadSettingsMapping(mappingPath)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Minute)
		defer cancel()
		client, err := newCloudClient(ctx)
		if err != nil {
			return err
		}
		result, err := doc.TransferSettings(ctx, client, args[0], args[1], doc.SettingsOptions{Mapping: mapping, ReplaceViewport: replaceViewport})
		if result != nil {
			return afterCommit(result.State, errors.Join(err, printSettingsResult(cmd.OutOrStdout(), result)))
		}
		return err
	}}
	transfer.Flags().Var(checkedString(&mappingPath, "", validString(requireNonEmptyPath)), "mapping", "Required JSON array mapping 0-based source_page and destination_page indexes")
	transfer.Flags().BoolVar(&replaceViewport, "replace-viewport", false, "Replace differing destination view settings and report every difference")
	if err := transfer.MarkFlagRequired("mapping"); err != nil {
		panic(err)
	} // Registered flag is a constructor invariant.
	group.AddCommand(transfer)
	return group
}

func printSettingsResult(w io.Writer, result *doc.SettingsResult) error {
	var buf bytes.Buffer
	agent.PrintKeyValues(&buf, []agent.KeyValuePair{
		{Key: "state", Value: string(result.State)},
		{Key: "source_id", Value: result.SourceID},
		{Key: "destination_id", Value: result.DestinationID},
		{Key: "source_hash", Value: result.SourceHash},
		{Key: "destination_hash", Value: result.DestinationHash},
		{Key: "root_hash", Value: result.RootHash},
		{Key: "generation", Value: fmt.Sprint(result.Generation)},
	})
	for _, name := range result.Uploaded {
		agent.PrintKeyValues(&buf, []agent.KeyValuePair{{Key: "uploaded", Value: name}})
	}
	rows := make([][]string, 0, len(result.ViewportDifferences))
	for _, diff := range result.ViewportDifferences {
		destination, source := "absent", "absent"
		if diff.Destination != nil {
			destination = string(diff.Destination)
		}
		if diff.Source != nil {
			source = string(diff.Source)
		}
		rows = append(rows, []string{diff.Field, fmt.Sprint(diff.Destination != nil), destination, fmt.Sprint(diff.Source != nil), source})
	}
	if len(rows) > 0 {
		agent.PrintTable(&buf, []string{"FIELD", "DESTINATION PRESENT", "DESTINATION VALUE", "SOURCE PRESENT", "SOURCE VALUE"}, rows)
	}
	if _, err := io.Copy(w, &buf); err != nil {
		return fmt.Errorf("writing settings verification output: %w", err)
	}
	return nil
}
