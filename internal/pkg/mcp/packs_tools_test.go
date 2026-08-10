package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go-arch/internal/pkg/packs"
)

// makeFakePackDir creates a minimal valid pack module tree on disk:
// go-arch.yaml manifest + templates/ + optional hooks.
func makeFakePackDir(t *testing.T, withHooks bool) string {
	t.Helper()
	dir := t.TempDir()

	manifest := `contract_version: 1
name: express
version: 1.0.0
`
	if withHooks {
		manifest += `hooks:
  post-generate:
    - command: "echo"
      args: ["hi"]
`
	}
	if err := os.WriteFile(filepath.Join(dir, "go-arch.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "templates", "common"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "templates", "common", "readme.tmpl"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestInstallTemplateHandler verifies install_template dispatches through the
// real handler with an injectable downloader (FakeDownloader, no network).
func TestInstallTemplateHandler(t *testing.T) {
	t.Run("installs with hooks disabled by default", func(t *testing.T) {
		old := os.Getenv("GO_ARCH_PACKS_DIR")
		defer os.Setenv("GO_ARCH_PACKS_DIR", old)
		os.Setenv("GO_ARCH_PACKS_DIR", t.TempDir())

		out := captureStdout(func() {
			handleInstallTemplate(1, "github.com/user/go-arch-express", false,
				&packs.FakeDownloader{Dir: makeFakePackDir(t, true)})
		})

		if !strings.Contains(out, "installed with hooks disabled") {
			t.Errorf("expected hooks-disabled message, got:\n%s", out)
		}
		if strings.Contains(out, `"error"`) {
			t.Errorf("unexpected error response:\n%s", out)
		}

		// Verify the pack landed on disk.
		listed, err := packs.List()
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, p := range listed {
			if p.Name == "express" && p.Version == "1.0.0" {
				found = true
			}
		}
		if !found {
			t.Errorf("installed pack express@1.0.0 not found in %q: %+v", os.Getenv("GO_ARCH_PACKS_DIR"), listed)
		}
	})

	t.Run("allows hooks when allowHooks true", func(t *testing.T) {
		old := os.Getenv("GO_ARCH_PACKS_DIR")
		defer os.Setenv("GO_ARCH_PACKS_DIR", old)
		os.Setenv("GO_ARCH_PACKS_DIR", t.TempDir())

		out := captureStdout(func() {
			handleInstallTemplate(1, "github.com/user/go-arch-express@v1.0.0", true,
				&packs.FakeDownloader{Dir: makeFakePackDir(t, true)})
		})

		if !strings.Contains(out, "installed with hooks enabled") {
			t.Errorf("expected hooks-enabled message, got:\n%s", out)
		}
	})

	t.Run("invalid ref errors", func(t *testing.T) {
		out := captureStdout(func() {
			handleInstallTemplate(1, "@v1.0.0", false, &packs.FakeDownloader{})
		})
		if !strings.Contains(out, "Invalid pack reference") {
			t.Errorf("expected invalid ref error, got:\n%s", out)
		}
	})

	t.Run("missing module errors", func(t *testing.T) {
		out := captureStdout(func() {
			handleInstallTemplate(1, "", false, &packs.FakeDownloader{})
		})
		if !strings.Contains(out, "module is required") {
			t.Errorf("expected missing-module error, got:\n%s", out)
		}
	})

	t.Run("download failure surfaces", func(t *testing.T) {
		old := os.Getenv("GO_ARCH_PACKS_DIR")
		defer os.Setenv("GO_ARCH_PACKS_DIR", old)
		os.Setenv("GO_ARCH_PACKS_DIR", t.TempDir())

		out := captureStdout(func() {
			handleInstallTemplate(1, "github.com/user/nonexistent", false,
				&packs.FakeDownloader{Err: os.ErrNotExist})
		})
		if !strings.Contains(out, "Pack install failed") {
			t.Errorf("expected install failure, got:\n%s", out)
		}
	})
}

// TestListPacksHandler verifies list_packs returns installed packs as JSON and
// an empty list when none are installed.
func TestListPacksHandler(t *testing.T) {
	t.Run("returns installed packs", func(t *testing.T) {
		old := os.Getenv("GO_ARCH_PACKS_DIR")
		defer os.Setenv("GO_ARCH_PACKS_DIR", old)
		os.Setenv("GO_ARCH_PACKS_DIR", t.TempDir())

		// Install a pack first via the real install path (fake downloader).
		handleInstallTemplate(1, "github.com/user/go-arch-express", false,
			&packs.FakeDownloader{Dir: makeFakePackDir(t, false)})

		out := captureStdout(func() {
			handleToolCall(1, "list_packs", json.RawMessage(`{}`))
		})

		// The captured output is JSON-RPC-serialized, so inner quotes are
		// escaped (\"name\": \"express\"). Assert on the escaped form.
		if !strings.Contains(out, `\"name\": \"express\"`) {
			t.Errorf("expected express in list_packs output, got:\n%s", out)
		}
		if !strings.Contains(out, `\"version\": \"1.0.0\"`) {
			t.Errorf("expected 1.0.0 in list_packs output, got:\n%s", out)
		}
	})

	t.Run("returns empty array when none installed", func(t *testing.T) {
		old := os.Getenv("GO_ARCH_PACKS_DIR")
		defer os.Setenv("GO_ARCH_PACKS_DIR", old)
		os.Setenv("GO_ARCH_PACKS_DIR", t.TempDir()) // empty dir

		out := captureStdout(func() {
			handleToolCall(1, "list_packs", json.RawMessage(`{}`))
		})

		if !strings.Contains(out, "[]") {
			t.Errorf("expected empty JSON array, got:\n%s", out)
		}
	})
}

// TestRemovePackHandler verifies remove_pack removes an installed pack and
// resolves latest version for bare names.
func TestRemovePackHandler(t *testing.T) {
	t.Run("removes installed pack", func(t *testing.T) {
		old := os.Getenv("GO_ARCH_PACKS_DIR")
		defer os.Setenv("GO_ARCH_PACKS_DIR", old)
		os.Setenv("GO_ARCH_PACKS_DIR", t.TempDir())

		handleInstallTemplate(1, "github.com/user/go-arch-express", false,
			&packs.FakeDownloader{Dir: makeFakePackDir(t, false)})

		out := captureStdout(func() {
			handleRemovePack(1, "express")
		})
		if !strings.Contains(out, "removed") {
			t.Errorf("expected removal message, got:\n%s", out)
		}

		listed, err := packs.List()
		if err != nil {
			t.Fatal(err)
		}
		if len(listed) != 0 {
			t.Errorf("expected empty pack list after remove, got %+v", listed)
		}
	})

	t.Run("not installed errors", func(t *testing.T) {
		old := os.Getenv("GO_ARCH_PACKS_DIR")
		defer os.Setenv("GO_ARCH_PACKS_DIR", old)
		os.Setenv("GO_ARCH_PACKS_DIR", t.TempDir())

		out := captureStdout(func() {
			handleRemovePack(1, "ghost")
		})
		if !strings.Contains(out, "not installed") && !strings.Contains(out, "Failed") {
			t.Errorf("expected failure message, got:\n%s", out)
		}
	})
}

// TestUpdatePackHandler verifies update_pack re-fetches and reinstalls.
func TestUpdatePackHandler(t *testing.T) {
	t.Run("updates installed pack", func(t *testing.T) {
		old := os.Getenv("GO_ARCH_PACKS_DIR")
		defer os.Setenv("GO_ARCH_PACKS_DIR", old)
		os.Setenv("GO_ARCH_PACKS_DIR", t.TempDir())

		// Install v1.0.0 first (writes sidecar with module ref).
		handleInstallTemplate(1, "github.com/user/go-arch-express@v1.0.0", false,
			&packs.FakeDownloader{Dir: makeFakePackDir(t, false)})

		// Update: fake downloader returns a v2.0.0 pack dir.
		v2 := makeFakePackDir(t, false)
		// The update path reads the module ref from the sidecar and re-fetches
		// @latest; the fake returns the v2 dir.
		out := captureStdout(func() {
			handleUpdatePack(1, "express", false, &packs.FakeDownloader{Dir: v2})
		})
		if !strings.Contains(out, "updated") {
			t.Errorf("expected update message, got:\n%s", out)
		}
	})

	t.Run("not installed errors", func(t *testing.T) {
		old := os.Getenv("GO_ARCH_PACKS_DIR")
		defer os.Setenv("GO_ARCH_PACKS_DIR", old)
		os.Setenv("GO_ARCH_PACKS_DIR", t.TempDir())

		out := captureStdout(func() {
			handleUpdatePack(1, "ghost", false, &packs.FakeDownloader{})
		})
		if !strings.Contains(out, "Failed to update pack") {
			t.Errorf("expected failure message, got:\n%s", out)
		}
	})
}
