package scaffold

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/SalvucciFacundo/go-arch/v2/internal/pkg/generators"
	"github.com/SalvucciFacundo/go-arch/v2/internal/pkg/hooks"
	"github.com/SalvucciFacundo/go-arch/v2/internal/pkg/packs"
	"github.com/SalvucciFacundo/go-arch/v2/internal/pkg/template"
	"github.com/SalvucciFacundo/go-arch/v2/internal/ui"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/samber/oops"
)

// --------------- V2 Pack Fixture Helpers ---------------

// genPackFixture creates a v2 pack with generators in t.TempDir.
// Returns the pack dir, loaded manifest, and source string.
func genPackFixture(t *testing.T, baseDir string) (string, *packs.Manifest, string) {
	t.Helper()

	packDir := filepath.Join(baseDir, "mygen@1.0.0")
	mustMkdirSG(t, packDir)

	manifestYAML := `contract_version: 2
name: mygen
version: 1.0.0
generators:
  docker:
    description: "Generate Docker config"
    steps:
      - type: template
        from: "docker/Dockerfile.tmpl"
        to: "Dockerfile"
      - type: binary
        from: "assets/docker-compose.yml"
        to: "docker-compose.yml"
        mode: 0644
      - type: prompt
        name: "port"
        message: "Expose port?"
        default: "8080"
        required: false
  advanced:
    description: "Advanced generator with run step"
    steps:
      - type: template
        from: "advanced/setup.go.tmpl"
        to: "internal/setup.go"
      - type: run
        command: "echo"
        args: ["generated"]
`
	mustWriteFileSG(t, filepath.Join(packDir, "go-arch.yaml"), manifestYAML)

	manifest, err := packs.Load(packDir)
	if err != nil {
		t.Fatalf("Load v2 pack manifest: %v", err)
	}

	// Create templates — use ProjectConfig fields only.
	tmplDocker := filepath.Join(packDir, "templates", "docker")
	mustMkdirSG(t, tmplDocker)
	mustWriteFileSG(t, filepath.Join(tmplDocker, "Dockerfile.tmpl"),
		`FROM golang:1.24 AS builder
# Project: {{ .ProjectName }}
# Module: {{ .ModuleName }}
WORKDIR /app
`)

	tmplAdv := filepath.Join(packDir, "templates", "advanced")
	mustMkdirSG(t, tmplAdv)
	mustWriteFileSG(t, filepath.Join(tmplAdv, "setup.go.tmpl"),
		`package internal
// Generated for {{ .ProjectName }}
`)

	// Create binary asset.
	assetsDir := filepath.Join(packDir, "assets")
	mustMkdirSG(t, assetsDir)
	mustWriteFileSG(t, filepath.Join(assetsDir, "docker-compose.yml"),
		`version: "3.8"
services:
  app:
    build: .
`)

	source := fmt.Sprintf("pack:%s@%s", manifest.Name, manifest.Version)
	return packDir, manifest, source
}

// genPackFixtureSimple creates a minimal v2 pack with a template-only generator.
func genPackFixtureSimple(t *testing.T, baseDir string) (string, *packs.Manifest, string) {
	t.Helper()

	packDir := filepath.Join(baseDir, "simple@1.0.0")
	mustMkdirSG(t, packDir)

	manifestYAML := `contract_version: 2
name: simple
version: 1.0.0
generators:
  basic:
    description: "Basic generator"
    steps:
      - type: template
        from: "main.go.tmpl"
        to: "main.go"
`
	mustWriteFileSG(t, filepath.Join(packDir, "go-arch.yaml"), manifestYAML)

	manifest, err := packs.Load(packDir)
	if err != nil {
		t.Fatalf("Load v2 simple pack: %v", err)
	}

	tmplDir := filepath.Join(packDir, "templates")
	mustMkdirSG(t, tmplDir)
	mustWriteFileSG(t, filepath.Join(tmplDir, "main.go.tmpl"),
		`package main
import "fmt"
func main() { fmt.Println("{{ .ProjectName }}") }
`)

	source := fmt.Sprintf("pack:%s@%s", manifest.Name, manifest.Version)
	return packDir, manifest, source
}

func mustMkdirSG(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", path, err)
	}
}

func mustWriteFileSG(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}

// ensureManifestDir creates the .go-arch/ directory with a minimal manifest
// so that manifestDir() returns "." (the CWD) instead of ProjectName.
func ensureManifestDir(t *testing.T, projDir string) {
	t.Helper()
	archDir := filepath.Join(projDir, ".go-arch")
	mustMkdirSG(t, archDir)
	mustWriteFileSG(t, filepath.Join(archDir, "manifest.yaml"),
		"version: 1\nfiles: {}\n")
}

