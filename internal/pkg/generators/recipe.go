package generators

import (
	"os"
	"strings"
	"time"

	"github.com/SalvucciFacundo/go-arch/v2/internal/pkg/hooks"
	"github.com/samber/oops"
	"gopkg.in/yaml.v3"
)

// Generator describes a YAML recipe DSL that declares a set of steps
// to produce project files. It belongs to a v2 pack manifest under
// the generators: key.
type Generator struct {
	Description string        `yaml:"description,omitempty"`
	Steps       []Step        `yaml:"steps"`
	Pre         []hooks.Entry `yaml:"pre,omitempty"`
	Post        []hooks.Entry `yaml:"post,omitempty"`
}

// RunsCommands returns true when the generator declares at least one
// run: step or any pre:/post: hooks, meaning the generator may execute
// arbitrary shell commands if HooksEnabled is true.
func (g Generator) RunsCommands() bool {
	if len(g.Pre) > 0 || len(g.Post) > 0 {
		return true
	}
	for _, s := range g.Steps {
		if s.Type == "run" {
			return true
		}
	}
	return false
}

// Step represents a single entry in a generator recipe. The Type field
// drives which subset of fields is meaningful; the union is handled by
// the custom UnmarshalYAML below.
type Step struct {
	Type          string            // "template"|"binary"|"run"|"prompt"|"use"
	From          string            // template/binary source path
	To            string            // template/binary target path
	Mode          os.FileMode       // binary step: default 0644
	Command       string            // run step: command
	Args          []string          // run step: argv
	Shell         bool              // run step: true when parsed from string form
	Cwd           string            // run step: working directory
	Env           map[string]string // run step: env overrides
	Timeout       time.Duration     // run step: per-step timeout
	Silent        bool              // run step: suppress output
	IgnoreFailure bool              // run step: continue on non-zero exit
	Name          string            // prompt step: identifier
	Message       string            // prompt step: prompt text
	Default       string            // prompt step: default value
	Required      bool              // prompt step: is required
	Value         string            // use step: "builtin/<name>"
	Index         int               // source position for errors (0-based)
}

// Record is what generators.Run returns per file written.
// scaffold consumes these to update the manifest.
type Record struct {
	Path         string            // relative to project root
	Origin       string            // "generator" or "template"
	Source       string            // "pack:<name>@<version>"
	TemplatePath string            // for template origin: pack-relative path
	Metadata     map[string]string // includes "generator" and "args" (JSON)
}

// UnmarshalYAML implements hybrid dispatch for Step:
//
//   - Scalar (string) → run step in shell form (Shell=true, Command
//     extracted via strings.Fields).
//   - Mapping (object) → typed step with field validation.
//
// This mirrors the hooks.Entry UnmarshalYAML pattern.
func (s *Step) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		return s.unmarshalString(value)

	case yaml.MappingNode:
		return s.unmarshalObject(value)

	default:
		return oops.
			Code(CodeUnknownStepType).
			Errorf("generator step must be a string or an object, got %s", yamlKind(value.Kind))
	}
}

func (s *Step) unmarshalString(value *yaml.Node) error {
	parts := strings.Fields(value.Value)
	if len(parts) == 0 {
		return oops.
			Code(CodeUnknownStepType).
			Errorf("empty string generator step")
	}
	s.Type = "run"
	s.Command = parts[0]
	s.Args = parts[1:]
	s.Shell = true
	return nil
}

// recognizedStepKeys lists the YAML keys allowed inside an object-form step.
// run-only keys are validated per step type in Validate.
var recognizedStepKeys = map[string]bool{
	"type":           true,
	"from":           true,
	"to":             true,
	"mode":           true,
	"command":        true,
	"args":           true,
	"cwd":            true,
	"env":            true,
	"timeout":        true,
	"silent":         true,
	"ignore_failure": true,
	"name":           true,
	"message":        true,
	"default":        true,
	"required":       true,
	"value":          true,
}

