package main

import (
	"bytes"
	"errors"
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

type mainResult struct {
	stdout   string
	stderr   string
	exitCode int
}

func runMainProcess(t *testing.T, args ...string) mainResult {
	t.Helper()
	cmd := exec.Command(os.Args[0], args...)
	cmd.Env = append(os.Environ(), runMainEnv+"=1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("running main with %q: %v", args, err)
		}
		exitCode = exitErr.ExitCode()
	}
	return mainResult{stdout: stdout.String(), stderr: stderr.String(), exitCode: exitCode}
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
	for _, mode := range []string{"0", "1"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("AGENT", mode)
			t.Setenv("COLUMNS", "200")
			t.Setenv("XDG_CACHE_HOME", t.TempDir())
			ts, _ := setupCLITestEnvWithFailure(t, "/sync/v3/root")
			defer ts.Close()

			missingStroke := filepath.Join(t.TempDir(), "missing.rm")
			png := filepath.Join(t.TempDir(), "page.png")
			cases := []struct {
				name string
				args []string
			}{
				{"local file", []string{"stroke", "inspect", missingStroke}},
				{"cloud request", []string{"doc", "render", "doc-1", "--no-cache", "-o", png}},
			}
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					want := inProcessError(t, tc.args...).Error() + "\n"
					got := runMainProcess(t, tc.args...)
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
	for _, mode := range []string{"0", "1"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("AGENT", mode)
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
					want := usageScreen(t, tc.command...) + "\n" + tc.message + "\n"
					got := runMainProcess(t, append(tc.command, tc.args...)...)
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

func TestMainReportsUnknownCommandOnce(t *testing.T) {
	got := runMainProcess(t, "bogus")
	want := "unknown command \"bogus\" for \"remarkable\"\n"
	if got.exitCode != 1 {
		t.Errorf("exit status = %d, want 1", got.exitCode)
	}
	if got.stderr != want {
		t.Errorf("stderr = %q, want only the error once: %q", got.stderr, want)
	}
}
