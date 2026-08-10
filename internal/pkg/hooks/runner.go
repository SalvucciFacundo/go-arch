package hooks

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/samber/oops"
)

// CommandRunner abstracts process execution so that FakeRunner can drive
// unit tests without spawning real subprocesses.
type CommandRunner interface {
	Run(ctx context.Context, name string, args []string, opts RunOpts) (exitCode int, err error)
}

// RunOpts configures a single command execution.
type RunOpts struct {
	Dir    string
	Env    []string
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// RealRunner implements CommandRunner via os/exec.CommandContext.
type RealRunner struct{}

// Run spawns a subprocess and blocks until it completes.
//
// Exit codes: 0 on success, cmd.ProcessState.ExitCode() on non-zero exits,
// -1 when the command could not be started (e.g. exec.ErrNotFound).
func (RealRunner) Run(ctx context.Context, name string, args []string, opts RunOpts) (int, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if opts.Dir != "" {
		cmd.Dir = opts.Dir
	}
	cmd.Env = opts.Env
	if opts.Stdin != nil {
		cmd.Stdin = opts.Stdin
	}
	cmd.Stdout = opts.Stdout
	cmd.Stderr = opts.Stderr

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode(), err
		}
		return -1, err
	}
	return 0, nil
}

// Runner orchestrates hook execution for a given lifecycle type.
type Runner struct {
	cfg *Config
	cmd CommandRunner
	out io.Writer // ui.Out — NEVER os.Stdout in MCP
}

// NewRunner returns a Runner that reads its entries from cfg, delegates
// subprocess execution to cmd, and writes hook output to out.
func NewRunner(cfg *Config, cmd CommandRunner, out io.Writer) *Runner {
	return &Runner{cfg: cfg, cmd: cmd, out: out}
}

// CommandRunner returns the underlying process executor so callers
// (e.g. scaffold) can build a generators.Runner for run: steps.
func (r *Runner) CommandRunner() CommandRunner {
	return r.cmd
}

const defaultHookTimeout = 30 * time.Second

// Fire runs every hook entry registered for t in order.
//
// Execution stops at the first failure unless the entry has ignore_failure
// set. When ignore_failure is true the runner writes a warning to r.out
// and continues. defaultCwd becomes the working directory for hooks that
// do not specify their own cwd: override.
func (r *Runner) Fire(t Type, ctx EnvContext, defaultCwd string) error {
	entries := r.entriesFor(t)
	if len(entries) == 0 {
		return nil
	}

	parentEnv := os.Environ()

	for _, entry := range entries {
		name, argv := ResolveCommand(entry)
		env := BuildEnv(parentEnv, ctx, entry.Env)
		dir := ResolveDir(defaultCwd, entry.Cwd)

		timeout, disableCtx := ResolveTimeout(entry)
		runCtx := context.Background()
		var cancel context.CancelFunc
		if !disableCtx {
			runCtx, cancel = context.WithTimeout(runCtx, timeout)
		}

		var stdout, stderr io.Writer
		if entry.Silent {
			stdout, stderr = io.Discard, io.Discard
		} else {
			stdout, stderr = r.out, r.out
		}

		exitCode, runErr := r.cmd.Run(runCtx, name, argv, RunOpts{
			Dir:    dir,
			Env:    env,
			Stdin:  strings.NewReader(""),
			Stdout: stdout,
			Stderr: stderr,
		})

		if cancel != nil {
			cancel()
		}

		failed := runErr != nil || exitCode != 0
		if failed {
			err := runErr
			if err == nil {
				// CommandRunner returned non-zero with nil error — treat
				// as a synthetic exit error for error classification.
				err = fmt.Errorf("exit status %d", exitCode)
			}

			if entry.IgnoreFailure {
				fmt.Fprintf(r.out, "⚠ hook '%s' failed (ignored): %v\n", entry.Command, err)
				continue
			}

			if errors.Is(err, context.DeadlineExceeded) {
				return oops.
					Code(CodeHookTimeout).
					Hint("Increase the timeout: field or optimize the hook command").
					Wrap(err)
			}
			// Object-form: OS-level command resolution failed.
			if errors.Is(err, exec.ErrNotFound) {
				return oops.
					Code(CodeHookCommandNotFound).
					Hint("Install the missing tool or check your PATH").
					Wrap(err)
			}
			// String-form via sh -c (or cmd /c on Windows): the shell
			// itself was found, but the inner command is missing.
			// POSIX sh convention: exit 127 = command not found.
			// cmd.exe convention:    exit 9009 = command not found.
			if exitCode == 127 || exitCode == 9009 {
				return oops.
					Code(CodeHookCommandNotFound).
					Hint("Install the missing tool or check your PATH").
					Wrap(err)
			}
			return oops.
				Code(CodeHookFailed).
				Hint("Fix the hook command or set ignore_failure: true").
				Wrap(err)
		}
	}

	return nil
}

// entriesFor returns the hook entries registered for t, or nil when there
// are none. Nil config/hooks map is treated as empty.
func (r *Runner) entriesFor(t Type) []Entry {
	if r.cfg == nil || r.cfg.Hooks == nil {
		return nil
	}
	return r.cfg.Hooks[t]
}

// ResolveCommand returns the (name, args) tuple that should be passed to
// the CommandRunner. Shell entries are dispatched via sh -c (or cmd /c on
// Windows); object entries use argv-direct execution.
func ResolveCommand(entry Entry) (string, []string) {
	if entry.Shell {
		cmdLine := entry.Command
		for _, a := range entry.Args {
			cmdLine += " " + a
		}
		if runtime.GOOS == "windows" {
			return "cmd", []string{"/c", cmdLine}
		}
		return "sh", []string{"-c", cmdLine}
	}
	return entry.Command, entry.Args
}

// ResolveDir joins the per-entry cwd override onto defaultCwd.
func ResolveDir(defaultCwd, override string) string {
	if override == "" {
		return defaultCwd
	}
	return filepath.Join(defaultCwd, override)
}

// ResolveTimeout returns the effective timeout and a flag indicating
// whether the timeout should be disabled.
//
//   - TimeoutSet && Timeout >  0 → use the configured timeout
//   - TimeoutSet && Timeout == 0 → disable timeout (context.Background)
//   - !TimeoutSet                → use defaultHookTimeout (30s)
func ResolveTimeout(entry Entry) (time.Duration, bool) {
	if entry.TimeoutSet {
		if entry.Timeout == 0 {
			return 0, true
		}
		return entry.Timeout, false
	}
	return defaultHookTimeout, false
}
