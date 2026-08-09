package packs

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/samber/oops"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// fakePackDir creates a minimal valid pack directory structure under dir/name
// and returns the full path to the pack root.
func fakePackDir(t *testing.T, parent, name, version string) string {
	t.Helper()
	root := filepath.Join(parent, name)
	if err := os.MkdirAll(filepath.Join(root, "templates", "common"), 0755); err != nil {
		t.Fatal(err)
	}
	yaml := `contract_version: 1
name: ` + name + `
version: ` + version + `
`
	if err := os.WriteFile(filepath.Join(root, "go-arch.yaml"), []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	// Add a sample template file.
	if err := os.WriteFile(filepath.Join(root, "templates", "common", "handler.tmpl"), []byte("package {{.ModuleName}}"), 0644); err != nil {
		t.Fatal(err)
	}
	return root
}

// fakePackDirWithHooks creates a pack dir with hooks declared.
func fakePackDirWithHooks(t *testing.T, parent, name, version string) string {
	t.Helper()
	root := filepath.Join(parent, name)
	if err := os.MkdirAll(filepath.Join(root, "templates"), 0755); err != nil {
		t.Fatal(err)
	}
	yaml := `contract_version: 1
name: ` + name + `
version: ` + version + `
hooks:
  post-new:
    - command: go
      args:
        - mod
        - tidy
`
	if err := os.WriteFile(filepath.Join(root, "go-arch.yaml"), []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	return root
}

// fakePackDirNoTemplates creates a pack dir without a templates/ directory.
func fakePackDirNoTemplates(t *testing.T, parent, name, version string) string {
	t.Helper()
	root := filepath.Join(parent, name)
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	yaml := `contract_version: 1
name: ` + name + `
version: ` + version + `
`
	if err := os.WriteFile(filepath.Join(root, "go-arch.yaml"), []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	return root
}

// readSidecarFile reads and parses pack.json from the given pack install dir.
func readSidecarFile(t *testing.T, dir string) Sidecar {
	t.Helper()
	path := filepath.Join(dir, "pack.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading pack.json at %s: %v", path, err)
	}
	var s Sidecar
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatalf("unmarshal pack.json: %v", err)
	}
	return s
}

// assertNoDir asserts that the directory does not exist.
func assertNoDir(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err == nil {
		t.Errorf("directory should not exist but found: %s (%v)", path, info.Name())
	}
	if !os.IsNotExist(err) {
		// Some other error (e.g. permission) — still not the expected state.
		t.Errorf("unexpected error checking dir %s: %v", path, err)
	}
}

// assertCode checks that err wraps an oops error with the given code.
func assertCode(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected an error with code %q, got nil", want)
	}
	var oe oops.OopsError
	if !errors.As(err, &oe) {
		t.Fatalf("expected oops.OopsError, got %T: %v", err, err)
	}
	got, ok := oe.Code().(string)
	if !ok {
		t.Fatalf("oops code is not a string: %T", oe.Code())
	}
	if got != want {
		t.Errorf("code: got %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// 2.1 RED — Install tests (FakeDownloader)
// ---------------------------------------------------------------------------

func TestInstall_Success(t *testing.T) {
	srcRoot := t.TempDir()
	packRoot := fakePackDir(t, srcRoot, "express", "1.0.0")

	baseDir := t.TempDir()
	t.Setenv(EnvPacksDir, baseDir)

	dl := &FakeDownloader{Dir: packRoot}

	confirm := func(name string) (bool, error) {
		return false, nil // no hooks, answer doesn't matter
	}

	_, err := Install(context.Background(), dl, "github.com/org/express", "1.0.0", confirm)
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	// Verify destination exists.
	dst := Path("express", "1.0.0")
	if _, err := os.Stat(dst); err != nil {
		t.Fatalf("installed pack dir does not exist: %v", err)
	}

	// Verify manifest is present and valid.
	m, err := Load(dst)
	if err != nil {
		t.Fatalf("re-validating manifest at dst: %v", err)
	}
	if m.Name != "express" {
		t.Errorf("manifest name = %q, want %q", m.Name, "express")
	}

	// Verify templates/ was copied.
	tmplDir := filepath.Join(dst, "templates")
	info, err := os.Stat(tmplDir)
	if err != nil || !info.IsDir() {
		t.Fatalf("templates/ not found or not a directory at %s", tmplDir)
	}

	// Verify template file was copied.
	handlerPath := filepath.Join(tmplDir, "common", "handler.tmpl")
	if _, err := os.Stat(handlerPath); err != nil {
		t.Errorf("handler.tmpl not copied: %v", err)
	}

	// Verify sidecar written.
	sidecar := readSidecarFile(t, dst)
	if sidecar.ModuleRef != "github.com/org/express@1.0.0" {
		t.Errorf("sidecar ModuleRef = %q, want %q", sidecar.ModuleRef, "github.com/org/express@1.0.0")
	}
	if sidecar.InstalledAt.IsZero() {
		t.Error("sidecar InstalledAt should not be zero")
	}
}

func TestInstall_ModuleNotFound(t *testing.T) {
	baseDir := t.TempDir()
	t.Setenv(EnvPacksDir, baseDir)

	notFoundErr := oops.Code(CodePackNotFound).Errorf("pack %q not found in module proxy", "no-such-pack@1.0.0")
	dl := &FakeDownloader{Err: notFoundErr}

	confirm := func(name string) (bool, error) {
		t.Error("confirm should not be called when download fails")
		return false, nil
	}

	_, err := Install(context.Background(), dl, "github.com/org/no-such-pack", "1.0.0", confirm)
	if err == nil {
		t.Fatal("expected error for module-not-found, got nil")
	}
	assertCode(t, err, CodePackNotFound)
	if !strings.Contains(err.Error(), "no-such-pack") {
		t.Errorf("error should mention the pack, got: %v", err)
	}

	// Verify no partial state.
	entries, _ := os.ReadDir(baseDir)
	if len(entries) > 0 {
		t.Errorf("packs dir should be empty but has entries: %v", entries)
	}
}

func TestInstall_FetchFailed(t *testing.T) {
	baseDir := t.TempDir()
	t.Setenv(EnvPacksDir, baseDir)

	fetchErr := oops.Code(CodePackFetchFailed).Errorf("go mod download: network unreachable")
	dl := &FakeDownloader{Err: fetchErr}

	confirm := func(name string) (bool, error) {
		t.Error("confirm should not be called when download fails")
		return false, nil
	}

	_, err := Install(context.Background(), dl, "github.com/org/express", "1.0.0", confirm)
	if err == nil {
		t.Fatal("expected error for fetch failure, got nil")
	}
	assertCode(t, err, CodePackFetchFailed)

	// Verify no directory created.
	entries, _ := os.ReadDir(baseDir)
	if len(entries) > 0 {
		t.Errorf("packs dir should be empty but has entries: %v", entries)
	}
}

func TestInstall_CorruptedZiphashAbort(t *testing.T) {
	// go mod download returns a non-zero exit code (download failure).
	baseDir := t.TempDir()
	t.Setenv(EnvPacksDir, baseDir)

	dlErr := oops.Code(CodePackInstallFailed).Wrapf(
		errors.New("ziphash mismatch"), "module corrupt")
	dl := &FakeDownloader{Err: dlErr}

	confirm := func(name string) (bool, error) {
		t.Error("confirm should not be called when download fails")
		return false, nil
	}

	_, err := Install(context.Background(), dl, "github.com/org/express", "1.0.0", confirm)
	if err == nil {
		t.Fatal("expected error for corrupted download, got nil")
	}

	// Verify no directory created.
	assertNoDir(t, Path("express", "1.0.0"))
}

func TestInstall_NoTemplates(t *testing.T) {
	srcRoot := t.TempDir()
	packRoot := fakePackDirNoTemplates(t, srcRoot, "express", "1.0.0")

	baseDir := t.TempDir()
	t.Setenv(EnvPacksDir, baseDir)

	dl := &FakeDownloader{Dir: packRoot}

	confirm := func(name string) (bool, error) {
		t.Error("confirm should not be called before templates check")
		return false, nil
	}

	_, err := Install(context.Background(), dl, "github.com/org/express", "1.0.0", confirm)
	if err == nil {
		t.Fatal("expected error for missing templates/, got nil")
	}
	assertCode(t, err, CodePackNoTemplates)
	if !strings.Contains(err.Error(), "templates") {
		t.Errorf("error should mention 'templates', got: %v", err)
	}

	// Verify no directory created.
	assertNoDir(t, Path("express", "1.0.0"))
}

func TestInstall_HooksAccept(t *testing.T) {
	srcRoot := t.TempDir()
	packRoot := fakePackDirWithHooks(t, srcRoot, "express", "1.0.0")

	baseDir := t.TempDir()
	t.Setenv(EnvPacksDir, baseDir)

	dl := &FakeDownloader{Dir: packRoot}

	confirm := func(name string) (bool, error) {
		if name != "express" {
			t.Errorf("confirm called with name %q, want %q", name, "express")
		}
		return true, nil
	}

	_, err := Install(context.Background(), dl, "github.com/org/express", "1.0.0", confirm)
	if err != nil {
		t.Fatalf("Install with hooks accept failed: %v", err)
	}

	dst := Path("express", "1.0.0")
	sidecar := readSidecarFile(t, dst)
	if !sidecar.HooksEnabled {
		t.Error("sidecar HooksEnabled should be true when user accepts hooks")
	}
}

func TestInstall_HooksDecline(t *testing.T) {
	srcRoot := t.TempDir()
	packRoot := fakePackDirWithHooks(t, srcRoot, "express", "1.0.0")

	baseDir := t.TempDir()
	t.Setenv(EnvPacksDir, baseDir)

	dl := &FakeDownloader{Dir: packRoot}

	confirm := func(name string) (bool, error) {
		if name != "express" {
			t.Errorf("confirm called with name %q, want %q", name, "express")
		}
		return false, nil
	}

	_, err := Install(context.Background(), dl, "github.com/org/express", "1.0.0", confirm)
	if err != nil {
		t.Fatalf("Install with hooks decline failed: %v", err)
	}

	dst := Path("express", "1.0.0")
	sidecar := readSidecarFile(t, dst)
	if sidecar.HooksEnabled {
		t.Error("sidecar HooksEnabled should be false when user declines hooks")
	}
}

func TestInstall_NoHooks(t *testing.T) {
	// Pack without hooks — confirm is never called, HooksEnabled=false in sidecar.
	srcRoot := t.TempDir()
	packRoot := fakePackDir(t, srcRoot, "express", "1.0.0")

	baseDir := t.TempDir()
	t.Setenv(EnvPacksDir, baseDir)

	dl := &FakeDownloader{Dir: packRoot}

	confirm := func(name string) (bool, error) {
		t.Error("confirm should not be called when pack has no hooks")
		return false, nil
	}

	_, err := Install(context.Background(), dl, "github.com/org/express", "1.0.0", confirm)
	if err != nil {
		t.Fatalf("Install without hooks failed: %v", err)
	}

	dst := Path("express", "1.0.0")
	sidecar := readSidecarFile(t, dst)
	if sidecar.HooksEnabled {
		t.Error("sidecar HooksEnabled should be false when pack has no hooks")
	}
}

func TestInstall_IdempotentReinstall(t *testing.T) {
	srcRoot := t.TempDir()
	packRoot := fakePackDir(t, srcRoot, "express", "1.0.0")

	baseDir := t.TempDir()
	t.Setenv(EnvPacksDir, baseDir)

	dl := &FakeDownloader{Dir: packRoot}
	confirm := func(name string) (bool, error) { return false, nil }

	// First install.
	_, err := Install(context.Background(), dl, "github.com/org/express", "1.0.0", confirm)
	if err != nil {
		t.Fatalf("first Install failed: %v", err)
	}

	dst := Path("express", "1.0.0")
	if _, err := os.Stat(dst); err != nil {
		t.Fatalf("installed dir missing after first install: %v", err)
	}

	// Second install (same version).
	_, err = Install(context.Background(), dl, "github.com/org/express", "1.0.0", confirm)
	if err != nil {
		t.Fatalf("second Install (reinstall) failed: %v", err)
	}

	// Verify dir still exists and is valid.
	if _, err := os.Stat(dst); err != nil {
		t.Fatalf("installed dir missing after reinstall: %v", err)
	}

	m, err := Load(dst)
	if err != nil {
		t.Fatalf("re-validating manifest after reinstall: %v", err)
	}
	if m.Name != "express" {
		t.Errorf("manifest name after reinstall = %q, want %q", m.Name, "express")
	}
}

func TestInstall_RevalidateFailure(t *testing.T) {
	// Simulate post-copy validation failure: source dir exists but has no
	// go-arch.yaml, so Load() fails after copy.
	srcRoot := t.TempDir()

	baseDir := t.TempDir()
	t.Setenv(EnvPacksDir, baseDir)

	confirm := func(name string) (bool, error) { return false, nil }

	// Sabotage: provide a source dir without go-arch.yaml.
	badRoot := filepath.Join(srcRoot, "bad-pack")
	if err := os.MkdirAll(badRoot, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(badRoot, "templates"), 0755); err != nil {
		t.Fatal(err)
	}
	badDl := &FakeDownloader{Dir: badRoot}

	_, err := Install(context.Background(), badDl, "github.com/org/express", "1.0.0", confirm)
	if err == nil {
		t.Fatal("expected error for revalidation failure, got nil")
	}

	// Verify that partial state was cleaned up.
	assertNoDir(t, Path("express", "1.0.0"))
}

// ---------------------------------------------------------------------------
// Remove tests
// ---------------------------------------------------------------------------

func TestRemove_Success(t *testing.T) {
	baseDir := t.TempDir()
	t.Setenv(EnvPacksDir, baseDir)

	// Install a pack first.
	srcRoot := t.TempDir()
	packRoot := fakePackDir(t, srcRoot, "express", "1.0.0")
	dl := &FakeDownloader{Dir: packRoot}
	confirm := func(name string) (bool, error) { return false, nil }

	_, err := Install(context.Background(), dl, "github.com/org/express", "1.0.0", confirm)
	if err != nil {
		t.Fatalf("setup Install failed: %v", err)
	}

	// Now remove it.
	if err := Remove("express", "1.0.0"); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	assertNoDir(t, Path("express", "1.0.0"))
}

func TestRemove_NotInstalled(t *testing.T) {
	baseDir := t.TempDir()
	t.Setenv(EnvPacksDir, baseDir)

	err := Remove("express", "1.0.0")
	if err == nil {
		t.Fatal("expected error for removing non-installed pack, got nil")
	}
	assertCode(t, err, CodePackNotInstalled)
}

// ---------------------------------------------------------------------------
// List tests
// ---------------------------------------------------------------------------

func TestList_Empty(t *testing.T) {
	baseDir := t.TempDir()
	t.Setenv(EnvPacksDir, baseDir)

	packs, err := List()
	if err != nil {
		t.Fatalf("List empty: unexpected error: %v", err)
	}
	if len(packs) > 0 {
		t.Errorf("List on empty dir should return empty, got %d entries", len(packs))
	}
}

func TestList_WithPacks(t *testing.T) {
	baseDir := t.TempDir()
	t.Setenv(EnvPacksDir, baseDir)

	srcRoot := t.TempDir()
	confirm := func(name string) (bool, error) { return false, nil }

	// Install express@1.0.0.
	packRoot1 := fakePackDir(t, srcRoot, "express", "1.0.0")
	dl1 := &FakeDownloader{Dir: packRoot1}
	if _, err := Install(context.Background(), dl1, "github.com/org/express", "1.0.0", confirm); err != nil {
		t.Fatalf("setup Install express: %v", err)
	}

	// Install echo@0.5.0.
	packRoot2 := fakePackDir(t, srcRoot, "echo", "0.5.0")
	dl2 := &FakeDownloader{Dir: packRoot2}
	if _, err := Install(context.Background(), dl2, "github.com/org/echo", "0.5.0", confirm); err != nil {
		t.Fatalf("setup Install echo: %v", err)
	}

	packs, err := List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(packs) != 2 {
		t.Fatalf("expected 2 packs, got %d: %v", len(packs), packs)
	}

	// Verify sorted by name.
	if packs[0].Name != "echo" {
		t.Errorf("first pack should be echo, got %q", packs[0].Name)
	}
	if packs[1].Name != "express" {
		t.Errorf("second pack should be express, got %q", packs[1].Name)
	}
}

// ---------------------------------------------------------------------------
// Update tests
// ---------------------------------------------------------------------------

func TestUpdate_Success(t *testing.T) {
	baseDir := t.TempDir()
	t.Setenv(EnvPacksDir, baseDir)

	srcRoot := t.TempDir()
	confirm := func(name string) (bool, error) { return false, nil }

	// First install express@1.0.0.
	packRoot := fakePackDir(t, srcRoot, "express", "1.0.0")
	dl := &FakeDownloader{Dir: packRoot}
	if _, err := Install(context.Background(), dl, "github.com/org/express", "1.0.0", confirm); err != nil {
		t.Fatalf("setup Install: %v", err)
	}

	// Now update with a newer version.
	packRoot2 := fakePackDir(t, srcRoot, "express", "1.1.0")
	dl2 := &FakeDownloader{Dir: packRoot2}

	m, err := Update(context.Background(), dl2, "express", confirm)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if m.Version != "1.1.0" {
		t.Errorf("updated manifest version = %q, want %q", m.Version, "1.1.0")
	}

	// Both versions should coexist.
	if _, err := os.Stat(Path("express", "1.0.0")); err != nil {
		t.Errorf("old version should still exist: %v", err)
	}
	if _, err := os.Stat(Path("express", "1.1.0")); err != nil {
		t.Errorf("new version should exist: %v", err)
	}
}

func TestUpdate_NotInstalled(t *testing.T) {
	baseDir := t.TempDir()
	t.Setenv(EnvPacksDir, baseDir)

	confirm := func(name string) (bool, error) { return false, nil }

	_, err := Update(context.Background(), nil, "express", confirm)
	if err == nil {
		t.Fatal("expected error for updating non-installed pack, got nil")
	}
	assertCode(t, err, CodePackNotInstalled)
}

func TestUpdate_RePromptHooks(t *testing.T) {
	baseDir := t.TempDir()
	t.Setenv(EnvPacksDir, baseDir)

	srcRoot := t.TempDir()

	// First install express without hooks.
	packRoot := fakePackDir(t, srcRoot, "express", "1.0.0")
	dl := &FakeDownloader{Dir: packRoot}
	confirm := func(name string) (bool, error) { return false, nil }
	if _, err := Install(context.Background(), dl, "github.com/org/express", "1.0.0", confirm); err != nil {
		t.Fatalf("setup Install: %v", err)
	}

	// Now update with a version that has hooks.
	packRoot2 := fakePackDirWithHooks(t, srcRoot, "express", "1.1.0")
	dl2 := &FakeDownloader{Dir: packRoot2}

	var confirmCalled bool
	confirm2 := func(name string) (bool, error) {
		confirmCalled = true
		return true, nil
	}

	_, err := Update(context.Background(), dl2, "express", confirm2)
	if err != nil {
		t.Fatalf("Update with hooks re-prompt failed: %v", err)
	}
	if !confirmCalled {
		t.Error("confirm should be called when update brings new hooks")
	}

	// New version sidecar should reflect hooks accepted.
	sidecar := readSidecarFile(t, Path("express", "1.1.0"))
	if !sidecar.HooksEnabled {
		t.Error("sidecar HooksEnabled should be true after accepting hooks in update")
	}
}

// ---------------------------------------------------------------------------
// Network-gated offline cached install test (RealDownloader)
// ---------------------------------------------------------------------------

// seedModuleCache creates a minimal Go module cache entry under gomodcache
// for module@version. It writes the extracted module directory and the cache
// download files so that go mod download can resolve the module via a
// file:// proxy with zero network.
//
// Go's proxy protocol first resolves the user-supplied version (e.g. "1.0.0")
// via the .info file, then fetches the canonical version files (e.g. "v1.0.0.*")
// using the Version field from the .info JSON. We seed BOTH sets.
func seedModuleCache(t *testing.T, gomodcache, module, version string) string {
	t.Helper()

	// Canonical form with "v" prefix for the extracted directory.
	canonical := "v" + version
	extractedDir := filepath.Join(gomodcache, module+"@"+canonical)

	// --- extracted module directory ---
	if err := os.MkdirAll(filepath.Join(extractedDir, "templates", "common"), 0755); err != nil {
		t.Fatal(err)
	}
	goMod := fmt.Sprintf("module %s\n\ngo 1.21\n", module)
	if err := os.WriteFile(filepath.Join(extractedDir, "go.mod"), []byte(goMod), 0644); err != nil {
		t.Fatal(err)
	}
	manifestYAML := `contract_version: 1
name: testcache
version: ` + version + `
`
	if err := os.WriteFile(filepath.Join(extractedDir, "go-arch.yaml"), []byte(manifestYAML), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(extractedDir, "templates", "common", "hello.tmpl"),
		[]byte("// {{.ProjectName}}"),
		0644,
	); err != nil {
		t.Fatal(err)
	}

	// --- cache download files ---
	cacheDir := filepath.Join(gomodcache, "cache", "download", module, "@v")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Canonical-version files (.mod, .zip, .ziphash) — Go fetches these
	// after resolving the canonical version from the .info file.
	if err := os.WriteFile(filepath.Join(cacheDir, canonical+".mod"), []byte(goMod), 0644); err != nil {
		t.Fatal(err)
	}

	infoJSON := fmt.Sprintf(`{"Version":"%s","Time":"2025-01-01T00:00:00Z"}`, canonical)

	// .zip — archive/zip of the extracted directory.
	zipPath := filepath.Join(cacheDir, canonical+".zip")
	if err := createModuleZip(extractedDir, zipPath); err != nil {
		t.Fatal(err)
	}

	// .ziphash — Go module sum hash: "h1:" + base64(raw-sha256).
	zipData, err := os.ReadFile(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(zipData)
	ziphash := "h1:" + base64.StdEncoding.EncodeToString(h[:])
	if err := os.WriteFile(filepath.Join(cacheDir, canonical+".ziphash"), []byte(ziphash), 0644); err != nil {
		t.Fatal(err)
	}

	// User-supplied-version .info — Go uses this for initial resolution
	// (e.g. "go mod download mod@1.0.0" queries "@v/1.0.0.info" first).
	if err := os.WriteFile(filepath.Join(cacheDir, version+".info"), []byte(infoJSON), 0644); err != nil {
		t.Fatal(err)
	}

	return extractedDir
}

// createModuleZip creates a zip file at dst from the contents of root.
// It preserves directory structure and file modes.
func createModuleZip(root, dst string) error {
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()

	w := zip.NewWriter(f)
	defer w.Close()

	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}

		hdr, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		hdr.Name = filepath.ToSlash(rel) // zip uses forward slashes
		if info.IsDir() {
			hdr.Name += "/"
		} else {
			hdr.Method = zip.Deflate
		}

		w, err := w.CreateHeader(hdr)
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	})
}

// TestInstall_OfflineCached skips during -short (network-gated) and when
// go is not available. It seeds GOMODCACHE, sets GOPROXY=off, and verifies
// RealDownloader resolves the module from the local cache with zero network.
func TestInstall_OfflineCached(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network-gated offline-cached test in -short mode")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not available, skipping RealDownloader test")
	}

	gomodcache := t.TempDir()
	module := "github.com/org/testcache"
	version := "1.0.0"

	expectedDir := seedModuleCache(t, gomodcache, module, version)

	// Use file:// proxy pointing to the seeded cache so Go resolves the
	// module from the local filesystem without network. GOPROXY=off
	// disables all lookups (including local cache) in Go 1.21+.
	t.Setenv("GOMODCACHE", gomodcache)
	t.Setenv("GOPROXY", "file://"+filepath.Join(gomodcache, "cache", "download"))
	t.Setenv("GONOSUMCHECK", "*")
	t.Setenv("GONOSUMDB", "*")
	t.Setenv("GOFLAGS", "-mod=mod")

	ctx := context.Background()
	dl := RealDownloader{}

	gotDir, err := dl.Download(ctx, module, version)
	if err != nil {
		t.Fatalf("RealDownloader.Download from seeded cache failed: %v", err)
	}

	if gotDir != expectedDir {
		t.Errorf("Download dir = %q, want %q", gotDir, expectedDir)
	}

	// Verify the returned directory contains our seeded files.
	if _, err := os.Stat(filepath.Join(gotDir, "go-arch.yaml")); err != nil {
		t.Errorf("go-arch.yaml missing in returned dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(gotDir, "templates", "common", "hello.tmpl")); err != nil {
		t.Errorf("hello.tmpl missing in returned dir: %v", err)
	}
}
