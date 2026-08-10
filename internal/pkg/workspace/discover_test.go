package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscover_UpwardWalk(t *testing.T) {
	root := t.TempDir()
	svc := filepath.Join(root, "services", "orders")
	if err := os.MkdirAll(svc, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, workspaceFileName), []byte("services: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Discover(svc)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	want := filepath.Join(root, workspaceFileName)
	if got != want {
		t.Errorf("Discover = %q, want %q", got, want)
	}
}

func TestDiscover_NoneFound(t *testing.T) {
	dir := t.TempDir()
	_, err := Discover(dir)
	if err == nil {
		t.Fatal("Discover: expected workspace_not_found")
	}
	if code := oopsCode(err); code != "workspace_not_found" {
		t.Errorf("code = %q, want workspace_not_found", code)
	}
}

func TestDiscover_ExactCwd(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, workspaceFileName), []byte("services: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if got != filepath.Join(root, workspaceFileName) {
		t.Errorf("Discover = %q, want %q", got, filepath.Join(root, workspaceFileName))
	}
}
