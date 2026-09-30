package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	cloud "github.com/alexgorbatchev/go-remarkable-cloud"
	"github.com/alexgorbatchev/remarkable-cli/internal/config"
)

var (
	cfgFlag      string
	cacheDirFlag string
	debugFlag    bool
	noCacheFlag  bool
)

type debugTransport struct {
	base    http.RoundTripper
	counter atomic.Int64
}

func (t *debugTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	num := t.counter.Add(1)
	start := time.Now()

	filename := req.Header.Get("rm-filename")
	fileTag := ""
	if filename != "" {
		fileTag = fmt.Sprintf(" (%s)", filename)
	}

	resp, err := t.base.RoundTrip(req)
	duration := time.Since(start).Round(time.Millisecond)

	if err != nil {
		fmt.Fprintf(os.Stderr, "[API #%d] %s %s%s -> ERR: %v (%s)\n", num, req.Method, req.URL.String(), fileTag, err, duration)
		return nil, err
	}

	fmt.Fprintf(os.Stderr, "[API #%d] %s %s%s -> %d %s (%s)\n", num, req.Method, req.URL.String(), fileTag, resp.StatusCode, http.StatusText(resp.StatusCode), duration)
	return resp, nil
}

func newCloudClient(ctx context.Context) (*cloud.Client, error) {
	configPath := config.ResolveConfigPath(cfgFlag)
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("credentials file not found at %s: run 'remarkable-cli auth pair <code>' first", configPath)
	}

	opts := []cloud.Option{
		cloud.WithConfigFile(configPath),
		cloud.WithAutoRenew(true),
		cloud.WithSaveOnRenew(true),
	}

	if host := os.Getenv("REMARKABLE_HOST"); host != "" {
		opts = append(opts,
			cloud.WithEndpoints(&cloud.Endpoints{
				RawHost:     host,
				WebappHost:  host,
				StorageHost: host,
			}),
			cloud.WithAuthBaseURL(host),
		)
	}

	if !noCacheFlag {
		opts = append(opts, cloud.WithCacheDir(config.ResolveCacheDir(cacheDirFlag)))
	}

	if debugFlag {
		baseTransport := http.DefaultTransport
		opts = append(opts, cloud.WithHTTPClient(&http.Client{
			Transport: &debugTransport{base: baseTransport},
			Timeout:   30 * time.Second,
		}))
	}

	client, err := cloud.NewClient(opts...)
	if err != nil {
		return nil, fmt.Errorf("initializing cloud client: %w", err)
	}

	return client, nil
}
