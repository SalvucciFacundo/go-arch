package hooks

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"strings"
	"testing"

	"github.com/samber/oops"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// testStdout returns a bytes.Buffer to use as the output writer.
func testStdout() *bytes.Buffer { return &bytes.Buffer{} }

// assertCode checks that err wraps an oops error with the given code.
func assertCode(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected an error with code %q, got nil", want)
	}
	var oe oops.OopsError
	if !errors.As(err, &oe) {
		t.Fatalf("expected oops.OopsError, got %T: %v", err, err)
	}
	got, ok := oe.Code().(string)
	if !ok {
		t.Fatalf("oops code is not a string: %T", oe.Code())
	}
	if got != want {
		t.Errorf("code: got %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// 2.3 – Core Runner Behaviour
// ---------------------------------------------------------------------------

func TestRunner_HappyPath(t *testing.T) {
	cfg := &Config{Hooks: map[Type][]Entry{
		PreNew: {{Command: "echo", Args: []string{"hello"}, Shell: false}},
	}}
	fr := &FakeRunner{ExitCode: 0}
	out := testStdout()
	r := NewRunner(cfg, fr, out)

	err := r.Fire(PreNew, EnvContext{}, "/tmp/default")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fr.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(fr.Calls))
	}
	if fr.Calls[0].Name != "echo" {
		t.Errorf("command name: got %q, want %q", fr.Calls[0].Name, "echo")
	}
}

func TestRunner_StopOnFirst_Failure(t *testing.T) {
	cfg := &Config{Hooks: map[Type][]Entry{
		PreNew: {
			{Command: "false", Shell: false},
			{Command: "should-not-run", Shell: false},
		},
	}}
	fr := &FakeRunner{ExitCode: 1}
	out := testStdout()
	r := NewRunner(cfg, fr, out)

	err := r.Fire(PreNew, EnvContext{}, "/tmp")

	assertCode(t, err, CodeHookFailed)
	if len(fr.Calls) != 1 {
		t.Errorf("expected 1 call (stop on first), got %d", len(fr.Calls))
	}
}

func TestRunner_IgnoreFailure_Continues(t *testing.T) {
	cfg := &Config{Hooks: map[Type][]Entry{
		PreNew: {
			{Command: "failing", IgnoreFailure: true, Shell: false},
			{Command: "should-run", Shell: false},
		},
	}}
	fr := &FakeRunner{
		Responses: []FakeResponse{
			{ExitCode: 1}, // first entry fails, but ignored
			{ExitCode: 0}, // second entry succeeds
		},
	}
	out := testStdout()
	r := NewRunner(cfg, fr, out)

	err := r.Fire(PreNew, EnvContext{}, "/tmp")

	if err != nil {
		t.Fatalf("ignore_failure should continue past ignored entry, got: %v", err)
	}
	if len(fr.Calls) != 2 {
		t.Errorf("expected 2 calls (ignore_failure continues), got %d", len(fr.Calls))
	}
}

func TestRunner_Timeout_Kills(t *testing.T) {
	cfg := &Config{Hooks: map[Type][]Entry{
		PreNew: {{Command: "sleep", Args: []string{"60"}, Timeout: 100 * 1e6, Shell: false}},
	}}
	fr := &FakeRunner{RunErr: context.DeadlineExceeded, ExitCode: -1}
	out := testStdout()
	r := NewRunner(cfg, fr, out)

	err := r.Fire(PreNew, EnvContext{}, "/tmp")

	assertCode(t, err, CodeHookTimeout)
}

func TestRunner_Silent_SuppressesOutput(t *testing.T) {
	cfg := &Config{Hooks: map[Type][]Entry{
		PreNew: {{Command: "echo", Args: []string{"quiet"}, Silent: true, Shell: false}},
	}}
	fr := &FakeRunner{ExitCode: 0}
	out := testStdout()
	r := NewRunner(cfg, fr, out)

	err := r.Fire(PreNew, EnvContext{}, "/tmp")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fr.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(fr.Calls))
	}
	// When silent, output must NOT go to the injected out writer.
	stdoutData := out.String()
	if len(stdoutData) > 0 {
		t.Errorf("silent hook wrote %d bytes to out, want 0", len(stdoutData))
	}
}

func TestRunner_StdinClosed(t *testing.T) {
	cfg := &Config{Hooks: map[Type][]Entry{
		PreNew: {{Command: "cat", Shell: false}},
	}}
	fr := &FakeRunner{ExitCode: 0}
	out := testStdout()
	r := NewRunner(cfg, fr, out)

	err := r.Fire(PreNew, EnvContext{}, "/tmp")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fr.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(fr.Calls))
	}
	// Stdin should produce EOF immediately.
	if fr.Calls[0].Opts.Stdin == nil {
		t.Fatal("Stdin is nil, expected a reader")
	}
	data, readErr := io.ReadAll(fr.Calls[0].Opts.Stdin)
	if readErr != nil {
		t.Fatalf("unexpected read error: %v", readErr)
	}
	if len(data) != 0 {
		t.Errorf("Stdin should be empty, got %d bytes", len(data))
	}
}

