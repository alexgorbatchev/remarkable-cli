package main

import (
	"context"
	"fmt"
	"os"

	cloud "github.com/alexgorbatchev/go-remarkable-cloud"
	"github.com/alexgorbatchev/remarkable-sync/internal/config"
)

var (
	cfgFlag      string
	cacheDirFlag string
)

func newCloudClient(ctx context.Context) (*cloud.Client, error) {
	configPath := config.ResolveConfigPath(cfgFlag)
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("credentials file not found at %s: run 'remarkable-sync auth pair <code>' first", configPath)
	}

	client, err := cloud.NewClient(
		cloud.WithConfigFile(configPath),
		cloud.WithAutoRenew(true),
		cloud.WithSaveOnRenew(true),
	)
	if err != nil {
		return nil, fmt.Errorf("initializing cloud client: %w", err)
	}

	return client, nil
}
