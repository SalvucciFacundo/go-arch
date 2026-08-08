package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGenerateComponentHandler verifies task 4.1: the generate_component tool
// handler accepts type=page and type=component and creates the expected files
// when dispatched against a tempdir web project.
func TestGenerateComponentHandler(t *testing.T) {
	t.Run("page", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "mcp-page-*")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(tmpDir)

		oldWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(oldWd)

		configYAML := `project_name: "."
module_name: github.com/test/mcppage
architecture: Standard
use_templ_htmx: true
`
		if err := os.WriteFile(filepath.Join(tmpDir, ".go-arch.yaml"), []byte(configYAML), 0644); err != nil {
			t.Fatal(err)
		}

		args, _ := json.Marshal(map[string]string{
			"type": "page",
			"name": "Dashboard",
		})
		var rawArgs json.RawMessage = args

		// Dispatch via handleToolCall (exercise real handler, not just enum)
		handleToolCall(1, "generate_component", rawArgs)

		targetFile := filepath.Join(tmpDir, "views", "pages", "dashboard.templ")
		content, err := os.ReadFile(targetFile)
		if err != nil {
			t.Fatalf("expected file views/pages/dashboard.templ to exist: %v", err)
		}

		contentStr := string(content)
		for _, want := range []string{"package pages", "templ Dashboard()", "@layouts.Base(0)"} {
			if !strings.Contains(contentStr, want) {
				t.Errorf("page output should contain %q; got:\n%s", want, contentStr)
			}
		}
	})

	t.Run("component", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "mcp-comp-*")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(tmpDir)

		oldWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(oldWd)

		configYAML := `project_name: "."
module_name: github.com/test/mcpcomp
architecture: Standard
use_templ_htmx: true
`
		if err := os.WriteFile(filepath.Join(tmpDir, ".go-arch.yaml"), []byte(configYAML), 0644); err != nil {
			t.Fatal(err)
		}

		args, _ := json.Marshal(map[string]string{
			"type": "component",
			"name": "UserCard",
		})
		var rawArgs json.RawMessage = args
		handleToolCall(1, "generate_component", rawArgs)

		targetFile := filepath.Join(tmpDir, "views", "components", "usercard.templ")
		content, err := os.ReadFile(targetFile)
		if err != nil {
			t.Fatalf("expected file views/components/usercard.templ to exist: %v", err)
		}

		contentStr := string(content)
		for _, want := range []string{"package components", "templ UserCard()", "hx-get="} {
			if !strings.Contains(contentStr, want) {
				t.Errorf("component output should contain %q; got:\n%s", want, contentStr)
			}
		}
	})
}
