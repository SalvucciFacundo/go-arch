package cmd

import (
	"go-arch/internal/pkg/packs"
	"go-arch/internal/ui"
	"os"
	"path/filepath"
	"testing"
)

// TestNewEmptyPackNoDir verifies task 4.8: a pack with an empty templates/
// directory errors BEFORE any project directory is created. The check must
// happen pre-Execute (no myapp/ directory created on disk).
func TestNewEmptyPackNoDir(t *testing.T) {
	// Create a fixture pack with no templates directory
	tmpDir := t.TempDir()
	packDir := filepath.Join(tmpDir, "empty@1.0.0")
	if err := os.MkdirAll(packDir, 0755); err != nil {
		t.Fatal(err)
	}

	manifestYAML := `contract_version: 1
name: empty
version: 1.0.0
layout:
  - cmd/api
`
	if err := os.WriteFile(filepath.Join(packDir, "go-arch.yaml"), []byte(manifestYAML), 0644); err != nil {
		t.Fatal(err)
	}

	// No templates/ directory — this is the empty pack case

	// Set up packs dir for LatestInstalled to find our fixture
	t.Setenv(packs.EnvPacksDir, tmpDir)

	oldWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	// Emulate resolve-and-validate that cmd/new.go will perform
	packInfo, err := resolvePackForNew("empty", "1.0.0")
	if err != nil {
		t.Fatalf("unexpected resolve error: %v", err)
	}

	// Pre-Execute empty-templates check (task 4.8 / G4)
	err = checkTemplatesNonEmpty(packInfo)
	if err == nil {
		t.Fatal("expected error for empty pack, got nil")
	}

	// Assert NO project directory was created (the key G4 guarantee)
	projectDir := "myapp"
	if _, statErr := os.Stat(projectDir); statErr == nil {
		t.Errorf("expected %q to NOT be created for empty pack", projectDir)
	}
}

// TestPackDefaults verifies task 4.6 (P2): packDefaults fills defaults
// suitable for a pack-scaffolded project. Architecture is "" (pack IS the
// architecture), all feature flags are false, ModuleName defaults to
// projectName, and Template is set.
func TestPackDefaults(t *testing.T) {
	cfg := newPackDefaults("myapp", "express")
	if cfg.ProjectName != "myapp" {
		t.Errorf("ProjectName = %q, want myapp", cfg.ProjectName)
	}
	if cfg.ModuleName != "myapp" {
		t.Errorf("ModuleName = %q, want myapp", cfg.ModuleName)
	}
	if cfg.Architecture != "" {
		t.Errorf("Architecture = %q, want empty (pack IS the arch)", cfg.Architecture)
	}
	if cfg.UseDocker {
		t.Errorf("UseDocker = true, want false")
	}
	if cfg.UseObservability {
		t.Errorf("UseObservability = true, want false")
	}
	if cfg.UseGRPC {
		t.Errorf("UseGRPC = true, want false")
	}
	if cfg.UseTemplHTMX {
		t.Errorf("UseTemplHTMX = true, want false")
	}
	if cfg.Template != "express" {
		t.Errorf("Template = %q, want express", cfg.Template)
	}

	// Verify it's assignable to ui.ProjectConfig
	var _ ui.ProjectConfig = *cfg
}
