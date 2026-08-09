package scaffold

import (
	"bytes"
	"go-arch/internal/pkg/template"
	"go-arch/internal/ui"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/samber/oops"
)

func TestScaffolder_Layouts(t *testing.T) {
	tests := []struct {
		name             string
		architecture     string
		expectFiles      []string
		useGRPC          bool
		useObservability bool
	}{
		{
			name:         "Minimalist Architecture",
			architecture: "Minimalist",
			expectFiles:  []string{"main.go", "go.mod", ".go-arch.yaml"},
		},
		{
			name:         "Standard Architecture",
			architecture: "Standard",
			expectFiles:  []string{"cmd/api/main.go", "internal/service", "go.mod"},
		},
		{
			name:         "Hexagonal Architecture",
			architecture: "Hexagonal",
			expectFiles:  []string{"cmd/api/main.go", "internal/domain", "internal/ports", "internal/adapters", "go.mod"},
		},
		{
			name:         "Microservices & Observability",
			architecture: "Hexagonal",
			expectFiles: []string{
				"api/proto/service.proto",
				"internal/adapters/grpc/server.go",
				"internal/telemetry/telemetry.go",
				"Makefile",
				"go.mod",
			},
			useGRPC:          true,
			useObservability: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir, err := os.MkdirTemp("", "scaffold-test-*")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(tempDir)

			oldWd, _ := os.Getwd()
			if err := os.Chdir(tempDir); err != nil {
				t.Fatal(err)
			}
			defer func() { _ = os.Chdir(oldWd) }()

			config := &ui.ProjectConfig{
				ProjectName:      "TestApp",
				ModuleName:       "github.com/test/app",
				Architecture:     tt.architecture,
				UseGRPC:          tt.useGRPC,
				UseObservability: tt.useObservability,
			}

			scaffolder := NewScaffolder(config)
			err = scaffolder.Execute()
			if err != nil {
				t.Fatalf("Execute failed: %v", err)
			}

			for _, f := range tt.expectFiles {
				path := filepath.Join("TestApp", f)
				if _, err := os.Stat(path); os.IsNotExist(err) {
					t.Errorf("expected %s to exist in %s layout", f, tt.architecture)
				}
			}
		})
	}
}

// TestProjectConfig_HasUseTemplHTMX verifies task 1.1: the ProjectConfig struct
// includes a UseTemplHTMX bool field that defaults to false.
func TestProjectConfig_HasUseTemplHTMX(t *testing.T) {
	config := &ui.ProjectConfig{}
	if config.UseTemplHTMX != false {
		t.Errorf("UseTemplHTMX should default to false, got %v", config.UseTemplHTMX)
	}
}

// TestScaffolder_FlagOFFNoWebDirs verifies task 4.1: when UseTemplHTMX=false,
// no web-related directories or files are created (Regression).
func TestScaffolder_FlagOFFNoWebDirs(t *testing.T) {
	architectures := []string{"Minimalist", "Standard", "Hexagonal"}
	for _, arch := range architectures {
		t.Run(arch+"_OFF", func(t *testing.T) {
			tempDir, err := os.MkdirTemp("", "scaffold-off-*")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(tempDir)

			oldWd, _ := os.Getwd()
			os.Chdir(tempDir)
			defer os.Chdir(oldWd)

			config := &ui.ProjectConfig{
				ProjectName:  "TestApp",
				ModuleName:   "github.com/test/app",
				Architecture: arch,
			}

			scaffolder := NewScaffolder(config)
			if err := scaffolder.Execute(); err != nil {
				t.Fatalf("Execute failed for %s: %v", arch, err)
			}

			// No web-specific directories should exist
			webDirs := []string{"views", "static"}
			for _, d := range webDirs {
				path := filepath.Join("TestApp", d)
				if _, err := os.Stat(path); err == nil {
					t.Errorf("expected %s to NOT exist when UseTemplHTMX=false in %s layout", d, arch)
				}
			}

			// No internal/handler/page.go (the web-specific handler file)
			// Standard legitimately creates internal/handler/ dir (scaffold.go:76),
			// but the page.go file must be absent for ALL architectures.
			pageGo := filepath.Join("TestApp", "internal", "handler", "page.go")
			if _, err := os.Stat(pageGo); err == nil {
				t.Errorf("expected internal/handler/page.go to NOT exist when UseTemplHTMX=false in %s layout", arch)
			}
		})
	}
}

