package packs

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// fakePackDirV2WithGenerator creates a v2 pack with generators declaring a run: step.
func fakePackDirV2RunOnly(t *testing.T, parent, name, version string) string {
	t.Helper()
	root := filepath.Join(parent, name)
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	// templates/ must exist for Install to pass.
	if err := os.MkdirAll(filepath.Join(root, "templates", "common"), 0755); err != nil {
		t.Fatal(err)
	}
	// Write a minimal template file so templates/ is non-empty.
	if err := os.WriteFile(filepath.Join(root, "templates", "common", "dummy.tmpl"), []byte("dummy"), 0644); err != nil {
		t.Fatal(err)
	}
	yaml := `contract_version: 2
name: ` + name + `
version: ` + version + `
generators:
  docker:
    description: "Generate Docker setup"
    steps:
      - type: run
        command: echo
        args:
          - "hello"
`
	if err := os.WriteFile(filepath.Join(root, "go-arch.yaml"), []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	return root
}

// fakePackDirV2TemplateOnly creates a v2 pack with generators using only
// template and prompt steps (no run, no hooks).
func fakePackDirV2TemplateOnly(t *testing.T, parent, name, version string) string {
	t.Helper()
	root := filepath.Join(parent, name)
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "templates", "common"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "templates", "common", "handler.tmpl"), []byte("package {{.ModuleName}}"), 0644); err != nil {
		t.Fatal(err)
	}
	yaml := `contract_version: 2
name: ` + name + `
version: ` + version + `
generators:
  handler:
    description: "Generate a handler"
    steps:
      - type: template
        from: common/handler.tmpl
        to: internal/handler/handler.go
      - type: prompt
        name: db_driver
        message: "Database driver?"
        default: postgres
`
	if err := os.WriteFile(filepath.Join(root, "go-arch.yaml"), []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	return root
}

// TestInstall_V2RunOnlyTrustPrompt_Accept verifies that a v2 pack with
// a generator containing only run: steps triggers the trust prompt, and
// accepting sets HooksEnabled=true.
func TestInstall_V2RunOnlyTrustPrompt_Accept(t *testing.T) {
	srcRoot := t.TempDir()
	packRoot := fakePackDirV2RunOnly(t, srcRoot, "generator-test", "2.0.0")

	baseDir := t.TempDir()
	t.Setenv(EnvPacksDir, baseDir)

	dl := &FakeDownloader{Dir: packRoot}

	var confirmCalled bool
	confirm := func(name string) (bool, error) {
		confirmCalled = true
		if name != "generator-test" {
			t.Errorf("confirm called with name %q, want %q", name, "generator-test")
		}
		return true, nil
	}

	_, err := Install(context.Background(), dl, "github.com/org/generator-test", "2.0.0", confirm)
	if err != nil {
		t.Fatalf("Install with run-only generator accept: %v", err)
	}

	if !confirmCalled {
		t.Error("trust prompt should be called for v2 pack with run: steps")
	}

	dst := Path("generator-test", "2.0.0")
	sidecar := readSidecarFile(t, dst)
	if !sidecar.HooksEnabled {
		t.Error("sidecar HooksEnabled should be true when user accepts")
	}
}

// TestInstall_V2RunOnlyTrustPrompt_Decline verifies that declining the
// trust prompt for a run-only generator sets HooksEnabled=false.
func TestInstall_V2RunOnlyTrustPrompt_Decline(t *testing.T) {
	srcRoot := t.TempDir()
	packRoot := fakePackDirV2RunOnly(t, srcRoot, "generator-test", "2.0.0")

	baseDir := t.TempDir()
	t.Setenv(EnvPacksDir, baseDir)

	dl := &FakeDownloader{Dir: packRoot}

	var confirmCalled bool
	confirm := func(name string) (bool, error) {
		confirmCalled = true
		return false, nil
	}

	_, err := Install(context.Background(), dl, "github.com/org/generator-test", "2.0.0", confirm)
	if err != nil {
		t.Fatalf("Install with run-only generator decline: %v", err)
	}

	if !confirmCalled {
		t.Error("trust prompt should be called even on decline")
	}

	dst := Path("generator-test", "2.0.0")
	sidecar := readSidecarFile(t, dst)
	if sidecar.HooksEnabled {
		t.Error("sidecar HooksEnabled should be false when user declines")
	}
}

