package config

import (
	"os"
	"path/filepath"
)

// ResolveConfigPath resolves the path to the reMarkable credentials file.
// Priority:
// 1. Explicit override flag
// 2. $REMARKABLE_CONFIG env var
// 3. XDG path: $XDG_CONFIG_HOME/remarkable-cli/config.json (or ~/.config/remarkable-cli/config.json)
// 4. Default fallback: ~/.rmapi
func ResolveConfigPath(override string) string {
	if override != "" {
		return override
	}
	if env := os.Getenv("REMARKABLE_CONFIG"); env != "" {
		return env
	}

	xdg := os.Getenv("XDG_CONFIG_HOME")
	if xdg == "" {
		if home, err := os.UserHomeDir(); err == nil {
			xdg = filepath.Join(home, ".config")
		}
	}
	if xdg != "" {
		candidate := filepath.Join(xdg, "remarkable-cli", "config.json")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".rmapi")
	}
	return ".rmapi"
}

// ResolveCacheDir resolves the content-addressed download cache. Cloud commands
// store the manifests and files they download through it (metadata, content,
// PDFs and strokes) under blobs/, each named by its hash, and never prune it.
// It holds no credentials, so deleting it only forces fresh downloads.
// --no-cache bypasses the cache without resolving this directory.
// Priority:
// 1. Explicit override flag
// 2. $REMARKABLE_CACHE_DIR env var
// 3. XDG path: $XDG_CACHE_HOME/remarkable-cli (or ~/.cache/remarkable-cli)
func ResolveCacheDir(override string) string {
	if override != "" {
		return override
	}
	if env := os.Getenv("REMARKABLE_CACHE_DIR"); env != "" {
		return env
	}

	xdg := os.Getenv("XDG_CACHE_HOME")
	if xdg == "" {
		if home, err := os.UserHomeDir(); err == nil {
			xdg = filepath.Join(home, ".cache")
		}
	}
	if xdg != "" {
		return filepath.Join(xdg, "remarkable-cli")
	}
	return ".cache/remarkable-cli"
}
