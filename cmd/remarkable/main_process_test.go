package main

import (
	"bytes"
	"context"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
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
	// stdout receives the child's standard output; nil captures it.
	stdout *os.File
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
	if run.stdout != nil {
		cmd.Stdout = run.stdout
	}
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

// TestDocLinksWithoutPDF runs doc links on a handwritten notebook, which has
// no PDF and therefore no links to read.
func TestDocLinksWithoutPDF(t *testing.T) {
	for _, mode := range errorModes {
		t.Run(mode.agent, func(t *testing.T) {
			t.Setenv("AGENT", mode.agent)
			ts, configPath := setupCLITestEnv(t)
			defer ts.Close()
			got := runMainProcess(t, mainRun{
				args: []string{"doc", "links", "notebook-1", "--no-cache"},
				env:  map[string]string{"REMARKABLE_HOST": ts.URL, "REMARKABLE_CONFIG": configPath},
			})
			want := mode.prefix + `document "notebook-1" has no background PDF` + "\n"
			if got.exitCode != 1 {
				t.Errorf("exit status = %d, want 1", got.exitCode)
			}
			if got.stdout != "" {
				t.Errorf("stdout = %q, want empty", got.stdout)
			}
			if got.stderr != want {
				t.Errorf("stderr = %q, want %q", got.stderr, want)
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
				{"pairing code", []string{"auth", "pair"}, []string{"abc"}, "pairing code must be exactly 8 characters (got 3)"},
				{"settings identities", []string{"doc", "settings", "transfer"}, []string{"source", "destination", "--mapping", "m.json"}, "source and destination must be UUIDs"},
				{"identical settings UUIDs", []string{"doc", "settings", "transfer"}, []string{"00000000-0000-4000-8000-000000000001", "00000000-0000-4000-8000-000000000001", "--mapping", "m.json"}, "source and destination must be separate documents"},
				{"empty search query", []string{"doc", "search"}, []string{"doc-1", ""}, "query must not be empty or only whitespace"},
				{"blank search query", []string{"doc", "search"}, []string{"doc-1", " \r\n\t", "--word"}, "query must not be empty or only whitespace"},
				{"import destination", []string{"doc", "import"}, []string{"destination", "--mapping", "m.json"}, "destination must be a UUID"},
				{"render output", []string{"doc", "render"}, []string{"doc-1", "-o", ""}, `invalid argument "" for "-o, --output" flag: path must not be empty`},
				{"import mapping", []string{"doc", "import"}, []string{"00000000-0000-4000-8000-000000000001", "--mapping", ""}, `invalid argument "" for "--mapping" flag: path must not be empty`},
				{"settings mapping", []string{"doc", "settings", "transfer"}, []string{"00000000-0000-4000-8000-000000000001", "00000000-0000-4000-8000-000000000002", "--mapping", ""}, `invalid argument "" for "--mapping" flag: path must not be empty`},
				{"archive output", []string{"doc", "archive"}, []string{"doc-1", "-o", ""}, `invalid argument "" for "-o, --output" flag: path must not be empty`},
				{"upload title", []string{"doc", "upload"}, []string{"in.pdf", "--title", " ", "--evidence", "e.json"}, `invalid argument " " for "--title" flag: upload title must be nonempty and contain no control characters`},
				{"upload folder", []string{"doc", "upload"}, []string{"in.pdf", "--title", "T", "--evidence", "e.json", "--folder", "inbox"}, `invalid argument "inbox" for "--folder" flag: upload folder must be a collection UUID`},
				{"upload evidence", []string{"doc", "upload"}, []string{"in.pdf", "--title", "T", "--evidence", ""}, `invalid argument "" for "--evidence" flag: upload evidence path is required`},
				{"cat format", []string{"doc", "cat"}, []string{"doc-1", "--format", "png"}, `invalid argument "png" for "--format" flag: unsupported format "png" (choose from: pdf, text, rm, svg)`},
				{"sync format", []string{"doc", "sync"}, []string{"doc-1", "--format", "pdf"}, `invalid argument "pdf" for "--format" flag: unsupported format "pdf" (choose from: png, svg, rm)`},
				{"cat page", []string{"doc", "cat"}, []string{"doc-1", "--page", "-1"}, `invalid argument "-1" for "--page" flag: page index must be 0 or greater`},
				{"links page", []string{"doc", "links"}, []string{"doc-1", "--page", "-1"}, `invalid argument "-1" for "--page" flag: page index must be 0 or greater`},
				{"render page", []string{"doc", "render"}, []string{"doc-1", "-o", "out.png", "--page", "-1"}, `invalid argument "-1" for "--page" flag: page index must be 0 or greater`},
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

func TestEveryRunnableCommandSilencesUsageOnceRunning(t *testing.T) {
	for key, value := range isolatedMainEnv(t) {
		t.Setenv(key, value)
	}
	root := newRootCmd()
	root.SetOut(new(bytes.Buffer))
	root.SetErr(new(bytes.Buffer))
	prepareRoot(root)
	ctx, cancel := context.WithCancel(t.Context())
	cancel() // Command bodies stop at their first cloud request.

	// Each command runs with the first argument list its own validator accepts.
	argLists := [][]string{
		nil,
		{"12345678"},
		{"00000000-0000-4000-8000-000000000001"},
		{"00000000-0000-4000-8000-000000000001", "00000000-0000-4000-8000-000000000002"},
	}
	var ran []string
	var visit func(*cobra.Command)
	visit = func(c *cobra.Command) {
		for _, child := range c.Commands() {
			visit(child)
		}
		if c.RunE == nil {
			return
		}
		i := slices.IndexFunc(argLists, func(args []string) bool { return c.ValidateArgs(args) == nil })
		if i < 0 {
			t.Fatalf("%s: no test argument list passes its validator", c.CommandPath())
		}
		if c.SilenceUsage {
			t.Fatalf("%s: usage silenced before RunE ran", c.CommandPath())
		}
		c.SetContext(ctx)
		_ = c.RunE(c, argLists[i]) // Only the usage setting matters here.
		if !c.SilenceUsage {
			t.Errorf("%s: RunE did not silence usage", c.CommandPath())
		}
		ran = append(ran, c.CommandPath())
	}
	visit(root)
	for _, generated := range []string{"remarkable completion bash", "remarkable completion zsh", "remarkable completion fish", "remarkable completion powershell"} {
		if !slices.Contains(ran, generated) {
			t.Errorf("Cobra-generated %q was not checked; checked: %q", generated, ran)
		}
	}
}

func TestMainReportsGeneratedCommandFailureWithoutUsage(t *testing.T) {
	for _, mode := range errorModes {
		t.Run(mode.agent, func(t *testing.T) {
			t.Setenv("AGENT", mode.agent)
			readOnly, err := os.Open(os.Args[0])
			if err != nil {
				t.Fatal(err)
			}
			defer readOnly.Close()
			// Writing the script to a read-only descriptor fails inside the
			// completion command's RunE.
			got := runMainProcess(t, mainRun{args: []string{"completion", "bash"}, stdout: readOnly})
			want := mode.prefix + "write /dev/stdout: bad file descriptor\n"
			if got.exitCode != 1 {
				t.Errorf("exit status = %d, want 1", got.exitCode)
			}
			if got.stderr != want {
				t.Errorf("stderr = %q, want only the error once: %q", got.stderr, want)
			}
		})
	}
}

func TestCompletionWritesToConfiguredOutput(t *testing.T) {
	// prepareRoot runs after the caller sets output, so the completion command
	// captures that writer rather than the process's stdout.
	out, err := executeRoot("completion", "bash")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "# bash completion V2 for remarkable") {
		t.Fatalf("completion script missing from configured output: %.80q", out)
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

// TestMainHintsPairingWhenCloudRejectsCredentials runs commands against a cloud
// that rejects every token with a newline-terminated reason, as the reMarkable
// service does.
func TestMainHintsPairingWhenCloudRejectsCredentials(t *testing.T) {
	rejecting := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "invalid Authorization header", http.StatusUnauthorized)
	}))
	defer rejecting.Close()
	paired := writeCredentials(t, pairedCredentials)
	unpaired := writeCredentials(t, "")
	png := filepath.Join(t.TempDir(), "page.png")
	const hint = ": run 'remarkable auth pair <code>' with a new code from https://my.remarkable.com/device/desktop/connect\n"
	cases := []struct {
		name    string
		args    []string
		config  string
		message string
	}{
		{
			"rejected session and device tokens",
			[]string{"doc", "render", "doc-1", "--no-cache", "-o", png},
			paired,
			`resolving document "doc-1": get root state: auth renewal failed after 401: unauthorized: missing or invalid credentials: renew user token failed with status 401: invalid Authorization header`,
		},
		{
			"rejected device token",
			[]string{"auth", "token"},
			paired,
			"fetching user token: unauthorized: missing or invalid credentials: renew user token failed with status 401: invalid Authorization header",
		},
		{
			"no tokens",
			[]string{"doc", "list", "--no-cache"},
			unpaired,
			"listing cloud items: get root state: unauthorized: missing or invalid credentials: neither user token nor device token is configured",
		},
	}
	for _, mode := range errorModes {
		t.Run(mode.agent, func(t *testing.T) {
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					got := runMainProcess(t, mainRun{
						args: tc.args,
						env:  map[string]string{"AGENT": mode.agent, "REMARKABLE_HOST": rejecting.URL, "REMARKABLE_CONFIG": tc.config},
					})
					want := mode.prefix + tc.message + hint
					if got.exitCode != 1 {
						t.Errorf("exit status = %d, want 1", got.exitCode)
					}
					if got.stdout != "" {
						t.Errorf("stdout = %q, want empty", got.stdout)
					}
					if got.stderr != want {
						t.Errorf("stderr = %q, want %q", got.stderr, want)
					}
				})
			}
		})
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
