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
	cmd.Flags().BoolVar(&opts.InitializePages, "initialize-pages", false, "Create native page IDs for every PDF page (at most 816) so doc import and doc settings transfer need no tablet open first")
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

// uploadNextStep names what a caller does next with the document's native pages.
func uploadNextStep(nativePages string) string {
	const recovery = "; use doc upload-check with this evidence before any retry"
	if nativePages == doc.NativePagesInitialized {
		return "Run doc inspect --pages, then doc import and doc settings transfer without opening the tablet first" + recovery
	}
	return "Open on tablet, sync, then inspect --pages before doc import" + recovery
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
		{Key: "next_step", Value: uploadNextStep(evidence.NativePages)},
	})
	for _, name := range evidence.Result.Uploaded {
		fmt.Fprintf(&buf, "uploaded: %s\n", name)
	}
	if _, err := io.Copy(w, &buf); err != nil {
		return fmt.Errorf("writing upload evidence output: %w", err)
	}
	return nil
}
