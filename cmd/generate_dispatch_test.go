package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --------------- Arg Validation Tests ---------------

func TestGenerateArgs_ListFlag_ZeroArgs(t *testing.T) {
	// --list should allow 0 positional args.
	defer generateCmd.Flags().Set("list", "false") // reset for next test

	tmpDir, err := os.MkdirTemp("", "gen-args-list-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	// Write minimal .go-arch.yaml so cobra doesn't fail on config read.
	configYAML := `project_name: "TestArgs"
module_name: github.com/test/args
architecture: Standard
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".go-arch.yaml"), []byte(configYAML), 0644); err != nil {
		t.Fatal(err)
	}

	buf := new(bytes.Buffer)
	RootCmd.SetOut(buf)
	RootCmd.SetErr(buf)
	RootCmd.SetArgs([]string{"generate", "--list"})

	err = RootCmd.Execute()
	if err != nil {
		t.Fatalf("generate --list with 0 args should succeed: %v\nOutput: %s", err, buf.String())
	}
}

func TestGenerateArgs_NoList_ExactTwoArgs(t *testing.T) {
	// Without --list, exactly 2 positional args are required.
	tmpDir, err := os.MkdirTemp("", "gen-args-exact-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	configYAML := `project_name: "TestArgs"
module_name: github.com/test/args
architecture: Standard
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".go-arch.yaml"), []byte(configYAML), 0644); err != nil {
		t.Fatal(err)
	}

	// Reset list flag for clean state.
	_ = generateCmd.Flags().Set("list", "false")

	// 1 arg → should fail.
	buf := new(bytes.Buffer)
	RootCmd.SetOut(buf)
	RootCmd.SetErr(buf)
	RootCmd.SetArgs([]string{"generate", "service"})

	err = RootCmd.Execute()
	if err == nil {
		t.Fatal("generate with 1 arg should fail (needs 2)")
	}
	// Error should mention arguments.
	if !strings.Contains(err.Error(), "accepts") && !strings.Contains(err.Error(), "arg") {
		t.Errorf("expected arg count error, got: %v", err)
	}

	// 3 args → should fail.
	RootCmd.SetArgs([]string{"generate", "service", "User", "extra"})
	err = RootCmd.Execute()
	if err == nil {
		t.Fatal("generate with 3 args should fail (needs 2)")
	}
}

// --------------- --list Tests ---------------

func TestGenerateList_WithoutPack(t *testing.T) {
	// --list without a project template should only show components.
	tmpDir, err := os.MkdirTemp("", "gen-list-nopack-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	configYAML := `project_name: "TestList"
module_name: github.com/test/list
architecture: Standard
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".go-arch.yaml"), []byte(configYAML), 0644); err != nil {
		t.Fatal(err)
	}

	buf := new(bytes.Buffer)
	RootCmd.SetOut(buf)
	RootCmd.SetErr(buf)
	RootCmd.SetArgs([]string{"generate", "--list"})

	err = RootCmd.Execute()
	if err != nil {
		t.Fatalf("generate --list: %v", err)
	}

	out := buf.String()
	// Should mention component types.
	for _, want := range []string{"service", "repository", "handler", "crud", "page", "component"} {
		if !strings.Contains(out, want) {
			t.Errorf("--list should mention %q; got:\n%s", want, out)
		}
	}
	// Should mention "no builtin generators" since v2 ships with empty registry.
	if !strings.Contains(out, "no builtin generators registered") {
		t.Errorf("--list should show 'no builtin generators registered'; got:\n%s", out)
	}
}

func TestGenerateList_Deterministic(t *testing.T) {
	// --list output should be deterministic across two runs.
	tmpDir, err := os.MkdirTemp("", "gen-list-det-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	configYAML := `project_name: "TestDet"
module_name: github.com/test/det
architecture: Standard
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".go-arch.yaml"), []byte(configYAML), 0644); err != nil {
		t.Fatal(err)
	}

	buf1 := new(bytes.Buffer)
	RootCmd.SetOut(buf1)
	RootCmd.SetArgs([]string{"generate", "--list"})
	if err := RootCmd.Execute(); err != nil {
		t.Fatalf("run 1: %v", err)
	}

	buf2 := new(bytes.Buffer)
	RootCmd.SetOut(buf2)
	RootCmd.SetArgs([]string{"generate", "--list"})
	if err := RootCmd.Execute(); err != nil {
		t.Fatalf("run 2: %v", err)
	}

	if buf1.String() != buf2.String() {
		t.Errorf("--list output should be deterministic:\nRun1:\n%s\nRun2:\n%s",
			buf1.String(), buf2.String())
	}
}

