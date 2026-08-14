package template

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/samber/oops"
)

func TestEngine_Render(t *testing.T) {
	engine := NewEngine()

	data := struct {
		ProjectName      string
		ModuleName       string
		UseObservability bool
		UseGRPC          bool
		UseTemplHTMX     bool
	}{
		ProjectName:      "TestApp",
		ModuleName:       "github.com/test/app",
		UseObservability: true,
		UseGRPC:          true,
		UseTemplHTMX:     false,
	}

	var buf bytes.Buffer
	err := engine.Render(&buf, "common/go.mod.tmpl", data)

	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	if buf.Len() == 0 {
		t.Error("expected rendered output, got empty buffer")
	}
}

func TestEngine_FuncMap(t *testing.T) {
	engine := NewEngine()
	funcMap := engine.getFuncMap()

	tests := []struct {
		name     string
		funcName string
		input    string
		want     string
	}{
		{
			name:     "lower function",
			funcName: "lower",
			input:    "HELLO",
			want:     "hello",
		},
		{
			name:     "upper function",
			funcName: "upper",
			input:    "world",
			want:     "WORLD",
		},
		{
			name:     "plural regular",
			funcName: "plural",
			input:    "User",
			want:     "Users",
		},
		{
			name:     "plural category",
			funcName: "plural",
			input:    "Category",
			want:     "Categories",
		},
		{
			name:     "plural address",
			funcName: "plural",
			input:    "Address",
			want:     "Addresses",
		},
		{
			name:     "plural person",
			funcName: "plural",
			input:    "Person",
			want:     "People",
		},
		{
			name:     "title function",
			funcName: "title",
			input:    "product",
			want:     "Product",
		},
		{
			name:     "title empty",
			funcName: "title",
			input:    "",
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, ok := funcMap[tt.funcName].(func(string) string)
			if !ok {
				t.Fatalf("function %s not found or has wrong signature", tt.funcName)
			}
			got := f(tt.input)
			if got != tt.want {
				t.Errorf("%s(%q) = %q; want %q", tt.funcName, tt.input, got, tt.want)
			}
		})
	}
}

