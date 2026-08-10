package workspace

import (
	"os"
	"path/filepath"

	"github.com/samber/oops"
)

// workspaceFileName is the fixed workspace file name.
const workspaceFileName = "go-arch.workspace.yaml"

// Discover walks upward from cwd looking for a go-arch.workspace.yaml file.
// It returns the absolute path of the first match, or workspace_not_found.
//
// An explicit --workspace flag is handled at the command layer and wins over
// discovery (spec workspace-discovery).
func Discover(cwd string) (string, error) {
	dir, err := filepath.Abs(cwd)
	if err != nil {
		return "", oops.Code("workspace_invalid").Wrapf(err, "cannot resolve cwd %s", cwd)
	}

	for {
		candidate := filepath.Join(dir, workspaceFileName)
		if fi, err := os.Stat(candidate); err == nil && !fi.IsDir() {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", oops.
		Code("workspace_not_found").
		Hint("Run inside a monorepo with a go-arch.workspace.yaml, or pass --workspace <path>").
		Errorf("no %s found from %s or any parent", workspaceFileName, cwd)
}
