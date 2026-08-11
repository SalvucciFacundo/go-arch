package generators

import (
	"errors"
	"strings"
	"testing"

	"github.com/SalvucciFacundo/go-arch/v2/internal/pkg/hooks"
	"github.com/samber/oops"
	"gopkg.in/yaml.v3"
)

func TestStep_UnmarshalYAML_AllStepTypes(t *testing.T) {
	y := `
steps:
  - type: template
    from: common/handler.tmpl
    to: internal/handler/handler.go
  - type: binary
    from: assets/logo.png
    to: static/logo.png
    mode: 0755
  - type: run
    command: go
    args:
      - generate
      - ./...
    timeout: 30s
    ignore_failure: true
  - type: prompt
    name: db_driver
    message: "Database driver?"
    default: postgres
    required: true
  - type: use
    value: builtin/lint
`
	var g Generator
	if err := yaml.Unmarshal([]byte(y), &g); err != nil {
		t.Fatalf("unmarshal all step types: %v", err)
	}

	if len(g.Steps) != 5 {
		t.Fatalf("expected 5 steps, got %d", len(g.Steps))
	}

	// Template step.
	s0 := g.Steps[0]
	if s0.Type != "template" || s0.From != "common/handler.tmpl" || s0.To != "internal/handler/handler.go" {
		t.Errorf("template step: got type=%q from=%q to=%q", s0.Type, s0.From, s0.To)
	}

	// Binary step.
	s1 := g.Steps[1]
	if s1.Type != "binary" || s1.From != "assets/logo.png" || s1.To != "static/logo.png" {
		t.Errorf("binary step: got type=%q from=%q to=%q mode=%o", s1.Type, s1.From, s1.To, s1.Mode)
	}
	if s1.Mode != 0755 {
		t.Errorf("binary step mode = %o, want 0755", s1.Mode)
	}

	// Run step.
	s2 := g.Steps[2]
	if s2.Type != "run" || s2.Command != "go" {
		t.Errorf("run step: got type=%q command=%q", s2.Type, s2.Command)
	}
	if len(s2.Args) != 2 {
		t.Errorf("run step args = %v, want [generate ./...]", s2.Args)
	}
	if s2.Timeout == 0 {
		t.Error("run step timeout should be set to 30s")
	}
	if !s2.IgnoreFailure {
		t.Error("run step ignore_failure should be true")
	}

	// Prompt step.
	s3 := g.Steps[3]
	if s3.Type != "prompt" || s3.Name != "db_driver" || s3.Message != "Database driver?" {
		t.Errorf("prompt step: got type=%q name=%q message=%q", s3.Type, s3.Name, s3.Message)
	}
	if s3.Default != "postgres" {
		t.Errorf("prompt step default = %q, want postgres", s3.Default)
	}
	if !s3.Required {
		t.Error("prompt step required should be true")
	}

	// Use step.
	s4 := g.Steps[4]
	if s4.Type != "use" || s4.Value != "builtin/lint" {
		t.Errorf("use step: got type=%q value=%q", s4.Type, s4.Value)
	}
}

func TestStep_UnmarshalYAML_RunStringShorthand(t *testing.T) {
	y := `
steps:
  - go generate ./...
`
	var g Generator
	if err := yaml.Unmarshal([]byte(y), &g); err != nil {
		t.Fatalf("unmarshal string shorthand: %v", err)
	}
	if len(g.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(g.Steps))
	}
	s := g.Steps[0]
	if s.Type != "run" {
		t.Errorf("string shorthand step type = %q, want run", s.Type)
	}
	if s.Command != "go" {
		t.Errorf("string shorthand command = %q, want go", s.Command)
	}
	if !s.Shell {
		t.Error("string shorthand should set Shell=true")
	}
}

