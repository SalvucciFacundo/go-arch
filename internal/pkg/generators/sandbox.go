package generators

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/samber/oops"
)

// ValidateTarget checks that a recipe step's target path resolves inside
// projectRoot. It uses a separator-aware prefix check to prevent sibling
// false-positives (e.g. root /home/u/myapp, target /home/u/myapp-evil).
//
// Returns recipe_path_escape with the resolved offending path when the
// target escapes the project root.
func ValidateTarget(projectRoot, relPath string) error {
	cleanedRoot := filepath.Clean(projectRoot)
	cleanedRel := filepath.Clean(relPath)

	// 1. Reject absolute paths.
	if filepath.IsAbs(cleanedRel) {
		return oops.
			Code(CodeRecipePathEscape).
			Errorf("path escapes project root: %q is absolute", relPath)
	}

	// 2. Reject paths containing .. segments.
	if strings.Contains(cleanedRel, "..") {
		return oops.
			Code(CodeRecipePathEscape).
			Errorf("path escapes project root: %q contains ..", relPath)
	}

	// 3. Join root + relPath and resolve symlinks.
	joined := filepath.Join(cleanedRoot, cleanedRel)
	resolved, err := resolveExistingSymlinkPath(joined)
	if err != nil {
		return oops.
			Code(CodeRecipePathEscape).
			Wrapf(err, "path escapes project root: cannot resolve %q", joined)
	}
	resolved = filepath.Clean(resolved)

	// 4. Windows: compare volume names first.
	rootVol := filepath.VolumeName(cleanedRoot)
	resolvedVol := filepath.VolumeName(resolved)
	if rootVol != resolvedVol {
		return oops.
			Code(CodeRecipePathEscape).
			Errorf("path escapes project root: volume %q ≠ %q", resolvedVol, rootVol)
	}

	// 5. Separator-aware prefix check.
	//    Use root + os.PathSeparator to prevent sibling false-positives
	//    (e.g. "/home/u/myapp-evil" must NOT match prefix "/home/u/myapp").
	boundary := cleanedRoot + string(os.PathSeparator)
	if !strings.HasPrefix(resolved, boundary) && resolved != cleanedRoot {
		return oops.
			Code(CodeRecipePathEscape).
			Errorf("path escapes project root: %q resolves to %q (outside %q)", relPath, resolved, cleanedRoot)
	}

	return nil
}

// resolveExistingSymlinkPath resolves symlinks on the longest existing
// prefix of path, then appends the remaining (non-existing) suffix.
// This handles the common case where the target file doesn't exist yet
// but a parent directory is a symlink.
func resolveExistingSymlinkPath(path string) (string, error) {
	// Fast path: all components exist.
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return resolved, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}

	// Walk up until we find an existing prefix, resolve symlinks there,
	// then append the rest.
	dir := filepath.Dir(path)
	for dir != path && dir != "." {
		resolved, err := filepath.EvalSymlinks(dir)
		if err == nil {
			rel, _ := filepath.Rel(dir, path)
			return filepath.Join(resolved, rel), nil
		}
		path = dir
		dir = filepath.Dir(path)
	}

	// Fallback: nothing resolvable — use the path as-is.
	return filepath.Clean(path), nil
}