// TestGoMod_TemplConditionalOff verifies task 1.2/4.5: when UseTemplHTMX=false,
// go.mod does NOT contain a templ require block.
func TestGoMod_TemplConditionalOff(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "scaffold-gomod-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(oldWd)

	config := &ui.ProjectConfig{
		ProjectName:  "TestApp",
		ModuleName:   "github.com/test/app",
		Architecture: "Minimalist",
	}

	scaffolder := NewScaffolder(config)
	if err := scaffolder.Execute(); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	goModPath := filepath.Join("TestApp", "go.mod")
	content, err := os.ReadFile(goModPath)
	if err != nil {
		t.Fatalf("cannot read go.mod: %v", err)
	}

	if strings.Contains(string(content), "github.com/a-h/templ") {
		t.Errorf("go.mod should NOT contain github.com/a-h/templ when UseTemplHTMX=false")
	}
}

// TestConfig_TemplRoundTripOff verifies task 1.3: .go-arch.yaml contains
// use_templ_htmx: false when the flag is OFF.
func TestConfig_TemplRoundTripOff(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "scaffold-config-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(oldWd)

	config := &ui.ProjectConfig{
		ProjectName:  "TestApp",
		ModuleName:   "github.com/test/app",
		Architecture: "Minimalist",
	}

	scaffolder := NewScaffolder(config)
	if err := scaffolder.Execute(); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	yamlPath := filepath.Join("TestApp", ".go-arch.yaml")
	content, err := os.ReadFile(yamlPath)
	if err != nil {
		t.Fatalf("cannot read .go-arch.yaml: %v", err)
	}

	if !strings.Contains(string(content), "use_templ_htmx:") {
		t.Errorf(".go-arch.yaml must contain use_templ_htmx field; got:\n%s", content)
	}

	if !strings.Contains(string(content), "use_templ_htmx: false") {
		t.Errorf(".go-arch.yaml must contain use_templ_htmx: false; got:\n%s", content)
	}
}

// TestHexagonalBuildFix_OFF verifies task 4.6/1.4: a freshly scaffolded
// Hexagonal project with UseTemplHTMX=false compiles cleanly with go build ./...
func TestHexagonalBuildFix_OFF(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "scaffold-hex-build-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(oldWd)

	config := &ui.ProjectConfig{
		ProjectName:  "TestHex",
		ModuleName:   "github.com/test/hexapp",
		Architecture: "Hexagonal",
	}

	scaffolder := NewScaffolder(config)
	if err := scaffolder.Execute(); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	projectDir := filepath.Join(tempDir, "TestHex")
	os.Chdir(projectDir)

	// go mod tidy resolves dependencies and populates go.sum
	tidy := exec.Command("go", "mod", "tidy")
	tidyOut, err := tidy.CombinedOutput()
	if err != nil {
		t.Fatalf("go mod tidy failed: %v\nOutput: %s", err, tidyOut)
	}

	// go build ./... must pass (the actual hex fix verification)
	build := exec.Command("go", "build", "./...")
	buildOut, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("go build failed (hex build fix regression): %v\nOutput: %s", err, buildOut)
	}
}

// TestHexagonalMainTemplate_NoEmptyImports verifies task 1.4: the rendered
// hexagonal main.go does NOT import internal/adapters or internal/domain
// (those dirs are empty on fresh scaffold and break the build).
func TestHexagonalMainTemplate_NoEmptyImports(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "scaffold-hex-main-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(oldWd)

	config := &ui.ProjectConfig{
		ProjectName:  "TestHex",
		ModuleName:   "github.com/test/hexapp",
		Architecture: "Hexagonal",
	}

	scaffolder := NewScaffolder(config)
	if err := scaffolder.Execute(); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	mainPath := filepath.Join("TestHex", "cmd", "api", "main.go")
	content, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatalf("cannot read main.go: %v", err)
	}

	contentStr := string(content)
	for _, badImport := range []string{
		`"github.com/test/hexapp/internal/adapters"`,
		`"github.com/test/hexapp/internal/domain"`,
		`internal/adapters`,
		`internal/domain`,
	} {
		if strings.Contains(contentStr, badImport) {
			t.Errorf("hexagonal main.go must not import empty %s package; got:\n%s", badImport, contentStr)
		}
	}
}