func TestStep_UnmarshalYAML_RunObjectForm(t *testing.T) {
	y := `
steps:
  - type: run
    command: go
    args:
      - mod
      - tidy
    cwd: ./internal
    env:
      GOFLAGS: -mod=mod
    timeout: 2m
    silent: true
`
	var g Generator
	if err := yaml.Unmarshal([]byte(y), &g); err != nil {
		t.Fatalf("unmarshal run object: %v", err)
	}
	s := g.Steps[0]
	if s.Type != "run" {
		t.Errorf("type = %q, want run", s.Type)
	}
	if s.Command != "go" {
		t.Errorf("command = %q, want go", s.Command)
	}
	if len(s.Args) != 2 || s.Args[0] != "mod" || s.Args[1] != "tidy" {
		t.Errorf("args = %v, want [mod tidy]", s.Args)
	}
	if s.Cwd != "./internal" {
		t.Errorf("cwd = %q, want ./internal", s.Cwd)
	}
	if s.Env["GOFLAGS"] != "-mod=mod" {
		t.Errorf("env GOFLAGS = %q", s.Env["GOFLAGS"])
	}
	if s.Timeout == 0 {
		t.Error("timeout should be set")
	}
	if !s.Silent {
		t.Error("silent should be true")
	}
	if s.Shell {
		t.Error("object form should NOT set Shell")
	}
}

func TestValidate_RejectsUnknownStepType(t *testing.T) {
	g := Generator{
		Steps: []Step{
			{Type: "conditional"},
		},
	}
	err := Validate("test", g)
	if err == nil {
		t.Fatal("expected error for unknown step type")
	}
	code := oopsCode(err)
	if code != CodeInvalidPackManifest {
		t.Errorf("code = %q, want %q", code, CodeInvalidPackManifest)
	}
	if !strings.Contains(err.Error(), "unknown step type") || !strings.Contains(err.Error(), "conditional") {
		t.Errorf("error should name 'conditional' as unknown, got: %v", err)
	}
	if !strings.Contains(err.Error(), "step 0") {
		t.Errorf("error should name step 0, got: %v", err)
	}
}

func TestValidate_MissingFromTo(t *testing.T) {
	tests := []struct {
		name string
		s    Step
		miss string
	}{
		{"template missing from", Step{Type: "template", To: "handler.go"}, "from"},
		{"template missing to", Step{Type: "template", From: "handler.tmpl"}, "to"},
		{"binary missing from", Step{Type: "binary", To: "logo.png"}, "from"},
		{"binary missing to", Step{Type: "binary", From: "logo.png"}, "to"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := Generator{Steps: []Step{tt.s}}
			err := Validate("test", g)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.miss) {
				t.Errorf("error should mention %q, got: %v", tt.miss, err)
			}
		})
	}
}

func TestValidate_EmptyRecipe(t *testing.T) {
	g := Generator{Steps: []Step{}}
	err := Validate("test", g)
	if err == nil {
		t.Fatal("expected error for empty recipe")
	}
	code := oopsCode(err)
	if code != CodeInvalidPackManifest {
		t.Errorf("code = %q, want %q", code, CodeInvalidPackManifest)
	}
	if !strings.Contains(err.Error(), "has no steps") {
		t.Errorf("error should mention 'no steps', got: %v", err)
	}
}

func TestValidate_MalformedUseReference(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{"no builtin prefix", "external/docker"},
		{"builtin with no name", "builtin/"},
		{"just builtin", "builtin"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := Generator{Steps: []Step{
				{Type: "use", Value: tt.value},
			}}
			err := Validate("test", g)
			if err == nil {
				t.Fatal("expected error for malformed use reference")
			}
			if !strings.Contains(err.Error(), "builtin/") && !strings.Contains(err.Error(), tt.value) {
				t.Errorf("error should mention builtin reference, got: %v", err)
			}
		})
	}
}

func TestValidate_DuplicatePromptNames(t *testing.T) {
	g := Generator{Steps: []Step{
		{Type: "prompt", Name: "db", Message: "DB?"},
		{Type: "prompt", Name: "db", Message: "DB again?"},
	}}
	err := Validate("test", g)
	if err == nil {
		t.Fatal("expected error for duplicate prompt name")
	}
	if !strings.Contains(err.Error(), "duplicate prompt name") {
		t.Errorf("error should mention duplicate prompt name, got: %v", err)
	}
}

