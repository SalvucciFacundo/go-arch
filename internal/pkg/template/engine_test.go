package template

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEngine_Render(t *testing.T) {
	engine := NewEngine()

	data := struct {
		ProjectName      string
		ModuleName       string
		UseObservability bool
		UseGRPC          bool
	}{
		ProjectName:      "TestApp",
		ModuleName:       "github.com/test/app",
		UseObservability: true,
		UseGRPC:          true,
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
	}{
		ModuleName:       "github.com/test/custom",
		UseObservability: false,
		UseGRPC:          false,
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