// --------------- GeneratePackGenerator Tests ---------------

func TestGeneratePackGenerator_E2E_TemplateBinaryRun(t *testing.T) {
	tmpDir := t.TempDir()
	packDir, packManifest, source := genPackFixture(t, tmpDir)

	oldWd, _ := os.Getwd()
	projDir := t.TempDir()
	ensureManifestDir(t, projDir)
	if err := os.Chdir(projDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	mustWriteFileSG(t, ".go-arch.yaml", `project_name: TestGen
module_name: github.com/test/gen
architecture: Standard
`)

	config := &ui.ProjectConfig{
		ProjectName:  "TestGen",
		ModuleName:   "github.com/test/gen",
		Architecture: "Standard",
	}

	hooksCfg := &hooks.Config{}
	hr := hooks.NewRunner(hooksCfg, hooks.RealRunner{}, io.Discard)

	pi := packs.PackInfo{Dir: packDir, Manifest: packManifest}
	scaffolder := NewScaffolder(config,
		WithRunner(hr),
		WithPackInfo(pi),
	)

	args := map[string]any{"port": "3000"}
	err := scaffolder.GeneratePackGenerator("docker", args)
	if err != nil {
		t.Fatalf("GeneratePackGenerator: %v", err)
	}

	// Template file must exist.
	dockerfile := filepath.Join(projDir, "Dockerfile")
	data, err := os.ReadFile(dockerfile)
	if err != nil {
		t.Fatalf("ReadFile(Dockerfile): %v", err)
	}
	if !strings.Contains(string(data), "TestGen") {
		t.Errorf("Dockerfile should contain TestGen, got: %s", string(data))
	}

	// Binary file must exist.
	composePath := filepath.Join(projDir, "docker-compose.yml")
	if _, err := os.Stat(composePath); os.IsNotExist(err) {
		t.Fatal("docker-compose.yml should exist")
	}

	// Load manifest and verify provenance.
	m, err := LoadManifest(projDir)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}

	// Template file entry.
	dockerEntry, ok := m.Files["Dockerfile"]
	if !ok {
		t.Fatal("manifest missing Dockerfile entry")
	}
	if dockerEntry.Origin != OriginTemplate {
		t.Errorf("Dockerfile Origin = %q, want %q", dockerEntry.Origin, OriginTemplate)
	}
	if dockerEntry.TemplatePath != "docker/Dockerfile.tmpl" {
		t.Errorf("Dockerfile TemplatePath = %q, want %q", dockerEntry.TemplatePath, "docker/Dockerfile.tmpl")
	}
	if dockerEntry.Source != source {
		t.Errorf("Dockerfile Source = %q, want %q", dockerEntry.Source, source)
	}
	if dockerEntry.Metadata == nil {
		t.Fatal("Dockerfile Metadata is nil")
	}
	if dockerEntry.Metadata["generator"] != "docker" {
		t.Errorf("Dockerfile metadata.generator = %q, want %q",
			dockerEntry.Metadata["generator"], "docker")
	}
	// Args metadata must be JSON.
	argsJSON, ok := dockerEntry.Metadata["args"]
	if !ok {
		t.Fatal("Dockerfile metadata.args missing")
	}
	var decodedArgs map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &decodedArgs); err != nil {
		t.Errorf("metadata.args is not valid JSON: %v (raw: %s)", err, argsJSON)
	}
	if decodedArgs["port"] != "3000" {
		t.Errorf("metadata.args.port = %v, want 3000", decodedArgs["port"])
	}

	// Binary file entry — origin: generator.
	composeEntry, ok := m.Files["docker-compose.yml"]
	if !ok {
		t.Fatal("manifest missing docker-compose.yml entry")
	}
	if composeEntry.Origin != OriginGenerator {
		t.Errorf("docker-compose.yml Origin = %q, want %q", composeEntry.Origin, OriginGenerator)
	}
	if composeEntry.Source != source {
		t.Errorf("docker-compose.yml Source = %q, want %q", composeEntry.Source, source)
	}
	if composeEntry.Metadata["generator"] != "docker" {
		t.Errorf("compose metadata.generator = %q, want %q",
			composeEntry.Metadata["generator"], "docker")
	}
}

