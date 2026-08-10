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
