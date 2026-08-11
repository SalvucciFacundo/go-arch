package generators

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/SalvucciFacundo/go-arch/v2/internal/pkg/hooks"
	"github.com/SalvucciFacundo/go-arch/v2/internal/pkg/template"
	"github.com/samber/oops"
)

// entriesFirer is the seam interface for executing run: steps and
// pre:/post: generator hooks. In Slice 2 tests use a fake; Slice 3
// ships the concrete *Runner that satisfies this interface.
type entriesFirer interface {
	FireEntries(entries []hooks.Entry, ctx hooks.EnvContext, cwd string) error
}

// PromptResolver resolves a prompt step's value. The executor calls
// this during the pre-flight phase for each prompt step. If a
// required prompt cannot be resolved, the executor aborts with
// generator_prompt_unresolvable or missing_generator_argument.
type PromptResolver interface {
	Resolve(name, message, def string, required bool) (string, error)
}

// RunOptions configures a generator execution.
type RunOptions struct {
	ProjectRoot    string              // target project root directory
	PackDir        string              // installed pack directory
	PackName       string              // pack display name
	PackVersion    string              // pack version
	GeneratorName  string              // name of the generator being executed
	HooksEnabled   bool                // sidecar trust gate
	CmdRunner      hooks.CommandRunner // reserved for Slice 3 real runner
	PromptResolver PromptResolver      // resolves prompt steps (pre-flight)
	ResolvedArgs   map[string]any      // accumulated resolved prompt values
	TemplateData   interface{}         // data passed to template rendering (ProjectConfig in scaffolding)
	Out            io.Writer           // output writer for warnings/errors
	Firer          entriesFirer        // seam — real Runner in Slice 3
}

// Run executes a generator recipe against opts.ProjectRoot.
//
// Pre-flight phase (no writes yet):
//  1. Resolve ALL prompt steps via PromptResolver → accumulated into opts.ResolvedArgs.
//  2. Validate ALL template/binary target paths via ValidateTarget.
//     If any path escapes, abort with zero writes.
//
// Execution phase:
//  3. Fire pre: hooks via Firer (gated by HooksEnabled).
//  4. Execute steps linearly. fail-fast unless ignore_failure.
//  5. Fire post: hooks on success (skipped on any step failure).
//
// Returns []Record for manifest recording and any execution error.
func Run(ctx context.Context, g Generator, opts RunOptions) ([]Record, error) {
	_ = ctx // reserved for future cancellation support

	// --- Pre-flight: prompt resolution ---
	for i, s := range g.Steps {
		s.Index = i
		if s.Type != "prompt" {
			continue
		}
		// Resolve from PromptResolver.
		if opts.PromptResolver != nil {
			val, err := opts.PromptResolver.Resolve(s.Name, s.Message, s.Default, s.Required)
			if err != nil {
				return nil, oops.
					Code(CodeGeneratorPromptUnresolvable).
					Wrapf(err, "prompt %q (step %d) could not be resolved", s.Name, i)
			}
			if opts.ResolvedArgs == nil {
				opts.ResolvedArgs = make(map[string]any)
			}
			opts.ResolvedArgs[s.Name] = val
		} else {
			// No resolver: use default or fail if required.
			if s.Required {
				return nil, oops.
					Code(CodeMissingGeneratorArgument).
					Errorf("prompt %q is required but no PromptResolver provided", s.Name)
			}
			if opts.ResolvedArgs == nil {
				opts.ResolvedArgs = make(map[string]any)
			}
			if s.Default != "" {
				opts.ResolvedArgs[s.Name] = s.Default
			} else {
				opts.ResolvedArgs[s.Name] = ""
			}
		}
	}

	// --- Pre-flight: sandbox validation (ALL file-writing targets) ---
	for i, s := range g.Steps {
		s.Index = i
		if s.Type == "template" || s.Type == "binary" {
			if err := ValidateTarget(opts.ProjectRoot, s.To); err != nil {
				return nil, oops.
					Code(CodeRecipePathEscape).
					Wrapf(err, "step %d (%s): target %q escapes project root", i, s.Type, s.To)
			}
		}
	}

	// --- Execution ---
	var records []Record
	hookCtx := hooks.EnvContext{
		PackName:      opts.PackName,
		PackVersion:   opts.PackVersion,
		GeneratorName: opts.GeneratorName,
	}

	// Pre hooks.
	if opts.HooksEnabled && len(g.Pre) > 0 && opts.Firer != nil {
		if err := opts.Firer.FireEntries(g.Pre, hookCtx, opts.ProjectRoot); err != nil {
			return records, oops.
				Code(CodeGeneratorStepFailed).
				Wrapf(err, "generator %q pre-hook failed", opts.GeneratorName)
		}
	} else if !opts.HooksEnabled && (len(g.Pre) > 0 || len(g.Post) > 0) {
		// Warn when generator hooks are skipped due to trust gate.
		fmt.Fprintf(opts.Out, "⚠ generator_run_skipped_trust: generator %q hooks skipped; hooks not enabled for pack %q\n",
			opts.GeneratorName, opts.PackName)
	}

	// Linear step execution.
	stepErr := runSteps(&opts, &records, g)

	// Post hooks (only on total success).
	if stepErr == nil && opts.HooksEnabled && len(g.Post) > 0 && opts.Firer != nil {
		if err := opts.Firer.FireEntries(g.Post, hookCtx, opts.ProjectRoot); err != nil {
			return records, oops.
				Code(CodeGeneratorStepFailed).
				Wrapf(err, "generator %q post-hook failed", opts.GeneratorName)
		}
	}

	return records, stepErr
}

