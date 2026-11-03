package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveConfigPath(t *testing.T) {
	t.Run("explicit override", func(t *testing.T) {
		if got := ResolveConfigPath("/tmp/custom.rmapi"); got != "/tmp/custom.rmapi" {
			t.Fatalf("expected /tmp/custom.rmapi, got %s", got)
		}
	})

	t.Run("env override", func(t *testing.T) {
		t.Setenv("REMARKABLE_CONFIG", "/tmp/env.rmapi")
		if got := ResolveConfigPath(""); got != "/tmp/env.rmapi" {
			t.Fatalf("expected /tmp/env.rmapi, got %s", got)
		}
	})

	t.Run("xdg config home override", func(t *testing.T) {
		t.Setenv("REMARKABLE_CONFIG", "")
		tmpXDG := t.TempDir()
		xdgConfigDir := filepath.Join(tmpXDG, "remarkable-cli")
		if err := os.MkdirAll(xdgConfigDir, 0755); err != nil {
			t.Fatal(err)
		}
		xdgConfigFile := filepath.Join(xdgConfigDir, "credentials")
		if err := os.WriteFile(xdgConfigFile, []byte("devicetoken: test-device\nusertoken: test-user\n"), 0600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("XDG_CONFIG_HOME", tmpXDG)

		if got := ResolveConfigPath(""); got != xdgConfigFile {
			t.Fatalf("expected %s, got %s", xdgConfigFile, got)
		}
	})

	t.Run("fallback to ~/.rmapi", func(t *testing.T) {
		tmpHome := t.TempDir()
		t.Setenv("HOME", tmpHome)
		t.Setenv("USERPROFILE", tmpHome)
		t.Setenv("REMARKABLE_CONFIG", "")
		t.Setenv("XDG_CONFIG_HOME", "")

		expected := filepath.Join(tmpHome, ".rmapi")
		if got := ResolveConfigPath(""); got != expected {
			t.Fatalf("expected %s, got %s", expected, got)
		}
	})
}

func TestResolveCacheDir(t *testing.T) {
	t.Run("explicit override", func(t *testing.T) {
		if got := ResolveCacheDir("/tmp/cache"); got != "/tmp/cache" {
			t.Fatalf("expected /tmp/cache, got %s", got)
		}
	})

	t.Run("env override", func(t *testing.T) {
		t.Setenv("REMARKABLE_CACHE_DIR", "/tmp/env_cache")
		if got := ResolveCacheDir(""); got != "/tmp/env_cache" {
			t.Fatalf("expected /tmp/env_cache, got %s", got)
		}
	})

	t.Run("xdg cache home", func(t *testing.T) {
		t.Setenv("REMARKABLE_CACHE_DIR", "")
		tmpXDG := t.TempDir()
		t.Setenv("XDG_CACHE_HOME", tmpXDG)
		expectedXDG := filepath.Join(tmpXDG, "remarkable-cli")
		if got := ResolveCacheDir(""); got != expectedXDG {
			t.Fatalf("expected %s, got %s", expectedXDG, got)
		}
	})

	t.Run("default fallback", func(t *testing.T) {
		tmpHome := t.TempDir()
		t.Setenv("HOME", tmpHome)
		t.Setenv("USERPROFILE", tmpHome)
		t.Setenv("REMARKABLE_CACHE_DIR", "")
		t.Setenv("XDG_CACHE_HOME", "")
		expected := filepath.Join(tmpHome, ".cache", "remarkable-cli")
		if got := ResolveCacheDir(""); got != expected {
			t.Fatalf("expected %s, got %s", expected, got)
		}
	})
}

func TestCredentialsPathIgnoresFormerJSONFilename(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("REMARKABLE_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", home)
	dir := filepath.Join(home, "remarkable-cli")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(dir, "config.json")
	if err := os.WriteFile(old, []byte("devicetoken: old\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := ResolveConfigPath(""); got != filepath.Join(home, ".rmapi") {
		t.Fatalf("selected former filename: %s", got)
	}
	credentials := filepath.Join(dir, "credentials")
	if err := os.WriteFile(credentials, []byte("devicetoken: current\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := ResolveConfigPath(""); got != credentials {
		t.Fatalf("credentials path: %s", got)
	}
	if got := ResolveConfigPath(old); got != old {
		t.Fatalf("explicit override ignored: %s", got)
	}
}
