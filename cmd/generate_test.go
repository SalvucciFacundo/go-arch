package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestTemplHint verifies task 3.1: templHint returns a string containing
// "templ generate" for page and component types.
func TestTemplHint(t *testing.T) {
	tests := []struct {
		name    string
		genType string
		want    string
	}{
		{"page", "page", "templ generate"},
		{"component", "component", "templ generate"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := templHint(tt.genType)
			if !strings.Contains(got, tt.want) {
				t.Errorf("templHint(%q) = %q, want it to contain %q", tt.genType, got, tt.want)
			}
		})
	}
}

// TestGenerateCLI verifies task 3.1: smoke test (page gen shows templ hint)
// and help lists all six types. Both run inside one function because cobra
// RootCmd has global state that persists across test functions.
func TestGenerateCLI(t *testing.T) {
	t.Run("smoke: page generation shows templ hint", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "gen-page-smoke-*")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(tmpDir)

		oldWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(oldWd)

		configYAML := `project_name: "."
module_name: github.com/test/smokeapp
architecture: Standard
use_templ_htmx: true
`
		if err := os.WriteFile(filepath.Join(tmpDir, ".go-arch.yaml"), []byte(configYAML), 0644); err != nil {
			t.Fatal(err)
		}

		buf := new(bytes.Buffer)
		RootCmd.SetOut(buf)
		RootCmd.SetErr(buf)
		RootCmd.SetArgs([]string{"generate", "page", "Dashboard"})

		if err := RootCmd.Execute(); err != nil {
			t.Fatalf("generate page Dashboard failed: %v\nOutput: %s", err, buf.String())
		}

		out := buf.String()
		if !strings.Contains(out, "templ generate") {
			t.Errorf("output should contain 'templ generate' hint; got:\n%s", out)
		}
	})

	t.Run("help lists all six types", func(t *testing.T) {
		buf := new(bytes.Buffer)
		RootCmd.SetOut(buf)
		RootCmd.SetErr(buf)
		RootCmd.SetArgs([]string{"generate", "--help"})

		if err := RootCmd.Execute(); err != nil {
			t.Fatalf("generate --help failed: %v", err)
		}

		out := buf.String()
		for _, want := range []string{"service", "repository", "handler", "crud", "page", "component"} {
			if !strings.Contains(out, want) {
				t.Errorf("generate --help should mention %q; got:\n%s", want, out)
			}
		}
	})
}
