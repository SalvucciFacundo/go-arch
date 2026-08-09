package hooks

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/samber/oops"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

// rawConfig is the YAML deserialization wrapper for the hooks: key.
// Using rawHooksMap gives us a chance to intercept the value nodes
// before decoding them into []Entry, which lets us distinguish
// scalar-not-list from a valid sequence.
type rawConfig struct {
	Hooks rawHooksMap `yaml:"hooks,omitempty"`
}

// rawHooksMap is map[Type][]Entry with a custom UnmarshalYAML that
// validates the outer mapping and intercepts scalar values before
// yaml.v3 would reject them as incompatible with []Entry.
type rawHooksMap map[Type][]Entry

func (m *rawHooksMap) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.MappingNode {
		return oops.
			Code(CodeInvalidHookConfig).
			Hint("See docs/hooks.md for the hooks schema").
			Errorf("hooks: must be a mapping of hook type to entry list")
	}

	result := make(rawHooksMap)

	for i := 0; i < len(value.Content); i += 2 {
		keyNode := value.Content[i]
		valNode := value.Content[i+1]
		typ := Type(keyNode.Value)

		if !validTypes[typ] {
			return oops.
				Code(CodeUnknownHookType).
				Hint("Valid hook types: pre-new, post-new, pre-generate, post-generate").
				Errorf("unknown hook type %q", typ)
		}

		// The value for a hook type must be a sequence.
		if valNode.Kind != yaml.SequenceNode {
			return oops.
				Code(CodeInvalidHookConfig).
				Hint("See docs/hooks.md for the hooks schema").
				Errorf("hooks.%s: expected a list of entries, got %s", typ, yamlKind(valNode.Kind))
		}

		var entries []Entry
		if err := valNode.Decode(&entries); err != nil {
			return oops.
				Code(CodeInvalidHookConfig).
				Hint("See docs/hooks.md for the hooks schema").
				Wrapf(err, "hooks.%s: invalid entry list", typ)
		}

		result[typ] = entries
	}

	*m = result
	return nil
}

// Load reads a hooks configuration file at path.
//
// If the file does not exist, Load returns an empty Config and nil error.
// This is a deliberate no-op design so that commands work out of the box
// on machines that have never run go-arch setup.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Config{Hooks: make(map[Type][]Entry)}, nil
	}
	if err != nil {
		return nil, err
	}

	var raw rawConfig
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	if raw.Hooks == nil {
		return &Config{Hooks: make(map[Type][]Entry)}, nil
	}

	return &Config{Hooks: map[Type][]Entry(raw.Hooks)}, nil
}

// ResolveConfigPath returns the path to the hooks configuration file.
//
// If Viper has loaded a config file (via .ConfigFileUsed()), that path
// is returned. Otherwise it falls back to $HOME/.go-arch.yaml.
func ResolveConfigPath() string {
	if used := viper.ConfigFileUsed(); used != "" {
		return used
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".go-arch.yaml")
	}
	return filepath.Join(home, ".go-arch.yaml")
}

// recognizedEntryKeys lists the YAML keys allowed inside an object-form hook entry.
var recognizedEntryKeys = map[string]bool{
	"command":        true,
	"args":           true,
	"cwd":            true,
	"env":            true,
	"timeout":        true,
	"silent":         true,
	"ignore_failure": true,
}

// UnmarshalYAML implements yaml.Unmarshaler for hybrid dispatch.
//
// String shorthand ("gofmt -w .") → Shell=true, command parsed via strings.Fields.
// Object form ({command, args, ...}) → Shell=false, each known key decoded.
func (e *Entry) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		return e.unmarshalString(value)

	case yaml.MappingNode:
		return e.unmarshalObject(value)

	default:
		return oops.
			Code(CodeInvalidHookConfig).
			Hint("See docs/hooks.md for the hooks schema").
			Errorf("hook entry must be a string or an object, got %s", yamlKind(value.Kind))
	}
}

func (e *Entry) unmarshalString(value *yaml.Node) error {
	parts := strings.Fields(value.Value)
	if len(parts) == 0 {
		return oops.
			Code(CodeInvalidHookConfig).
			Hint("See docs/hooks.md for the hooks schema").
			Errorf("empty string hook entry")
	}
	e.Command = parts[0]
	e.Args = parts[1:]
	e.Shell = true
	return nil
}