func TestGeneratePackGenerator_PreFlightPromptAbort_ZeroWrites(t *testing.T) {
	tmpDir := t.TempDir()
	packDir, packManifest, _ := genPackFixture(t, tmpDir)

	oldWd, _ := os.Getwd()
	projDir := t.TempDir()
	ensureManifestDir(t, projDir)
	if err := os.Chdir(projDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	mustWriteFileSG(t, ".go-arch.yaml", `project_name: TestGen
module_name: github.com/test/gen
architecture: Standard
`)

	config := &ui.ProjectConfig{
		ProjectName:  "TestGen",
		ModuleName:   "github.com/test/gen",
		Architecture: "Standard",
	}

	pi := packs.PackInfo{Dir: packDir, Manifest: packManifest}
	hooksCfg := &hooks.Config{}
	hr := hooks.NewRunner(hooksCfg, hooks.RealRunner{}, io.Discard)
	scaffolder := NewScaffolder(config, WithRunner(hr), WithPackInfo(pi))

	// Empty args — prompt "port" has default "8080", so it resolves fine.
	args := map[string]any{}
	err := scaffolder.GeneratePackGenerator("docker", args)
	if err != nil {
		t.Fatalf("GeneratePackGenerator with defaults: %v", err)
	}

	// Files should exist (prompt resolved with default).
	dockerPath := filepath.Join(projDir, "Dockerfile")
	if _, err := os.Stat(dockerPath); os.IsNotExist(err) {
		t.Fatal("Dockerfile should exist after successful generation")
	}

	// Defaulted prompt value should be in metadata.
	m, err := LoadManifest(projDir)
	if err == nil {
		if entry, ok := m.Files["Dockerfile"]; ok {
			if argsJSON, ok := entry.Metadata["args"]; ok {
				var decoded map[string]any
				if json.Unmarshal([]byte(argsJSON), &decoded) == nil {
					if decoded["port"] != "8080" {
						t.Errorf("default port should be 8080, got %v", decoded["port"])
					}
				}
			}
		}
	}
}

func TestGeneratePackGenerator_PathEscape_ZeroWrites(t *testing.T) {
	tmpDir := t.TempDir()
	packDir := filepath.Join(tmpDir, "escape@1.0.0")
	mustMkdirSG(t, packDir)

	manifestYAML := `contract_version: 2
name: escape
version: 1.0.0
generators:
  bad:
    steps:
      - type: template
        from: "main.go.tmpl"
        to: "/etc/passwd"
`
	mustWriteFileSG(t, filepath.Join(packDir, "go-arch.yaml"), manifestYAML)

	packManifest, err := packs.Load(packDir)
	if err != nil {
		t.Fatalf("Load escape pack: %v", err)
	}

	tmplDir := filepath.Join(packDir, "templates")
	mustMkdirSG(t, tmplDir)
	mustWriteFileSG(t, filepath.Join(tmplDir, "main.go.tmpl"), "package main")

	oldWd, _ := os.Getwd()
	projDir := t.TempDir()
	ensureManifestDir(t, projDir)
	if err := os.Chdir(projDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	mustWriteFileSG(t, ".go-arch.yaml", `project_name: TestGen
module_name: github.com/test/gen
architecture: Standard
`)

	config := &ui.ProjectConfig{
		ProjectName:  "TestGen",
		ModuleName:   "github.com/test/gen",
		Architecture: "Standard",
	}

	pi := packs.PackInfo{Dir: packDir, Manifest: packManifest}
	scaffolder := NewScaffolder(config, WithPackInfo(pi))

	err = scaffolder.GeneratePackGenerator("bad", nil)
	if err == nil {
		t.Fatal("expected error for path escape")
	}
	code := oopsCodeSG(err)
	if code != generators.CodeRecipePathEscape {
		t.Errorf("error code = %q, want %q", code, generators.CodeRecipePathEscape)
	}
}

func TestGeneratePackGenerator_HooksEnabledFalse_RunSkipped_TemplateWritten(t *testing.T) {
	tmpDir := t.TempDir()
	packDir := filepath.Join(tmpDir, "trust@1.0.0")
	mustMkdirSG(t, packDir)

	manifestYAML := `contract_version: 2
name: trust
version: 1.0.0
generators:
  gen:
    steps:
      - type: template
        from: "output.tmpl"
        to: "output.txt"
      - type: run
        command: "echo"
        args: ["hello"]
`
	mustWriteFileSG(t, filepath.Join(packDir, "go-arch.yaml"), manifestYAML)

	packManifest, err := packs.Load(packDir)
	if err != nil {
		t.Fatalf("Load trust pack: %v", err)
	}

	tmplDir := filepath.Join(packDir, "templates")
	mustMkdirSG(t, tmplDir)
	mustWriteFileSG(t, filepath.Join(tmplDir, "output.tmpl"), "hello: {{ .ProjectName }}")

	// Write a sidecar with HooksEnabled=false
	mustWriteFileSG(t, filepath.Join(packDir, "pack.json"),
		`{"hooks_enabled": false, "installed_at": "2025-01-01T00:00:00Z", "module_ref": "trust@1.0.0"}`)

	oldWd, _ := os.Getwd()
	projDir := t.TempDir()
	ensureManifestDir(t, projDir)
	if err := os.Chdir(projDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	mustWriteFileSG(t, ".go-arch.yaml", `project_name: TestTrust
module_name: github.com/test/trust
architecture: Standard
`)

	config := &ui.ProjectConfig{
		ProjectName:  "TestTrust",
		ModuleName:   "github.com/test/trust",
		Architecture: "Standard",
	}

	pi := packs.PackInfo{Dir: packDir, Manifest: packManifest}
	hooksCfg := &hooks.Config{}
	hr := hooks.NewRunner(hooksCfg, hooks.RealRunner{}, io.Discard)
	scaffolder := NewScaffolder(config, WithRunner(hr), WithPackInfo(pi))

	err = scaffolder.GeneratePackGenerator("gen", nil)
	if err != nil {
		t.Fatalf("GeneratePackGenerator with HooksEnabled=false: %v", err)
	}

	// Template file must exist.
	outPath := filepath.Join(projDir, "output.txt")
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile(output.txt): %v", err)
	}
	if !strings.Contains(string(data), "TestTrust") {
		t.Errorf("output.txt should contain TestTrust, got: %s", string(data))
	}

	// Manifest should have the entry.
	m, err := LoadManifest(projDir)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	entry, ok := m.Files["output.txt"]
	if !ok {
		t.Fatal("manifest missing output.txt entry")
	}
	if entry.Origin != OriginTemplate {
		t.Errorf("output.txt Origin = %q, want %q", entry.Origin, OriginTemplate)
	}
	if entry.Metadata["generator"] != "gen" {
		t.Errorf("output.txt metadata.generator = %q, want %q",
			entry.Metadata["generator"], "gen")
	}
}

func TestGeneratePackGenerator_UnknownGenerator(t *testing.T) {
	tmpDir := t.TempDir()
	packDir, packManifest, _ := genPackFixtureSimple(t, tmpDir)

	oldWd, _ := os.Getwd()
	projDir := t.TempDir()
	ensureManifestDir(t, projDir)
	if err := os.Chdir(projDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	mustWriteFileSG(t, ".go-arch.yaml", `project_name: TestGen
module_name: github.com/test/gen
architecture: Standard
`)

	config := &ui.ProjectConfig{
		ProjectName:  "TestGen",
		ModuleName:   "github.com/test/gen",
		Architecture: "Standard",
	}

	pi := packs.PackInfo{Dir: packDir, Manifest: packManifest}
	scaffolder := NewScaffolder(config, WithPackInfo(pi))

	err := scaffolder.GeneratePackGenerator("nonexistent", nil)
	if err == nil {
		t.Fatal("expected error for unknown generator")
	}
	code := oopsCodeSG(err)
	if code != generators.CodeUnknownGenerator {
		t.Errorf("error code = %q, want %q", code, generators.CodeUnknownGenerator)
	}
	if !strings.Contains(err.Error(), "basic") {
		t.Errorf("error should list available generators, got: %v", err)
	}
}

func TestGeneratePackGenerator_TemplateStepMetadata(t *testing.T) {
	tmpDir := t.TempDir()
	packDir, packManifest, source := genPackFixtureSimple(t, tmpDir)

	oldWd, _ := os.Getwd()
	projDir := t.TempDir()
	ensureManifestDir(t, projDir)
	if err := os.Chdir(projDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	mustWriteFileSG(t, ".go-arch.yaml", `project_name: TestMeta
module_name: github.com/test/meta
architecture: Standard
`)

	config := &ui.ProjectConfig{
		ProjectName:  "TestMeta",
		ModuleName:   "github.com/test/meta",
		Architecture: "Standard",
	}

	pi := packs.PackInfo{Dir: packDir, Manifest: packManifest}
	scaffolder := NewScaffolder(config, WithPackInfo(pi))

	args := map[string]any{"mode": "fast"}
	err := scaffolder.GeneratePackGenerator("basic", args)
	if err != nil {
		t.Fatalf("GeneratePackGenerator: %v", err)
	}

	m, err := LoadManifest(projDir)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}

	entry, ok := m.Files["main.go"]
	if !ok {
		t.Fatal("manifest missing main.go entry")
	}
	if entry.Origin != OriginTemplate {
		t.Errorf("main.go Origin = %q, want %q", entry.Origin, OriginTemplate)
	}
	if entry.TemplatePath != "main.go.tmpl" {
		t.Errorf("main.go TemplatePath = %q, want %q", entry.TemplatePath, "main.go.tmpl")
	}
	if entry.Source != source {
		t.Errorf("main.go Source = %q, want %q", entry.Source, source)
	}
	if entry.Metadata["generator"] != "basic" {
		t.Errorf("main.go metadata.generator = %q, want %q",
			entry.Metadata["generator"], "basic")
	}
	argsJSON, ok := entry.Metadata["args"]
	if !ok {
		t.Fatal("main.go metadata.args missing")
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &decoded); err != nil {
		t.Errorf("metadata.args is not valid JSON: %v (raw: %s)", err, argsJSON)
	}
	if decoded["mode"] != "fast" {
		t.Errorf("metadata.args.mode = %v, want fast", decoded["mode"])
	}
}

func TestGeneratePackGenerator_TemplateDataIsolation(t *testing.T) {
	tmpDir := t.TempDir()
	packDir := filepath.Join(tmpDir, "iso@1.0.0")
	mustMkdirSG(t, packDir)

	manifestYAML := `contract_version: 2
name: iso
version: 1.0.0
generators:
  test:
    steps:
      - type: prompt
        name: "secret"
        message: "Secret?"
        default: "TOP_SECRET"
        required: false
      - type: template
        from: "out.tmpl"
        to: "output.txt"
`
	mustWriteFileSG(t, filepath.Join(packDir, "go-arch.yaml"), manifestYAML)

	packManifest, err := packs.Load(packDir)
	if err != nil {
		t.Fatalf("Load iso pack: %v", err)
	}

	tmplDir := filepath.Join(packDir, "templates")
	mustMkdirSG(t, tmplDir)
	mustWriteFileSG(t, filepath.Join(tmplDir, "out.tmpl"), "name: {{ .ProjectName }}")

	oldWd, _ := os.Getwd()
	projDir := t.TempDir()
	ensureManifestDir(t, projDir)
	if err := os.Chdir(projDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	mustWriteFileSG(t, ".go-arch.yaml", `project_name: IsoTest
module_name: github.com/test/iso
architecture: Standard
`)

	config := &ui.ProjectConfig{
		ProjectName:  "IsoTest",
		ModuleName:   "github.com/test/iso",
		Architecture: "Standard",
	}

	pi := packs.PackInfo{Dir: packDir, Manifest: packManifest}
	scaffolder := NewScaffolder(config, WithPackInfo(pi))

	err = scaffolder.GeneratePackGenerator("test", nil)
	if err != nil {
		t.Fatalf("GeneratePackGenerator: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(projDir, "output.txt"))
	if err != nil {
		t.Fatalf("ReadFile(output.txt): %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "IsoTest") {
		t.Errorf("output should contain project name, got: %s", content)
	}
	if strings.Contains(content, "TOP_SECRET") {
		t.Error("prompt value 'TOP_SECRET' leaked into template output — data isolation broken")
	}
	if strings.Contains(content, "secret") {
		t.Error("prompt name 'secret' leaked into template output — data isolation broken")
	}
}

// oopsCodeSG extracts the oops error code from an error using errors.As.
func oopsCodeSG(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	// Check error string for template codes as fallback.
	if strings.Contains(msg, template.CodeGeneratorTemplateNotFound) {
		return template.CodeGeneratorTemplateNotFound
	}

	// Use errors.As to extract OopsError.
	var oErr oops.OopsError
	if errors.As(err, &oErr) {
		if code, ok := oErr.Code().(string); ok {
			return code
		}
	}

	// Fallback: check message for known codes.
	for _, code := range []string{
		string(generators.CodeRecipePathEscape),
		string(generators.CodeUnknownGenerator),
		string(generators.CodePackNotInstalled),
		string(generators.CodeUnknownBuiltin),
		string(generators.CodeMissingGeneratorArgument),
		string(generators.CodeGeneratorPromptUnresolvable),
	} {
		if strings.Contains(msg, code) {
			return code
		}
	}
	return ""
}
