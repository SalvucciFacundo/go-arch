package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/samber/oops"
)

func writeWorkspaceFile(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "go-arch.workspace.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write workspace file: %v", err)
	}
	return path
}

func TestLoad_ValidWorkspace(t *testing.T) {
	dir := t.TempDir()
	// Create service dirs so path validation sees them at command time (load does not check existence).
	path := writeWorkspaceFile(t, dir, `services:
  - name: orders
    path: services/orders
  - name: users
    path: services/users
    template: express
`)

	ws, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if ws.Dir != dir {
		t.Errorf("Dir = %q, want %q", ws.Dir, dir)
	}
	if len(ws.Services) != 2 {
		t.Fatalf("len(Services) = %d, want 2", len(ws.Services))
	}
	if ws.Services[0].Name != "orders" || ws.Services[0].Path != "services/orders" {
		t.Errorf("Services[0] = %+v", ws.Services[0])
	}
	if ws.Services[1].Template != "express" {
		t.Errorf("Services[1].Template = %q, want express", ws.Services[1].Template)
	}
	svc, ok := ws.Find("users")
	if !ok || svc.Name != "users" {
		t.Errorf("Find(users) = %+v, %v; want found", svc, ok)
	}
	if _, ok := ws.Find("billing"); ok {
		t.Error("Find(billing) found, want not found")
	}
}

func TestLoad_DuplicateServiceName(t *testing.T) {
	dir := t.TempDir()
	path := writeWorkspaceFile(t, dir, `services:
  - name: orders
    path: services/orders
  - name: orders
    path: services/orders2
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load: expected error for duplicate service name")
	}
	if code := oopsCode(err); code != "service_duplicate" {
		t.Errorf("code = %q, want service_duplicate (err: %v)", code, err)
	}
}

func TestLoad_UnknownKeyRejected(t *testing.T) {
	dir := t.TempDir()
	path := writeWorkspaceFile(t, dir, `frobnicate: true
services:
  - name: orders
    path: services/orders
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load: expected error for unknown top-level key")
	}
	if code := oopsCode(err); code != "workspace_invalid" {
		t.Errorf("code = %q, want workspace_invalid (err: %v)", code, err)
	}
}

func TestLoad_UnknownServiceKeyRejected(t *testing.T) {
	dir := t.TempDir()
	path := writeWorkspaceFile(t, dir, `services:
  - name: orders
    path: services/orders
    frobnicate: true
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load: expected error for unknown service key")
	}
	if code := oopsCode(err); code != "workspace_invalid" {
		t.Errorf("code = %q, want workspace_invalid (err: %v)", code, err)
	}
}

func TestLoad_MissingName(t *testing.T) {
	dir := t.TempDir()
	path := writeWorkspaceFile(t, dir, `services:
  - path: services/orders
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load: expected error for missing name")
	}
	if code := oopsCode(err); code != "workspace_invalid" {
		t.Errorf("code = %q, want workspace_invalid (err: %v)", code, err)
	}
}

func TestLoad_MissingPath(t *testing.T) {
	dir := t.TempDir()
	path := writeWorkspaceFile(t, dir, `services:
  - name: orders
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load: expected error for missing path")
	}
	if code := oopsCode(err); code != "workspace_invalid" {
		t.Errorf("code = %q, want workspace_invalid (err: %v)", code, err)
	}
}

func TestLoad_BadSlug(t *testing.T) {
	dir := t.TempDir()
	path := writeWorkspaceFile(t, dir, `services:
  - name: "Bad Name!"
    path: services/orders
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load: expected error for bad slug")
	}
	if code := oopsCode(err); code != "workspace_invalid" {
		t.Errorf("code = %q, want workspace_invalid (err: %v)", code, err)
	}
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "nope.yaml"))
	if err == nil {
		t.Fatal("Load: expected error for missing file")
	}
	if code := oopsCode(err); code != "workspace_not_found" {
		t.Errorf("code = %q, want workspace_not_found (err: %v)", code, err)
	}
}

func TestLoad_NoServices(t *testing.T) {
	dir := t.TempDir()
	path := writeWorkspaceFile(t, dir, `services: []`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load: expected error for empty services list")
	}
	if code := oopsCode(err); code != "workspace_invalid" {
		t.Errorf("code = %q, want workspace_invalid (err: %v)", code, err)
	}
}

// oopsCode extracts the oops code from an error (oops does not render it in Error()).
func oopsCode(err error) string {
	var oe oops.OopsError
	if !errors.As(err, &oe) {
		return ""
	}
	if c, ok := oe.Code().(string); ok {
		return c
	}
	return ""
}
