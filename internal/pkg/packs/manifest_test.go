package packs

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SalvucciFacundo/go-arch/v2/internal/pkg/hooks"
	"github.com/samber/oops"
)

func TestManifest_Load_ValidMinimal(t *testing.T) {
	dir := t.TempDir()
	yaml := `contract_version: 1
name: express
version: 1.2.0
`
	writeGoArchYAML(t, dir, yaml)

	m, err := Load(dir)
	if err != nil {
		t.Fatalf("Load valid minimal manifest: unexpected error: %v", err)
	}
	if m.ContractVersion != 1 {
		t.Errorf("ContractVersion = %d, want 1", m.ContractVersion)
	}
	if m.Name != "express" {
		t.Errorf("Name = %q, want %q", m.Name, "express")
	}
	if m.Version != "1.2.0" {
		t.Errorf("Version = %q, want %q", m.Version, "1.2.0")
	}
	if len(m.Layout) != 0 {
		t.Errorf("Layout = %v, want empty", m.Layout)
	}
	if len(m.Hooks) != 0 {
		t.Errorf("Hooks = %v, want empty", m.Hooks)
	}
	if len(m.BinaryAssets) != 0 {
		t.Errorf("BinaryAssets = %v, want empty", m.BinaryAssets)
	}
}

func TestManifest_Load_ValidFull(t *testing.T) {
	dir := t.TempDir()
	yaml := `contract_version: 1
name: express
version: 1.2.0
layout:
  - cmd
  - internal
hooks:
  post-new:
    - command: go
      args:
        - mod
        - tidy
binary_assets:
  - source: assets/htmx.min.js
    target: static/js/htmx.min.js
`
	writeGoArchYAML(t, dir, yaml)

	m, err := Load(dir)
	if err != nil {
		t.Fatalf("Load valid full manifest: unexpected error: %v", err)
	}
	if m.Name != "express" {
		t.Errorf("Name = %q, want %q", m.Name, "express")
	}
	if m.Version != "1.2.0" {
		t.Errorf("Version = %q, want %q", m.Version, "1.2.0")
	}
	if len(m.Layout) != 2 {
		t.Fatalf("Layout len = %d, want 2", len(m.Layout))
	}
	if m.Layout[0] != "cmd" || m.Layout[1] != "internal" {
		t.Errorf("Layout = %v, want [cmd internal]", m.Layout)
	}
	if len(m.Hooks) != 1 {
		t.Fatalf("Hooks len = %d, want 1", len(m.Hooks))
	}
	entries, ok := m.Hooks[hooks.PostNew]
	if !ok || len(entries) != 1 {
		t.Fatalf("Hooks[PostNew] missing or wrong size")
	}
	if entries[0].Command != "go" {
		t.Errorf("hook command = %q, want go", entries[0].Command)
	}
	if len(m.BinaryAssets) != 1 {
		t.Fatalf("BinaryAssets len = %d, want 1", len(m.BinaryAssets))
	}
	if m.BinaryAssets[0].Source != "assets/htmx.min.js" {
		t.Errorf("BinaryAssets[0].Source = %q", m.BinaryAssets[0].Source)
	}
	if m.BinaryAssets[0].Target != "static/js/htmx.min.js" {
		t.Errorf("BinaryAssets[0].Target = %q", m.BinaryAssets[0].Target)
	}
}

func TestManifest_Load_UnknownKey(t *testing.T) {
	dir := t.TempDir()
	yaml := `contract_version: 1
name: express
version: 1.0.0
bogus_key: "x"
`
	writeGoArchYAML(t, dir, yaml)

	_, err := Load(dir)
	if err == nil {
		t.Fatal("Load with unknown key should return error")
	}
	code := oopsCode(err)
	if code != CodeInvalidPackManifest {
		t.Errorf("oops code = %q, want %q", code, CodeInvalidPackManifest)
	}
	if !strings.Contains(err.Error(), "bogus_key") {
		t.Errorf("error should mention unknown key 'bogus_key', got: %v", err)
	}
}