func TestValidate_LinearOnly(t *testing.T) {
	// Steps should be validated in order; no branching/conditionals allowed.
	// This is implicit — the validator processes each step sequentially.
	g := Generator{Steps: []Step{
		{Type: "template", From: "a.tmpl", To: "a.go"},
		{Type: "template", From: "b.tmpl", To: "b.go"},
		{Type: "template", From: "c.tmpl", To: "c.go"},
	}}
	err := Validate("test", g)
	if err != nil {
		t.Fatalf("valid linear recipe should not error: %v", err)
	}
}

func TestValidate_ValidRecipe(t *testing.T) {
	g := Generator{
		Description: "A test generator",
		Steps: []Step{
			{Type: "template", From: "common/handler.tmpl", To: "internal/handler/handler.go"},
			{Type: "run", Command: "go", Args: []string{"mod", "tidy"}},
			{Type: "prompt", Name: "db", Message: "Database?"},
			{Type: "use", Value: "builtin/lint"},
		},
	}
	err := Validate("docker", g)
	if err != nil {
		t.Fatalf("valid recipe should pass: %v", err)
	}
}

func TestGenerator_RunsCommands(t *testing.T) {
	tests := []struct {
		name string
		g    Generator
		want bool
	}{
		{"template-only", Generator{Steps: []Step{{Type: "template"}}}, false},
		{"binary-only", Generator{Steps: []Step{{Type: "binary"}}}, false},
		{"has run step", Generator{Steps: []Step{{Type: "run"}}}, true},
		{"has pre hooks", Generator{Steps: []Step{{Type: "template"}}, Pre: []hooks.Entry{{Command: "echo"}}}, true},
		{"has post hooks", Generator{Steps: []Step{{Type: "template"}}, Post: []hooks.Entry{{Command: "echo"}}}, true},
		{"empty", Generator{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.g.RunsCommands()
			if got != tt.want {
				t.Errorf("RunsCommands() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRecord(t *testing.T) {
	r := Record{
		Path:         "internal/handler/handler.go",
		Origin:       "template",
		Source:       "pack:express@1.2.0",
		TemplatePath: "common/handler.tmpl",
		Metadata: map[string]string{
			"generator": "docker",
			"args":      `{"db_driver":"postgres"}`,
		},
	}
	if r.Path != "internal/handler/handler.go" {
		t.Errorf("Path = %q", r.Path)
	}
	if r.Origin != "template" {
		t.Errorf("Origin = %q", r.Origin)
	}
	if r.Source != "pack:express@1.2.0" {
		t.Errorf("Source = %q", r.Source)
	}
	if r.TemplatePath != "common/handler.tmpl" {
		t.Errorf("TemplatePath = %q", r.TemplatePath)
	}
	if r.Metadata["generator"] != "docker" {
		t.Errorf("Metadata generator = %q", r.Metadata["generator"])
	}
}

func TestRegistry_Empty(t *testing.T) {
	// v2.0 ships with empty registry.
	if len(BuiltinRegistry) != 0 {
		t.Errorf("builtin registry should be empty initially, got %d entries", len(BuiltinRegistry))
	}

	_, err := Lookup("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown builtin")
	}
	code := oopsCode(err)
	if code != CodeUnknownBuiltin {
		t.Errorf("code = %q, want %q", code, CodeUnknownBuiltin)
	}
}

func TestRegistry_RegisterAndLookup(t *testing.T) {
	called := false
	fn := func(g Generator, args map[string]any) ([]Record, error) {
		called = true
		return nil, nil
	}
	Register("test-builtin", fn)

	got, err := Lookup("test-builtin")
	if err != nil {
		t.Fatalf("Lookup should succeed: %v", err)
	}
	if got == nil {
		t.Fatal("got nil function")
	}

	// Call the function to verify it's the right one.
	_, err = got(Generator{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Error("registered function was not the one returned by Lookup")
	}

	// Clean up registry for other tests.
	delete(BuiltinRegistry, "test-builtin")
}

// oopsCode extracts the oops code from an error, or returns "" if not set.
func oopsCode(err error) string {
	var oErr oops.OopsError
	if err == nil {
		return ""
	}
	if errors.As(err, &oErr) {
		if code, ok := oErr.Code().(string); ok {
			return code
		}
	}
	return ""
}
