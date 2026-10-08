package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	cloud "github.com/alexgorbatchev/go-remarkable-cloud"
)

func TestAuthPairPermissionsAndTokens(t *testing.T) {
	const (
		expectedDeviceToken = "DEVICE-TOKEN-xyz-12345"
		expectedUserToken   = "USER-TOKEN-abc-67890"
	)

	var (
		deviceCalls atomic.Int32
		userCalls   atomic.Int32
		failNextUser atomic.Bool
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token/json/2/device/new":
			deviceCalls.Add(1)
			body, _ := io.ReadAll(r.Body)
			var payload struct {
				Code string `json:"code"`
			}
			_ = json.Unmarshal(body, &payload)
			if len(payload.Code) != 8 {
				http.Error(w, "invalid code length", http.StatusBadRequest)
				return
			}
			w.Write([]byte(expectedDeviceToken))

		case "/token/json/2/user/new", "/token/v2/user":
			userCalls.Add(1)
			if failNextUser.Swap(false) {
				http.Error(w, "service unavailable", http.StatusServiceUnavailable)
				return
			}
			authHeader := r.Header.Get("Authorization")
			expectedBearer := "Bearer " + expectedDeviceToken
			if authHeader != expectedBearer {
				http.Error(w, "invalid device token bearer", http.StatusUnauthorized)
				return
			}
			w.Write([]byte(expectedUserToken))

		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	t.Setenv("REMARKABLE_HOST", server.URL)

	// Subtest 1: Missing credentials directory and permissions (0700 dir, 0600 file) + correct token storage
	t.Run("missing_directory_and_permissions", func(t *testing.T) {
		tempDir := t.TempDir()
		configDir := filepath.Join(tempDir, "fresh-dir", "remarkable-cli")
		configPath := filepath.Join(configDir, "credentials")

		out, err := executeRoot("auth", "pair", "12345678", "--config", configPath)
		if err != nil {
			t.Fatalf("auth pair failed: %v (output: %s)", err, out)
		}
		if !strings.Contains(out, "paired") {
			t.Fatalf("auth pair output missing 'paired': %s", out)
		}

		// Verify directory mode is 0700
		dirInfo, err := os.Stat(configDir)
		if err != nil {
			t.Fatalf("stat config dir failed: %v", err)
		}
		if perm := dirInfo.Mode().Perm(); perm != 0700 {
			t.Errorf("config directory mode = %04o, want 0700", perm)
		}

		// Verify file mode is 0600
		fileInfo, err := os.Stat(configPath)
		if err != nil {
			t.Fatalf("stat config file failed: %v", err)
		}
		if perm := fileInfo.Mode().Perm(); perm != 0600 {
			t.Errorf("config file mode = %04o, want 0600", perm)
		}

		// Verify saved tokens
		cfg, err := cloud.ReadConfigFile(configPath)
		if err != nil {
			t.Fatalf("read config file failed: %v", err)
		}
		if cfg.DeviceToken != expectedDeviceToken {
			t.Errorf("saved deviceToken = %q, want %q", cfg.DeviceToken, expectedDeviceToken)
		}
		if cfg.UserToken != expectedUserToken {
			t.Errorf("saved userToken = %q, want %q", cfg.UserToken, expectedUserToken)
		}

		// Verify subsequent 'auth token' works and uses the saved device token
		tokenOut, err := executeRoot("auth", "token", "--config", configPath)
		if err != nil {
			t.Fatalf("auth token failed: %v (out: %s)", err, tokenOut)
		}
		if strings.TrimSpace(tokenOut) != expectedUserToken {
			t.Errorf("auth token output = %q, want %q", strings.TrimSpace(tokenOut), expectedUserToken)
		}
	})

	// Subtest 2: Pairing over an existing 0644 file tightens mode to 0600
	t.Run("existing_0644_file_tightened", func(t *testing.T) {
		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, "existing-credentials")
		if err := os.WriteFile(configPath, []byte("devicetoken: old\n"), 0644); err != nil {
			t.Fatal(err)
		}
		// Confirm it was 0644
		if fi, err := os.Stat(configPath); err != nil || fi.Mode().Perm() != 0644 {
			t.Fatalf("failed to create 0644 file: %v", err)
		}

		out, err := executeRoot("auth", "pair", "87654321", "--config", configPath)
		if err != nil {
			t.Fatalf("auth pair failed: %v (out: %s)", err, out)
		}

		fileInfo, err := os.Stat(configPath)
		if err != nil {
			t.Fatalf("stat config file failed: %v", err)
		}
		if perm := fileInfo.Mode().Perm(); perm != 0600 {
			t.Errorf("existing file mode after pair = %04o, want 0600", perm)
		}

		cfg, err := cloud.ReadConfigFile(configPath)
		if err != nil {
			t.Fatalf("read config file failed: %v", err)
		}
		if cfg.DeviceToken != expectedDeviceToken {
			t.Errorf("saved deviceToken = %q, want %q", cfg.DeviceToken, expectedDeviceToken)
		}
	})

	// Subtest 3: Device paired but initial user token renewal fails (503)
	t.Run("partial_failure_saves_device_token", func(t *testing.T) {
		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, "partial-credentials")
		failNextUser.Store(true)

		_, err := executeRoot("auth", "pair", "12345678", "--config", configPath)
		if err == nil {
			t.Fatal("expected error on renewal failure, got nil")
		}
		if exitStatus(err) != 5 {
			t.Errorf("exit status for 503 renewal failure = %d, want 5", exitStatus(err))
		}

		// Device token must still be saved!
		fileInfo, err := os.Stat(configPath)
		if err != nil {
			t.Fatalf("config file was not created on partial failure: %v", err)
		}
		if perm := fileInfo.Mode().Perm(); perm != 0600 {
			t.Errorf("partial failure file mode = %04o, want 0600", perm)
		}

		cfg, err := cloud.ReadConfigFile(configPath)
		if err != nil {
			t.Fatalf("read config file failed: %v", err)
		}
		if cfg.DeviceToken != expectedDeviceToken {
			t.Errorf("saved deviceToken on partial failure = %q, want %q", cfg.DeviceToken, expectedDeviceToken)
		}
	})
}