func TestManifest_Load_MissingName(t *testing.T) {
	dir := t.TempDir()
	yaml := `contract_version: 1
version: 1.0.0
`
	writeGoArchYAML(t, dir, yaml)

	_, err := Load(dir)
	if err == nil {
		t.Fatal("Load with missing name should return error")
	}
	code := oopsCode(err)
	if code != CodeInvalidPackManifest {
		t.Errorf("oops code = %q, want %q", code, CodeInvalidPackManifest)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "name") {
		t.Errorf("error should mention 'name', got: %v", err)
	}
}

func TestManifest_Load_BadSlug(t *testing.T) {
	dir := t.TempDir()
	yaml := `contract_version: 1
name: "Bad Name!"
version: 1.0.0
`
	writeGoArchYAML(t, dir, yaml)

	_, err := Load(dir)
	if err == nil {
		t.Fatal("Load with bad slug should return error")
	}
	code := oopsCode(err)
	if code != CodeInvalidPackManifest {
		t.Errorf("oops code = %q, want %q", code, CodeInvalidPackManifest)
	}
}

func TestManifest_Load_BadSemver(t *testing.T) {
	dir := t.TempDir()
	yaml := `contract_version: 1
name: express
version: "not-a-version"
`
	writeGoArchYAML(t, dir, yaml)

	_, err := Load(dir)
	if err == nil {
		t.Fatal("Load with bad semver should return error")
	}
	code := oopsCode(err)
	if code != CodeInvalidPackManifest {
		t.Errorf("oops code = %q, want %q", code, CodeInvalidPackManifest)
	}
	if !strings.Contains(err.Error(), "version") {
		t.Errorf("error should mention 'version', got: %v", err)
	}
}

func TestManifest_Load_ContractMismatch(t *testing.T) {
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
	code := oopsCode(err)
	if code != CodeContractVersionMismatch {
		t.Errorf("oops code = %q, want %q", code, CodeContractVersionMismatch)
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "contract v99") {
		t.Errorf("error should contain 'contract v99', got: %v", errStr)
	}
	if !strings.Contains(errStr, "v1") {
		t.Errorf("error should contain supported version 'v1', got: %v", errStr)
	}
}

func TestManifest_Load_MissingContract(t *testing.T) {
	dir := t.TempDir()
	yaml := `name: express
version: 1.0.0
`
	writeGoArchYAML(t, dir, yaml)

	_, err := Load(dir)
	if err == nil {
		t.Fatal("Load with missing contract version should return error")
	}
	code := oopsCode(err)
	if code != CodeInvalidPackManifest {
		t.Errorf("oops code = %q, want %q", code, CodeInvalidPackManifest)
	}
}

func TestManifest_Load_MissingFile(t *testing.T) {
	dir := t.TempDir()
	_, err := Load(dir)
	if err == nil {
		t.Fatal("Load with missing go-arch.yaml should return error")
	}
	code := oopsCode(err)
	if code != CodeInvalidPackManifest {
		t.Errorf("oops code = %q, want %q", code, CodeInvalidPackManifest)
	}
}

func TestManifest_Load_MissingVersion(t *testing.T) {
	dir := t.TempDir()
	yaml := `contract_version: 1
name: express
`
	writeGoArchYAML(t, dir, yaml)

	_, err := Load(dir)
	if err == nil {
		t.Fatal("Load with missing version should return error")
	}
	code := oopsCode(err)
	if code != CodeInvalidPackManifest {
		t.Errorf("oops code = %q, want %q", code, CodeInvalidPackManifest)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "version") {
		t.Errorf("error should mention 'version', got: %v", err)
	}
}

// writeGoArchYAML writes a go-arch.yaml file in the given directory.
func writeGoArchYAML(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "go-arch.yaml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// oopsCode extracts the oops code from an error, or returns "" if not set.
func oopsCode(err error) string {
	var oErr oops.OopsError
	if errors.As(err, &oErr) {
		if code, ok := oErr.Code().(string); ok {
			return code
		}
	}
	return ""
}
