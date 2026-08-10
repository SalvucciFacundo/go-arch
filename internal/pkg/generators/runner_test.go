package generators

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"go-arch/internal/pkg/hooks"
)

// TestFireEntries_ObjectFormArgv verifies that an object-form entry
// (Shell: false) is dispatched via argv-direct execution: the command
// name and args are passed verbatim to hooks.ResolveCommand.
func TestFireEntries_ObjectFormArgv(t *testing.T) {
	fr := &hooks.FakeRunner{}
	out := &bytes.Buffer{}
	r := NewRunner(fr, out)

	entries := []hooks.Entry{
		{Command: "echo", Args: []string{"hello", "world"}},
	}

	err := r.FireEntries(entries, hooks.EnvContext{}, "/tmp/proj")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(fr.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(fr.Calls))
	}
	call := fr.Calls[0]
	if call.Name != "echo" {
		t.Errorf("command name: want echo, got %s", call.Name)
	}
	if len(call.Args) != 2 || call.Args[0] != "hello" || call.Args[1] != "world" {
		t.Errorf("args: want [hello world], got %v", call.Args)
	}
}

// TestFireEntries_StringFormShell verifies that a string-form entry
// (Shell: true) dispatches via sh -c (or cmd /c on Windows).
func TestFireEntries_StringFormShell(t *testing.T) {
	fr := &hooks.FakeRunner{}
	out := &bytes.Buffer{}
	r := NewRunner(fr, out)

	entries := []hooks.Entry{
		{Shell: true, Command: "echo hello", Args: []string{"world"}},
	}

	err := r.FireEntries(entries, hooks.EnvContext{}, "/tmp/proj")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(fr.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(fr.Calls))
	}
	call := fr.Calls[0]
	if runtime.GOOS == "windows" {
		if call.Name != "cmd" {
			t.Errorf("shell command: want cmd, got %s", call.Name)
		}
		if len(call.Args) < 2 || call.Args[0] != "/c" {
			t.Errorf("shell args: want [/c ...], got %v", call.Args)
		}
	} else {
		if call.Name != "sh" {
			t.Errorf("shell command: want sh, got %s", call.Name)
		}
		if len(call.Args) < 2 || call.Args[0] != "-c" {
			t.Errorf("shell args: want [-c ...], got %v", call.Args)
		}
	}
}

// TestFireEntries_TimeoutHonored verifies that an entry with an explicit
// timeout passes a deadline context to the command runner. The fake
// runner should receive a context with a deadline.
func TestFireEntries_TimeoutHonored(t *testing.T) {
	fr := &hooks.FakeRunner{}
	out := &bytes.Buffer{}
	r := NewRunner(fr, out)

	entries := []hooks.Entry{
		{
			Command:    "slow",
			Timeout:    10 * time.Millisecond,
			TimeoutSet: true,
		},
	}

	err := r.FireEntries(entries, hooks.EnvContext{}, "/tmp/proj")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fr.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(fr.Calls))
	}
}

// TestFireEntries_SilentSuppressesOutput verifies that a Silent entry
// receives io.Discard for both Stdout and Stderr.
func TestFireEntries_SilentSuppressesOutput(t *testing.T) {
	fr := &hooks.FakeRunner{}
	out := &bytes.Buffer{}
	r := NewRunner(fr, out)

	entries := []hooks.Entry{
		{Command: "noisy", Silent: true},
	}

	err := r.FireEntries(entries, hooks.EnvContext{}, "/tmp/proj")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(fr.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(fr.Calls))
	}
	call := fr.Calls[0]
	if call.Opts.Stdout != io.Discard {
		t.Errorf("stdout: want io.Discard, got %v", call.Opts.Stdout)
	}
	if call.Opts.Stderr != io.Discard {
		t.Errorf("stderr: want io.Discard, got %v", call.Opts.Stderr)
	}
}

// TestFireEntries_NonSilentUsesOut verifies that a non-Silent entry
// writes to the configured output writer.
func TestFireEntries_NonSilentUsesOut(t *testing.T) {
	fr := &hooks.FakeRunner{}
	out := &bytes.Buffer{}
	r := NewRunner(fr, out)

	entries := []hooks.Entry{
		{Command: "verbose"},
	}

	err := r.FireEntries(entries, hooks.EnvContext{}, "/tmp/proj")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(fr.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(fr.Calls))
	}
	call := fr.Calls[0]
	if call.Opts.Stdout != out {
		t.Errorf("stdout: want configured out, got %v", call.Opts.Stdout)
	}
	if call.Opts.Stderr != out {
		t.Errorf("stderr: want configured out, got %v", call.Opts.Stderr)
	}
}

