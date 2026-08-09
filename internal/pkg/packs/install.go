package packs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/samber/oops"
)

// InstalledPack is the return type for List — a summary of an installed pack.
type InstalledPack struct {
	Name    string
	Version string
}

// Install fetches a pack module, validates it, prompts for hook trust if
// needed, and materializes it under ~/.go-arch/packs/<name>@<version>/.
//
// Parameters:
//   - dl: the Downloader used to fetch the module (injected for testability)
//   - module: the Go module path (e.g. "github.com/org/go-arch-express")
//   - version: the module version (e.g. "1.0.0")
//   - confirm: called with the pack name when hooks are declared; returns
//     (accepted, error). Must be injectable so tests can simulate accept
//     and decline without stdin. The real implementation uses survey.AskOne.
func Install(ctx context.Context, dl Downloader, module, version string, confirm func(packName string) (bool, error)) (*Manifest, error) {
	modRef := module + "@" + version

	// 1. Download.
	srcDir, err := dl.Download(ctx, module, version)
	if err != nil {
		return nil, err
	}

	// 2. Load manifest from the downloaded source.
	m, err := Load(srcDir)
	if err != nil {
		return nil, err
	}

	// 3. Validate templates/ directory exists.
	tmplDir := filepath.Join(srcDir, "templates")
	info, err := os.Stat(tmplDir)
	if err != nil || !info.IsDir() {
		return nil, oops.
			Code(CodePackNoTemplates).
			Errorf("pack %q has no templates directory", m.Name)
	}

	// 4. Build sidecar and prompt for hooks trust.
	sc := Sidecar{
		InstalledAt: nowUTC(),
		ModuleRef:   modRef,
	}
	if len(m.Hooks) > 0 || anyGeneratorRunsCommands(m) {
		accepted, err := confirm(m.Name)
		if err != nil {
			return nil, fmt.Errorf("trust prompt: %w", err)
		}
		sc.HooksEnabled = accepted
	} else {
		sc.HooksEnabled = false
	}

	// 5. Install: atomic replace via RemoveAll + Rename.
	dst := Path(m.Name, m.Version)

	// Remove existing installation (idempotent re-install).
	if _, err := os.Stat(dst); err == nil {
		if err := os.RemoveAll(dst); err != nil {
			return nil, oops.
				Code(CodePackInstallFailed).
				Wrapf(err, "removing previous installation at %s", dst)
		}
	}

	// Copy to a temp directory first, then rename atomically.
	tmpDir, err := os.MkdirTemp(filepath.Dir(dst), ".tmp-"+m.Name+"-")
	if err != nil {
		return nil, oops.
			Code(CodePackInstallFailed).
			Wrapf(err, "creating temp dir for install")
	}
	defer os.RemoveAll(tmpDir) // cleanup temp on failure

	if err := copyDir(srcDir, tmpDir); err != nil {
		return nil, oops.
			Code(CodePackInstallFailed).
			Wrapf(err, "copying pack to temp dir")
	}

	// Write sidecar inside the temp dir before rename.
	if err := writeSidecar(tmpDir, sc); err != nil {
		return nil, oops.
			Code(CodePackInstallFailed).
			Wrapf(err, "writing sidecar")
	}

	// Atomic rename (POSIX: replaces empty dir atomically; we already
	// RemoveAll'd dst, so the parent is clear).
	if err := os.Rename(tmpDir, dst); err != nil {
		return nil, oops.
			Code(CodePackInstallFailed).
			Wrapf(err, "moving pack to %s", dst)
	}

	// 6. Re-validate manifest from the installed destination.
	m2, err := Load(dst)
	if err != nil {
		// Clean up partial state.
		os.RemoveAll(dst)
		return nil, err
	}

	return m2, nil
}

// Remove deletes an installed pack version.
//
// Returns CodePackNotInstalled if the pack directory does not exist.
func Remove(name, version string) error {
	p := Path(name, version)
	if _, err := os.Stat(p); err != nil {
		if os.IsNotExist(err) {
			return oops.
				Code(CodePackNotInstalled).
				Errorf("pack %q@%s is not installed", name, version)
		}
		return fmt.Errorf("checking pack %s@%s: %w", name, version, err)
	}

	if err := os.RemoveAll(p); err != nil {
		return fmt.Errorf("removing pack %s@%s: %w", name, version, err)
	}
	return nil
}

// List returns all installed packs sorted by name (then version).
func List() ([]InstalledPack, error) {
	base := BaseDir()
	entries, err := os.ReadDir(base)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // no packs dir = empty list
		}
		return nil, fmt.Errorf("reading packs directory: %w", err)
	}

	var packs []InstalledPack
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if strings.HasPrefix(entry.Name(), ".") {
			continue // skip temp dirs
		}
		name, ver, ok := parsePackDirName(entry.Name())
		if !ok {
			continue
		}
		packs = append(packs, InstalledPack{Name: name, Version: ver})
	}

	sort.Slice(packs, func(i, j int) bool {
		if packs[i].Name != packs[j].Name {
			return packs[i].Name < packs[j].Name
		}
		return packs[i].Version < packs[j].Version
	})

	return packs, nil
}

// parsePackDirName splits a directory name like "express@1.0.0" into
// name and version. Returns false if the format is invalid.
func parsePackDirName(dirName string) (name, version string, ok bool) {
	idx := strings.LastIndex(dirName, "@")
	if idx <= 0 || idx == len(dirName)-1 {
		return "", "", false
	}
	return dirName[:idx], dirName[idx+1:], true
}

// Update fetches the @latest version of a pack and installs it.
//
// Requires at least one version installed to determine the module ref
// from the existing sidecar. Re-prompts for hooks if the new version
// declares hooks.
func Update(ctx context.Context, dl Downloader, name string, confirm func(packName string) (bool, error)) (*Manifest, error) {
	// Find the latest installed version to read its sidecar for module ref.
	latestVer, err := LatestInstalled(name)
	if err != nil {
		return nil, err // CodePackNotInstalled
	}

	// Read the sidecar to get the original module ref.
	sc, err := ReadSidecar(Path(name, latestVer))
	if err != nil {
		return nil, fmt.Errorf("reading sidecar for %s@%s: %w", name, latestVer, err)
	}

	if sc.ModuleRef == "" {
		return nil, oops.
			Code(CodePackNotInstalled).
			Errorf("pack %q has no module ref in sidecar; reinstall manually", name)
	}

	// Parse module and version from the stored ref.
	module, _, err := ParseRef(sc.ModuleRef)
	if err != nil {
		return nil, fmt.Errorf("parsing module ref %q: %w", sc.ModuleRef, err)
	}

	// Fetch @latest.
	srcDir, err := dl.Download(ctx, module, "latest")
	if err != nil {
		return nil, err
	}

	// Load manifest to discover the resolved version and pack name.
	m, err := Load(srcDir)
	if err != nil {
		return nil, err
	}

	// Install with the resolved version.
	return Install(ctx, dl, module, m.Version, confirm)
}

// nowUTC returns the current time in UTC. Extracted for testability.
var nowUTC = func() time.Time {
	return time.Now().UTC()
}

// anyGeneratorRunsCommands returns true when any generator in the manifest
// declares run: steps or pre:/post: hooks that could execute commands.
func anyGeneratorRunsCommands(m *Manifest) bool {
	for _, g := range m.Generators {
		if g.RunsCommands() {
			return true
		}
	}
	return false
}