// TestScaffolder_FlagONWebFiles verifies task 4.2: when UseTemplHTMX=true,
// all web-related directories and files exist for all architectures.
func TestScaffolder_FlagONWebFiles(t *testing.T) {
	architectures := []string{"Minimalist", "Standard", "Hexagonal"}
	for _, arch := range architectures {
		t.Run(arch+"_ON", func(t *testing.T) {
			tempDir, err := os.MkdirTemp("", "scaffold-on-*")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(tempDir)

			oldWd, _ := os.Getwd()
			os.Chdir(tempDir)
			defer os.Chdir(oldWd)

			config := &ui.ProjectConfig{
				ProjectName:  "TestApp",
				ModuleName:   "github.com/test/app",
				Architecture: arch,
				UseTemplHTMX: true,
			}

			scaffolder := NewScaffolder(config)
			if err := scaffolder.Execute(); err != nil {
				t.Fatalf("Execute failed for %s: %v", arch, err)
			}

			// Web directories must exist
			webDirs := []string{
				"views/layouts", "views/pages", "views/components",
				"static/css", "static/js",
			}
			for _, d := range webDirs {
				path := filepath.Join("TestApp", d)
				if info, err := os.Stat(path); err != nil || !info.IsDir() {
					t.Errorf("expected directory %s to exist when UseTemplHTMX=true in %s layout", d, arch)
				}
			}

			// Web files must exist
			webFiles := []string{
				"views/layouts/base.templ",
				"views/pages/home.templ",
				"views/components/counter.templ",
				"static/css/style.css",
				"static/js/htmx.min.js",
				"internal/handler/page.go",
				"README.md",
			}
			for _, f := range webFiles {
				path := filepath.Join("TestApp", f)
				if _, err := os.Stat(path); os.IsNotExist(err) {
					t.Errorf("expected file %s to exist when UseTemplHTMX=true in %s layout", f, arch)
				}
			}
		})
	}
}

// TestScaffolder_WebMainPathContent verifies task 4.3: web main path is
// correct per architecture and contains expected content/imports.
func TestScaffolder_WebMainPathContent(t *testing.T) {
	archTests := []struct {
		arch       string
		expectPath string // relative to TestApp
	}{
		{"Minimalist", "main.go"},
		{"Standard", "cmd/api/main.go"},
		{"Hexagonal", "cmd/api/main.go"},
	}

	for _, tt := range archTests {
		t.Run(tt.arch+"_main", func(t *testing.T) {
			tempDir, err := os.MkdirTemp("", "scaffold-main-*")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(tempDir)

			oldWd, _ := os.Getwd()
			os.Chdir(tempDir)
			defer os.Chdir(oldWd)

			config := &ui.ProjectConfig{
				ProjectName:  "TestApp",
				ModuleName:   "github.com/test/app",
				Architecture: tt.arch,
				UseTemplHTMX: true,
			}

			scaffolder := NewScaffolder(config)
			if err := scaffolder.Execute(); err != nil {
				t.Fatalf("Execute failed for %s: %v", tt.arch, err)
			}

			mainPath := filepath.Join("TestApp", tt.expectPath)
			content, err := os.ReadFile(mainPath)
			if err != nil {
				t.Fatalf("cannot read web main at %s: %v", tt.expectPath, err)
			}

			contentStr := string(content)

			// Must contain: http.ListenAndServe and internal/handler
			if !strings.Contains(contentStr, "http.ListenAndServe") {
				t.Errorf("web main must contain http.ListenAndServe; got:\n%s", contentStr)
			}
			if !strings.Contains(contentStr, "internal/handler") {
				t.Errorf("web main must import internal/handler; got:\n%s", contentStr)
			}

			// Must NOT contain: bare internal/adapters or internal/domain import paths
			// (the hex bug imports empty dirs — subpackages like .../adapters/grpc are fine)
			for _, badImport := range []string{
				`"github.com/test/app/internal/adapters"`,
				`"github.com/test/app/internal/domain"`,
			} {
				if strings.Contains(contentStr, badImport) {
					t.Errorf("web main must NOT import bare %s (empty package); got:\n%s", badImport, contentStr)
				}
			}
		})
	}
}

