package generators

import (
	"fmt"
	"strings"

	"github.com/samber/oops"
)

// Validate checks a generator recipe for structural correctness.
// Returns invalid_pack_manifest naming the generator and the failing step
// index on any validation failure.
func Validate(genName string, g Generator) error {
	if len(g.Steps) == 0 {
		return oops.
			Code(CodeInvalidPackManifest).
			Errorf("generator %q has no steps", genName)
	}

	promptNames := make(map[string]int) // name → step index

	for i, s := range g.Steps {
		s.Index = i // materialize index for error reporting

		switch s.Type {
		case "template", "binary":
			if s.From == "" {
				return oops.
					Code(CodeInvalidPackManifest).
					Errorf("generator %q step %d: missing required field \"from\"", genName, i)
			}
			if s.To == "" {
				return oops.
					Code(CodeInvalidPackManifest).
					Errorf("generator %q step %d: missing required field \"to\"", genName, i)
			}

		case "run":
			// run steps are structurally valid with command (validated by UnmarshalYAML)

		case "prompt":
			if s.Name == "" {
				return oops.
					Code(CodeInvalidPackManifest).
					Errorf("generator %q step %d: prompt step missing required field \"name\"", genName, i)
			}
			if s.Message == "" {
				return oops.
					Code(CodeInvalidPackManifest).
					Errorf("generator %q step %d: prompt step missing required field \"message\"", genName, i)
			}
			// Check for duplicate prompt names.
			if prev, ok := promptNames[s.Name]; ok {
				return oops.
					Code(CodeInvalidPackManifest).
					Errorf("generator %q step %d: duplicate prompt name %q (also at step %d)", genName, i, s.Name, prev)
			}
			promptNames[s.Name] = i

		case "use":
			if !strings.HasPrefix(s.Value, "builtin/") || len(s.Value) <= len("builtin/") {
				return oops.
					Code(CodeInvalidPackManifest).
					Errorf("generator %q step %d: use value %q must match builtin/<name>", genName, i, s.Value)
			}

		default:
			return oops.
				Code(CodeInvalidPackManifest).
				Errorf("generator %q step %d: unknown step type %q", genName, i, s.Type)
		}
	}

	// Validate pre/post hooks structurally (they use hooks.Entry which is
	// already validated at decode time by hooks.Entry.UnmarshalYAML).
	for i, pre := range g.Pre {
		if pre.Command == "" {
			return oops.
				Code(CodeInvalidPackManifest).
				Errorf("generator %q pre hook %d: missing command", genName, i)
		}
	}
	for i, post := range g.Post {
		if post.Command == "" {
			return oops.
				Code(CodeInvalidPackManifest).
				Errorf("generator %q post hook %d: missing command", genName, i)
		}
	}

	return nil
}

// SupportedStepTypes returns a slice of the valid step type strings.
func SupportedStepTypes() []string {
	return []string{"template", "binary", "run", "prompt", "use"}
}

// FormatStepTypes returns a comma-separated list for error messages.
func FormatStepTypes() string {
	return fmt.Sprintf("%q, %q, %q, %q, %q",
		"template", "binary", "run", "prompt", "use")
}

// FormatStepTypesOr returns an "or"-separated list for error messages.
func FormatStepTypesOr() string {
	return `"template", "binary", "run", "prompt", or "use"`
}
