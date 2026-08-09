package hooks

import "time"

// Type identifies a hook lifecycle point.
type Type string

const (
	PreNew       Type = "pre-new"
	PostNew      Type = "post-new"
	PreGenerate  Type = "pre-generate"
	PostGenerate Type = "post-generate"
)

// validTypes is the authoritative set of recognised hook types.
var validTypes = map[Type]bool{
	PreNew: true, PostNew: true, PreGenerate: true, PostGenerate: true,
}

// Entry represents one hook command as parsed from .go-arch.yaml.
//
// Shell is true when the entry originated from the YAML string shorthand.
type Entry struct {
	Command       string            // required; command name or empty for missing
	Args          []string          // argv for object form
	Cwd           string            // per-hook working directory override
	Env           map[string]string // per-hook environment overrides
	Timeout       time.Duration     // per-hook timeout; 0 disables
	Silent        bool              // suppress stdout/stderr echo
	IgnoreFailure bool              // continue on non-zero exit
	Shell         bool              // true when parsed from string form
}

// Config holds the parsed hooks configuration.
type Config struct {
	Hooks map[Type][]Entry
}

// EnvContext provides lifecycle metadata for hook environment construction.
type EnvContext struct {
	ProjectName string // project_name from .go-arch.yaml
	ProjectPath string // absolute project directory path
	Arch        string // architecture value (Minimalist, Standard, Hexagonal)
	HookType    Type   // the hook type currently firing (drives HOOK_TYPE env var)
}