// TestScaffolder_HtmxByteIdentity verifies task 4.4: embedded htmx.min.js
// is byte-identical to the generated file (binary copy, not rendered).
func TestScaffolder_HtmxByteIdentity(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "scaffold-htmx-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(oldWd)

	config := &ui.ProjectConfig{
		ProjectName:  "TestApp",
		ModuleName:   "github.com/test/app",
		Architecture: "Minimalist",
		UseTemplHTMX: true,
	}

	scaffolder := NewScaffolder(config)
	if err := scaffolder.Execute(); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Read the embedded source
	embedded, err := template.TemplatesFS.ReadFile("templates/web/htmx.min.js")
	if err != nil {
		t.Fatalf("cannot read embedded htmx.min.js: %v", err)
	}

	// Read the generated file
	generated, err := os.ReadFile(filepath.Join("TestApp", "static", "js", "htmx.min.js"))
	if err != nil {
		t.Fatalf("cannot read generated htmx.min.js: %v", err)
	}

	// Must be byte-identical
	if !bytes.Equal(embedded, generated) {
		t.Errorf("embedded and generated htmx.min.js are NOT byte-identical")
		t.Errorf("embedded: %d bytes, generated: %d bytes", len(embedded), len(generated))
	}
}

// TestScaffolder_ConfigAndContentRoundTrip verifies task 4.5:
// - .go-arch.yaml contains use_templ_htmx: true
// - go.mod contains github.com/a-h/templ
// - README.md contains templ generate instructions + BSD-2-Clause
// - counter.templ contains htmx attributes (hx-post, hx-target, hx-swap)
func TestScaffolder_ConfigAndContentRoundTrip(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "scaffold-roundtrip-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(oldWd)

	config := &ui.ProjectConfig{
		ProjectName:  "TestApp",
		ModuleName:   "github.com/test/app",
		Architecture: "Minimalist",
		UseTemplHTMX: true,
	}

	scaffolder := NewScaffolder(config)
	if err := scaffolder.Execute(); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// 1. Config round-trip: .go-arch.yaml must contain use_templ_htmx: true
	yamlPath := filepath.Join("TestApp", ".go-arch.yaml")
	yamlContent, err := os.ReadFile(yamlPath)
	if err != nil {
		t.Fatalf("cannot read .go-arch.yaml: %v", err)
	}
	yamlStr := string(yamlContent)
	if !strings.Contains(yamlStr, "use_templ_htmx: true") {
		t.Errorf(".go-arch.yaml must contain use_templ_htmx: true; got:\n%s", yamlStr)
	}

	// 2. go.mod must contain templ require
	goModPath := filepath.Join("TestApp", "go.mod")
	goModContent, err := os.ReadFile(goModPath)
	if err != nil {
		t.Fatalf("cannot read go.mod: %v", err)
	}
	goModStr := string(goModContent)
	if !strings.Contains(goModStr, "github.com/a-h/templ") {
		t.Errorf("go.mod must contain github.com/a-h/templ when UseTemplHTMX=true; got:\n%s", goModStr)
	}

	// 3. README.md must contain templ generate instructions + BSD-2-Clause
	readmePath := filepath.Join("TestApp", "README.md")
	readmeContent, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("cannot read README.md: %v", err)
	}
	readmeStr := string(readmeContent)
	if !strings.Contains(readmeStr, "templ generate") {
		t.Errorf("README.md must contain 'templ generate' instructions; got:\n%s", readmeStr)
	}
	if !strings.Contains(readmeStr, "BSD-2-Clause") {
		t.Errorf("README.md must contain BSD-2-Clause htmx attribution; got:\n%s", readmeStr)
	}

	// 4. counter.templ must contain htmx attributes
	counterPath := filepath.Join("TestApp", "views", "components", "counter.templ")
	counterContent, err := os.ReadFile(counterPath)
	if err != nil {
		t.Fatalf("cannot read counter.templ: %v", err)
	}
	counterStr := string(counterContent)
	for _, attr := range []string{"hx-post", "hx-target", "hx-swap"} {
		if !strings.Contains(counterStr, attr) {
			t.Errorf("counter.templ must contain %s attribute; got:\n%s", attr, counterStr)
		}
	}
}

