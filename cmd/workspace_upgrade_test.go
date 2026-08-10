package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

// makeTestService creates a minimal go-arch service (config + manifest) in dir.
func makeTestService(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := `project_name: ` + name + `
module_name: example.com/` + name + `
architecture: hexagonal
`
	if err := os.WriteFile(filepath.Join(dir, ".go-arch.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := `files: {}
`
	if err := os.WriteFile(filepath.Join(dir, ".go-arch-manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
}

// makeTestWorkspace creates a monorepo with two services and a workspace file.
func makeTestWorkspace(t *testing.T) (root, wsPath string) {
	t.Helper()
	root = t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "services", "orders"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "services", "users"), 0o755); err != nil {
		t.Fatal(err)
	}
	makeTestService(t, filepath.Join(root, "services", "orders"), "orders")
	makeTestService(t, filepath.Join(root, "services", "users"), "users")

	wsPath = filepath.Join(root, "go-arch.workspace.yaml")
	ws := `services:
  - name: orders
    path: services/orders
  - name: users
    path: services/users
`
	if err := os.WriteFile(wsPath, []byte(ws), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, wsPath
}

func runWorkspaceUpgrade(t *testing.T, root, wsFlag string, yes bool) (string, error) {
	t.Helper()
	oldWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(oldWd) }()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	viper.Reset()
	defer viper.Reset()

	var buf strings.Builder
	workspaceUpgradeCmd.SetOut(&buf)
	workspaceUpgradeCmd.SetErr(&buf)
	if yes {
		_ = workspaceUpgradeCmd.Flags().Set("yes", "true")
	}
	if wsFlag != "" {
		_ = workspaceUpgradeCmd.Flags().Set("workspace", wsFlag)
	}
	err := workspaceUpgradeCmd.RunE(workspaceUpgradeCmd, nil)
	return buf.String(), err
}

func TestWorkspaceUpgrade_DryRunPlansBoth(t *testing.T) {
	root, wsPath := makeTestWorkspace(t)
	out, err := runWorkspaceUpgrade(t, root, wsPath, false)
	if err != nil {
		t.Fatalf("workspace upgrade dry-run: %v", err)
	}
	if !strings.Contains(out, "orders") || !strings.Contains(out, "users") {
		t.Errorf("summary missing services:\n%s", out)
	}
	if !strings.Contains(out, "Workspace upgrade summary:") {
		t.Errorf("missing summary header:\n%s", out)
	}
}

func TestWorkspaceUpgrade_MissingWorkspace(t *testing.T) {
	dir := t.TempDir()
	oldWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(oldWd) }()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	viper.Reset()
	defer viper.Reset()

	var buf strings.Builder
	workspaceUpgradeCmd.SetOut(&buf)
	err := workspaceUpgradeCmd.RunE(workspaceUpgradeCmd, nil)
	if err == nil {
		t.Fatal("workspace upgrade: expected workspace_not_found")
	}
	if !strings.Contains(err.Error(), "workspace") {
		t.Errorf("error should mention workspace: %v", err)
	}
}

func TestWorkspaceUpgrade_MissingServicePath(t *testing.T) {
	root := t.TempDir()
	wsPath := filepath.Join(root, "go-arch.workspace.yaml")
	ws := `services:
  - name: ghost
    path: services/ghost
`
	if err := os.WriteFile(wsPath, []byte(ws), 0o644); err != nil {
		t.Fatal(err)
	}
	oldWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(oldWd) }()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	viper.Reset()
	defer viper.Reset()

	var buf strings.Builder
	workspaceUpgradeCmd.SetOut(&buf)
	err := workspaceUpgradeCmd.RunE(workspaceUpgradeCmd, nil)
	if err == nil {
		t.Fatal("workspace upgrade: expected failure for missing service path")
	}
	if !strings.Contains(buf.String(), "ghost") {
		t.Errorf("summary should name the failed service:\n%s", buf.String())
	}
}
