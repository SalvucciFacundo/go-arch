package hooks

import (
	"fmt"
	"strings"
)

// BuildEnv constructs the process environment for a hook execution.
//
// Layers (applied in order, later wins):
//  1. parent — the parent process environment (os.Environ() from the caller)
//  2. standard vars — PROJECT_NAME, PROJECT_PATH, ARCHITECTURE, HOOK_TYPE
//  3. perEntry — per-hook env: overrides from configuration
//
// BuildEnv never modifies the parent slice.
func BuildEnv(parent []string, ctx EnvContext, perEntry map[string]string) []string {
	// Collect into a map for easy layering.
	result := make(map[string]string)

	// Layer 1: parent env.
	for _, e := range parent {
		// Only set if non-empty; discard malformed entries.
		if e != "" {
			k, _, ok := strings.Cut(e, "=")
			if ok {
				result[k] = e
			}
			// Malformed entries (no "=") are silently discarded.
		}
	}

	// Layer 2: standard vars (overwrite parent).
	standardEnvs := map[string]string{
		"PROJECT_NAME": ctx.ProjectName,
		"PROJECT_PATH": ctx.ProjectPath,
		"ARCHITECTURE": ctx.Arch,
		"HOOK_TYPE":    string(ctx.HookType),
	}
	for k, v := range standardEnvs {
		result[k] = fmt.Sprintf("%s=%s", k, v)
	}

	// Layer 3: per-entry overrides (win over everything).
	for k, v := range perEntry {
		result[k] = fmt.Sprintf("%s=%s", k, v)
	}

	// Flatten to a slice.
	out := make([]string, 0, len(result))
	for _, v := range result {
		out = append(out, v)
	}
	return out
}