// TestHexagonalBuildFix_ON verifies task 4.7: a freshly scaffolded Hexagonal
// project with UseTemplHTMX=true builds cleanly after templ generate.
// If the `templ` binary is not in PATH, the test is skipped (as designed).
func TestHexagonalBuildFix_ON(t *testing.T) {
	if _, err := exec.LookPath("templ"); err != nil {
		t.Skip("templ binary not installed — skipping ON build test (requires templ generate)")
	}

	tempDir, err := os.MkdirTemp("", "scaffold-hex-on-build-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(oldWd)

	config := &ui.ProjectConfig{
		ProjectName:  "TestHexWeb",
		ModuleName:   "github.com/test/hexweb",
		Architecture: "Hexagonal",
		UseTemplHTMX: true,
	}

	scaffolder := NewScaffolder(config)
	if err := scaffolder.Execute(); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	projectDir := filepath.Join(tempDir, "TestHexWeb")
	os.Chdir(projectDir)

	// templ generate must run FIRST: creates Go source in views/ so those
	// directories become valid Go packages. go mod tidy needs them to exist.
	gen := exec.Command("templ", "generate")
	if genOut, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("templ generate failed: %v\nOutput: %s", err, genOut)
	}

	// go mod tidy resolves the templ runtime dep (added by generated Go code)
	tidy := exec.Command("go", "mod", "tidy")
	if tidyOut, err := tidy.CombinedOutput(); err != nil {
		t.Fatalf("go mod tidy failed: %v\nOutput: %s", err, tidyOut)
	}

	// go build ./... must pass (hex + web combined build)
	build := exec.Command("go", "build", "./...")
	if buildOut, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build failed (hex+ON web build verification): %v\nOutput: %s", err, buildOut)
	}
}

