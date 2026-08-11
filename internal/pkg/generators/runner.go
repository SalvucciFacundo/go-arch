package generators

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/SalvucciFacundo/go-arch/v2/internal/pkg/hooks"
	"github.com/samber/oops"
)

// Runner executes run: steps and generator hooks via the hooks.CommandRunner
// interface. It satisfies the entriesFirer seam defined in executor.go.
type Runner struct {
	cmd hooks.CommandRunner
	out io.Writer
}

// NewRunner returns a Runner that delegates subprocess execution to cmd
// and writes hook/step output to out.
func NewRunner(cmd hooks.CommandRunner, out io.Writer) *Runner {
	return &Runner{cmd: cmd, out: out}
}

// FireEntries executes a list of hook entries sequentially. Each entry is
// resolved through the exported hooks helpers (ResolveCommand, BuildEnv,
// ResolveTimeout, ResolveDir). Execution stops at the first failure unless
// the entry has ignore_failure set. When ignore_failure is true, a warning
// is written to r.out and execution continues.
//
// FireEntries satisfies the entriesFirer interface defined in executor.go.
func (r *Runner) FireEntries(entries []hooks.Entry, ctx hooks.EnvContext, cwd string) error {
	if len(entries) == 0 {
		return nil
	}

	parentEnv := os.Environ()

	for _, entry := range entries {
		name, argv := hooks.ResolveCommand(entry)
		env := hooks.BuildEnv(parentEnv, ctx, entry.Env)
		dir := hooks.ResolveDir(cwd, entry.Cwd)

		timeout, disableCtx := hooks.ResolveTimeout(entry)
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

		exitCode, runErr := r.cmd.Run(runCtx, name, argv, hooks.RunOpts{
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
				// CommandRunner returned non-zero with nil error.
				err = fmt.Errorf("exit status %d", exitCode)
			}

			if entry.IgnoreFailure {
				fmt.Fprintf(r.out, "⚠ hook '%s' failed (ignored): %v\n", entry.Command, err)
				continue
			}

			if errors.Is(err, context.DeadlineExceeded) {
				return oops.
					Code(CodeGeneratorStepFailed).
					Hint("Increase the timeout field or optimize the command").
					Wrap(err)
			}
			if errors.Is(err, exec.ErrNotFound) {
				return oops.
					Code(CodeGeneratorStepFailed).
					Hint("Install the missing tool or check your PATH").
					Wrap(err)
			}
			// Non-zero exit with non-ErrNotFound: use hooks.CodeHookFailed
			// style but keep generator error codes.
			if exitCode == 127 || exitCode == 9009 {
				return oops.
					Code(CodeGeneratorStepFailed).
					Hint("Install the missing tool or check your PATH").
					Wrap(err)
			}
			return oops.
				Code(CodeGeneratorStepFailed).
				Hint("Fix the command or set ignore_failure: true").
				Wrap(err)
		}
	}

	return nil
}

// Ensure Runner satisfies entriesFirer at compile time.
var _ entriesFirer = (*Runner)(nil)
