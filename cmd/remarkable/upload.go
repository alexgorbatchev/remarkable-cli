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

func newDocUploadCmd() *cobra.Command {
	var opts doc.UploadOptions
	cmd := &cobra.Command{Use: "upload <pdf>", Short: "Upload a local PDF as a separate cloud document", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Minute)
		defer cancel()
		client, err := newCloudClient(ctx)
		if err != nil {
			return err
		}
		opts.OnProgress = func(evidence *doc.UploadEvidence) error {
			return printUploadEvidence(cmd.OutOrStdout(), opts.Evidence, evidence)
		}
		evidence, err := doc.UploadPDF(ctx, client, args[0], opts)
		if err != nil && evidence != nil {
			return afterCommit(evidence.Result.State, errors.Join(err, printUploadEvidence(cmd.OutOrStdout(), opts.Evidence, evidence)))
		}
		return err
	}}
	cmd.Flags().Var(checkedString(&opts.Title, "", validString(doc.ValidateUploadTitle)), "title", "Required display title; existing titles in the destination folder are rejected")
	cmd.Flags().Var(checkedString(&opts.Folder, "", validString(doc.ValidateUploadFolder)), "folder", "Destination folder UUID (default: root)")
	cmd.Flags().Var(checkedString(&opts.Evidence, "", validString(doc.ValidateUploadEvidencePath)), "evidence", "Required new JSON recovery path; its parent directory must exist")
	for _, name := range []string{"title", "evidence"} {
		if err := cmd.MarkFlagRequired(name); err != nil {
			panic(err)
		} // Registered flags are constructor invariants.
	}
	return cmd
}

func newDocUploadCheckCmd() *cobra.Command {
	return &cobra.Command{Use: "upload-check <evidence>", Short: "Freshly check an upload's saved identity and bytes without retrying", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := doc.ReadUploadEvidence(args[0]); err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Minute)
		defer cancel()
		client, err := newCloudClient(ctx)
		if err != nil {
			return err
		}
		evidence, checkErr := doc.CheckUpload(ctx, client, args[0])
		if checkErr != nil {
			return checkErr
		}
		return printUploadEvidence(cmd.OutOrStdout(), args[0], evidence)
	}}
}

func printUploadEvidence(w io.Writer, path string, evidence *doc.UploadEvidence) error {
	var buf bytes.Buffer
	agent.PrintKeyValues(&buf, []agent.KeyValuePair{
		{Key: "state", Value: string(evidence.Result.State)},
		{Key: "document_id", Value: evidence.Result.ID},
		{Key: "title", Value: evidence.Title},
		{Key: "folder", Value: evidence.Folder},
		{Key: "pages", Value: fmt.Sprint(evidence.Pages)},
		{Key: "document_hash", Value: evidence.Result.DocumentHash},
		{Key: "root_hash", Value: evidence.Result.RootHash},
		{Key: "generation", Value: fmt.Sprint(evidence.Result.Generation)},
		{Key: "evidence", Value: path},
		{Key: "native_pages", Value: evidence.NativePages},
		{Key: "next_step", Value: "Open on tablet, sync, then inspect --pages before doc import; use doc upload-check with this evidence before any retry"},
	})
	for _, name := range evidence.Result.Uploaded {
		fmt.Fprintf(&buf, "uploaded: %s\n", name)
	}
	if _, err := io.Copy(w, &buf); err != nil {
		return fmt.Errorf("writing upload evidence output: %w", err)
	}
	return nil
}
