package cmd

import (
	"bytes"
	"go-arch/internal/ui"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTemplateList_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("GO_ARCH_PACKS_DIR", filepath.Join(tmpDir, ".go-arch", "packs"))

	// Capture output via ui.Out redirect.
	var buf bytes.Buffer
	old := ui.Out
	ui.Out = &buf
	defer func() { ui.Out = old }()

	err := templateListCmd.RunE(templateListCmd, nil)
	if err != nil {
		t.Fatalf("template list failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "No packs installed") {
		t.Errorf("output = %q; want it to contain 'No packs installed'", output)
	}
}

func TestTemplateRemove_NotInstalled(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("GO_ARCH_PACKS_DIR", filepath.Join(tmpDir, ".go-arch", "packs"))

	err := templateRemoveCmd.RunE(templateRemoveCmd, []string{"express"})
	if err == nil {
		t.Fatal("expected error removing non-installed pack 'express'")
	}
	if !strings.Contains(err.Error(), "express") {
		t.Errorf("error = %q; want it to mention 'express'", err.Error())
	}
}

func TestTemplateCmds_Registered(t *testing.T) {
	// Verify subcommands exist on the template command.
	found := make(map[string]bool)
	for _, c := range templateCmd.Commands() {
		found[c.Name()] = true
	}
	for _, name := range []string{"install", "list", "remove", "update"} {
		if !found[name] {
			t.Errorf("template subcommand %q not registered", name)
		}
	}
}

// Ensure the HOME env and GO_ARCH_PACKS_DIR are isolated for pack tests.
func TestTemplateList_IsolatedEnv(t *testing.T) {
	tmpDir := t.TempDir()
	packsDir := filepath.Join(tmpDir, ".go-arch", "packs")
	t.Setenv("HOME", tmpDir)
	t.Setenv("GO_ARCH_PACKS_DIR", packsDir)

	// Ensure the packs dir exists (empty).
	if err := os.MkdirAll(packsDir, 0755); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	old := ui.Out
	ui.Out = &buf
	defer func() { ui.Out = old }()

	err := templateListCmd.RunE(templateListCmd, nil)
	if err != nil {
		t.Fatalf("template list failed: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "No packs installed") {
		t.Errorf("output = %q; want it to contain 'No packs installed'", output)
	}
}

func TestTemplateRemove_VersionNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("GO_ARCH_PACKS_DIR", filepath.Join(tmpDir, ".go-arch", "packs"))

	// Remove specific version of a non-installed pack.
	err := templateRemoveCmd.RunE(templateRemoveCmd, []string{"express@1.0.0"})
	if err == nil {
		t.Fatal("expected error removing non-installed pack version")
	}
	if !strings.Contains(err.Error(), "express") || !strings.Contains(err.Error(), "not installed") {
		t.Errorf("error = %q; want it to mention 'express' and 'not installed'", err.Error())
	}
}

// TestTemplateList_WithPacks verifies list output format when packs are installed.
func TestTemplateList_WithPacks(t *testing.T) {
	tmpDir := t.TempDir()
	packsDir := filepath.Join(tmpDir, ".go-arch", "packs")
	t.Setenv("HOME", tmpDir)
	t.Setenv("GO_ARCH_PACKS_DIR", packsDir)

	// Create a synthetic installed pack.
	packDir := filepath.Join(packsDir, "echo@0.5.0")
	if err := os.MkdirAll(packDir, 0755); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	old := ui.Out
	ui.Out = &buf
	defer func() { ui.Out = old }()

	err := templateListCmd.RunE(templateListCmd, nil)
	if err != nil {
		t.Fatalf("template list failed: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "echo@0.5.0") {
		t.Errorf("output = %q; want it to contain 'echo@0.5.0'", output)
	}
}
