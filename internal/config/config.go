package config

import (
	"os"
	"path/filepath"
)

// ResolveConfigPath resolves the path to the reMarkable credentials file.
// Priority:
// 1. Explicit override flag
// 2. $REMARKABLE_CONFIG env var
// 3. XDG path: $XDG_CONFIG_HOME/remarkable-sync/config.json (or ~/.config/remarkable-sync/config.json)
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
		candidate := filepath.Join(xdg, "remarkable-sync", "config.json")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".rmapi")
	}
	return ".rmapi"
}

// ResolveCacheDir resolves the directory for caching stationery templates and temporary files.
// Priority:
// 1. Explicit override flag
// 2. $REMARKABLE_CACHE_DIR env var
// 3. XDG path: $XDG_CACHE_HOME/remarkable-sync (or ~/.cache/remarkable-sync)
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
		return filepath.Join(xdg, "remarkable-sync")
	}
	return ".cache/remarkable-sync"
}
