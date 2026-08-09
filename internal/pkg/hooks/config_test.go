package hooks

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/samber/oops"
)

func TestConfig_Load_MissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent.yaml")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load on missing file should return nil error, got: %v", err)
	}
	if cfg == nil {
		t.Fatal("Load on missing file should return non-nil Config")
	}
	if cfg.Hooks == nil {
		t.Error("returned Config.Hooks should be non-nil map")
	}
	if len(cfg.Hooks) != 0 {
		t.Errorf("returned Config.Hooks should be empty, got %d entries", len(cfg.Hooks))
	}
}

func TestConfig_Load_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".go-arch.yaml")
	if err := os.WriteFile(path, []byte("{}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load on empty file should not error: %v", err)
	}
	if cfg == nil || cfg.Hooks == nil || len(cfg.Hooks) != 0 {
		t.Error("empty YAML should produce empty hooks map")
	}
}

func TestConfig_Load_StringForm(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".go-arch.yaml")
	yaml := `hooks:
  post-generate:
    - "gofmt -w ."`
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load with string form should not error: %v", err)
	}
	entries := cfg.Hooks[PostGenerate]
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	e := entries[0]
	if e.Command != "gofmt" {
		t.Errorf("Command = %q, want %q", e.Command, "gofmt")
	}
	if len(e.Args) != 2 || e.Args[0] != "-w" || e.Args[1] != "." {
		t.Errorf("Args = %v, want [-w .]", e.Args)
	}
	if !e.Shell {
		t.Error("string form should set Shell=true")
	}
}

func TestConfig_Load_ObjectForm(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".go-arch.yaml")
	yaml := `hooks:
  post-generate:
    - command: "go"
      args: ["mod", "tidy"]
      timeout: "60s"`
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load with object form should not error: %v", err)
	}
	entries := cfg.Hooks[PostGenerate]
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	e := entries[0]
	if e.Command != "go" {
		t.Errorf("Command = %q, want %q", e.Command, "go")
	}
	if len(e.Args) != 2 || e.Args[0] != "mod" || e.Args[1] != "tidy" {
		t.Errorf("Args = %v, want [mod tidy]", e.Args)
	}
	if e.Timeout.String() != "1m0s" {
		t.Errorf("Timeout = %v, want 1m0s", e.Timeout)
	}
	if e.Shell {
		t.Error("object form should set Shell=false")
	}
}

func TestConfig_Load_MixedList(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".go-arch.yaml")
	yaml := `hooks:
  post-generate:
    - "gofmt -w ."
    - command: "go"
      args: ["mod", "tidy"]`
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load with mixed list should not error: %v", err)
	}
	entries := cfg.Hooks[PostGenerate]
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if !entries[0].Shell {
		t.Error("first entry (string form) should have Shell=true")
	}
	if entries[1].Shell {
		t.Error("second entry (object form) should have Shell=false")
	}
}

func TestConfig_Load_AllFourTypes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".go-arch.yaml")
	yaml := `hooks:
  pre-new:
    - "echo pre"
  post-new:
    - "echo post"
  pre-generate:
    - "echo pre-gen"
  post-generate:
    - "echo post-gen"`
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load with all four types should not error: %v", err)
	}
	if len(cfg.Hooks) != 4 {
		t.Errorf("expected 4 hook types, got %d", len(cfg.Hooks))
	}
	for _, typ := range []Type{PreNew, PostNew, PreGenerate, PostGenerate} {
		if entries, ok := cfg.Hooks[typ]; !ok || len(entries) != 1 {
			t.Errorf("type %s: got %d entries, want 1", typ, len(entries))
		}
	}
}

func TestConfig_Load_UnknownType(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".go-arch.yaml")
	yaml := `hooks:
  bogus-hook:
    - "echo hi"`
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatal("Load with unknown hook type should return error")
	}
	if !strings.Contains(err.Error(), "unknown hook type") && !strings.Contains(err.Error(), "bogus-hook") {
		t.Errorf("error should mention unknown hook type, got: %v", err)
	}
}

func TestConfig_Load_UnknownObjectKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".go-arch.yaml")
	yaml := `hooks:
  post-generate:
    - command: "gofmt"
      unknown_field: true`
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatal("Load with unknown object key should return error")
	}
	if !strings.Contains(err.Error(), "unknown_field") {
		t.Errorf("error should mention 'unknown_field', got: %v", err)
	}
}

func TestConfig_Load_MissingCommand(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".go-arch.yaml")
	yaml := `hooks:
  post-generate:
    - args: ["-w", "."]`
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatal("Load with missing command should return error")
	}
	if !strings.Contains(err.Error(), "requires a command") && !strings.Contains(err.Error(), "command") {
		t.Errorf("error should mention missing command, got: %v", err)
	}
}