// runSteps executes the generator's steps linearly and appends
// records for template and binary steps.
func runSteps(opts *RunOptions, records *[]Record, g Generator) error {
	for i, s := range g.Steps {
		s.Index = i
		switch s.Type {
		case "template":
			rec, err := executeTemplateStep(opts, s)
			if err != nil {
				return oops.
					Code(CodeGeneratorStepFailed).
					Wrapf(err, "step %d (template) failed", i)
			}
			*records = append(*records, *rec)

		case "binary":
			rec, err := executeBinaryStep(opts, s)
			if err != nil {
				return oops.
					Code(CodeGeneratorStepFailed).
					Wrapf(err, "step %d (binary) failed", i)
			}
			*records = append(*records, *rec)

		case "run":
			if !opts.HooksEnabled {
				fmt.Fprintf(opts.Out, "⚠ %s: generator %q step %d (run) skipped; hooks not enabled for pack %q\n",
					CodeGeneratorRunSkippedTrust, opts.GeneratorName, i, opts.PackName)
				continue
			}
			if opts.Firer == nil {
				fmt.Fprintf(opts.Out, "⚠ %s: no firer configured; skipping run step %d\n",
					CodeGeneratorRunSkippedTrust, i)
				continue
			}
			entry := hooks.Entry{
				Command:       s.Command,
				Args:          s.Args,
				Shell:         s.Shell,
				Cwd:           s.Cwd,
				Env:           s.Env,
				Timeout:       s.Timeout,
				TimeoutSet:    s.Timeout > 0 || s.TimeoutSet(),
				Silent:        s.Silent,
				IgnoreFailure: s.IgnoreFailure,
			}
			if err := opts.Firer.FireEntries([]hooks.Entry{entry}, hooks.EnvContext{
				PackName:      opts.PackName,
				PackVersion:   opts.PackVersion,
				GeneratorName: opts.GeneratorName,
			}, opts.ProjectRoot); err != nil {
				if s.IgnoreFailure {
					fmt.Fprintf(opts.Out, "⚠ step %d (run) failed (ignored): %v\n", i, err)
					continue
				}
				return oops.
					Code(CodeGeneratorStepFailed).
					Wrapf(err, "step %d (run) failed", i)
			}

		case "use":
			builtinName := s.Value[len("builtin/"):]
			fn, err := Lookup(builtinName)
			if err != nil {
				return err
			}
			builtinRecords, fnErr := fn(g, opts.ResolvedArgs)
			if fnErr != nil {
				return oops.
					Code(CodeGeneratorStepFailed).
					Wrapf(fnErr, "step %d (use: %s) failed", i, builtinName)
			}
			*records = append(*records, builtinRecords...)

		case "prompt":
			// Already resolved in pre-flight; skip at execution.

		default:
			return oops.
				Code(CodeUnknownStepType).
				Errorf("step %d: unknown step type %q", i, s.Type)
		}
	}
	return nil
}

// executeTemplateStep renders a pack template and writes the result to the
// project target. Returns a Record with Origin "template".
func executeTemplateStep(opts *RunOptions, s Step) (*Record, error) {
	engine := template.NewEngine()

	targetPath := filepath.Join(opts.ProjectRoot, s.To)
	dir := filepath.Dir(targetPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	f, err := os.Create(targetPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	data := opts.TemplateData
	if data == nil {
		data = map[string]interface{}{}
	}

	if err := engine.RenderPackOnly(f, opts.PackDir, s.From, data); err != nil {
		f.Close()
		// Clean up partial file on failure (best-effort).
		os.Remove(targetPath)
		return nil, err
	}

	source := fmt.Sprintf("pack:%s@%s", opts.PackName, opts.PackVersion)

	return &Record{
		Path:         s.To,
		Origin:       "template",
		Source:       source,
		TemplatePath: s.From,
		Metadata: map[string]string{
			"generator": opts.GeneratorName,
		},
	}, nil
}

// executeBinaryStep copies a pack asset to the project target with the
// specified file mode. Returns a Record with Origin "generator".
func executeBinaryStep(opts *RunOptions, s Step) (*Record, error) {
	srcPath := filepath.Join(opts.PackDir, s.From)

	targetPath := filepath.Join(opts.ProjectRoot, s.To)
	dir := filepath.Dir(targetPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	src, err := os.Open(srcPath)
	if err != nil {
		return nil, err
	}
	defer src.Close()

	mode := s.Mode
	if mode == 0 {
		mode = 0644
	}

	dst, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return nil, err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		os.Remove(targetPath)
		return nil, err
	}

	source := fmt.Sprintf("pack:%s@%s", opts.PackName, opts.PackVersion)

	return &Record{
		Path:   s.To,
		Origin: "generator",
		Source: source,
		Metadata: map[string]string{
			"generator": opts.GeneratorName,
		},
	}, nil
}

// TimeoutSet adapter for Step — Step.Timeout is a duration, and we need
// to distinguish "not set" from "explicitly zero". For now, any non-zero
// timeout means it was set.
func (s Step) TimeoutSet() bool {
	return s.Timeout > 0
}