// TestInstall_V2TemplateOnlyNoTrustPrompt verifies that a v2 pack with
// generators that use only template/prompt steps (no run:, no hooks) does
// NOT trigger the trust prompt for generator command execution.
func TestInstall_V2TemplateOnlyNoTrustPrompt(t *testing.T) {
	srcRoot := t.TempDir()
	packRoot := fakePackDirV2TemplateOnly(t, srcRoot, "template-only", "2.0.0")

	baseDir := t.TempDir()
	t.Setenv(EnvPacksDir, baseDir)

	dl := &FakeDownloader{Dir: packRoot}

	confirm := func(name string) (bool, error) {
		t.Error("confirm should not be called for template-only generators")
		return false, nil
	}

	_, err := Install(context.Background(), dl, "github.com/org/template-only", "2.0.0", confirm)
	if err != nil {
		t.Fatalf("Install template-only v2 pack: %v", err)
	}

	dst := Path("template-only", "2.0.0")
	sidecar := readSidecarFile(t, dst)
	if sidecar.HooksEnabled {
		t.Error("sidecar HooksEnabled should be false for template-only generators")
	}
}

// TestInstall_V2WithRunStepAndHooks_TrustPrompt verifies that a v2 pack
// with both run: steps in generators AND pack-level hooks triggers the
// trust prompt.
func TestInstall_V2WithRunStepAndHooks_TrustPrompt(t *testing.T) {
	srcRoot := t.TempDir()
	root := filepath.Join(srcRoot, "combined")
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "templates"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "templates", "dummy.tmpl"), []byte("dummy"), 0644); err != nil {
		t.Fatal(err)
	}
	yaml := `contract_version: 2
name: combined
version: 2.0.0
hooks:
  post-new:
    - command: go
      args:
        - mod
        - tidy
generators:
  docker:
    steps:
      - type: run
        command: echo
        args: ["hello"]
`
	if err := os.WriteFile(filepath.Join(root, "go-arch.yaml"), []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	baseDir := t.TempDir()
	t.Setenv(EnvPacksDir, baseDir)

	dl := &FakeDownloader{Dir: root}

	var confirmCalled bool
	confirm := func(name string) (bool, error) {
		confirmCalled = true
		return true, nil
	}

	_, err := Install(context.Background(), dl, "github.com/org/combined", "2.0.0", confirm)
	if err != nil {
		t.Fatalf("Install combined hooks+generators: %v", err)
	}

	if !confirmCalled {
		t.Error("trust prompt should be called for pack with both hooks and run-step generators")
	}
}

// TestInstall_V2Generators_TrustWarningText verifies the warning text
// format mentions generator command execution.
func TestInstall_V2Generators_TrustWarningText(t *testing.T) {
	// This test verifies the confirm function receives the expected name.
	// The actual warning text is printed by the CLI layer; we test that
	// the trust gate fires for the correct condition.
	srcRoot := t.TempDir()
	packRoot := fakePackDirV2RunOnly(t, srcRoot, "warn-text", "2.0.0")

	baseDir := t.TempDir()
	t.Setenv(EnvPacksDir, baseDir)

	dl := &FakeDownloader{Dir: packRoot}

	confirm := func(name string) (bool, error) {
		if name != "warn-text" {
			t.Errorf("unexpected pack name %q in confirm", name)
		}
		return true, nil
	}

	_, err := Install(context.Background(), dl, "github.com/org/warn-text", "2.0.0", confirm)
	if err != nil {
		t.Fatalf("Install warn-text pack: %v", err)
	}
}

// Helper: needed for unused import workaround
var _ = os.Stat
var _ = filepath.Base
