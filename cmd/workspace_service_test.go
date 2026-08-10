package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

// setupServiceGenerateEnv prepares a workspace with a minimal web service so
// generate can create a component inside it.
func setupServiceGenerateEnv(t *testing.T) (root, wsPath, svcDir string) {
	t.Helper()
	root = t.TempDir()
	svcDir = filepath.Join(root, "services", "orders")
	if err := os.MkdirAll(svcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := `project_name: orders
module_name: example.com/orders
architecture: Hexagonal
use_templ_htmx: true
`
	if err := os.WriteFile(filepath.Join(svcDir, ".go-arch.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := `files: {}
routes: []
`
	if err := os.MkdirAll(filepath.Join(svcDir, ".go-arch"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(svcDir, ".go-arch", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	wsPath = filepath.Join(root, "go-arch.workspace.yaml")
	ws := `services:
  - name: orders
    path: services/orders
`
	if err := os.WriteFile(wsPath, []byte(ws), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, wsPath, svcDir
}

func TestGenerate_ServiceFlag_LandsInService(t *testing.T) {
	root, wsPath, svcDir := setupServiceGenerateEnv(t)

	oldWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(oldWd) }()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	viper.Reset()
	defer viper.Reset()

	var buf strings.Builder
	generateCmd.SetOut(&buf)
	generateCmd.SetErr(&buf)
	_ = generateCmd.Flags().Set("service", "orders")
	_ = generateCmd.Flags().Set("workspace", wsPath)

	err := generateCmd.RunE(generateCmd, []string{"service", "Order"})
	if err != nil {
		t.Fatalf("generate --service: %v\n%s", err, buf.String())
	}

	// Hexagonal service → internal/domain/<Name>_service.go inside the service dir.
	generated := filepath.Join(svcDir, "internal", "domain", "Order_service.go")
	if _, err := os.Stat(generated); err != nil {
		t.Errorf("expected generated file at %s (err: %v)", generated, err)
	}

	// CWD must be restored to the monorepo root.
	cwd, _ := os.Getwd()
	if cwd != root {
		t.Errorf("CWD = %q, want %q (restore failed)", cwd, root)
	}
}

func TestGenerate_ServiceFlag_UnknownService(t *testing.T) {
	root, wsPath, _ := setupServiceGenerateEnv(t)

	oldWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(oldWd) }()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	viper.Reset()
	defer viper.Reset()

	_ = generateCmd.Flags().Set("service", "billing")
	_ = generateCmd.Flags().Set("workspace", wsPath)

	err := generateCmd.RunE(generateCmd, []string{"service", "Order"})
	if err == nil {
		t.Fatal("generate --service billing: expected service_not_found")
	}
	if !strings.Contains(err.Error(), "billing") {
		t.Errorf("error should name the service: %v", err)
	}
}

func TestGenerate_ServiceFlag_NoWorkspace(t *testing.T) {
	dir := t.TempDir()
	oldWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(oldWd) }()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	viper.Reset()
	defer viper.Reset()

	_ = generateCmd.Flags().Set("service", "orders")
	_ = generateCmd.Flags().Set("workspace", "")

	err := generateCmd.RunE(generateCmd, []string{"service", "Order"})
	if err == nil {
		t.Fatal("generate --service without workspace: expected error")
	}
	if !strings.Contains(err.Error(), "workspace context") {
		t.Errorf("error should explain the flag needs workspace context: %v", err)
	}
}

func TestWorkspaceCheck_AllServices(t *testing.T) {
	root, wsPath := makeTestWorkspace(t)

	oldWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(oldWd) }()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	viper.Reset()
	defer viper.Reset()

	var buf strings.Builder
	workspaceCheckCmd.SetOut(&buf)
	workspaceCheckCmd.SetErr(&buf)
	_ = workspaceCheckCmd.Flags().Set("workspace", wsPath)

	err := workspaceCheckCmd.RunE(workspaceCheckCmd, nil)
	// The services are empty fixtures → architecture violations are expected.
	// What matters: both services were processed and a summary was printed.
	if err == nil {
		t.Error("workspace check: expected non-zero for violating services")
	}
	out := buf.String()
	if !strings.Contains(out, "Workspace check summary:") {
		t.Errorf("missing summary header:\n%s", out)
	}
	if !strings.Contains(out, "orders") || !strings.Contains(out, "users") {
		t.Errorf("summary missing services:\n%s", out)
	}
}