// --------------- Unknown Generator Tests ---------------

func TestGenerateUnknownGenerator(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gen-unknown-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	configYAML := `project_name: "TestUnknown"
module_name: github.com/test/unknown
architecture: Standard
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".go-arch.yaml"), []byte(configYAML), 0644); err != nil {
		t.Fatal(err)
	}

	// Reset the list flag to ensure clean state after other tests.
	_ = generateCmd.Flags().Set("list", "false")

	buf := new(bytes.Buffer)
	RootCmd.SetOut(buf)
	RootCmd.SetErr(buf)
	RootCmd.SetArgs([]string{"generate", "bogus", "Whatever"})

	err = RootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for unknown generator")
	}

	out := buf.String()
	errMsg := err.Error() + "\n" + out
	// Error should contain the unknown generator name.
	if !strings.Contains(errMsg, "bogus") {
		t.Errorf("error should mention 'bogus': %v", err)
	}
	// Should list available names.
	if !strings.Contains(errMsg, "unknown_generator") && !strings.Contains(errMsg, "unknown generator") {
		t.Errorf("error should contain unknown_generator: %v", err)
	}
}

// TestGenerate_PackNotInstalled verifies that generate emits pack_not_installed
// when .go-arch.yaml declares template: <pack> but the pack is not installed
// and the requested type is not a component type.
func TestGenerate_PackNotInstalled_Error(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gen-pni-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	// Declare template: missingpack (not installed), no pack dirs set up.
	configYAML := `project_name: "TestPNI"
module_name: github.com/test/pni
architecture: Standard
template: missingpack@1.0.0
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".go-arch.yaml"), []byte(configYAML), 0644); err != nil {
		t.Fatal(err)
	}

	_ = generateCmd.Flags().Set("list", "false")

	buf := new(bytes.Buffer)
	RootCmd.SetOut(buf)
	RootCmd.SetErr(buf)
	RootCmd.SetArgs([]string{"generate", "docker", "whatever"})

	err = RootCmd.Execute()
	if err == nil {
		t.Fatal("expected pack_not_installed error for missing pack")
	}

	errMsg := err.Error() + "\n" + buf.String()
	if !strings.Contains(errMsg, "missingpack") {
		t.Errorf("error should name pack 'missingpack', got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "not installed") {
		t.Errorf("error should say 'not installed', got: %s", errMsg)
	}
}

// TestGenerate_PackNotInstalled_ComponentStillWorks verifies that component
// types work even when the template pack is not installed.
func TestGenerate_PackNotInstalled_ComponentStillWorks(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gen-pni-comp-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	configYAML := `project_name: "TestPNIComp"
module_name: github.com/test/pnicomp
architecture: Standard
template: missingpack@1.0.0
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".go-arch.yaml"), []byte(configYAML), 0644); err != nil {
		t.Fatal(err)
	}

	_ = generateCmd.Flags().Set("list", "false")

	buf := new(bytes.Buffer)
	RootCmd.SetOut(buf)
	RootCmd.SetErr(buf)
	RootCmd.SetArgs([]string{"generate", "service", "Order"})

	err = RootCmd.Execute()
	if err != nil {
		t.Fatalf("component generation should still work without pack: %v\nOutput: %s", err, buf.String())
	}
}

// TestGenerate_UnknownGenerator_GroupedListing verifies that the
// unknown_generator error groups available names by source.
func TestGenerate_UnknownGenerator_GroupedListing(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gen-ug-grouped-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	configYAML := `project_name: "TestUGGrouped"
module_name: github.com/test/uggrouped
architecture: Standard
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".go-arch.yaml"), []byte(configYAML), 0644); err != nil {
		t.Fatal(err)
	}

	_ = generateCmd.Flags().Set("list", "false")

	buf := new(bytes.Buffer)
	RootCmd.SetOut(buf)
	RootCmd.SetErr(buf)
	RootCmd.SetArgs([]string{"generate", "bogus999", "Whatevs"})

	err = RootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for unknown generator")
	}

	errMsg := err.Error() + "\n" + buf.String()
	// Should contain grouped listing.
	if !strings.Contains(errMsg, "component types:") {
		t.Errorf("error should list component types, got: %s", errMsg)
	}
	// Should mention service, repository etc.
	for _, want := range []string{"service", "repository", "handler", "crud", "page", "component"} {
		if !strings.Contains(errMsg, want) {
			t.Errorf("error should mention component type %q, got: %s", want, errMsg)
		}
	}
}

