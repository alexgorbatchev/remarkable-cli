package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	cloud "github.com/alexgorbatchev/go-remarkable-cloud"
	"github.com/alexgorbatchev/remarkable-sync/internal/agent"
	"github.com/alexgorbatchev/remarkable-sync/internal/config"
	"github.com/spf13/cobra"
)

func runAuthStatus(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(cmd.Context(), 15*time.Second)
	defer cancel()

	client, err := newCloudClient(ctx)
	if err != nil {
		agent.PrintStatus(cmd.OutOrStdout(), "error", fmt.Sprintf("Authentication failed: %v", err))
		return err
	}

	start := time.Now()
	root, err := client.GetRootState(ctx)
	latency := time.Since(start)

	if err != nil {
		agent.PrintStatus(cmd.OutOrStdout(), "error", fmt.Sprintf("Connection failed: %v", err))
		return err
	}

	items, _ := client.ListItems(ctx)

	if agent.IsAgentMode() {
		agent.PrintKeyValues(cmd.OutOrStdout(), []agent.KeyValuePair{
			{Key: "status", Value: "connected"},
			{Key: "generation", Value: fmt.Sprintf("%d", root.Generation)},
			{Key: "items", Value: fmt.Sprintf("%d", len(items))},
			{Key: "latency_ms", Value: fmt.Sprintf("%d", latency.Milliseconds())},
		})
		return nil
	}

	agent.PrintStatus(cmd.OutOrStdout(), "ok", "Connected to reMarkable Cloud Sync v3")
	agent.PrintSeparator(cmd.OutOrStdout())
	agent.PrintKeyValues(cmd.OutOrStdout(), []agent.KeyValuePair{
		{Key: "Status", Value: "connected"},
		{Key: "Generation", Value: fmt.Sprintf("%d", root.Generation)},
		{Key: "Total Items", Value: fmt.Sprintf("%d documents & collections", len(items))},
		{Key: "API Latency", Value: fmt.Sprintf("%s", latency.Round(time.Millisecond))},
		{Key: "Config Path", Value: config.ResolveConfigPath(cfgFlag)},
	})
	return nil
}

func newAuthCmd() *cobra.Command {
	authCmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage reMarkable Cloud authentication and device pairing",
	}

	authStatusCmd := &cobra.Command{
		Use:   "status",
		Short: "Check connection status to reMarkable Cloud API",
		RunE:  runAuthStatus,
	}

	authPairCmd := &cobra.Command{
		Use:   "pair <code>",
		Short: "Pair device using one-time code from my.remarkable.com",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			code := strings.TrimSpace(args[0])
			if len(code) != 8 {
				return fmt.Errorf("pairing code must be exactly 8 characters (got %d)", len(code))
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), 20*time.Second)
			defer cancel()

			client, err := cloud.NewClient()
			if err != nil {
				return fmt.Errorf("initializing client: %w", err)
			}

			deviceToken, err := client.PairDevice(ctx, code)
			if err != nil {
				agent.PrintStatus(cmd.OutOrStdout(), "error", fmt.Sprintf("Pairing failed: %v", err))
				return err
			}

			configPath := config.ResolveConfigPath(cfgFlag)
			if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
				return fmt.Errorf("creating config directory: %w", err)
			}

			content := fmt.Sprintf("devicetoken: %s\n", deviceToken)
			if err := os.WriteFile(configPath, []byte(content), 0600); err != nil {
				return fmt.Errorf("saving credentials to %s: %w", configPath, err)
			}

			agent.PrintStatus(cmd.OutOrStdout(), "ok", fmt.Sprintf("Device successfully paired. Credentials saved to %s", configPath))
			return nil
		},
	}

	authTokenCmd := &cobra.Command{
		Use:   "token",
		Short: "Print current active bearer user token",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 15*time.Second)
			defer cancel()

			client, err := newCloudClient(ctx)
			if err != nil {
				return err
			}

			token, err := client.RenewToken(ctx)
			if err != nil {
				return fmt.Errorf("fetching user token: %w", err)
			}

			fmt.Fprintln(cmd.OutOrStdout(), token)
			return nil
		},
	}

	authCmd.AddCommand(authStatusCmd)
	authCmd.AddCommand(authPairCmd)
	authCmd.AddCommand(authTokenCmd)
	return authCmd
}

func newStatusShortcutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Check connection status to reMarkable Cloud API (alias for 'auth status')",
		RunE:  runAuthStatus,
	}
}