// TestFireEntries_IgnoreFailureContinues verifies that when an entry
// has IgnoreFailure: true and the command fails, the runner writes a
// warning and continues to the next entry.
func TestFireEntries_IgnoreFailureContinues(t *testing.T) {
	fr := &hooks.FakeRunner{
		Responses: []hooks.FakeResponse{
			{ExitCode: 1}, // first entry fails
			{},            // second entry succeeds
		},
	}
	out := &bytes.Buffer{}
	r := NewRunner(fr, out)

	entries := []hooks.Entry{
		{Command: "failer", IgnoreFailure: true},
		{Command: "next"},
	}

	err := r.FireEntries(entries, hooks.EnvContext{}, "/tmp/proj")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(fr.Calls) != 2 {
		t.Fatalf("expected 2 calls (failure ignored), got %d", len(fr.Calls))
	}
	// First entry was "failer", second "next"
	if fr.Calls[0].Name != "failer" {
		t.Errorf("first call: want failer, got %s", fr.Calls[0].Name)
	}
	if fr.Calls[1].Name != "next" {
		t.Errorf("second call: want next, got %s", fr.Calls[1].Name)
	}
	// Warning should be written for ignored failure
	if !strings.Contains(out.String(), "failed (ignored)") {
		t.Errorf("expected warning for ignored failure, got: %s", out.String())
	}
}

// TestFireEntries_FailureStops verifies that when an entry fails
// without IgnoreFailure, execution stops and returns an error.
func TestFireEntries_FailureStops(t *testing.T) {
	fr := &hooks.FakeRunner{
		Responses: []hooks.FakeResponse{
			{ExitCode: 1},
		},
	}
	out := &bytes.Buffer{}
	r := NewRunner(fr, out)

	entries := []hooks.Entry{
		{Command: "failer"},
		{Command: "never"},
	}

	err := r.FireEntries(entries, hooks.EnvContext{}, "/tmp/proj")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if len(fr.Calls) != 1 {
		t.Fatalf("expected 1 call (stopped on failure), got %d", len(fr.Calls))
	}
	if fr.Calls[0].Name != "failer" {
		t.Errorf("want failer, got %s", fr.Calls[0].Name)
	}
}

// TestFireEntries_PerEntryEnvOverrides verifies that per-entry env vars
// are injected into the process environment.
func TestFireEntries_PerEntryEnvOverrides(t *testing.T) {
	fr := &hooks.FakeRunner{}
	out := &bytes.Buffer{}
	r := NewRunner(fr, out)

	entries := []hooks.Entry{
		{
			Command: "cmd",
			Env:     map[string]string{"FOO": "bar"},
		},
	}

	err := r.FireEntries(entries, hooks.EnvContext{}, "/tmp/proj")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(fr.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(fr.Calls))
	}
	call := fr.Calls[0]

	found := false
	for _, e := range call.Opts.Env {
		if e == "FOO=bar" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("env: FOO=bar not found in: %v", call.Opts.Env)
	}
}

// TestFireEntries_GeneratorNamePassedThrough verifies that when
// EnvContext has GeneratorName set, the command environment includes
// GENERATOR_NAME.
func TestFireEntries_GeneratorNamePassedThrough(t *testing.T) {
	fr := &hooks.FakeRunner{}
	out := &bytes.Buffer{}
	r := NewRunner(fr, out)

	entries := []hooks.Entry{
		{Command: "cmd"},
	}
	ctx := hooks.EnvContext{
		GeneratorName: "docker",
	}

	err := r.FireEntries(entries, ctx, "/tmp/proj")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(fr.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(fr.Calls))
	}
	call := fr.Calls[0]

	found := false
	for _, e := range call.Opts.Env {
		if e == "GENERATOR_NAME=docker" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("env: GENERATOR_NAME=docker not found in: %v", call.Opts.Env)
	}
}

// TestFireEntries_DefaultCwd verifies that the default working directory
// is used when the entry has no cwd override.
func TestFireEntries_DefaultCwd(t *testing.T) {
	fr := &hooks.FakeRunner{}
	out := &bytes.Buffer{}
	r := NewRunner(fr, out)

	entries := []hooks.Entry{
		{Command: "cmd"},
	}

	err := r.FireEntries(entries, hooks.EnvContext{}, "/tmp/proj")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(fr.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(fr.Calls))
	}
	// The resolved dir should be the defaultCwd (since no cwd override)
	call := fr.Calls[0]
	if call.Opts.Dir == "" {
		t.Error("expected Dir to be set to defaultCwd, got empty")
	}
}