// TestHandlerFunctional verifies task 4.8: the generated handler/page.go
// serves GET / (200 + counter markup) and POST /counter (200 + "1"/"2")
// proving sync.Mutex-guarded state persists across invocations.
// If the `templ` binary is not in PATH, the test is skipped.
func TestHandlerFunctional(t *testing.T) {
	if _, err := exec.LookPath("templ"); err != nil {
		t.Skip("templ binary not installed — skipping handler functional test (requires templ generate)")
	}

	tempDir, err := os.MkdirTemp("", "scaffold-handler-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(oldWd)

	config := &ui.ProjectConfig{
		ProjectName:  "TestHandler",
		ModuleName:   "github.com/test/handlerapp",
		Architecture: "Minimalist",
		UseTemplHTMX: true,
	}

	scaffolder := NewScaffolder(config)
	if err := scaffolder.Execute(); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	projectDir := filepath.Join(tempDir, "TestHandler")

	// templ generate must run FIRST to create views/*/..._templ.go
	gen := exec.Command("templ", "generate")
	gen.Dir = projectDir
	if genOut, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("templ generate failed: %v\nOutput: %s", err, genOut)
	}

	// go mod tidy resolves dependencies (templ runtime, etc.)
	tidy := exec.Command("go", "mod", "tidy")
	tidy.Dir = projectDir
	if tidyOut, err := tidy.CombinedOutput(); err != nil {
		t.Fatalf("go mod tidy failed: %v\nOutput: %s", err, tidyOut)
	}

	// Write a test file in the generated project that exercises the handler
	// using httptest (same process, no HTTP server needed).
	testFile := filepath.Join(projectDir, "internal", "handler", "page_test.go")
	testContent := `package handler

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// TestHandlerFunctional verifies the full handler contract in a single test
// function so package-level counter state starts from 0.
func TestHandlerFunctional(t *testing.T) {
	// GET / → 200 + counter markup
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	PageHandler(w, r)
	if w.Code != 200 {
		t.Errorf("GET /: expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Count:") {
		t.Errorf("GET /: expected body to contain 'Count:' counter markup, got: %s", body)
	}

	// First POST /counter → 200 + "Count: 1" (first increment from 0)
	w1 := httptest.NewRecorder()
	r1 := httptest.NewRequest("POST", "/counter", nil)
	CounterHandler(w1, r1)
	if w1.Code != 200 {
		t.Errorf("POST /counter: expected 200, got %d", w1.Code)
	}
	body1 := w1.Body.String()
	if !strings.Contains(body1, "Count: 1") {
		t.Errorf("first POST /counter: expected body to contain 'Count: 1', got: %s", body1)
	}

	// Second POST /counter → 200 + "Count: 2" (state persists)
	w2 := httptest.NewRecorder()
	r2 := httptest.NewRequest("POST", "/counter", nil)
	CounterHandler(w2, r2)
	if w2.Code != 200 {
		t.Errorf("second POST /counter: expected 200, got %d", w2.Code)
	}
	body2 := w2.Body.String()
	if !strings.Contains(body2, "Count: 2") {
		t.Errorf("second POST /counter: expected body to contain 'Count: 2', got: %s", body2)
	}
}
`
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("cannot write handler test file: %v", err)
	}

	// Run go test in the generated project's handler package
	testCmd := exec.Command("go", "test", "-v", "./internal/handler/...")
	testCmd.Dir = projectDir
	testOut, err := testCmd.CombinedOutput()
	outStr := string(testOut)
	t.Logf("handler test output:\n%s", outStr)
	if err != nil {
		t.Fatalf("handler functional test failed: %v\nOutput: %s", err, outStr)
	}
}

// ──────────────────────────────────────────────────────────
// Phase 2 — page/component generation (Strict TDD)
// ──────────────────────────────────────────────────────────

// oopsCode extracts the oops code from an error, or returns "".
func oopsCode(err error) string {
	if oe, ok := oops.AsOops(err); ok {
		code := oe.Code()
		if s, ok := code.(string); ok {
			return s
		}
	}
	return ""
}

// TestIsValidGoIdentifier verifies task 2.1: the helper rejects invalid names
// and accepts valid CamelCase / lowercase Go identifiers.
func TestIsValidGoIdentifier(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"CamelCase Dashboard", "Dashboard", true},
		{"CamelCase UserCard", "UserCard", true},
		{"lowercase dashboard", "dashboard", true},
		{"hyphenated user-card", "user-card", false},
		{"leading digit 123Name", "123Name", false},
		{"empty string", "", false},
		{"keyword if", "if", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidGoIdentifier(tt.input)
			if got != tt.want {
				t.Errorf("isValidGoIdentifier(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// TestGenerateComponent_Page verifies task 2.2: generating a page in a web
// project produces the expected file with correct content.
func TestGenerateComponent_Page(t *testing.T) {
	tests := []struct {
		name      string
		inputName string
		wantFile  string
		wantFunc  string
	}{
		{"CamelCase Dashboard", "Dashboard", "views/pages/dashboard.templ", "templ Dashboard()"},
		{"lowercase dashboard", "dashboard", "views/pages/dashboard.templ", "templ Dashboard()"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir, err := os.MkdirTemp("", "page-gen-*")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(tmpDir)

			oldWd, _ := os.Getwd()
			os.Chdir(tmpDir)
			defer os.Chdir(oldWd)

			config := &ui.ProjectConfig{
				ProjectName:  ".",
				ModuleName:   "github.com/test/pageapp",
				Architecture: "Standard",
				UseTemplHTMX: true,
			}

			scaffolder := NewScaffolder(config)
			err = scaffolder.GenerateComponent("page", tt.inputName)
			if err != nil {
				t.Fatalf("GenerateComponent page %s failed: %v", tt.inputName, err)
			}

			fullPath := filepath.Join(tmpDir, tt.wantFile)
			content, err := os.ReadFile(fullPath)
			if err != nil {
				t.Fatalf("expected file %s to exist: %v", tt.wantFile, err)
			}

			contentStr := string(content)
			for _, want := range []string{"package pages", tt.wantFunc, "@layouts.Base(0)"} {
				if !strings.Contains(contentStr, want) {
					t.Errorf("expected %s to contain %q; got:\n%s", tt.wantFile, want, contentStr)
				}
			}
		})
	}
}

// TestGenerateComponent_Component verifies task 2.3: generating a component
// in a web project produces the expected file with hx-* attributes.
func TestGenerateComponent_Component(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "comp-gen-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	config := &ui.ProjectConfig{
		ProjectName:  ".",
		ModuleName:   "github.com/test/compapp",
		Architecture: "Standard",
		UseTemplHTMX: true,
	}

	scaffolder := NewScaffolder(config)
	err = scaffolder.GenerateComponent("component", "UserCard")
	if err != nil {
		t.Fatalf("GenerateComponent component failed: %v", err)
	}

	wantFile := "views/components/usercard.templ"
	fullPath := filepath.Join(tmpDir, wantFile)
	content, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatalf("expected file %s to exist: %v", wantFile, err)
	}

	contentStr := string(content)
	for _, want := range []string{
		"package components",
		"templ UserCard()",
		"hx-get=",
	} {
		if !strings.Contains(contentStr, want) {
			t.Errorf("expected %s to contain %q; got:\n%s", wantFile, want, contentStr)
		}
	}
}