func TestConfig_Load_BadTimeout(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".go-arch.yaml")
	yaml := `hooks:
  post-generate:
    - command: "go"
      timeout: "forever"`
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatal("Load with bad timeout should return error")
	}
	if !strings.Contains(err.Error(), "invalid timeout") && !strings.Contains(err.Error(), "forever") {
		t.Errorf("error should mention invalid timeout, got: %v", err)
	}
}

func TestConfig_Load_TimeoutZeroDisables(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".go-arch.yaml")
	yaml := `hooks:
  post-generate:
    - command: "go"
      timeout: 0`
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load with timeout=0 should not error: %v", err)
	}
	e := cfg.Hooks[PostGenerate][0]
	if e.Timeout != 0 {
		t.Errorf("Timeout = %v, want 0 (disabled)", e.Timeout)
	}
}

func TestConfig_Load_ScalarNotList(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".go-arch.yaml")
	yaml := `hooks:
  pre-new: "echo hi"`
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatal("Load with scalar-instead-of-list should return error")
	}
	if !strings.Contains(err.Error(), "expected a list") && !strings.Contains(err.Error(), "scalar") {
		t.Errorf("error should mention list expected, got: %v", err)
	}
}

func TestConfig_Load_EmptyList(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".go-arch.yaml")
	yaml := `hooks:
  pre-generate: []`
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load with empty list should not error: %v", err)
	}
	if entries, ok := cfg.Hooks[PreGenerate]; !ok || len(entries) != 0 {
		t.Error("empty list should produce empty entry slice")
	}
}

func TestConfig_Load_MissingHooksKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".go-arch.yaml")
	yaml := `project_name: test
architecture: Standard`
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load with missing hooks key should not error: %v", err)
	}
	if cfg == nil || cfg.Hooks == nil || len(cfg.Hooks) != 0 {
		t.Error("missing hooks key should produce empty map")
	}
}

func TestConfig_Load_ExtraKeysIgnored(t *testing.T) {
	// Req 7 backward compatibility: Viper silently ignores unknown top-level
	// keys like "hooks:". This test verifies that Load() parses the hooks
	// section while ignoring other config keys (project_name, architecture,
	// etc.) — they don't leak into hooks parsing and don't cause errors.
	dir := t.TempDir()
	path := filepath.Join(dir, ".go-arch.yaml")
	yaml := `project_name: myapp
module_name: github.com/test/myapp
architecture: Hexagonal
hooks:
  post-generate:
    - "gofmt -w ."`
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load with extra keys should not error: %v", err)
	}
	entries := cfg.Hooks[PostGenerate]
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Command != "gofmt" {
		t.Errorf("Command = %q, want gofmt", entries[0].Command)
	}
	// Extra keys don't appear in hooks config (structurally impossible — rawConfig
	// only has a Hooks field, so yaml.v3 silently discards the rest).
}

func TestResolveConfigPath_DefaultToHome(t *testing.T) {
	path := ResolveConfigPath()
	if path == "" {
		t.Error("ResolveConfigPath should never return empty string")
	}
	if !strings.Contains(path, ".go-arch.yaml") {
		t.Errorf("ResolveConfigPath should point to .go-arch.yaml, got: %s", path)
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

func TestConfig_Load_OopsCodes(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want string
	}{
		{
			name: "unknown hook type",
			yaml: `hooks:
  bogus-hook:
    - "echo hi"`,
			want: CodeUnknownHookType,
		},
		{
			name: "unknown object key",
			yaml: `hooks:
  post-generate:
    - command: "gofmt"
      unknown_field: true`,
			want: CodeInvalidHookConfig,
		},
		{
			name: "missing command",
			yaml: `hooks:
  post-generate:
    - args: ["-w", "."]`,
			want: CodeInvalidHookConfig,
		},
		{
			name: "bad timeout",
			yaml: `hooks:
  post-generate:
    - command: "go"
      timeout: "forever"`,
			want: CodeInvalidHookConfig,
		},
		{
			name: "scalar not list",
			yaml: `hooks:
  pre-new: "echo hi"`,
			want: CodeInvalidHookConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, ".go-arch.yaml")
			if err := os.WriteFile(path, []byte(tt.yaml), 0644); err != nil {
				t.Fatal(err)
			}
			_, err := Load(path)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			got := oopsCode(err)
			if got != tt.want {
				t.Errorf("oops code = %q, want %q", got, tt.want)
			}
		})
	}
}
