package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveConfigPath(t *testing.T) {
	// 1. Explicit override
	if got := ResolveConfigPath("/tmp/custom.rmapi"); got != "/tmp/custom.rmapi" {
		t.Fatalf("expected /tmp/custom.rmapi, got %s", got)
	}

	// 2. Env override
	origEnv := os.Getenv("REMARKABLE_CONFIG")
	defer os.Setenv("REMARKABLE_CONFIG", origEnv)

	os.Setenv("REMARKABLE_CONFIG", "/tmp/env.rmapi")
	if got := ResolveConfigPath(""); got != "/tmp/env.rmapi" {
		t.Fatalf("expected /tmp/env.rmapi, got %s", got)
	}

	// 2b. XDG_CONFIG_HOME override
	origXDG := os.Getenv("XDG_CONFIG_HOME")
	defer os.Setenv("XDG_CONFIG_HOME", origXDG)
	os.Unsetenv("REMARKABLE_CONFIG")

	tmpXDG := t.TempDir()
	xdgConfigDir := filepath.Join(tmpXDG, "remarkable-cli")
	_ = os.MkdirAll(xdgConfigDir, 0755)
	xdgConfigFile := filepath.Join(xdgConfigDir, "config.json")
	_ = os.WriteFile(xdgConfigFile, []byte("{}"), 0644)
	os.Setenv("XDG_CONFIG_HOME", tmpXDG)

	if got := ResolveConfigPath(""); got != xdgConfigFile {
		t.Fatalf("expected %s, got %s", xdgConfigFile, got)
	}
	os.Unsetenv("XDG_CONFIG_HOME")

	// 3. Fallback to ~/.rmapi
	os.Unsetenv("REMARKABLE_CONFIG")
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".rmapi")
	if got := ResolveConfigPath(""); got != expected {
		t.Fatalf("expected %s, got %s", expected, got)
	}
}

func TestResolveCacheDir(t *testing.T) {
	// 1. Explicit override
	if got := ResolveCacheDir("/tmp/cache"); got != "/tmp/cache" {
		t.Fatalf("expected /tmp/cache, got %s", got)
	}

	// 2. Env override
	origEnv := os.Getenv("REMARKABLE_CACHE_DIR")
	defer os.Setenv("REMARKABLE_CACHE_DIR", origEnv)

	os.Setenv("REMARKABLE_CACHE_DIR", "/tmp/env_cache")
	if got := ResolveCacheDir(""); got != "/tmp/env_cache" {
		t.Fatalf("expected /tmp/env_cache, got %s", got)
	}

	// 2b. XDG_CACHE_HOME
	origXDG := os.Getenv("XDG_CACHE_HOME")
	defer os.Setenv("XDG_CACHE_HOME", origXDG)
	os.Unsetenv("REMARKABLE_CACHE_DIR")
	tmpXDG := t.TempDir()
	os.Setenv("XDG_CACHE_HOME", tmpXDG)
	expectedXDG := filepath.Join(tmpXDG, "remarkable-cli")
	if got := ResolveCacheDir(""); got != expectedXDG {
		t.Fatalf("expected %s, got %s", expectedXDG, got)
	}
	os.Unsetenv("XDG_CACHE_HOME")

	// 3. Default fallback
	os.Unsetenv("REMARKABLE_CACHE_DIR")
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".cache", "remarkable-cli")
	if got := ResolveCacheDir(""); got != expected {
		t.Fatalf("expected %s, got %s", expected, got)
	}
}