func (e *Entry) unmarshalObject(value *yaml.Node) error {
	hasCommand := false

	for i := 0; i < len(value.Content); i += 2 {
		key := value.Content[i].Value
		val := value.Content[i+1]

		if !recognizedEntryKeys[key] {
			return oops.
				Code(CodeInvalidHookConfig).
				Hint("See docs/hooks.md for the hooks schema").
				Errorf("unknown key %q in hook entry", key)
		}

		switch key {
		case "command":
			if val.Kind != yaml.ScalarNode {
				return oops.
					Code(CodeInvalidHookConfig).
					Hint("See docs/hooks.md for the hooks schema").
					Errorf("hooks entry: command must be a string")
			}
			e.Command = val.Value
			hasCommand = true

		case "args":
			if err := val.Decode(&e.Args); err != nil {
				return oops.
					Code(CodeInvalidHookConfig).
					Hint("See docs/hooks.md for the hooks schema").
					Wrapf(err, "hooks entry: args must be a list of strings")
			}

		case "cwd":
			if val.Kind != yaml.ScalarNode {
				return oops.
					Code(CodeInvalidHookConfig).
					Hint("See docs/hooks.md for the hooks schema").
					Errorf("hooks entry: cwd must be a string")
			}
			e.Cwd = val.Value

		case "env":
			if err := val.Decode(&e.Env); err != nil {
				return oops.
					Code(CodeInvalidHookConfig).
					Hint("See docs/hooks.md for the hooks schema").
					Wrapf(err, "hooks entry: env must be a string map")
			}

		case "timeout":
			if err := parseTimeoutNode(val, e); err != nil {
				return err
			}

		case "silent":
			if val.Kind != yaml.ScalarNode {
				return oops.
					Code(CodeInvalidHookConfig).
					Hint("See docs/hooks.md for the hooks schema").
					Errorf("hooks entry: silent must be a boolean")
			}
			e.Silent = val.Value == "true"

		case "ignore_failure":
			if val.Kind != yaml.ScalarNode {
				return oops.
					Code(CodeInvalidHookConfig).
					Hint("See docs/hooks.md for the hooks schema").
					Errorf("hooks entry: ignore_failure must be a boolean")
			}
			e.IgnoreFailure = val.Value == "true"
		}
	}

	if !hasCommand {
		return oops.
			Code(CodeInvalidHookConfig).
			Hint("See docs/hooks.md for the hooks schema").
			Errorf("hook entry requires a command")
	}

	// object form never uses shell dispatch.
	e.Shell = false
	return nil
}

// parseTimeoutNode decodes a YAML node as a timeout duration.
//
// Accepts:
//   - timeout: 0            (integer 0 → disabled)
//   - timeout: "0"          (string "0" → disabled)
//   - timeout: "30s"        (Go duration string)
func parseTimeoutNode(node *yaml.Node, e *Entry) error {
	switch node.Kind {
	case yaml.ScalarNode:
		// Integer 0 (or string "0") disables the timeout.
		if node.Tag == "!!int" || node.Value == "0" {
			if node.Value == "0" {
				e.Timeout = 0
				e.TimeoutSet = true
				return nil
			}
			// Non-zero integer — treat as seconds.
			d, err := time.ParseDuration(node.Value + "s")
			if err != nil {
				return oops.
					Code(CodeInvalidHookConfig).
					Hint("timeout must be a Go duration string like 30s, 2m, 500ms, or 0 to disable").
					Errorf("invalid timeout %q: %v", node.Value, err)
			}
			e.Timeout = d
			e.TimeoutSet = true
			return nil
		}
		d, err := time.ParseDuration(node.Value)
		if err != nil {
			return oops.
				Code(CodeInvalidHookConfig).
				Hint("timeout must be a Go duration string like 30s, 2m, 500ms, or 0 to disable").
				Errorf("invalid timeout %q: %v", node.Value, err)
		}
		e.Timeout = d
		e.TimeoutSet = true
		return nil

	default:
		return oops.
			Code(CodeInvalidHookConfig).
			Hint("timeout must be a duration string or 0").
			Errorf("timeout must be a string or number, got %s", yamlKind(node.Kind))
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