// TestGenerateComponent_Guards verifies task 2.4: gate, name validation, and
// collision checks work correctly for page/component types.
func TestGenerateComponent_Guards(t *testing.T) {
	t.Run("flag off rejected", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "guard-off-*")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(tmpDir)

		oldWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(oldWd)

		config := &ui.ProjectConfig{
			ProjectName:  ".",
			ModuleName:   "github.com/test/guardoff",
			Architecture: "Standard",
			UseTemplHTMX: false,
		}

		scaffolder := NewScaffolder(config)
		err = scaffolder.GenerateComponent("page", "Dashboard")
		if err == nil {
			t.Fatal("expected error when UseTemplHTMX is false, got nil")
		}
		if code := oopsCode(err); code != "web_scaffold_required" {
			t.Errorf("expected oops code web_scaffold_required, got %q; err: %v", code, err)
		}

		targetFile := filepath.Join(tmpDir, "views", "pages", "dashboard.templ")
		if _, statErr := os.Stat(targetFile); statErr == nil {
			t.Errorf("expected no file written when gate rejects; found %s", targetFile)
		}
	})

	t.Run("invalid name rejected", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "guard-name-*")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(tmpDir)

		oldWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(oldWd)

		config := &ui.ProjectConfig{
			ProjectName:  ".",
			ModuleName:   "github.com/test/guardname",
			Architecture: "Standard",
			UseTemplHTMX: true,
		}

		scaffolder := NewScaffolder(config)
		err = scaffolder.GenerateComponent("component", "user-card")
		if err == nil {
			t.Fatal("expected error for invalid name, got nil")
		}
		if code := oopsCode(err); code != "invalid_component_name" {
			t.Errorf("expected oops code invalid_component_name, got %q; err: %v", code, err)
		}

		targetFile := filepath.Join(tmpDir, "views", "components", "user-card.templ")
		if _, statErr := os.Stat(targetFile); statErr == nil {
			t.Errorf("expected no file written when name invalid; found %s", targetFile)
		}
	})

	t.Run("collision rejected", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "guard-collision-*")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(tmpDir)

		oldWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(oldWd)

		// Pre-create the target directories + file
		pagesDir := filepath.Join(tmpDir, "views", "pages")
		if err := os.MkdirAll(pagesDir, 0755); err != nil {
			t.Fatal(err)
		}
		existingContent := []byte("existing content")
		targetFile := filepath.Join(pagesDir, "dashboard.templ")
		if err := os.WriteFile(targetFile, existingContent, 0644); err != nil {
			t.Fatal(err)
		}

		config := &ui.ProjectConfig{
			ProjectName:  ".",
			ModuleName:   "github.com/test/guardcollision",
			Architecture: "Standard",
			UseTemplHTMX: true,
		}

		scaffolder := NewScaffolder(config)
		err = scaffolder.GenerateComponent("page", "Dashboard")
		if err == nil {
			t.Fatal("expected error for collision, got nil")
		}
		if code := oopsCode(err); code != "component_already_exists" {
			t.Errorf("expected oops code component_already_exists, got %q; err: %v", code, err)
		}

		// Original file must be unchanged
		contentAfter, err := os.ReadFile(targetFile)
		if err != nil {
			t.Fatalf("cannot re-read existing file: %v", err)
		}
		if !bytes.Equal(existingContent, contentAfter) {
			t.Errorf("existing file was modified; want %q, got %q", existingContent, contentAfter)
		}
	})

	t.Run("scaffold-shipped home.templ protected", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "guard-home-*")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(tmpDir)

		oldWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(oldWd)

		// Scaffold a full web project
		config := &ui.ProjectConfig{
			ProjectName:  "GuardHomeApp",
			ModuleName:   "github.com/test/guardhome",
			Architecture: "Standard",
			UseTemplHTMX: true,
		}

		scaffolder := NewScaffolder(config)
		if err := scaffolder.Execute(); err != nil {
			t.Fatalf("Execute scaffold failed: %v", err)
		}

		projDir := filepath.Join(tmpDir, "GuardHomeApp")
		os.Chdir(projDir)

		homePath := "views/pages/home.templ"
		originalContent, err := os.ReadFile(homePath)
		if err != nil {
			t.Fatalf("cannot read original home.templ: %v", err)
		}

		// Re-create scaffolder with "." since we chdir'd into the project
		scaffolder2 := NewScaffolder(&ui.ProjectConfig{
			ProjectName:  ".",
			ModuleName:   "github.com/test/guardhome",
			Architecture: "Standard",
			UseTemplHTMX: true,
		})
		err = scaffolder2.GenerateComponent("page", "Home")
		if err == nil {
			t.Fatal("expected collision error for scaffold-shipped home.templ, got nil")
		}
		if code := oopsCode(err); code != "component_already_exists" {
			t.Errorf("expected oops code component_already_exists, got %q; err: %v", code, err)
		}

		// Original home.templ must be byte-identical
		contentAfter, err := os.ReadFile(homePath)
		if err != nil {
			t.Fatalf("cannot re-read home.templ: %v", err)
		}
		if !bytes.Equal(originalContent, contentAfter) {
			t.Errorf("scaffold-shipped home.templ was modified! original: %d bytes, after: %d bytes",
				len(originalContent), len(contentAfter))
		}
	})

	t.Run("backend service unchanged", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "guard-backend-*")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(tmpDir)

		oldWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(oldWd)

		config := &ui.ProjectConfig{
			ProjectName:  ".",
			ModuleName:   "github.com/test/guardbackend",
			Architecture: "Standard",
			UseTemplHTMX: false,
		}

		scaffolder := NewScaffolder(config)
		err = scaffolder.GenerateComponent("service", "Order")
		if err != nil {
			t.Fatalf("backend service generation should still work: %v", err)
		}

		if _, statErr := os.Stat(filepath.Join(tmpDir, "internal", "service", "Order_service.go")); statErr != nil {
			t.Errorf("expected backend service file to be created: %v", statErr)
		}
	})
}

func TestScaffolder_CRUD(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "crud-integration-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(oldWd)

	// Mocking a project root
	config := &ui.ProjectConfig{
		ProjectName:  ".",
		ModuleName:   "github.com/test/crud",
		Architecture: "Hexagonal",
	}

	scaffolder := NewScaffolder(config)
	err = scaffolder.GenerateCRUD("User")
	if err != nil {
		t.Fatal(err)
	}

	expectedFiles := []string{
		"internal/domain/User.go",
		"internal/adapters/User_handler.go",
	}

	for _, f := range expectedFiles {
		if _, err := os.Stat(f); os.IsNotExist(err) {
			t.Errorf("expected crud file %s to exist", f)
		}
	}
}
