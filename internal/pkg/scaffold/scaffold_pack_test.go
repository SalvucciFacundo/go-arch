package scaffold

import (
	"bytes"
	"go-arch/internal/pkg/hooks"
	"go-arch/internal/pkg/packs"
	"go-arch/internal/ui"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// packFixture creates a minimal pack on disk for testing scaffoldPack.
// Returns the pack dir path and the parsed manifest.
func packFixture(t *testing.T, dir string) (string, *packs.Manifest) {
	t.Helper()

	packDir := filepath.Join(dir, "express@1.0.0")
	// Create the pack root with go-arch.yaml manifest
	if err := os.MkdirAll(packDir, 0755); err != nil {
		t.Fatal(err)
	}

	manifestYAML := `contract_version: 1
name: express
version: 1.0.0
layout:
  - cmd/api
  - internal/handler
hooks:
  post-new:
    - echo "$PACK_NAME $PACK_VERSION"
binary_assets:
  - source: assets/htmx.min.js
    target: static/js/htmx.min.js
`
	if err := os.WriteFile(filepath.Join(packDir, "go-arch.yaml"), []byte(manifestYAML), 0644); err != nil {
		t.Fatal(err)
	}

	manifest, err := packs.Load(packDir)
	if err != nil {
		t.Fatal(err)
	}

	// Create templates/ directory with .go-arch.yaml.tmpl, main.go.tmpl
	templatesDir := filepath.Join(packDir, "templates")
	if err := os.MkdirAll(templatesDir, 0755); err != nil {
		t.Fatal(err)
	}

	// .go-arch.yaml.tmpl → renders to .go-arch.yaml with template field
	configTmpl := `project_name: {{ .ProjectName }}
module_name: {{ .ModuleName }}
architecture: {{ .Architecture }}
template: {{ .Template }}
db_driver: {{ .DBDriver }}
use_docker: {{ .UseDocker }}
use_observability: {{ .UseObservability }}
use_grpc: {{ .UseGRPC }}
use_templ_htmx: {{ .UseTemplHTMX }}
go_arch_version: {{ .GoArchVersion }}
`
	if err := os.WriteFile(filepath.Join(templatesDir, ".go-arch.yaml.tmpl"), []byte(configTmpl), 0644); err != nil {
		t.Fatal(err)
	}

	// main.go.tmpl → renders to main.go
	mainTmpl := `package main

import "fmt"

func main() {
	fmt.Println("Hello from {{ .ProjectName }}")
}
`
	if err := os.WriteFile(filepath.Join(templatesDir, "main.go.tmpl"), []byte(mainTmpl), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a binary asset
	assetsDir := filepath.Join(packDir, "assets")
	if err := os.MkdirAll(assetsDir, 0755); err != nil {
		t.Fatal(err)
	}
	binaryData := []byte{0x48, 0x54, 0x4D, 0x58, 0x20, 0x42, 0x49, 0x4E, 0x41, 0x52, 0x59} // "HTMX BINARY"
	if err := os.WriteFile(filepath.Join(assetsDir, "htmx.min.js"), binaryData, 0644); err != nil {
		t.Fatal(err)
	}

	return packDir, manifest
}

// TestScaffoldPack_E2E verifies task 4.1: scaffoldPack generates files from a
// fixture pack, records manifest entries with pack source, copies binary
// assets, and fires pack-scoped hooks with PACK_NAME/PACK_VERSION.
func TestScaffoldPack_E2E(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "scaffold-pack-e2e-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	// Create fixture pack
	packDir, manifest := packFixture(t, tmpDir)

	packInfo := packs.PackInfo{Dir: packDir, Manifest: manifest}

	// Build config with pack defaults
	config := &ui.ProjectConfig{
		ProjectName:  "DemoApp",
		ModuleName:   "github.com/demo/app",
		Architecture: "",        // empty — pack IS the architecture
		Template:     "express", // set before scaffold
		DBDriver:     "None",
	}

	// Fake runner to verify hooks fire with PACK_NAME/PACK_VERSION
	fakeRunner := hooks.NewRunner(&hooks.Config{
		Hooks: map[hooks.Type][]hooks.Entry{
			hooks.PostNew: {
				{Command: "echo", Args: []string{"$PACK_NAME", "$PACK_VERSION"}, Shell: true},
			},
		},
	}, &hooks.FakeRunner{}, os.Stderr)

	scaffolder := NewScaffolder(config,
		WithPackInfo(packInfo),
		WithRunner(fakeRunner),
	)

	err = scaffolder.Execute()
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	projectDir := filepath.Join(tmpDir, config.ProjectName)

	// 1. Verify generated files exist (strip .tmpl convention)
	expectedFiles := []string{
		"main.go",
		".go-arch.yaml",
	}
	for _, f := range expectedFiles {
		fullPath := filepath.Join(projectDir, f)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			t.Errorf("expected file %q to exist", f)
		}
	}

	// 2. Verify .go-arch.yaml contains template field
	configFile := filepath.Join(projectDir, ".go-arch.yaml")
	configContent, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatalf("cannot read .go-arch.yaml: %v", err)
	}
	if !strings.Contains(string(configContent), "template: express") {
		t.Errorf(".go-arch.yaml must contain 'template: express'; got:\n%s", string(configContent))
	}
	// The project config MUST NOT have a hooks block (spec: pack hooks are pack-scoped only)
	if strings.Contains(string(configContent), "hooks:") {
		t.Errorf(".go-arch.yaml must NOT contain hooks block (pack hooks are pack-scoped)")
	}

	// 3. Verify main.go was rendered with project name
	mainFile := filepath.Join(projectDir, "main.go")
	mainContent, err := os.ReadFile(mainFile)
	if err != nil {
		t.Fatalf("cannot read main.go: %v", err)
	}
	if !strings.Contains(string(mainContent), "Hello from DemoApp") {
		t.Errorf("main.go must contain 'Hello from DemoApp'; got:\n%s", string(mainContent))
	}

	// 4. Verify manifest entries have source: pack:express@1.0.0
	m, err := LoadManifest(projectDir)
	if err != nil {
		t.Fatal(err)
	}

	wantSource := "pack:express@1.0.0"
	for path, entry := range m.Files {
		if entry.Source != wantSource {
			t.Errorf("manifest entry %q: source = %q, want %q", path, entry.Source, wantSource)
		}
	}

	// 5. Verify binary asset was copied verbatim
	binaryFile := filepath.Join(projectDir, "static", "js", "htmx.min.js")
	binaryContent, err := os.ReadFile(binaryFile)
	if err != nil {
		t.Errorf("expected binary file static/js/htmx.min.js to exist: %v", err)
	} else {
		expectedBinary := []byte{0x48, 0x54, 0x4D, 0x58, 0x20, 0x42, 0x49, 0x4E, 0x41, 0x52, 0x59}
		if !bytes.Equal(binaryContent, expectedBinary) {
			t.Errorf("binary file content mismatch: got %v, want %v", binaryContent, expectedBinary)
		}
		// Verify binary entry source
		binaryEntry, ok := m.Files["static/js/htmx.min.js"]
		if !ok {
			t.Errorf("manifest missing binary file entry for static/js/htmx.min.js")
		} else {
			if binaryEntry.Source != wantSource {
				t.Errorf("binary manifest entry source = %q, want %q", binaryEntry.Source, wantSource)
			}
			if binaryEntry.Origin != OriginBinary {
				t.Errorf("binary manifest entry origin = %q, want binary", binaryEntry.Origin)
			}
		}
	}

	// 6. Verify layout directories were created
	layoutDirs := []string{
		"cmd/api",
		"internal/handler",
	}
	for _, d := range layoutDirs {
		fullPath := filepath.Join(projectDir, d)
		if info, err := os.Stat(fullPath); err != nil || !info.IsDir() {
			t.Errorf("expected layout directory %q to exist", d)
		}
	}
}

