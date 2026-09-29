package main

import (
	"context"
	"fmt"
	"os"

	cloud "github.com/alexgorbatchev/go-remarkable-cloud"
	"github.com/alexgorbatchev/remarkable-sync/internal/agent"
	"github.com/alexgorbatchev/remarkable-sync/internal/sync"
	"github.com/spf13/cobra"
)

func newAuthCommand() *cobra.Command {
	var configFile string

	cmd := &cobra.Command{
		Use:   "auth <code>",
		Short: "Pair device token with 8-character pairing code",
		Long:  "Pairs your device with reMarkable Cloud using an 8-character one-time code from https://my.remarkable.com/pair/app",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			code := args[0]
			cfgPath := sync.ResolveConfigPath(configFile)

			client, err := cloud.NewClient(
				cloud.WithConfigFile(cfgPath),
				cloud.WithAutoRenew(false),
			)
			if err != nil {
				return fmt.Errorf("init client: %w", err)
			}

			ctx := context.Background()
			deviceToken, err := client.PairDevice(ctx, code)
			if err != nil {
				if agent.IsAgentMode() {
					fmt.Fprintf(cmd.ErrOrStderr(), "ERR: pairing failed: %v\n", err)
				} else {
					fmt.Fprintf(cmd.ErrOrStderr(), "[ERROR] Device pairing failed: %v\n", err)
				}
				os.Exit(2)
			}

			if agent.IsAgentMode() {
				fmt.Fprintln(cmd.OutOrStdout(), "status: paired")
				fmt.Fprintln(cmd.OutOrStdout(), "config: "+cfgPath)
				return nil
			}

			fmt.Fprintf(cmd.OutOrStdout(), "[OK] Device paired successfully (token length: %d)\n", len(deviceToken))
			fmt.Fprintf(cmd.OutOrStdout(), "Saved configuration to: %s\n", cfgPath)
			return nil
		},
	}

	cmd.Flags().StringVarP(&configFile, "config", "c", "", "Path to config file (defaults to ~/.rmapi)")
	return cmd
}