func (s *Step) unmarshalObject(value *yaml.Node) error {
	raw := make(map[string]*yaml.Node)

	for i := 0; i < len(value.Content); i += 2 {
		key := value.Content[i].Value
		val := value.Content[i+1]

		if !recognizedStepKeys[key] {
			return oops.
				Code(CodeUnknownStepType).
				Errorf("unknown key %q in generator step", key)
		}

		raw[key] = val
	}

	// type is required in object form.
	typeNode, ok := raw["type"]
	if !ok || typeNode.Kind != yaml.ScalarNode {
		return oops.
			Code(CodeUnknownStepType).
			Errorf("generator step requires a type field")
	}
	s.Type = typeNode.Value

	// Decode fields that map directly.
	if v, ok := raw["from"]; ok {
		if v.Kind != yaml.ScalarNode {
			return oops.
				Code(CodeUnknownStepType).
				Errorf("generator step: from must be a string")
		}
		s.From = v.Value
	}
	if v, ok := raw["to"]; ok {
		if v.Kind != yaml.ScalarNode {
			return oops.
				Code(CodeUnknownStepType).
				Errorf("generator step: to must be a string")
		}
		s.To = v.Value
	}
	if v, ok := raw["mode"]; ok {
		if err := v.Decode(&s.Mode); err != nil {
			return oops.
				Code(CodeUnknownStepType).
				Wrapf(err, "generator step: mode")
		}
	}
	if s.Mode == 0 {
		s.Mode = 0644
	}

	// Run-step fields.
	if v, ok := raw["command"]; ok {
		if v.Kind != yaml.ScalarNode {
			return oops.
				Code(CodeUnknownStepType).
				Errorf("generator step: command must be a string")
		}
		s.Command = v.Value
	}
	if v, ok := raw["args"]; ok {
		if err := v.Decode(&s.Args); err != nil {
			return oops.
				Code(CodeUnknownStepType).
				Wrapf(err, "generator step: args must be a list of strings")
		}
	}
	if v, ok := raw["cwd"]; ok {
		if v.Kind != yaml.ScalarNode {
			return oops.
				Code(CodeUnknownStepType).
				Errorf("generator step: cwd must be a string")
		}
		s.Cwd = v.Value
	}
	if v, ok := raw["env"]; ok {
		if err := v.Decode(&s.Env); err != nil {
			return oops.
				Code(CodeUnknownStepType).
				Wrapf(err, "generator step: env must be a string map")
		}
	}
	if v, ok := raw["timeout"]; ok {
		d, err := parseDurationNode(v)
		if err != nil {
			return oops.
				Code(CodeUnknownStepType).
				Wrapf(err, "generator step: timeout")
		}
		s.Timeout = d
	}
	if v, ok := raw["silent"]; ok {
		if v.Kind != yaml.ScalarNode {
			return oops.
				Code(CodeUnknownStepType).
				Errorf("generator step: silent must be a boolean")
		}
		s.Silent = v.Value == "true"
	}
	if v, ok := raw["ignore_failure"]; ok {
		if v.Kind != yaml.ScalarNode {
			return oops.
				Code(CodeUnknownStepType).
				Errorf("generator step: ignore_failure must be a boolean")
		}
		s.IgnoreFailure = v.Value == "true"
	}

	// Prompt-step fields.
	if v, ok := raw["name"]; ok {
		if v.Kind != yaml.ScalarNode {
			return oops.
				Code(CodeUnknownStepType).
				Errorf("generator step: name must be a string")
		}
		s.Name = v.Value
	}
	if v, ok := raw["message"]; ok {
		if v.Kind != yaml.ScalarNode {
			return oops.
				Code(CodeUnknownStepType).
				Errorf("generator step: message must be a string")
		}
		s.Message = v.Value
	}
	if v, ok := raw["default"]; ok {
		if v.Kind != yaml.ScalarNode {
			return oops.
				Code(CodeUnknownStepType).
				Errorf("generator step: default must be a string")
		}
		s.Default = v.Value
	}
	if v, ok := raw["required"]; ok {
		if v.Kind != yaml.ScalarNode {
			return oops.
				Code(CodeUnknownStepType).
				Errorf("generator step: required must be a boolean")
		}
		s.Required = v.Value == "true"
	}

	// Use-step fields.
	if v, ok := raw["value"]; ok {
		if v.Kind != yaml.ScalarNode {
			return oops.
				Code(CodeUnknownStepType).
				Errorf("generator step: value must be a string")
		}
		s.Value = v.Value
	}

	return nil
}

// parseDurationNode decodes a YAML node as a time.Duration.
// Accepts Go duration strings ("30s", "2m", "500ms") and integer 0.
func parseDurationNode(node *yaml.Node) (time.Duration, error) {
	switch node.Kind {
	case yaml.ScalarNode:
		// Integer 0 or string "0" disables timeout.
		if node.Tag == "!!int" || node.Value == "0" {
			if node.Value == "0" {
				return 0, nil
			}
			// Non-zero integer — treat as seconds.
			return time.ParseDuration(node.Value + "s")
		}
		return time.ParseDuration(node.Value)
	default:
		return 0, oops.
			Code(CodeUnknownStepType).
			Errorf("timeout must be a string or number")
	}
}

func yamlKind(k yaml.Kind) string {
	switch k {
	case yaml.ScalarNode:
		return "scalar"
	case yaml.SequenceNode:
		return "list"
	case yaml.MappingNode:
		return "mapping"
	default:
		return "unknown"
	}
}