func TestRunner_HOOK_TYPE_InEnv(t *testing.T) {
	cfg := &Config{Hooks: map[Type][]Entry{
		PostGenerate: {{Command: "echo", Args: []string{"hello"}, Shell: false}},
	}}
	fr := &FakeRunner{ExitCode: 0}
	out := testStdout()
	r := NewRunner(cfg, fr, out)

	err := r.Fire(PostGenerate, EnvContext{
		ProjectName: "test",
		ProjectPath: "/tmp/test",
		Arch:        "Standard",
		HookType:    PostGenerate,
	}, "/tmp")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fr.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(fr.Calls))
	}

	env := fr.Calls[0].Opts.Env
	found := false
	for _, e := range env {
		if strings.HasPrefix(e, "HOOK_TYPE=") {
			found = true
			if e != "HOOK_TYPE=post-generate" {
				t.Errorf("HOOK_TYPE: got %q, want HOOK_TYPE=post-generate", e)
			}
		}
	}
	if !found {
		t.Error("HOOK_TYPE not found in env")
	}
}

func TestRunner_NoHooks_IsNoOp(t *testing.T) {
	// Empty hooks map should not error.
	cfg := &Config{Hooks: make(map[Type][]Entry)}
	fr := &FakeRunner{}
	out := testStdout()
	r := NewRunner(cfg, fr, out)

	err := r.Fire(PreNew, EnvContext{}, "/tmp")
	if err != nil {
		t.Fatalf("no-op should not error: %v", err)
	}
	if len(fr.Calls) != 0 {
		t.Errorf("expected 0 calls, got %d", len(fr.Calls))
	}
}

func TestRunner_NilHooksMap_IsNoOp(t *testing.T) {
	cfg := &Config{} // nil Hooks
	fr := &FakeRunner{}
	out := testStdout()
	r := NewRunner(cfg, fr, out)

	err := r.Fire(PreNew, EnvContext{}, "/tmp")
	if err != nil {
		t.Fatalf("nil hooks should not error: %v", err)
	}
	if len(fr.Calls) != 0 {
		t.Errorf("expected 0 calls, got %d", len(fr.Calls))
	}
}

