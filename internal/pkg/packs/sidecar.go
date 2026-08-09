package packs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Sidecar is the CLI-owned metadata written next to an installed pack's
// go-arch.yaml. It records hook acceptance and the original module ref
// so Update can re-fetch without guessing the module path.
type Sidecar struct {
	// HooksEnabled records whether the user accepted hooks during install.
	HooksEnabled bool `json:"hooks_enabled"`
	// InstalledAt is the UTC timestamp when the pack was installed.
	InstalledAt time.Time `json:"installed_at"`
	// ModuleRef is the original module@version ref used for installation.
	ModuleRef string `json:"module_ref"`
}

// packJSONFile is the name of the sidecar file in the pack install directory.
const packJSONFile = "pack.json"

// writeSidecar writes the Sidecar to pack.json in the given pack directory.
func writeSidecar(packDir string, s Sidecar) error {
	path := filepath.Join(packDir, packJSONFile)
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal pack %s: %w", packJSONFile, err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// ReadSidecar reads and parses pack.json from the given pack directory.
// Exported so callers (cmd, mcp) can inspect the HooksEnabled flag
// before building a pack-scoped hooks runner.
func ReadSidecar(packDir string) (Sidecar, error) {
	var s Sidecar
	path := filepath.Join(packDir, packJSONFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil // fresh install, no sidecar yet
		}
		return s, fmt.Errorf("read %s: %w", path, err)
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return s, fmt.Errorf("unmarshal %s: %w", path, err)
	}
	return s, nil
}
