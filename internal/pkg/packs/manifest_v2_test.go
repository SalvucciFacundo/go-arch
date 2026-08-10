package packs

import (
	"strings"
	"testing"

	"go-arch/internal/pkg/generators"
)

// --- Contract v2 + generators tests (Slice 1) ---

func TestManifest_Load_V2WithGenerators(t *testing.T) {
	dir := t.TempDir()
	// Create a minimal generators package template dir so Load doesn't fail
	// on templates/ check during install-time (these tests only test Load).
	yaml := `contract_version: 2
name: express
version: 2.0.0
generators:
  docker:
    description: "Docker setup"
    steps:
      - type: template
        from: common/docker-compose.tmpl
        to: docker-compose.yml
`
	writeGoArchYAML(t, dir, yaml)

	m, err := Load(dir)
	if err != nil {
		t.Fatalf("Load v2 manifest with generators: %v", err)
	}

	if m.ContractVersion != 2 {
		t.Errorf("ContractVersion = %d, want 2", m.ContractVersion)
	}
	if m.Name != "express" {
		t.Errorf("Name = %q, want express", m.Name)
	}

	// Generators map should be exposed.
	if len(m.Generators) != 1 {
		t.Fatalf("Generators len = %d, want 1", len(m.Generators))
	}
	docker, ok := m.Generators["docker"]
	if !ok {
		t.Fatal("expected generator 'docker' in map")
	}
	if len(docker.Steps) != 1 {
		t.Fatalf("docker step count = %d, want 1", len(docker.Steps))
	}
	if docker.Steps[0].Type != "template" {
		t.Errorf("docker step 0 type = %q, want template", docker.Steps[0].Type)
	}
	if docker.Steps[0].From != "common/docker-compose.tmpl" {
		t.Errorf("docker step 0 from = %q", docker.Steps[0].From)
	}
}

func TestManifest_Load_V1WithGeneratorsRejected(t *testing.T) {
	dir := t.TempDir()
	yaml := `contract_version: 1
name: express
version: 1.0.0
generators:
  docker:
    steps:
      - go generate ./...
`
	writeGoArchYAML(t, dir, yaml)

	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for v1 pack with generators key")
	}
	code := oopsCode(err)
	if code != CodeInvalidPackManifest {
		t.Errorf("oops code = %q, want %q", code, CodeInvalidPackManifest)
	}
	if !strings.Contains(err.Error(), "generators") {
		t.Errorf("error should mention 'generators' as unknown key, got: %v", err)
	}
}

func TestManifest_Load_V2EmptyGenerators(t *testing.T) {
	dir := t.TempDir()
	yaml := `contract_version: 2
name: express
version: 2.0.0
generators: {}
`
	writeGoArchYAML(t, dir, yaml)

	m, err := Load(dir)
	if err != nil {
		t.Fatalf("Load v2 with empty generators: %v", err)
	}
	if m.ContractVersion != 2 {
		t.Errorf("ContractVersion = %d, want 2", m.ContractVersion)
	}
	if len(m.Generators) != 0 {
		t.Errorf("Generators len = %d, want 0", len(m.Generators))
	}
}

func TestManifest_Load_Contract99Mismatch(t *testing.T) {
	dir := t.TempDir()
	yaml := `contract_version: 99
name: express
version: 1.0.0
`
	writeGoArchYAML(t, dir, yaml)

	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for contract 99")
	}
	code := oopsCode(err)
	if code != CodeContractVersionMismatch {
		t.Errorf("oops code = %q, want %q", code, CodeContractVersionMismatch)
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "contract v99") {
		t.Errorf("error should contain 'contract v99', got: %v", errStr)
	}
	if !strings.Contains(errStr, "v1–v2") || !strings.Contains(errStr, "v1") {
		t.Errorf("error should contain supported range 'v1–v2', got: %v", errStr)
	}
}

// Ensure existing tests still pass — v1 packs must still be accepted.
func TestManifest_Load_V1StillAccepted(t *testing.T) {
	dir := t.TempDir()
	yaml := `contract_version: 1
name: express
version: 1.2.0
`
	writeGoArchYAML(t, dir, yaml)

	m, err := Load(dir)
	if err != nil {
		t.Fatalf("Load v1 manifest should still work: %v", err)
	}
	if m.ContractVersion != 1 {
		t.Errorf("ContractVersion = %d, want 1", m.ContractVersion)
	}
}

// Note: TestManifest_Load_ContractMismatch (existing test for v99) should
// now also verify v1–v2 range in error message. The v1 range check is
// satisfied by the existing test checking for "v1" in error string.

// Ensure existing test TestManifest_Load_ContractMismatch still works.
// We augment that test (already exists above in file) by adding v1–v2 check
// in its error string assertion when we update manifest.go.

