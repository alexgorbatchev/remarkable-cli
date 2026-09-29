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

func newStatusCommand() *cobra.Command {
	var configFile string

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Check reMarkable Cloud connectivity and auth status",
		Long:  "Inspects reMarkable Cloud token, endpoint discovery, and library reachability.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfgPath := sync.ResolveConfigPath(configFile)

			client, err := cloud.NewClient(
				cloud.WithConfigFile(cfgPath),
				cloud.WithAutoRenew(true),
				cloud.WithSaveOnRenew(true),
			)
			if err != nil {
				if agent.IsAgentMode() {
					fmt.Fprintf(cmd.ErrOrStderr(), "ERR: config error: %v\n", err)
				} else {
					fmt.Fprintf(cmd.ErrOrStderr(), "[ERROR] reMarkable config error: %v\n", err)
				}
				os.Exit(3)
			}

			ctx := context.Background()
			items, err := client.ListItems(ctx)
			if err != nil {
				if agent.IsAgentMode() {
					fmt.Fprintf(cmd.ErrOrStderr(), "ERR: remarkable cloud unavailable: %v\n", err)
				} else {
					fmt.Fprintf(cmd.ErrOrStderr(), "[ERROR] reMarkable Cloud unavailable: %v\n", err)
				}
				os.Exit(3)
			}

			if agent.IsAgentMode() {
				fmt.Fprintln(cmd.OutOrStdout(), "status: connected")
				fmt.Fprintf(cmd.OutOrStdout(), "items: %d\n", len(items))
				return nil
			}

			fmt.Fprintf(cmd.OutOrStdout(), "[OK] reMarkable Cloud connected (%d items found)\n", len(items))
			return nil
		},
	}

	cmd.Flags().StringVarP(&configFile, "config", "c", "", "Path to config file (defaults to ~/.rmapi)")
	return cmd
}
