package cmd

import (
	"github.com/SalvucciFacundo/go-arch/v2/internal/pkg/packs"
	"github.com/SalvucciFacundo/go-arch/v2/internal/ui"
	"os"
	"path/filepath"
	"strings"
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

// TestNewTemplatePackHooksFire verifies CRITICAL FIX #1: pack-declared hooks fire
// in the production call path (runNewWithTemplate uses the pack manifest hooks,
// reading the sidecar's HooksEnabled flag). Before this fix, the runner was
// always built from the user's global hooks config, ignoring the pack manifest.
func TestNewTemplatePackHooksFire(t *testing.T) {
	// --- Subtest: hooks enabled — hook fires ---
	t.Run("hooks-enabled", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create pack fixture with a post-new hook that writes a marker file.
		packDir := filepath.Join(tmpDir, "express@1.0.0")
		if err := os.MkdirAll(packDir, 0755); err != nil {
			t.Fatal(err)
		}

		// Manifest with a post-new hook
		manifestYAML := `contract_version: 1
name: express
version: 1.0.0
layout:
  - cmd/api
hooks:
  post-new:
    - touch hook_marker
`
		if err := os.WriteFile(filepath.Join(packDir, "go-arch.yaml"), []byte(manifestYAML), 0644); err != nil {
			t.Fatal(err)
		}

		// Sidecar: hooks enabled
		sidecarJSON := `{"hooks_enabled": true, "installed_at": "2026-01-01T00:00:00Z", "module_ref": "github.com/test/express@1.0.0"}`
		if err := os.WriteFile(filepath.Join(packDir, "pack.json"), []byte(sidecarJSON), 0644); err != nil {
			t.Fatal(err)
		}

		// Templates dir (minimal: required for non-empty check)
		templatesDir := filepath.Join(packDir, "templates")
		if err := os.MkdirAll(templatesDir, 0755); err != nil {
			t.Fatal(err)
		}
		mainTmpl := `package main

import "fmt"

func main() {
	fmt.Println("Hello from {{ .ProjectName }}")
}
`
		if err := os.WriteFile(filepath.Join(templatesDir, "main.go.tmpl"), []byte(mainTmpl), 0644); err != nil {
			t.Fatal(err)
		}
		configTmpl := `project_name: {{ .ProjectName }}
module_name: {{ .ModuleName }}
architecture: {{ .Architecture }}
template: {{ .Template }}
`
		if err := os.WriteFile(filepath.Join(templatesDir, ".go-arch.yaml.tmpl"), []byte(configTmpl), 0644); err != nil {
			t.Fatal(err)
		}

		// Set up packs dir
		t.Setenv(packs.EnvPacksDir, tmpDir)

		oldWd, _ := os.Getwd()
		if err := os.Chdir(tmpDir); err != nil {
			t.Fatal(err)
		}
		defer func() { _ = os.Chdir(oldWd) }()

		// Redirect ui.Out to avoid test noise
		origOut := ui.Out
		ui.Out, _ = os.Create(filepath.Join(tmpDir, "ui.log"))
		defer func() { ui.Out = origOut }()

		// Call runNewWithTemplate — the PRODUCTION call path.
		err := runNewWithTemplate("myapp", "express@1.0.0")
		if err != nil {
			t.Fatalf("runNewWithTemplate failed: %v", err)
		}

		// Verify the project dir was created
		projectDir := filepath.Join(tmpDir, "myapp")
		if _, statErr := os.Stat(projectDir); statErr != nil {
			t.Fatalf("project directory not created: %v", statErr)
		}

		// Assert the hook fired: the hook creates "hook_marker" in CWD
		// which is the project dir at post-new time.
		markerFile := filepath.Join(projectDir, "hook_marker")
		if _, err := os.Stat(markerFile); os.IsNotExist(err) {
			t.Error("expected hook_marker file — pack hook did NOT fire in production path")
		}
	})

	// --- Subtest: hooks disabled — hook does NOT fire ---
	t.Run("hooks-disabled", func(t *testing.T) {
		tmpDir := t.TempDir()

		packDir := filepath.Join(tmpDir, "express@1.0.0")
		if err := os.MkdirAll(packDir, 0755); err != nil {
			t.Fatal(err)
		}

		manifestYAML := `contract_version: 1
name: express
version: 1.0.0
layout:
  - cmd/api
hooks:
  post-new:
    - touch hook_marker
`
		if err := os.WriteFile(filepath.Join(packDir, "go-arch.yaml"), []byte(manifestYAML), 0644); err != nil {
			t.Fatal(err)
		}

		// Sidecar: hooks DISABLED (user declined trust prompt)
		sidecarJSON := `{"hooks_enabled": false, "installed_at": "2026-01-01T00:00:00Z", "module_ref": "github.com/test/express@1.0.0"}`
		if err := os.WriteFile(filepath.Join(packDir, "pack.json"), []byte(sidecarJSON), 0644); err != nil {
			t.Fatal(err)
		}

		templatesDir := filepath.Join(packDir, "templates")
		if err := os.MkdirAll(templatesDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(templatesDir, "main.go.tmpl"), []byte(`package main

import "fmt"

func main() {
	fmt.Println("Hello from {{ .ProjectName }}")
}
`), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(templatesDir, ".go-arch.yaml.tmpl"), []byte(`project_name: {{ .ProjectName }}
module_name: {{ .ModuleName }}
architecture: {{ .Architecture }}
template: {{ .Template }}
`), 0644); err != nil {
			t.Fatal(err)
		}

		t.Setenv(packs.EnvPacksDir, tmpDir)

		oldWd, _ := os.Getwd()
		if err := os.Chdir(tmpDir); err != nil {
			t.Fatal(err)
		}
		defer func() { _ = os.Chdir(oldWd) }()

		origOut := ui.Out
		ui.Out, _ = os.Create(filepath.Join(tmpDir, "ui.log"))
		defer func() { ui.Out = origOut }()

		err := runNewWithTemplate("myapp", "express@1.0.0")
		if err != nil {
			t.Fatalf("runNewWithTemplate failed: %v", err)
		}

		projectDir := filepath.Join(tmpDir, "myapp")
		markerFile := filepath.Join(projectDir, "hook_marker")
		if _, err := os.Stat(markerFile); err == nil {
			t.Error("hook_marker was created — pack hook fired despite HooksEnabled=false")
		}
	})
}

// TestNewTemplateNotInstalledHint verifies that the error when a pack is not
// installed includes the hint to run "go-arch template install".
func TestNewTemplateNotInstalledHint(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	t.Setenv(packs.EnvPacksDir, tmpDir)

	err := runNewWithTemplate("myapp", "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent pack, got nil")
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "template install") {
		t.Errorf("error should contain hint about 'template install', got: %v", errStr)
	}
}