// TestScaffoldPack_HooksEnvVars verifies that pack-scoped hooks receive
// PACK_NAME and PACK_VERSION in their environment.
func TestScaffoldPack_HooksEnvVars(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "scaffold-pack-hooks-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	packDir, manifest := packFixture(t, tmpDir)
	packInfo := packs.PackInfo{Dir: packDir, Manifest: manifest}

	// Verify the pack manifest has hooks
	if _, ok := manifest.Hooks[hooks.PostNew]; !ok {
		t.Fatal("fixture pack must have post-new hooks")
	}

	// FakeRunner records executed commands so we can inspect env
	fr := &hooks.FakeRunner{}

	config := &ui.ProjectConfig{
		ProjectName:  "HookApp",
		ModuleName:   "github.com/hook/app",
		Architecture: "",
		Template:     "express",
		DBDriver:     "None",
	}

	runner := hooks.NewRunner(&hooks.Config{
		Hooks: map[hooks.Type][]hooks.Entry{
			hooks.PostNew: {
				{Command: "echo", Args: []string{"$PACK_NAME", "$PACK_VERSION"}, Shell: true},
			},
		},
	}, fr, os.Stderr)

	scaffolder := NewScaffolder(config,
		WithPackInfo(packInfo),
		WithRunner(runner),
	)

	err = scaffolder.Execute()
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// FakeRunner should have recorded at least one execution
	if len(fr.Calls) == 0 {
		t.Fatal("expected hooks to fire, but FakeRunner recorded no commands")
	}

	// Find a call with PACK_NAME/PACK_VERSION in the env
	found := false
	for _, cmd := range fr.Calls {
		for _, e := range cmd.Opts.Env {
			if strings.HasPrefix(e, "PACK_NAME=") {
				if want := "PACK_NAME=express"; e != want {
					t.Errorf("expected %q in hook env, got %q", want, e)
				}
				foundName := false
				for _, e2 := range cmd.Opts.Env {
					if e2 == "PACK_VERSION=1.0.0" {
						foundName = true
						break
					}
				}
				if !foundName {
					t.Errorf("expected PACK_VERSION=1.0.0 in hook env; got: %v", cmd.Opts.Env)
				}
				found = true
				break
			}
		}
		if found {
			break
		}
	}
	if !found {
		t.Errorf("PACK_NAME and PACK_VERSION not found in hook env; calls: %+v", fr.Calls)
	}
}
