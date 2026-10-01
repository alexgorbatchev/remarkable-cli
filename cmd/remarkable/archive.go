package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	"github.com/alexgorbatchev/remarkable-cli/internal/agent"
	"github.com/alexgorbatchev/remarkable-cli/internal/doc"
	"github.com/spf13/cobra"
)

func newDocArchiveCmd() *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "archive <id-or-name>",
		Short: "Save a complete native document backup with verification evidence",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if output == "" {
				return fmt.Errorf("archive output path required: specify -o or --output")
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Minute)
			defer cancel()
			client, err := newCloudClient(ctx)
			if err != nil {
				return err
			}
			snapshot, err := doc.ArchiveDocument(ctx, client, args[0], output)
			if err != nil {
				return err
			}
			return printArchiveEvidence(cmd.OutOrStdout(), output, snapshot)
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "", "Required destination ZIP path; existing paths are preserved")
	if err := cmd.MarkFlagRequired("output"); err != nil {
		panic(err)
	} // Constructor invariant: the flag was just registered.
	return cmd
}

func printArchiveEvidence(w io.Writer, output string, snapshot *doc.ArchiveSnapshot) error {
	// The formatting primitives write to memory so the final output write can
	// return failures to the command instead of silently losing evidence.
	var buf bytes.Buffer
	agent.PrintKeyValues(&buf, []agent.KeyValuePair{
		{Key: "state", Value: "verified"},
		{Key: "output", Value: output},
		{Key: "document_id", Value: snapshot.DocumentID},
		{Key: "document_hash", Value: snapshot.DocumentHash},
		{Key: "root_hash", Value: snapshot.Root.Hash},
		{Key: "generation", Value: fmt.Sprint(snapshot.Root.Generation)},
		{Key: "manifest_sha256", Value: snapshot.ManifestSHA256},
	})
	rows := make([][]string, 0, len(snapshot.Files))
	for _, file := range snapshot.Files {
		rows = append(rows, []string{file.Name, file.Hash, file.SHA256, fmt.Sprint(file.Size)})
	}
	agent.PrintTable(&buf, []string{"NAME", "HASH", "SHA256", "BYTES"}, rows)
	if _, err := io.Copy(w, &buf); err != nil {
		return fmt.Errorf("writing archive evidence: %w", err)
	}
	return nil
}
