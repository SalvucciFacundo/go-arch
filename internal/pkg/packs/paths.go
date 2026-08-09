package packs

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/samber/oops"
)

// EnvPacksDir overrides the default packs installation directory.
// Tests set this to t.TempDir() to isolate filesystem operations.
const EnvPacksDir = "GO_ARCH_PACKS_DIR"

// BaseDir returns the packs installation root directory.
//
// Respects the GO_ARCH_PACKS_DIR environment variable for testing.
// Defaults to $HOME/.go-arch/packs.
func BaseDir() string {
	if dir := os.Getenv(EnvPacksDir); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".go-arch", "packs")
	}
	return filepath.Join(home, ".go-arch", "packs")
}

// Path returns the installed directory for a specific pack version.
func Path(name, version string) string {
	return filepath.Join(BaseDir(), name+"@"+version)
}

// ParseRef splits a pack reference into name and version.
//
// "express@1.0.0" → ("express", "1.0.0")
// "express" → ("express", "")
func ParseRef(ref string) (name, version string, err error) {
	if ref == "" {
		return "", "", fmt.Errorf("pack ref must not be empty")
	}

	// Split at the first @ to handle scoped scenarios.
	idx := strings.Index(ref, "@")
	if idx == -1 {
		return ref, "", nil
	}
	name = ref[:idx]
	version = ref[idx+1:]

	if name == "" {
		return "", "", fmt.Errorf("pack ref must not be empty (name before @<version>)")
	}
	return name, version, nil
}

// ValidateSlug checks that the pack name matches the slug convention.
//
// Valid: lowercase alphanumeric + dashes (^[a-z0-9]+(-[a-z0-9]+)*$).
func ValidateSlug(name string) error {
	if !slugRE.MatchString(name) {
		return fmt.Errorf("%q is not a valid pack slug (lowercase alphanumeric + dashes)", name)
	}
	return nil
}

// LatestInstalled returns the highest installed version of a pack.
//
// Scans BaseDir() for directories matching <name>@<version> and returns
// the greatest semver. Returns CodePackNotInstalled if no match is found.
func LatestInstalled(name string) (string, error) {
	base := BaseDir()
	entries, err := os.ReadDir(base)
	if err != nil {
		if os.IsNotExist(err) {
			return "", oops.
				Code(CodePackNotInstalled).
				Errorf("pack %q is not installed", name)
		}
		return "", fmt.Errorf("reading packs directory: %w", err)
	}

	var versions []string
	prefix := name + "@"

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dirName := entry.Name()
		if !strings.HasPrefix(dirName, prefix) {
			continue
		}
		ver := strings.TrimPrefix(dirName, prefix)
		if ver != "" {
			versions = append(versions, ver)
		}
	}

	if len(versions) == 0 {
		return "", oops.
			Code(CodePackNotInstalled).
			Errorf("pack %q is not installed", name)
	}

	sort.Slice(versions, func(i, j int) bool {
		return compareSemver(versions[i], versions[j]) < 0
	})

	return versions[len(versions)-1], nil
}

// compareSemver compares two simple semver strings (MAJOR.MINOR.PATCH[-prerelease]).
//
// Returns < 0 if a < b, 0 if equal, > 0 if a > b.
// Prerelease versions sort before the corresponding release version.
func compareSemver(a, b string) int {
	return compareSemverParts(parseSemver(a), parseSemver(b))
}

type semverPart struct {
	major int64
	minor int64
	patch int64
	pre   string
}

func parseSemver(v string) semverPart {
	var part semverPart

	// Split prerelease suffix.
	base, pre, _ := strings.Cut(v, "-")
	part.pre = pre

	// Remove build metadata.
	if idx := strings.Index(base, "+"); idx >= 0 {
		base = base[:idx]
	}

	nums := strings.Split(base, ".")
	if len(nums) >= 1 {
		part.major, _ = strconv.ParseInt(nums[0], 10, 64)
	}
	if len(nums) >= 2 {
		part.minor, _ = strconv.ParseInt(nums[1], 10, 64)
	}
	if len(nums) >= 3 {
		part.patch, _ = strconv.ParseInt(nums[2], 10, 64)
	}

	return part
}

func compareSemverParts(a, b semverPart) int {
	if d := diff(a.major, b.major); d != 0 {
		return d
	}
	if d := diff(a.minor, b.minor); d != 0 {
		return d
	}
	if d := diff(a.patch, b.patch); d != 0 {
		return d
	}
	// Prerelease sorting: prerelease < release, compare lexicographically.
	if a.pre == "" && b.pre == "" {
		return 0
	}
	if a.pre == "" {
		return 1 // release > prerelease
	}
	if b.pre == "" {
		return -1 // prerelease < release
	}
	return strings.Compare(a.pre, b.pre)
}

func diff(a, b int64) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}