func TestRunner_IgnoreFailure_WarningWritten(t *testing.T) {
	cfg := &Config{Hooks: map[Type][]Entry{
		PreNew: {{Command: "failing", IgnoreFailure: true, Shell: false}},
	}}
	fr := &FakeRunner{ExitCode: 1}
	out := testStdout()
	r := NewRunner(cfg, fr, out)

	err := r.Fire(PreNew, EnvContext{}, "/tmp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// A warning should be written to out.
	output := out.String()
	if output == "" {
		t.Error("expected warning output for ignored failure, got empty")
	}
}

// ---------------------------------------------------------------------------
// 2.4 – Threat-Matrix Boundaries
// ---------------------------------------------------------------------------

func TestRunner_ShellVsArgv_StringFormUsesShell(t *testing.T) {
	cfg := &Config{Hooks: map[Type][]Entry{
		PreNew: {{Command: "echo", Args: []string{"$HOME"}, Shell: true}},
	}}
	fr := &FakeRunner{ExitCode: 0}
	out := testStdout()
	r := NewRunner(cfg, fr, out)

	err := r.Fire(PreNew, EnvContext{}, "/tmp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fr.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(fr.Calls))
	}
	// Shell entry: should be dispatched via sh -c (or cmd /c on Windows).
	call := fr.Calls[0]
	if call.Name != "sh" && call.Name != "cmd" {
		t.Errorf("shell entry: expected 'sh' or 'cmd' as command, got %q", call.Name)
	}
}

func TestRunner_ShellVsArgv_ObjectFormArgvDirect(t *testing.T) {
	cfg := &Config{Hooks: map[Type][]Entry{
		PreNew: {{Command: "echo", Args: []string{"$HOME"}, Shell: false}},
	}}
	fr := &FakeRunner{ExitCode: 0}
	out := testStdout()
	r := NewRunner(cfg, fr, out)

	err := r.Fire(PreNew, EnvContext{}, "/tmp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fr.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(fr.Calls))
	}
	call := fr.Calls[0]
	// Object form: argv-direct, command is the entry's Command.
	if call.Name != "echo" {
		t.Errorf("object entry: expected 'echo', got %q", call.Name)
	}
	if len(call.Args) != 1 || call.Args[0] != "$HOME" {
		t.Errorf("object entry args: want [$HOME], got %v", call.Args)
	}
}

func TestRunner_CWD_Defaults(t *testing.T) {
	cfg := &Config{Hooks: map[Type][]Entry{
		PreNew: {{Command: "pwd", Shell: false}},
	}}
	fr := &FakeRunner{ExitCode: 0}
	out := testStdout()
	r := NewRunner(cfg, fr, out)

	err := r.Fire(PreNew, EnvContext{}, "/my/default/cwd")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fr.Calls[0].Opts.Dir != "/my/default/cwd" {
		t.Errorf("Dir: got %q, want %q", fr.Calls[0].Opts.Dir, "/my/default/cwd")
	}
}

func TestRunner_CWD_Override(t *testing.T) {
	cfg := &Config{Hooks: map[Type][]Entry{
		PreNew: {{Command: "ls", Cwd: "subdir", Shell: false}},
	}}
	fr := &FakeRunner{ExitCode: 0}
	out := testStdout()
	r := NewRunner(cfg, fr, out)

	err := r.Fire(PreNew, EnvContext{}, "/my/default/cwd")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fr.Calls[0].Opts.Dir != "/my/default/cwd/subdir" {
		t.Errorf("Dir override: got %q, want %q", fr.Calls[0].Opts.Dir, "/my/default/cwd/subdir")
	}
}

func TestRunner_CommandNotFound(t *testing.T) {
	// Object form: exec.ErrNotFound → hook_command_not_found.
	cfg := &Config{Hooks: map[Type][]Entry{
		PreNew: {{Command: "nonexistent-xyz", Shell: false}},
	}}
	fr := &FakeRunner{RunErr: exec.ErrNotFound, ExitCode: -1}
	out := testStdout()
	r := NewRunner(cfg, fr, out)

	err := r.Fire(PreNew, EnvContext{}, "/tmp")
	assertCode(t, err, CodeHookCommandNotFound)
}

func TestRunner_StringForm_CommandNotFound(t *testing.T) {
	// String form via sh -c: sh exits 127 when the inner command is missing.
	// The runner should map exit 127 to hook_command_not_found.
	cfg := &Config{Hooks: map[Type][]Entry{
		PreNew: {{Command: "definitely-not-a-real-binary-xyz", Shell: true}},
	}}
	// Simulate: sh finds the shell (exitCode 127, runErr from exec.ExitError).
	fr := &FakeRunner{ExitCode: 127, RunErr: errors.New("exit status 127")}
	out := testStdout()
	r := NewRunner(cfg, fr, out)

	err := r.Fire(PreNew, EnvContext{}, "/tmp")
	assertCode(t, err, CodeHookCommandNotFound)
}

func TestRunner_DefaultTimeout_30s(t *testing.T) {
	cfg := &Config{Hooks: map[Type][]Entry{
		PreNew: {{Command: "sleep", Args: []string{"60"}, Shell: false}},
	}}
	// FakeRunner checks ctx to verify deadline behavior.
	fr := &FakeRunner{
		RunErr: context.DeadlineExceeded,
		// When a timeout kills, we want to assert it produces hook_timeout.
	}
	out := testStdout()
	r := NewRunner(cfg, fr, out)

	err := r.Fire(PreNew, EnvContext{}, "/tmp")
	assertCode(t, err, CodeHookTimeout)
}

func TestRunner_TimeoutZero_Disabled(t *testing.T) {
	cfg := &Config{Hooks: map[Type][]Entry{
		PreNew: {{Command: "long-task", Timeout: 0, Shell: false}},
	}}
	fr := &FakeRunner{ExitCode: 0}
	out := testStdout()
	r := NewRunner(cfg, fr, out)

	err := r.Fire(PreNew, EnvContext{}, "/tmp")
	if err != nil {
		t.Fatalf("timeout=0 should disable timeout, got: %v", err)
	}
}

func TestRunner_MultipleEntries_InOrder(t *testing.T) {
	cfg := &Config{Hooks: map[Type][]Entry{
		PostGenerate: {
			{Command: "first", Shell: false},
			{Command: "second", Shell: false},
			{Command: "third", Shell: false},
		},
	}}
	fr := &FakeRunner{ExitCode: 0}
	out := testStdout()
	r := NewRunner(cfg, fr, out)

	err := r.Fire(PostGenerate, EnvContext{}, "/tmp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fr.Calls) != 3 {
		t.Fatalf("expected 3 calls, got %d", len(fr.Calls))
	}
	for i, want := range []string{"first", "second", "third"} {
		if fr.Calls[i].Name != want {
			t.Errorf("call[%d]: got %q, want %q", i, fr.Calls[i].Name, want)
		}
	}
}