func TestEngine_Lookup(t *testing.T) {
	engine := NewEngine()

	// Create a temporary folder that simulates the external templates FS
	localTmplDir := filepath.Join(".go-arch", "templates", "common")
	if err := os.MkdirAll(localTmplDir, 0755); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(".go-arch")

	tmplPath := filepath.Join(localTmplDir, "go.mod.tmpl")
	content := "module {{ .ModuleName }}\n// CUSTOM TEMPLATE"
	if err := os.WriteFile(tmplPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	data := struct {
		ModuleName       string
		UseObservability bool
		UseGRPC          bool
		UseTemplHTMX     bool
	}{
		ModuleName:       "github.com/test/custom",
		UseObservability: false,
		UseGRPC:          false,
		UseTemplHTMX:     false,
	}

	var buf bytes.Buffer
	err := engine.Render(&buf, "common/go.mod.tmpl", data)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	if !strings.Contains(buf.String(), "// CUSTOM TEMPLATE") {
		t.Errorf("expected output to contain custom content, got %q", buf.String())
	}
}

func TestEngine_RenderPackOnly(t *testing.T) {
	// Create a temporary pack directory structure.
	packDir := t.TempDir()
	templateDir := filepath.Join(packDir, "templates", "common")
	if err := os.MkdirAll(templateDir, 0755); err != nil {
		t.Fatal(err)
	}
	tmplContent := "module {{ .ModuleName }}\n// PACK TEMPLATE"
	if err := os.WriteFile(filepath.Join(templateDir, "go.mod.tmpl"), []byte(tmplContent), 0644); err != nil {
		t.Fatal(err)
	}

	engine := NewEngine()

	data := struct {
		ModuleName string
	}{
		ModuleName: "github.com/test/packonly",
	}

	t.Run("pack template exists renders from pack", func(t *testing.T) {
		var buf bytes.Buffer
		err := engine.RenderPackOnly(&buf, packDir, "common/go.mod.tmpl", data)
		if err != nil {
			t.Fatalf("RenderPackOnly failed: %v", err)
		}
		got := buf.String()
		if !strings.Contains(got, "PACK TEMPLATE") {
			t.Errorf("expected pack template content, got: %q", got)
		}
		if !strings.Contains(got, "github.com/test/packonly") {
			t.Errorf("expected module name in output, got: %q", got)
		}
	})

	t.Run("missing template returns generator_template_not_found", func(t *testing.T) {
		var buf bytes.Buffer
		err := engine.RenderPackOnly(&buf, packDir, "nonexistent.tmpl", data)
		if err == nil {
			t.Fatal("expected error for missing template, got nil")
		}
		code := oopsCode(err)
		if code != CodeGeneratorTemplateNotFound {
			t.Errorf("error code = %q, want %q; error: %v", code, CodeGeneratorTemplateNotFound, err)
		}
	})

	t.Run("embedded template NOT used as fallback", func(t *testing.T) {
		// common/go.mod.tmpl exists in embedded FS. RenderPackOnly must
		// NOT fall back to it when the pack doesn't have the template.
		emptyPackDir := t.TempDir()
		var buf bytes.Buffer
		err := engine.RenderPackOnly(&buf, emptyPackDir, "common/go.mod.tmpl", data)
		if err == nil {
			t.Fatal("expected error when pack template is missing, but got nil (likely fell back to embedded)")
		}
		code := oopsCode(err)
		if code != CodeGeneratorTemplateNotFound {
			t.Errorf("error code = %q, want %q; error: %v", code, CodeGeneratorTemplateNotFound, err)
		}
	})
}

func TestEngine_RenderTo_Quiet(t *testing.T) {
	engine := NewEngine()

	// Create a local template override to trigger the "custom template" print
	localTmplDir := filepath.Join(".go-arch", "templates", "common")
	if err := os.MkdirAll(localTmplDir, 0755); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(".go-arch")

	tmplPath := filepath.Join(localTmplDir, "go.mod.tmpl")
	if err := os.WriteFile(tmplPath, []byte("module {{ .ModuleName }}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	data := struct{ ModuleName string }{ModuleName: "github.com/test/quiet"}

	t.Run("quiet-false prints to stdout", func(t *testing.T) {
		var buf bytes.Buffer
		// RenderTo with quiet=false should print "Using custom template" to stdout
		// We can't test stdout directly without swapping it, but we verify
		// the method renders correctly and doesn't panic
		err := engine.RenderTo(&buf, "common/go.mod.tmpl", data, false)
		if err != nil {
			t.Fatalf("RenderTo(quiet=false) failed: %v", err)
		}
		want := "module github.com/test/quiet\n"
		if buf.String() != want {
			t.Errorf("RenderTo(quiet=false) = %q, want %q", buf.String(), want)
		}
	})

	t.Run("quiet-true suppresses stdout print", func(t *testing.T) {
		var buf bytes.Buffer
		err := engine.RenderTo(&buf, "common/go.mod.tmpl", data, true)
		if err != nil {
			t.Fatalf("RenderTo(quiet=true) failed: %v", err)
		}
		want := "module github.com/test/quiet\n"
		if buf.String() != want {
			t.Errorf("RenderTo(quiet=true) = %q, want %q", buf.String(), want)
		}
	})

	t.Run("Render delegates to RenderTo quiet=false", func(t *testing.T) {
		var buf bytes.Buffer
		err := engine.Render(&buf, "common/go.mod.tmpl", data)
		if err != nil {
			t.Fatalf("Render failed: %v", err)
		}
		want := "module github.com/test/quiet\n"
		if buf.String() != want {
			t.Errorf("Render = %q, want %q", buf.String(), want)
		}
		// Render should STILL print "Using custom template" to stdout
		// (existing behavior preserved)
	})
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

// TestEngine_DeterministicConfig verifies .go-arch.yaml and config.go render
// byte-identically on repeated renders (no `generated_at: {{ now }}`), so
// upgrade re-render is stable.
func TestEngine_DeterministicConfig(t *testing.T) {
	engine := NewEngine()
	data := map[string]interface{}{
		"ProjectName":      "TestApp",
		"ModuleName":       "github.com/test/app",
		"Architecture":     "Standard",
		"DBDriver":         "PostgreSQL",
		"UseDocker":        true,
		"UseObservability": false,
		"UseGRPC":          false,
		"UseTemplHTMX":     false,
		"GoArchVersion":    "v2.0.2",
	}

	for _, tmpl := range []string{"common/config.tmpl", "common/config_go.tmpl"} {
		var b1, b2 bytes.Buffer
		if err := engine.RenderTo(&b1, tmpl, data, false); err != nil {
			t.Fatalf("%s first render: %v", tmpl, err)
		}
		if err := engine.RenderTo(&b2, tmpl, data, false); err != nil {
			t.Fatalf("%s second render: %v", tmpl, err)
		}
		if b1.String() != b2.String() {
			t.Errorf("%s is non-deterministic (re-render differs)", tmpl)
		}
		if strings.Contains(b1.String(), "generated_at") {
			t.Errorf("%s still contains generated_at (non-deterministic)", tmpl)
		}
		if tmpl == "common/config.tmpl" && !strings.Contains(b1.String(), "scaffold_prod_v1: true") {
			t.Errorf("config.tmpl missing scaffold_prod_v1 marker:\n%s", b1.String())
		}
	}
}