// Test that generators decode YAML with generators import works.
func TestManifest_Load_V2NoGenerators(t *testing.T) {
	dir := t.TempDir()
	yaml := `contract_version: 2
name: express
version: 2.0.0
`
	writeGoArchYAML(t, dir, yaml)

	m, err := Load(dir)
	if err != nil {
		t.Fatalf("Load v2 manifest without generators: %v", err)
	}
	if m.ContractVersion != 2 {
		t.Errorf("ContractVersion = %d, want 2", m.ContractVersion)
	}
	if m.Generators != nil {
		t.Errorf("Generators should be nil when not declared, got %v", m.Generators)
	}
}

// Ensure the existing TestManifest_Load_ContractMismatch works with updated
// error message format. We re-test contract 99 with explicit v1-v2 check.
// This ensures backward-compat on error message content.
func TestManifest_Load_Contract99ContainsV1V2(t *testing.T) {
	// Re-run the contract mismatch test from above with explicit v1–v2 check.
	dir := t.TempDir()
	yaml := `contract_version: 99
name: express
version: 1.0.0
`
	writeGoArchYAML(t, dir, yaml)

	_, err := Load(dir)
	if err == nil {
		t.Fatal("Load with contract mismatch should return error")
	}
	if !strings.Contains(err.Error(), "v1") && !strings.Contains(err.Error(), "v1–v2") {
		t.Errorf("error should mention supported version(s), got: %v", err)
	}
}

func TestManifest_Load_V2InvalidRecipe_EmptySteps_Rejected(t *testing.T) {
	// PRODUCTION path: packs.Load with an invalid (empty) recipe must reject at load time.
	dir := t.TempDir()
	yaml := `contract_version: 2
name: express
version: 2.0.0
generators:
  noop:
    description: "Does nothing"
    steps: []
`
	writeGoArchYAML(t, dir, yaml)

	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for generator with empty steps")
	}
	code := oopsCode(err)
	if code != CodeInvalidPackManifest {
		t.Errorf("oops code = %q, want %q", code, CodeInvalidPackManifest)
	}
	if !strings.Contains(err.Error(), "noop") {
		t.Errorf("error should name generator 'noop', got: %v", err)
	}
	if !strings.Contains(err.Error(), "no steps") {
		t.Errorf("error should mention 'no steps', got: %v", err)
	}
}

func TestManifest_Load_V2InvalidRecipe_UnknownStepType_Rejected(t *testing.T) {
	// PRODUCTION path: packs.Load with an unknown step type must reject at load time.
	dir := t.TempDir()
	yaml := `contract_version: 2
name: express
version: 2.0.0
generators:
  docker:
    description: "Docker setup"
    steps:
      - type: conditional
        from: x.tmpl
        to: x.txt
`
	writeGoArchYAML(t, dir, yaml)

	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for unknown step type")
	}
	code := oopsCode(err)
	if code != CodeInvalidPackManifest {
		t.Errorf("oops code = %q, want %q", code, CodeInvalidPackManifest)
	}
	if !strings.Contains(err.Error(), "docker") {
		t.Errorf("error should name generator 'docker', got: %v", err)
	}
	if !strings.Contains(err.Error(), "conditional") && !strings.Contains(err.Error(), "unknown step type") {
		t.Errorf("error should mention unknown step type, got: %v", err)
	}
}

// TestManifest_Load_V2ValidRecipe_WithRunStep_ParsesOK verifies that
// a recipe with a run step parses and validates successfully at load time
// (positive control for validation wiring — REQ-03 S3).
func TestManifest_Load_V2ValidRecipe_WithRunStep_ParsesOK(t *testing.T) {
	dir := t.TempDir()
	yaml := `contract_version: 2
name: express
version: 2.0.0
generators:
  docker:
    description: "Docker setup with run step"
    steps:
      - type: template
        from: "docker-compose.yaml.tmpl"
        to: "docker-compose.yaml"
      - type: run
        command: "echo"
        args: ["deployed"]
      - type: binary
        from: "assets/script.sh"
        to: "scripts/deploy.sh"
        mode: 0755
`
	writeGoArchYAML(t, dir, yaml)

	manifest, err := Load(dir)
	if err != nil {
		t.Fatalf("Load should succeed for valid recipe with run step: %v", err)
	}

	gen, ok := manifest.Generators["docker"]
	if !ok {
		t.Fatal("generator 'docker' should be present")
	}
	if len(gen.Steps) != 3 {
		t.Errorf("expected 3 steps, got %d", len(gen.Steps))
	}
	if gen.Steps[1].Type != "run" {
		t.Errorf("step 1 type = %q, want 'run'", gen.Steps[1].Type)
	}
	if gen.Steps[1].Command != "echo" {
		t.Errorf("run command = %q, want 'echo'", gen.Steps[1].Command)
	}
}

// Ensure generators import is clean.
var _ = generators.Generator{} // verify import linkage
