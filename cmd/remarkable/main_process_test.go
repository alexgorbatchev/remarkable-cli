package main

import (
	"bytes"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// runMainEnv makes the test binary run main instead of the tests, so a test can
// observe main's real stderr and exit status in a separate process.
const runMainEnv = "REMARKABLE_TEST_RUN_MAIN"

func TestMain(m *testing.M) {
	if os.Getenv(runMainEnv) == "1" {
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// errorModes pairs each output mode with the prefix main writes before the error.
var errorModes = []struct{ agent, prefix string }{
	{"0", "[ERROR] "},
	{"1", "ERR: "},
}

// mainRun describes one child-process run of main.
type mainRun struct {
	args []string
	// env overrides the isolated defaults from isolatedMainEnv.
	env map[string]string
}

type mainResult struct {
	stdout   string
	stderr   string
	exitCode int
	// env holds the variables the child received on top of the test's own.
	env map[string]string
}

// isolatedMainEnv points every credential, cache and cloud setting at
// temporary or closed locations, so a child run can never read the
// developer's credentials or reach the live cloud.
func isolatedMainEnv(t *testing.T) map[string]string {
	t.Helper()
	home := t.TempDir()
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	return map[string]string{
		"HOME":                 home,
		"XDG_CONFIG_HOME":      filepath.Join(home, "config"),
		"XDG_CACHE_HOME":       filepath.Join(home, "cache"),
		"REMARKABLE_CONFIG":    filepath.Join(home, "missing-config.json"),
		"REMARKABLE_CACHE_DIR": filepath.Join(home, "cache", "remarkable-cli"),
		"REMARKABLE_HOST":      closed.URL,
	}
}

func runMainProcess(t *testing.T, run mainRun) mainResult {
	t.Helper()
	env := isolatedMainEnv(t)
	maps.Copy(env, run.env)
	cmd := exec.Command(os.Args[0], run.args...)
	// exec.Cmd uses the last value of a duplicated key, so these entries
	// replace any inherited value.
	cmd.Env = append(os.Environ(), runMainEnv+"=1")
	for key, value := range env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("running main with %q: %v", run.args, err)
		}
		exitCode = exitErr.ExitCode()
	}
	return mainResult{stdout: stdout.String(), stderr: stderr.String(), exitCode: exitCode, env: env}
}

// usageScreen renders the usage screen Cobra prints after an invocation error
// for the command that args select.
func usageScreen(t *testing.T, args ...string) string {
	t.Helper()
	root := newRootCmd()
	target, _, err := root.Find(args)
	if err != nil {
		t.Fatalf("finding command for %q: %v", args, err)
	}
	target.InitDefaultHelpFlag()
	return target.UsageString()
}

// inProcessError returns the error a command returns, which main must report.
func inProcessError(t *testing.T, args ...string) error {
	t.Helper()
	out, err := executeRoot(args...)
	if err == nil {
		t.Fatalf("expected %q to fail; output: %s", args, out)
	}
	return err
}

func TestMainReportsRuntimeFailureOnceWithoutUsage(t *testing.T) {
	for _, mode := range errorModes {
		t.Run(mode.agent, func(t *testing.T) {
			t.Setenv("AGENT", mode.agent)
			t.Setenv("COLUMNS", "200")
			t.Setenv("XDG_CACHE_HOME", t.TempDir())
			ts, configPath := setupCLITestEnvWithFailure(t, "/sync/v3/root")
			defer ts.Close()
			cloudEnv := map[string]string{"REMARKABLE_HOST": ts.URL, "REMARKABLE_CONFIG": configPath}

			missingStroke := filepath.Join(t.TempDir(), "missing.rm")
			png := filepath.Join(t.TempDir(), "page.png")
			cases := []struct {
				name string
				args []string
				env  map[string]string
			}{
				{"local file", []string{"stroke", "inspect", missingStroke}, nil},
				{"cloud request", []string{"doc", "render", "doc-1", "--no-cache", "-o", png}, cloudEnv},
			}
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					want := mode.prefix + inProcessError(t, tc.args...).Error() + "\n"
					got := runMainProcess(t, mainRun{args: tc.args, env: tc.env})
					if got.exitCode != 1 {
						t.Errorf("exit status = %d, want 1", got.exitCode)
					}
					if got.stdout != "" {
						t.Errorf("stdout = %q, want empty", got.stdout)
					}
					if got.stderr != want {
						t.Errorf("stderr = %q, want only the error once: %q", got.stderr, want)
					}
				})
			}
		})
	}
}

func TestMainReportsInvocationErrorOnceAfterUsage(t *testing.T) {
	for _, mode := range errorModes {
		t.Run(mode.agent, func(t *testing.T) {
			t.Setenv("AGENT", mode.agent)
			t.Setenv("COLUMNS", "200")
			cases := []struct {
				name    string
				command []string
				args    []string
				message string
			}{
				{"unknown flag", []string{"stroke", "inspect"}, []string{"--bogus", "x.rm"}, "unknown flag: --bogus"},
				{"argument count", []string{"stroke", "inspect"}, nil, "accepts 1 arg(s), received 0"},
				{"required flag", []string{"doc", "render"}, []string{"doc-1"}, `required flag(s) "output" not set`},
			}
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					want := usageScreen(t, tc.command...) + "\n" + mode.prefix + tc.message + "\n"
					got := runMainProcess(t, mainRun{args: append(tc.command, tc.args...)})
					if got.exitCode != 1 {
						t.Errorf("exit status = %d, want 1", got.exitCode)
					}
					if got.stdout != "" {
						t.Errorf("stdout = %q, want empty", got.stdout)
					}
					if got.stderr != want {
						t.Errorf("stderr = %q, want usage followed by the error once: %q", got.stderr, want)
					}
				})
			}
		})
	}
}

func TestMainProcessIgnoresInheritedCredentials(t *testing.T) {
	t.Setenv("AGENT", "1")
	// The test process points at working credentials and a reachable cloud; a
	// child run must use its isolated, missing credentials instead.
	ts, _ := setupCLITestEnv(t)
	defer ts.Close()
	got := runMainProcess(t, mainRun{args: []string{"doc", "list"}})
	want := "ERR: credentials file not found at " + got.env["REMARKABLE_CONFIG"] + ": run 'remarkable auth pair <code>' first\n"
	if got.exitCode != 1 {
		t.Errorf("exit status = %d, want 1", got.exitCode)
	}
	if got.stdout != "" {
		t.Errorf("stdout = %q, want empty", got.stdout)
	}
	if got.stderr != want {
		t.Errorf("stderr = %q, want %q", got.stderr, want)
	}
}

func TestMainReportsUnknownCommandOnce(t *testing.T) {
	for _, mode := range errorModes {
		t.Run(mode.agent, func(t *testing.T) {
			t.Setenv("AGENT", mode.agent)
			got := runMainProcess(t, mainRun{args: []string{"bogus"}})
			want := mode.prefix + "unknown command \"bogus\" for \"remarkable\"\n"
			if got.exitCode != 1 {
				t.Errorf("exit status = %d, want 1", got.exitCode)
			}
			if got.stderr != want {
				t.Errorf("stderr = %q, want only the error once: %q", got.stderr, want)
			}
		})
	}
}
