package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// makeMCWorkspace creates a temp monorepo with two services and a workspace
// file, returns root + workspace path. Each service has config + manifest.
func makeMCWorkspace(t *testing.T) (root, wsPath string) {
	t.Helper()
	root = t.TempDir()
	for _, name := range []string{"orders", "users"} {
		dir := filepath.Join(root, "services", name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		cfg := "project_name: " + name + "\nmodule_name: example.com/" + name + "\narchitecture: Hexagonal\n"
		if err := os.WriteFile(filepath.Join(dir, ".go-arch.yaml"), []byte(cfg), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(dir, ".go-arch"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, ".go-arch", "manifest.yaml"), []byte("files: {}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
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

// callTool runs handleToolCall and returns the captured stdout.
func callTool(t *testing.T, name string, args interface{}) string {
	t.Helper()
	raw, _ := json.Marshal(args)
	return captureStdout(func() {
		handleToolCall(1, name, raw)
	})
}

func TestMCWorkspaceList(t *testing.T) {
	t.Run("lists services", func(t *testing.T) {
		_, wsPath := makeMCWorkspace(t)
		out := callTool(t, "workspace_list", map[string]string{"workspacePath": wsPath})

		if !strings.Contains(out, `\"name\": \"orders\"`) || !strings.Contains(out, `\"name\": \"users\"`) {
			t.Errorf("expected both services in output, got:\n%s", out)
		}
	})

	t.Run("workspace not found errors", func(t *testing.T) {
		out := callTool(t, "workspace_list", map[string]string{"workspacePath": filepath.Join(t.TempDir(), "nope.yaml")})
		if !strings.Contains(out, "workspace_not_found") {
			t.Errorf("expected workspace_not_found, got:\n%s", out)
		}
	})
}

func TestMCWorkspaceUpgrade(t *testing.T) {
	t.Run("batch dry-run", func(t *testing.T) {
		root, wsPath := makeMCWorkspace(t)
		oldWd, _ := os.Getwd()
		defer func() { _ = os.Chdir(oldWd) }()
		if err := os.Chdir(root); err != nil {
			t.Fatal(err)
		}

		out := callTool(t, "workspace_upgrade", map[string]string{"workspacePath": wsPath})

		if !strings.Contains(out, `\"status\": \"ok\"`) {
			t.Errorf("expected ok status, got:\n%s", out)
		}
		if !strings.Contains(out, "orders") || !strings.Contains(out, "users") {
			t.Errorf("expected both services, got:\n%s", out)
		}
		// Dry-run: nothing written.
		if _, err := os.Stat(filepath.Join(root, "services", "orders", ".go-arch", "manifest.yaml")); err != nil {
			t.Errorf("manifest should still exist: %v", err)
		}
	})

	t.Run("single service", func(t *testing.T) {
		_, wsPath := makeMCWorkspace(t)
		out := callTool(t, "workspace_upgrade", map[string]string{
			"workspacePath": wsPath,
			"service":       "orders",
		})
		if strings.Contains(out, "users") {
			t.Errorf("single-service upgrade should not include users:\n%s", out)
		}
	})

	t.Run("service not found", func(t *testing.T) {
		_, wsPath := makeMCWorkspace(t)
		out := callTool(t, "workspace_upgrade", map[string]string{
			"workspacePath": wsPath,
			"service":       "ghost",
		})
		if !strings.Contains(out, "service_not_found") {
			t.Errorf("expected service_not_found, got:\n%s", out)
		}
	})

	t.Run("legacy service skipped", func(t *testing.T) {
		root := t.TempDir()
		// One service WITH manifest, one WITHOUT.
		dir := filepath.Join(root, "services", "orders")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, ".go-arch.yaml"), []byte("project_name: orders\narchitecture: Hexagonal\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		legacyDir := filepath.Join(root, "services", "legacy")
		if err := os.MkdirAll(legacyDir, 0o755); err != nil {
			t.Fatal(err)
		}
		// No .go-arch.yaml → legacy.
		wsPath := filepath.Join(root, "go-arch.workspace.yaml")
		ws := `services:
  - name: orders
    path: services/orders
  - name: legacy
    path: services/legacy
`
		if err := os.WriteFile(wsPath, []byte(ws), 0o644); err != nil {
			t.Fatal(err)
		}

		out := callTool(t, "workspace_upgrade", map[string]string{"workspacePath": wsPath})
		if !strings.Contains(out, "skipped") || !strings.Contains(out, "service_no_manifest") {
			t.Errorf("expected legacy service skipped with service_no_manifest, got:\n%s", out)
		}
	})
}

func TestMCWorkspaceCheck(t *testing.T) {
	t.Run("checks all services", func(t *testing.T) {
		root, wsPath := makeMCWorkspace(t)
		oldWd, _ := os.Getwd()
		defer func() { _ = os.Chdir(oldWd) }()
		if err := os.Chdir(root); err != nil {
			t.Fatal(err)
		}
		out := callTool(t, "workspace_check", map[string]string{"workspacePath": wsPath})
		// Empty fixtures → architecture violations expected; status should be
		// ok or partial, but the tool must process both services.
		if !strings.Contains(out, "orders") || !strings.Contains(out, "users") {
			t.Errorf("expected both services in check output, got:\n%s", out)
		}
	})

	t.Run("single service", func(t *testing.T) {
		_, wsPath := makeMCWorkspace(t)
		out := callTool(t, "workspace_check", map[string]string{
			"workspacePath": wsPath,
			"service":       "orders",
		})
		if strings.Contains(out, "users") {
			t.Errorf("single-service check should not include users:\n%s", out)
		}
	})
}

func TestMCUpgradeProjectWithService(t *testing.T) {
	t.Run("upgrades service via workspace", func(t *testing.T) {
		root, wsPath := makeMCWorkspace(t)
		oldWd, _ := os.Getwd()
		defer func() { _ = os.Chdir(oldWd) }()
		if err := os.Chdir(root); err != nil {
			t.Fatal(err)
		}

		out := callTool(t, "upgrade_project", map[string]interface{}{
			"workspacePath": wsPath,
			"service":       "orders",
		})

		if !strings.Contains(out, "orders") {
			t.Errorf("expected orders in output, got:\n%s", out)
		}
	})

	t.Run("no params backward compatible", func(t *testing.T) {
		// Without service, upgrade_project must behave as before (projectPath
		// based). A nonexistent projectPath → error, not workspace error.
		out := callTool(t, "upgrade_project", map[string]string{"projectPath": filepath.Join(t.TempDir(), "nope")})
		if strings.Contains(out, "workspace_not_found") {
			t.Errorf("no-service upgrade_project should not use workspace resolution:\n%s", out)
		}
	})
}