// --------------- Pack Installed + Unknown Generator Tests ---------------

// TestGenerate_PackInstalled_UnknownGenerator_GroupedListing verifies that
// when the project declares a template pack that IS installed, but the
// requested generator does NOT exist in that pack, the error is
// unknown_generator (NOT pack_not_installed) and the grouped listing
// INCLUDES the pack's available generators.
// REQ-10 S1 / REQ-22 S3 regression guard.
func TestGenerate_PackInstalled_UnknownGenerator_GroupedListing(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gen-pkg-unknown-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	// Create a fixture v2 pack with generators "docker" and "service".
	packDir := filepath.Join(tmpDir, "testpack@1.0.0")
	if err := os.MkdirAll(packDir, 0755); err != nil {
		t.Fatal(err)
	}
	manifestYAML := `contract_version: 2
name: testpack
version: 1.0.0
generators:
  docker:
    description: "Generate Docker config"
    steps:
      - type: template
        from: "Dockerfile.tmpl"
        to: "Dockerfile"
  service:
    description: "Generate a service"
    steps:
      - type: template
        from: "handler.tmpl"
        to: "internal/handler/x.go"
`
	if err := os.WriteFile(filepath.Join(packDir, "go-arch.yaml"), []byte(manifestYAML), 0644); err != nil {
		t.Fatal(err)
	}

	// Create template files so Load succeeds.
	tmplDir := filepath.Join(packDir, "templates")
	if err := os.MkdirAll(tmplDir, 0755); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, filepath.Join(tmplDir, "Dockerfile.tmpl"), "FROM golang:1.24")
	mustWriteFile(t, filepath.Join(tmplDir, "handler.tmpl"), "package handler")

	// Point GO_ARCH_PACKS_DIR to our temp dir so packs.Path / LatestInstalled works.
	t.Setenv("GO_ARCH_PACKS_DIR", tmpDir)

	// Write .go-arch.yaml with template referencing the installed pack.
	configYAML := `project_name: "TestPackUK"
module_name: github.com/test/packuk
architecture: Standard
template: testpack@1.0.0
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".go-arch.yaml"), []byte(configYAML), 0644); err != nil {
		t.Fatal(err)
	}

	// Create .go-arch directory so manifest operations don't fail.
	archDir := filepath.Join(tmpDir, ".go-arch")
	if err := os.MkdirAll(archDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(archDir, "manifest.yaml"),
		[]byte("version: 1\nfiles: {}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Reset the list flag for clean state.
	_ = generateCmd.Flags().Set("list", "false")

	buf := new(bytes.Buffer)
	RootCmd.SetOut(buf)
	RootCmd.SetErr(buf)
	RootCmd.SetArgs([]string{"generate", "bogus", "Whatever"})

	err = RootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for unknown generator when pack is installed")
	}

	errMsg := err.Error() + "\n" + buf.String()

	// Must NOT claim the pack is not installed.
	if strings.Contains(errMsg, "not installed") {
		t.Errorf("error should NOT claim pack is not installed (pack IS installed). Got: %s", errMsg)
	}

	// Must contain unknown_generator.
	if !strings.Contains(errMsg, "unknown_generator") && !strings.Contains(errMsg, "unknown generator") {
		t.Errorf("error should contain unknown_generator, got: %s", errMsg)
	}

	// Must list the pack's available generators.
	if !strings.Contains(errMsg, "docker") {
		t.Errorf("error should list pack generator 'docker', got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "service") {
		t.Errorf("error should list pack generator 'service', got: %s", errMsg)
	}

	// Must list component types.
	if !strings.Contains(errMsg, "component types:") {
		t.Errorf("error should list component types, got: %s", errMsg)
	}
}

// mustWriteFile writes a file or fails the test.
func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}