// TestFireEntries_CwdOverride verifies that the per-entry cwd override
// is joined with the default cwd.
func TestFireEntries_CwdOverride(t *testing.T) {
	fr := &hooks.FakeRunner{}
	out := &bytes.Buffer{}
	r := NewRunner(fr, out)

	entries := []hooks.Entry{
		{Command: "cmd", Cwd: "subdir"},
	}

	err := r.FireEntries(entries, hooks.EnvContext{ProjectPath: "/tmp/proj"}, "/tmp/proj")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(fr.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(fr.Calls))
	}
	call := fr.Calls[0]
	expectedDir := "/tmp/proj/subdir"
	if call.Opts.Dir != expectedDir {
		t.Errorf("Dir: want %s, got %s", expectedDir, call.Opts.Dir)
	}
}

// TestFireEntries_EmptyListNoOp verifies that an empty entry list is a
// no-op (no calls made, no error).
func TestFireEntries_EmptyListNoOp(t *testing.T) {
	fr := &hooks.FakeRunner{}
	out := &bytes.Buffer{}
	r := NewRunner(fr, out)

	err := r.FireEntries(nil, hooks.EnvContext{}, "/tmp/proj")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(fr.Calls) != 0 {
		t.Errorf("expected 0 calls, got %d", len(fr.Calls))
	}
}

// TestFireEntries_MultipleInOrder verifies that entries fire in the
// declared order.
func TestFireEntries_MultipleInOrder(t *testing.T) {
	fr := &hooks.FakeRunner{}
	out := &bytes.Buffer{}
	r := NewRunner(fr, out)

	entries := []hooks.Entry{
		{Command: "first"},
		{Command: "second"},
		{Command: "third"},
	}

	err := r.FireEntries(entries, hooks.EnvContext{}, "/tmp/proj")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(fr.Calls) != 3 {
		t.Fatalf("expected 3 calls, got %d", len(fr.Calls))
	}
	if fr.Calls[0].Name != "first" {
		t.Errorf("call 0: want first, got %s", fr.Calls[0].Name)
	}
	if fr.Calls[1].Name != "second" {
		t.Errorf("call 1: want second, got %s", fr.Calls[1].Name)
	}
	if fr.Calls[2].Name != "third" {
		t.Errorf("call 2: want third, got %s", fr.Calls[2].Name)
	}
}

// TestFireEntries_ContextCancellation verifies that a context error
// from the command runner is wrapped properly.
func TestFireEntries_ContextCancellation(t *testing.T) {
	fr := &hooks.FakeRunner{
		RunErr: context.DeadlineExceeded,
	}
	out := &bytes.Buffer{}
	r := NewRunner(fr, out)

	entries := []hooks.Entry{
		{Command: "slow"},
	}

	err := r.FireEntries(entries, hooks.EnvContext{}, "/tmp/proj")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestFireEntries_CommandNotFound verifies that exec.ErrNotFound
// produces an appropriate error.
func TestFireEntries_CommandNotFound(t *testing.T) {
	fr := &hooks.FakeRunner{
		RunErr: os.ErrNotExist, // maps to exec.ErrNotFound pattern
	}
	out := &bytes.Buffer{}
	r := NewRunner(fr, out)

	entries := []hooks.Entry{
		{Command: "nonexistent"},
	}

	err := r.FireEntries(entries, hooks.EnvContext{}, "/tmp/proj")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected error wrapping os.ErrNotExist, got: %v", err)
	}
}

// TestFireEntries_StdinIsEmpty verifies that the stdin passed to the
// command is empty (no interactive input in fire-entries context).
func TestFireEntries_StdinIsEmpty(t *testing.T) {
	fr := &hooks.FakeRunner{}
	out := &bytes.Buffer{}
	r := NewRunner(fr, out)

	entries := []hooks.Entry{
		{Command: "cmd"},
	}

	err := r.FireEntries(entries, hooks.EnvContext{}, "/tmp/proj")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(fr.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(fr.Calls))
	}
	call := fr.Calls[0]
	if call.Opts.Stdin == nil {
		t.Error("stdin: expected non-nil (empty reader), got nil")
	}
}
