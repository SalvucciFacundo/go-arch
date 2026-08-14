package scaffold

import (
	"bytes"
	"github.com/SalvucciFacundo/go-arch/v2/internal/pkg/hooks"
	"github.com/SalvucciFacundo/go-arch/v2/internal/pkg/template"
	"github.com/SalvucciFacundo/go-arch/v2/internal/ui"
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
// Phase 2 — manifest recording (upgrade-project, strict TDD)
// ──────────────────────────────────────────────────────────

// TestManifest_NewRecordsScaffoldEntries verifies task 1.6: scaffolding a new
// project records every createFile and createBinaryFile write in the manifest
// with matching sha256 hashes.
func TestManifest_NewRecordsScaffoldEntries(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "manifest-new-*")
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
		Architecture:     "Minimalist",
		UseTemplHTMX:     true,
		UseObservability: true,
		UseGRPC:          true,
		UseDocker:        true,
	}

	scaffolder := NewScaffolder(config)
	if err := scaffolder.Execute(); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	projectRoot := filepath.Join(tempDir, "TestApp")
	manifestPath := ManifestPath(projectRoot)
	if !ManifestExists(projectRoot) {
		t.Fatalf("manifest not found at %s", manifestPath)
	}

	m, err := LoadManifest(projectRoot)
	if err != nil {
		t.Fatalf("LoadManifest failed: %v", err)
	}

	if len(m.Files) == 0 {
		t.Fatal("manifest has no entries after scaffold")
	}

	// Every file written by createFile or createBinaryFile must have a matching
	// manifest entry with correct sha256.
	for path, entry := range m.Files {
		fullPath := filepath.Join(projectRoot, path)
		diskHash, hashErr := hashFile(fullPath)
		if hashErr != nil {
			t.Errorf("cannot hash %s: %v", fullPath, hashErr)
			continue
		}
		if entry.SHA256 != diskHash {
			t.Errorf("%s: manifest sha256 %q != disk sha256 %q", path, entry.SHA256, diskHash)
		}

		// Check origin classification
		if strings.HasSuffix(path, "htmx.min.js") {
			if entry.Origin != OriginBinary {
				t.Errorf("%s: origin = %q, want binary", path, entry.Origin)
			}
		} else {
			if entry.Origin != OriginScaffold {
				t.Errorf("%s: origin = %q, want scaffold", path, entry.Origin)
			}
		}
	}

	// Verify specific expected files are in the manifest
	expectedFiles := []string{
		"main.go",
		"go.mod",
		".go-arch.yaml",
		".env.example",
		".gitignore",
		"static/js/htmx.min.js",
	}
	for _, f := range expectedFiles {
		if _, ok := m.Files[f]; !ok {
			t.Errorf("expected file %q in manifest, but not found", f)
		}
	}

	t.Logf("Manifest recorded %d entries", len(m.Files))
}

// TestManifest_GenerateComponentRecords verifies that GenerateComponent
// records entries with origin: component and metadata entity_name.
func TestManifest_GenerateComponentRecords(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "manifest-comp-*")
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
	err = scaffolder.GenerateComponent("page", "Dashboard")
	if err != nil {
		t.Fatalf("GenerateComponent failed: %v", err)
	}

	m, err := LoadManifest(".")
	if err != nil {
		t.Fatalf("LoadManifest failed: %v", err)
	}

	targetPath := "views/pages/dashboard.templ"
	entry, ok := m.Files[targetPath]
	if !ok {
		t.Fatalf("expected manifest entry for %s", targetPath)
	}
	if entry.Origin != OriginComponent {
		t.Errorf("entry origin = %q, want component", entry.Origin)
	}
	if entry.Metadata["entity_name"] != "Dashboard" {
		t.Errorf("metadata entity_name = %q, want Dashboard", entry.Metadata["entity_name"])
	}

	// Verify sha256 matches disk
	diskHash, err := hashFile(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	if entry.SHA256 != diskHash {
		t.Errorf("manifest sha256 %q != disk sha256 %q", entry.SHA256, diskHash)
	}
}

// TestManifest_GenerateCRUDRecords verifies that GenerateCRUD records entries
// with origin: crud and metadata entity_name for all generated files.
func TestManifest_GenerateCRUDRecords(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "manifest-crud-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	config := &ui.ProjectConfig{
		ProjectName:  ".",
		ModuleName:   "github.com/test/crudapp",
		Architecture: "Hexagonal",
	}

	scaffolder := NewScaffolder(config)
	err = scaffolder.GenerateCRUD("Order")
	if err != nil {
		t.Fatalf("GenerateCRUD failed: %v", err)
	}

	m, err := LoadManifest(".")
	if err != nil {
		t.Fatalf("LoadManifest failed: %v", err)
	}

	if len(m.Files) == 0 {
		t.Fatal("manifest has no entries after GenerateCRUD")
	}

	for targetPath, entry := range m.Files {
		if entry.Origin != OriginCrud {
			t.Errorf("%s: origin = %q, want crud", targetPath, entry.Origin)
		}
		if entry.Metadata["entity_name"] != "Order" {
			t.Errorf("%s: metadata entity_name = %q, want Order", targetPath, entry.Metadata["entity_name"])
		}

		diskHash, hashErr := hashFile(targetPath)
		if hashErr != nil {
			t.Errorf("cannot hash %s: %v", targetPath, hashErr)
			continue
		}
		if entry.SHA256 != diskHash {
			t.Errorf("%s: manifest sha256 %q != disk sha256 %q", targetPath, entry.SHA256, diskHash)
		}
	}

	t.Logf("GenerateCRUD recorded %d entries", len(m.Files))
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

// ──────────────────────────────────────────────────────────
// Phase 2 — route registry & manifestDir (Strict TDD)
// ──────────────────────────────────────────────────────────

// TestManifestDir_CWD verifies task 2.1: manifestDir() returns "."
// when a manifest exists in the current working directory.
func TestManifestDir_CWD(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "manifestdir-cwd-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	// Create a manifest in CWD to simulate generate context.
	if err := os.MkdirAll(".go-arch", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ManifestPath("."), []byte("version: 1\n"), 0644); err != nil {
		t.Fatal(err)
	}

	config := &ui.ProjectConfig{
		ProjectName:  "realapp",
		ModuleName:   "github.com/test/realapp",
		Architecture: "Standard",
	}
	scaffolder := NewScaffolder(config)
	got := scaffolder.manifestDir()
	want := "."
	if got != want {
		t.Errorf("manifestDir() = %q, want %q", got, want)
	}
}

// TestManifestDir_New verifies task 2.1: manifestDir() returns ProjectName
// when no manifest exists in CWD (new context).
func TestManifestDir_New(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "manifestdir-new-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	// No manifest in CWD → new project context
	config := &ui.ProjectConfig{
		ProjectName:  "myapp",
		ModuleName:   "github.com/test/myapp",
		Architecture: "Standard",
	}
	scaffolder := NewScaffolder(config)
	got := scaffolder.manifestDir()
	want := "myapp"
	if got != want {
		t.Errorf("manifestDir() = %q, want %q", got, want)
	}
}

// TestManifestDir_NestedPathFix verifies task 2.1 (Fix 1): when generate runs
// in a directory where project_name differs from the actual directory name,
// files are written to CWD (not nested under project_name).
func TestManifestDir_NestedPathFix(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "nested-fix-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	// Set up a real project layout with a manifest and project_name: realapp.
	// The key bug was that files ended up at realapp/internal/handler/...
	// when they should be at ./internal/handler/...
	if err := os.MkdirAll(".go-arch", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ManifestPath("."), []byte("version: 1\n"), 0644); err != nil {
		t.Fatal(err)
	}

	config := &ui.ProjectConfig{
		ProjectName:  "realapp",
		ModuleName:   "github.com/test/nested",
		Architecture: "Standard",
	}
	scaffolder := NewScaffolder(config)
	err = scaffolder.GenerateComponent("service", "User")
	if err != nil {
		t.Fatalf("GenerateComponent failed: %v", err)
	}

	// The handler file must be at CWD, NOT nested under realapp/
	correctPath := filepath.Join(tmpDir, "internal", "service", "User_service.go")
	if _, err := os.Stat(correctPath); os.IsNotExist(err) {
		t.Errorf("file should exist at CWD path: %s", correctPath)
	}

	// The nested path must NOT exist
	nestedPath := filepath.Join(tmpDir, "realapp", "internal", "service", "User_service.go")
	if _, err := os.Stat(nestedPath); err == nil {
		t.Errorf("file should NOT exist at nested path: %s", nestedPath)
	}
}

// ──────────────────────────────────────────────────────────
// Task 2.2 — RoutesData + renderRoutesRegistry (Strict TDD)
// ──────────────────────────────────────────────────────────

// TestRoutesData_IsStruct verifies that RoutesData is a usable struct
// embedding ModuleName, Architecture, and Routes fields for the template.
func TestRoutesData_IsStruct(t *testing.T) {
	rd := RoutesData{
		ModuleName:   "github.com/test/app",
		Architecture: "Standard",
		Routes: []RouteEntry{
			{Entity: "User", Handler: "User", Origin: "crud"},
		},
	}
	if rd.ModuleName != "github.com/test/app" {
		t.Errorf("ModuleName = %q", rd.ModuleName)
	}
	if rd.Architecture != "Standard" {
		t.Errorf("Architecture = %q", rd.Architecture)
	}
	if len(rd.Routes) != 1 {
		t.Errorf("len(Routes) = %d, want 1", len(rd.Routes))
	}
}

// TestRenderRoutesRegistry_Standard verifies task 2.2: renderRoutesRegistry
// creates routes.go with handler.NewUserHandler().Register(mux) for Standard.
func TestRenderRoutesRegistry_Standard(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "routes-registry-std-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	// Create a manifest with a CRUD route
	if err := os.MkdirAll(".go-arch", 0755); err != nil {
		t.Fatal(err)
	}
	m := &Manifest{
		Version: 1,
		Files:   make(map[string]ManifestEntry),
		Routes: []RouteEntry{
			{Entity: "User", Handler: "User", Origin: "crud"},
		},
		dir: ".",
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}

	config := &ui.ProjectConfig{
		ProjectName:  ".",
		ModuleName:   "github.com/test/routesapp",
		Architecture: "Standard",
		UseTemplHTMX: true,
	}
	scaffolder := NewScaffolder(config)
	err = scaffolder.renderRoutesRegistry()
	if err != nil {
		t.Fatalf("renderRoutesRegistry failed: %v", err)
	}

	content, err := os.ReadFile("internal/router/routes.go")
	if err != nil {
		t.Fatalf("routes.go not created: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, `handler.NewUserHandler().Register(mux)`) {
		t.Errorf("routes.go should contain handler.NewUserHandler().Register(mux); got:\n%s", contentStr)
	}
	if !strings.Contains(contentStr, `"github.com/test/routesapp/internal/handler"`) {
		t.Errorf("routes.go should import internal/handler; got:\n%s", contentStr)
	}
}

// TestRenderRoutesRegistry_Hexagonal verifies task 2.2: renderRoutesRegistry
// creates routes.go with adapters.NewUserHandler().Register(mux) for Hexagonal.
func TestRenderRoutesRegistry_Hexagonal(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "routes-registry-hex-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	// Create a manifest with a CRUD route
	if err := os.MkdirAll(".go-arch", 0755); err != nil {
		t.Fatal(err)
	}
	m := &Manifest{
		Version: 1,
		Files:   make(map[string]ManifestEntry),
		Routes: []RouteEntry{
			{Entity: "User", Handler: "User", Origin: "crud"},
		},
		dir: ".",
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}

	config := &ui.ProjectConfig{
		ProjectName:  ".",
		ModuleName:   "github.com/test/hexroutes",
		Architecture: "Hexagonal",
		UseTemplHTMX: true,
	}
	scaffolder := NewScaffolder(config)
	err = scaffolder.renderRoutesRegistry()
	if err != nil {
		t.Fatalf("renderRoutesRegistry failed: %v", err)
	}

	content, err := os.ReadFile("internal/router/routes.go")
	if err != nil {
		t.Fatalf("routes.go not created: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, `adapters.NewUserHandler().Register(mux)`) {
		t.Errorf("routes.go should contain adapters.NewUserHandler().Register(mux); got:\n%s", contentStr)
	}
	if !strings.Contains(contentStr, `"github.com/test/hexroutes/internal/adapters"`) {
		t.Errorf("routes.go should import internal/adapters; got:\n%s", contentStr)
	}
}

// TestRenderRoutesRegistry_Deterministic verifies task 2.2: re-rendering
// with the same manifest produces byte-identical output.
func TestRenderRoutesRegistry_Deterministic(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "routes-det-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	if err := os.MkdirAll(".go-arch", 0755); err != nil {
		t.Fatal(err)
	}
	m := &Manifest{
		Version: 1,
		Files:   make(map[string]ManifestEntry),
		Routes: []RouteEntry{
			{Entity: "User", Handler: "User", Origin: "crud"},
			{Entity: "Order", Handler: "Order", Origin: "handler", RoutePattern: "GET /orders"},
		},
		dir: ".",
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}

	config := &ui.ProjectConfig{
		ProjectName:  ".",
		ModuleName:   "github.com/test/det",
		Architecture: "Standard",
		UseTemplHTMX: true,
	}
	scaffolder := NewScaffolder(config)
	if err := scaffolder.renderRoutesRegistry(); err != nil {
		t.Fatalf("first renderRoutesRegistry failed: %v", err)
	}

	first, err := os.ReadFile("internal/router/routes.go")
	if err != nil {
		t.Fatal(err)
	}

	// Second render must be byte-identical
	if err := scaffolder.renderRoutesRegistry(); err != nil {
		t.Fatalf("second renderRoutesRegistry failed: %v", err)
	}
	second, err := os.ReadFile("internal/router/routes.go")
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(first, second) {
		t.Errorf("re-render produced different output\nfirst: %s\nsecond: %s", first, second)
	}
}

// TestRenderRoutesRegistry_Empty verifies that empty routes produce a
// valid routes.go with no imports and empty Register body.
func TestRenderRoutesRegistry_Empty(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "routes-empty-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	if err := os.MkdirAll(".go-arch", 0755); err != nil {
		t.Fatal(err)
	}
	m := &Manifest{
		Version: 1,
		Files:   make(map[string]ManifestEntry),
		Routes:  []RouteEntry{}, // empty
		dir:     ".",
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}

	config := &ui.ProjectConfig{
		ProjectName:  ".",
		ModuleName:   "github.com/test/empty",
		Architecture: "Standard",
		UseTemplHTMX: true,
	}
	scaffolder := NewScaffolder(config)
	if err := scaffolder.renderRoutesRegistry(); err != nil {
		t.Fatalf("renderRoutesRegistry failed: %v", err)
	}

	content, err := os.ReadFile("internal/router/routes.go")
	if err != nil {
		t.Fatalf("routes.go not created: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "package router") {
		t.Errorf("routes.go should have package router; got:\n%s", contentStr)
	}
	if !strings.Contains(contentStr, "func Register(mux *http.ServeMux) {") {
		t.Errorf("routes.go should have func Register; got:\n%s", contentStr)
	}
	// Empty routes → no import block
	if strings.Contains(contentStr, "internal/handler") {
		t.Errorf("empty routes.go should NOT import internal/handler")
	}
}

// ──────────────────────────────────────────────────────────
// Task 2.6 — scaffoldWeb empty routes.go (Strict TDD)
// ──────────────────────────────────────────────────────────

// TestScaffoldWeb_EmptyRoutesGo verifies task 2.6 (Fix 3):
// scaffoldWeb creates internal/router/routes.go with empty route list
// so the project compiles immediately.
func TestScaffoldWeb_EmptyRoutesGo(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "web-empty-routes-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	config := &ui.ProjectConfig{
		ProjectName:  "WebEmptyRoutes",
		ModuleName:   "github.com/test/webempty",
		Architecture: "Minimalist",
		UseTemplHTMX: true,
	}
	scaffolder := NewScaffolder(config)
	if err := scaffolder.Execute(); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// routes.go must exist under the project directory
	routesPath := filepath.Join(tmpDir, "WebEmptyRoutes", "internal", "router", "routes.go")
	content, err := os.ReadFile(routesPath)
	if err != nil {
		t.Fatalf("routes.go not created: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "package router") {
		t.Errorf("routes.go should have package router; got:\n%s", contentStr)
	}
	if !strings.Contains(contentStr, "func Register(mux *http.ServeMux)") {
		t.Errorf("routes.go should have func Register; got:\n%s", contentStr)
	}
	// Empty list → no import
	if strings.Contains(contentStr, "internal/handler") {
		t.Errorf("empty routes.go should NOT import internal/handler")
	}
	// Internal router dir must exist
	routerDir := filepath.Join(tmpDir, "WebEmptyRoutes", "internal", "router")
	if _, err := os.Stat(routerDir); os.IsNotExist(err) {
		t.Errorf("internal/router directory not created")
	}
}

// TestScaffoldWeb_EmptyRoutesGoCompiles verifies task 2.6 (Fix 3): a
// non-web project should NOT create routes.go.
func TestScaffoldWeb_EmptyRoutesGoCompiles(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "web-build-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	config := &ui.ProjectConfig{
		ProjectName:  "WebBuild",
		ModuleName:   "github.com/test/webbuild",
		Architecture: "Minimalist",
		UseTemplHTMX: false,
	}
	scaffolder := NewScaffolder(config)
	if err := scaffolder.Execute(); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Non-web project — routes.go should NOT exist
	routesPath := filepath.Join(tmpDir, "WebBuild", "internal", "router", "routes.go")
	if _, err := os.Stat(routesPath); err == nil {
		t.Errorf("non-web project should NOT have routes.go")
	}
}

// ──────────────────────────────────────────────────────────
// Task 2.3 — GenerateComponent variadic options + WithRoute (Strict TDD)
// ──────────────────────────────────────────────────────────

// TestGenerateComponent_VariadicBackward verifies task 2.3: existing callers
// with zero options continue working unchanged.
func TestGenerateComponent_VariadicBackward(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "var-back-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	config := &ui.ProjectConfig{
		ProjectName:  ".",
		ModuleName:   "github.com/test/varback",
		Architecture: "Standard",
	}
	scaffolder := NewScaffolder(config)
	err = scaffolder.GenerateComponent("service", "Order")
	if err != nil {
		t.Fatalf("GenerateComponent with zero options failed: %v", err)
	}

	if _, err := os.Stat("internal/service/Order_service.go"); os.IsNotExist(err) {
		t.Error("expected Order_service.go to be created")
	}
}

// TestGenerateComponent_WithRoute_WebScaffoldRequired verifies task 2.3 (Fix 7):
// handler --route in non-web project returns web_scaffold_required.
func TestGenerateComponent_WithRoute_WebScaffoldRequired(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "var-wsr-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	config := &ui.ProjectConfig{
		ProjectName:  ".",
		ModuleName:   "github.com/test/varwsr",
		Architecture: "Standard",
		UseTemplHTMX: false,
	}
	scaffolder := NewScaffolder(config)
	err = scaffolder.GenerateComponent("handler", "Stats", WithRoute("GET /stats"))
	if err == nil {
		t.Fatal("expected error for --route in non-web, got nil")
	}
	if code := oopsCode(err); code != "web_scaffold_required" {
		t.Errorf("expected oops code web_scaffold_required, got %q; err: %v", code, err)
	}
}

// TestGenerateComponent_WithRoute_InvalidPattern verifies task 2.3 (Fix 7):
// handler --route with bad pattern returns invalid_route_pattern.
func TestGenerateComponent_WithRoute_InvalidPattern(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "var-ip-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	config := &ui.ProjectConfig{
		ProjectName:  ".",
		ModuleName:   "github.com/test/varip",
		Architecture: "Standard",
		UseTemplHTMX: true,
	}
	scaffolder := NewScaffolder(config)
	err = scaffolder.GenerateComponent("handler", "X", WithRoute("BADPATTERN"))
	if err == nil {
		t.Fatal("expected error for bad pattern, got nil")
	}
	if code := oopsCode(err); code != "invalid_route_pattern" {
		t.Errorf("expected oops code invalid_route_pattern, got %q; err: %v", code, err)
	}
}

// TestGenerateComponent_WithRoute_Registers verifies task 2.3 (Fix 5):
// handler --route "GET /stats" creates routes.go with mux.HandleFunc.
func TestGenerateComponent_WithRoute_Registers(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "var-reg-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	// Setup a manifest
	if err := os.MkdirAll(".go-arch", 0755); err != nil {
		t.Fatal(err)
	}
	m := &Manifest{Version: 1, Files: make(map[string]ManifestEntry), dir: "."}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}

	config := &ui.ProjectConfig{
		ProjectName:  ".",
		ModuleName:   "github.com/test/reg",
		Architecture: "Standard",
		UseTemplHTMX: true,
	}
	scaffolder := NewScaffolder(config)
	err = scaffolder.GenerateComponent("handler", "Stats", WithRoute("GET /stats"))
	if err != nil {
		t.Fatalf("GenerateComponent with WithRoute failed: %v", err)
	}

	// Verify handler file exists
	if _, err := os.Stat("internal/handler/Stats_handler.go"); os.IsNotExist(err) {
		t.Error("expected Stats_handler.go to be created")
	}

	// Verify routes.go has mux.HandleFunc line
	routesContent, err := os.ReadFile("internal/router/routes.go")
	if err != nil {
		t.Fatalf("routes.go not created: %v", err)
	}
	routesStr := string(routesContent)
	if !strings.Contains(routesStr, `mux.HandleFunc("GET /stats"`) {
		t.Errorf("routes.go should contain mux.HandleFunc for GET /stats; got:\n%s", routesStr)
	}
}

// TestGenerateComponent_WithoutRoute_RegistryUnchanged verifies task 2.3:
// handler without --route leaves routes.go byte-identical.
func TestGenerateComponent_WithoutRoute_RegistryUnchanged(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "var-noreg-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	if err := os.MkdirAll(".go-arch", 0755); err != nil {
		t.Fatal(err)
	}
	m := &Manifest{Version: 1, Files: make(map[string]ManifestEntry), dir: "."}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}

	config := &ui.ProjectConfig{
		ProjectName:  ".",
		ModuleName:   "github.com/test/noreg",
		Architecture: "Standard",
		UseTemplHTMX: true,
	}
	scaffolder := NewScaffolder(config)

	// Render empty routes.go first to establish baseline
	if err := scaffolder.renderRoutesRegistry(); err != nil {
		t.Fatal(err)
	}
	baseline, err := os.ReadFile("internal/router/routes.go")
	if err != nil {
		t.Fatal(err)
	}

	// Generate handler without --route
	err = scaffolder.GenerateComponent("handler", "Stats")
	if err != nil {
		t.Fatalf("GenerateComponent without route failed: %v", err)
	}

	// routes.go must be byte-identical to baseline
	after, err := os.ReadFile("internal/router/routes.go")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(baseline, after) {
		t.Errorf("routes.go changed after handler generate without --route\nbaseline:\n%s\nafter:\n%s", baseline, after)
	}
}

// ──────────────────────────────────────────────────────────
// Task 2.5 — GenerateCRUD web wiring (Strict TDD)
// ──────────────────────────────────────────────────────────

// TestGenerateCRUD_WebRegistry verifies task 2.5: GenerateCRUD in web project
// upserts route and re-renders routes.go with NewXHandler().Register(mux).
func TestGenerateCRUD_WebRegistry(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "crud-webreg-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	if err := os.MkdirAll(".go-arch", 0755); err != nil {
		t.Fatal(err)
	}
	m := &Manifest{Version: 1, Files: make(map[string]ManifestEntry), dir: "."}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}

	config := &ui.ProjectConfig{
		ProjectName:  ".",
		ModuleName:   "github.com/test/crudweb",
		Architecture: "Standard",
		UseTemplHTMX: true,
	}
	scaffolder := NewScaffolder(config)
	err = scaffolder.GenerateCRUD("User")
	if err != nil {
		t.Fatalf("GenerateCRUD failed: %v", err)
	}

	// routes.go must exist with handler.NewUserHandler().Register(mux)
	routesContent, err := os.ReadFile("internal/router/routes.go")
	if err != nil {
		t.Fatalf("routes.go not created: %v", err)
	}
	routesStr := string(routesContent)
	if !strings.Contains(routesStr, `handler.NewUserHandler().Register(mux)`) {
		t.Errorf("routes.go should contain handler.NewUserHandler().Register(mux); got:\n%s", routesStr)
	}

	// Manifest must have the route entry
	m2, err := LoadManifest(".")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range m2.Routes {
		if r.Entity == "User" && r.Origin == "crud" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("manifest should contain User crud route entry")
	}
}

// TestGenerateCRUD_NonWeb_HintOnly verifies task 2.5: GenerateCRUD in non-web
// project does NOT create routes.go.
func TestGenerateCRUD_NonWeb_HintOnly(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "crud-nonweb-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	if err := os.MkdirAll(".go-arch", 0755); err != nil {
		t.Fatal(err)
	}
	m := &Manifest{Version: 1, Files: make(map[string]ManifestEntry), dir: "."}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}

	config := &ui.ProjectConfig{
		ProjectName:  ".",
		ModuleName:   "github.com/test/nonweb",
		Architecture: "Standard",
		UseTemplHTMX: false,
	}
	scaffolder := NewScaffolder(config)
	err = scaffolder.GenerateCRUD("User")
	if err != nil {
		t.Fatalf("GenerateCRUD failed: %v", err)
	}

	// No routes.go should exist
	if _, err := os.Stat("internal/router/routes.go"); err == nil {
		t.Error("non-web project should NOT have routes.go after CRUD")
	}
}

// TestGenerateCRUD_Idempotent verifies task 2.5: CRUD twice → one entry.
func TestGenerateCRUD_Idempotent(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "crud-idem-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	if err := os.MkdirAll(".go-arch", 0755); err != nil {
		t.Fatal(err)
	}
	m := &Manifest{Version: 1, Files: make(map[string]ManifestEntry), dir: "."}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}

	config := &ui.ProjectConfig{
		ProjectName:  ".",
		ModuleName:   "github.com/test/idem",
		Architecture: "Standard",
		UseTemplHTMX: true,
	}
	scaffolder := NewScaffolder(config)
	if err := scaffolder.GenerateCRUD("User"); err != nil {
		t.Fatalf("first GenerateCRUD failed: %v", err)
	}
	if err := scaffolder.GenerateCRUD("User"); err != nil {
		t.Fatalf("second GenerateCRUD failed: %v", err)
	}

	// routes.go should have exactly one NewUserHandler().Register(mux)
	routesContent, err := os.ReadFile("internal/router/routes.go")
	if err != nil {
		t.Fatalf("routes.go not created: %v", err)
	}
	routesStr := string(routesContent)
	count := strings.Count(routesStr, "NewUserHandler().Register(mux)")
	if count != 1 {
		t.Errorf("expected exactly 1 NewUserHandler().Register(mux), got %d; content:\n%s", count, routesStr)
	}
}

// ──────────────────────────────────────────────────────────
// Task 2.4 — isValidRoutePattern (Strict TDD)
// ──────────────────────────────────────────────────────────

// TestIsValidRoutePattern verifies task 2.4: route pattern validation.
func TestIsValidRoutePattern(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		want    bool
	}{
		{"valid GET", "GET /stats", true},
		{"valid POST", "POST /users", true},
		{"valid PUT", "PUT /items/1", true},
		{"valid DELETE", "DELETE /items/1", true},
		{"valid PATCH", "PATCH /items/1", true},
		{"valid HEAD", "HEAD /health", true},
		{"valid OPTIONS", "OPTIONS /api", true},
		{"invalid method lowercase", "get /stats", false},
		{"invalid method bad", "FOO /bar", false},
		{"missing path", "GET", false},
		{"missing method", "/foo", false},
		{"path without leading slash", "GET stats", false},
		{"empty", "", false},
		{"three parts", "GET /a b", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidRoutePattern(tt.pattern)
			if got != tt.want {
				t.Errorf("isValidRoutePattern(%q) = %v, want %v", tt.pattern, got, tt.want)
			}
		})
	}
}

// TestScaffolder_NewUnchanged verifies task 2.1: the new command path resolution
// stays unchanged — files are written under project_name/ directory.
func TestScaffolder_NewUnchanged(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "new-unchanged-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	config := &ui.ProjectConfig{
		ProjectName:  "myapp",
		ModuleName:   "github.com/test/myapp",
		Architecture: "Minimalist",
	}
	scaffolder := NewScaffolder(config)
	if err := scaffolder.Execute(); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Files must exist under myapp/ directory
	projectDir := filepath.Join(tmpDir, "myapp")
	for _, f := range []string{"main.go", "go.mod", ".go-arch.yaml"} {
		if _, err := os.Stat(filepath.Join(projectDir, f)); os.IsNotExist(err) {
			t.Errorf("expected file under %s: %s", projectDir, f)
		}
	}
}

// TestS2Sanity_FullWorkflow verifies the full Phase 2 workflow: scaffold web,
// generate crud → routes.go has Register, generate handler --route → HandleFunc.
func TestS2Sanity_FullWorkflow(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "s2sanity-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	// 1. Scaffold web project (new)
	cfg := &ui.ProjectConfig{
		ProjectName:  ".",
		ModuleName:   "github.com/test/s2sanity",
		Architecture: "Standard",
		UseTemplHTMX: true,
	}
	s := NewScaffolder(cfg)
	if err := s.Execute(); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	t.Log("✅ new web: ok")

	// Verify empty routes.go exists
	routesPath := "internal/router/routes.go"
	if _, err := os.Stat(routesPath); os.IsNotExist(err) {
		t.Fatal("FAIL: empty routes.go not created")
	}
	t.Log("✅ empty routes.go: ok")

	// 2. Generate crud User
	if err := s.GenerateCRUD("User"); err != nil {
		t.Fatalf("GenerateCRUD failed: %v", err)
	}
	content, _ := os.ReadFile(routesPath)
	if !strings.Contains(string(content), "NewUserHandler().Register(mux)") {
		t.Fatalf("FAIL: routes.go missing Register line\nGot: %s", content)
	}
	t.Log("✅ generate crud User: routes registered")

	// 3. Generate handler with --route "GET /stats"
	if err := s.GenerateComponent("handler", "Stats", WithRoute("GET /stats")); err != nil {
		t.Fatalf("GenerateComponent handler failed: %v", err)
	}
	content, _ = os.ReadFile(routesPath)
	if !strings.Contains(string(content), `mux.HandleFunc("GET /stats"`) {
		t.Fatalf("FAIL: routes.go missing mux.HandleFunc\nGot: %s", content)
	}
	t.Log("✅ generate handler Stats --route: HandleFunc added")
	t.Log("=== S2 Sanity PASSED ===")
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

// ---------------------------------------------------------------------------
// 3.1 RED — hook fire order, CWD, version visibility, stop-on-first, output routing
// ---------------------------------------------------------------------------

func TestScaffolder_PreNew_BeforeMkdirAll(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "hooks-prenew-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	fr := &hooks.FakeRunner{}
	runner := hooks.NewRunner(&hooks.Config{
		Hooks: map[hooks.Type][]hooks.Entry{
			hooks.PreNew:  {{Command: "echo", Args: []string{"pre"}}},
			hooks.PostNew: {{Command: "echo", Args: []string{"post"}}},
		},
	}, fr, &bytes.Buffer{})

	config := &ui.ProjectConfig{
		ProjectName:  "DemoApp",
		ModuleName:   "github.com/test/demo",
		Architecture: "Minimalist",
	}
	scaffolder := NewScaffolder(config, WithRunner(runner), WithVersion("1.2.3"))

	// Verify project dir does NOT exist yet — pre-new must fire BEFORE MkdirAll
	projDir := filepath.Join(tmpDir, "DemoApp")
	if _, err := os.Stat(projDir); err == nil {
		t.Fatal("project dir already exists before Execute() — can't assert pre-new ordering")
	}

	if err := scaffolder.Execute(); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Two calls: pre-new then post-new
	if len(fr.Calls) < 2 {
		t.Fatalf("expected at least 2 hook calls, got %d", len(fr.Calls))
	}

	// Call 0: pre-new (uses "echo pre")
	if fr.Calls[0].Args[0] != "pre" {
		t.Errorf("first call should be pre-new (args: pre), got %v", fr.Calls[0].Args)
	}
	// Call 1: post-new (uses "echo post")
	if fr.Calls[1].Args[0] != "post" {
		t.Errorf("second call should be post-new (args: post), got %v", fr.Calls[1].Args)
	}
}

func TestScaffolder_PostNew_SeesVersion(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "hooks-postnew-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	fr := &hooks.FakeRunner{}
	var outBuf bytes.Buffer
	runner := hooks.NewRunner(&hooks.Config{
		Hooks: map[hooks.Type][]hooks.Entry{
			hooks.PreNew:  {{Command: "echo", Args: []string{"pre"}}},
			hooks.PostNew: {{Command: "echo", Args: []string{"post"}}},
		},
	}, fr, &outBuf)

	config := &ui.ProjectConfig{
		ProjectName:  "VerApp",
		ModuleName:   "github.com/test/ver",
		Architecture: "Minimalist",
	}
	version := "3.0.0-beta"
	scaffolder := NewScaffolder(config, WithRunner(runner), WithVersion(version))

	if err := scaffolder.Execute(); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// post-new must fire AFTER WriteVersionField writes go_arch_version to .go-arch.yaml
	configPath := filepath.Join(tmpDir, "VerApp", ".go-arch.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("cannot read .go-arch.yaml: %v", err)
	}
	if !strings.Contains(string(data), "go_arch_version: "+version) {
		t.Errorf("expected go_arch_version: %s in .go-arch.yaml\nGot: %s", version, string(data))
	}

	// Verify hook output went to outBuf (FakeRunner records calls; real output
	// would go to injected writer. Call existence is the test assertion here.)

	// Verify 2 calls (pre-new then post-new)
	if len(fr.Calls) != 2 {
		t.Errorf("expected 2 calls, got %d", len(fr.Calls))
	}
}

func TestScaffolder_PreNew_CWD_IsInvocationDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "hooks-cwdpre-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	fr := &hooks.FakeRunner{}
	runner := hooks.NewRunner(&hooks.Config{
		Hooks: map[hooks.Type][]hooks.Entry{
			hooks.PreNew:  {{Command: "pwd"}},
			hooks.PostNew: {{Command: "pwd"}},
		},
	}, fr, &bytes.Buffer{})

	config := &ui.ProjectConfig{
		ProjectName:  "CwdApp",
		ModuleName:   "github.com/test/cwd",
		Architecture: "Minimalist",
	}
	scaffolder := NewScaffolder(config, WithRunner(runner), WithVersion("1.0.0"))

	if err := scaffolder.Execute(); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if len(fr.Calls) < 2 {
		t.Fatalf("expected 2 calls, got %d", len(fr.Calls))
	}

	// pre-new CWD: invocation dir (tmpDir)
	preDir := fr.Calls[0].Opts.Dir
	if preDir != tmpDir {
		t.Errorf("pre-new CWD: got %q, want %q (invocation dir)", preDir, tmpDir)
	}

	// post-new CWD: the new project directory (tmpDir/CwdApp)
	projDir := filepath.Join(tmpDir, "CwdApp")
	postDir := fr.Calls[1].Opts.Dir
	if postDir != projDir {
		t.Errorf("post-new CWD: got %q, want %q (project dir)", postDir, projDir)
	}
}

func TestScaffolder_GenerateComponent_FiresHooks(t *testing.T) {
	// Scaffold a project first so GenerateComponent has a manifest to work with
	tmpDir, err := os.MkdirTemp("", "hooks-gencomp-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	// Scaffold a project in CWD (ProjectName=".")
	cfg := &ui.ProjectConfig{
		ProjectName:  ".",
		ModuleName:   "github.com/test/gencomp",
		Architecture: "Standard",
	}
	s := NewScaffolder(cfg)
	if err := s.Execute(); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Now wire a runner for the generate phase
	fr := &hooks.FakeRunner{}
	runner := hooks.NewRunner(&hooks.Config{
		Hooks: map[hooks.Type][]hooks.Entry{
			hooks.PreGenerate:  {{Command: "echo", Args: []string{"pre-gen"}}},
			hooks.PostGenerate: {{Command: "echo", Args: []string{"post-gen"}}},
		},
	}, fr, &bytes.Buffer{})

	s2 := NewScaffolder(cfg, WithRunner(runner))
	if err := s2.GenerateComponent("service", "Order"); err != nil {
		t.Fatalf("GenerateComponent failed: %v", err)
	}

	if len(fr.Calls) < 2 {
		t.Fatalf("expected 2 calls for generate, got %d", len(fr.Calls))
	}
	if fr.Calls[0].Args[0] != "pre-gen" {
		t.Errorf("first call should be pre-generate, got %v", fr.Calls[0].Args)
	}
	if fr.Calls[1].Args[0] != "post-gen" {
		t.Errorf("second call should be post-generate, got %v", fr.Calls[1].Args)
	}

	// post-generate CWD: project root (tmpDir)
	if fr.Calls[1].Opts.Dir != tmpDir {
		t.Errorf("post-generate CWD: got %q, want %q", fr.Calls[1].Opts.Dir, tmpDir)
	}
}

func TestScaffolder_GenerateCRUD_PostGenerate_AfterRoutesRegistry(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "hooks-gencrud-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	// Scaffold web project
	cfg := &ui.ProjectConfig{
		ProjectName:  ".",
		ModuleName:   "github.com/test/gencrud",
		Architecture: "Standard",
		UseTemplHTMX: true,
	}
	s := NewScaffolder(cfg)
	if err := s.Execute(); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	fr := &hooks.FakeRunner{}
	var outBuf bytes.Buffer
	runner := hooks.NewRunner(&hooks.Config{
		Hooks: map[hooks.Type][]hooks.Entry{
			hooks.PreGenerate:  {{Command: "echo", Args: []string{"pre-gen"}}},
			hooks.PostGenerate: {{Command: "echo", Args: []string{"post-gen"}}},
		},
	}, fr, &outBuf)

	s2 := NewScaffolder(cfg, WithRunner(runner))
	if err := s2.GenerateCRUD("Product"); err != nil {
		t.Fatalf("GenerateCRUD failed: %v", err)
	}

	if len(fr.Calls) < 2 {
		t.Fatalf("expected 2 calls, got %d", len(fr.Calls))
	}
	if fr.Calls[0].Args[0] != "pre-gen" {
		t.Errorf("first call should be pre-generate, got %v", fr.Calls[0].Args)
	}
	if fr.Calls[1].Args[0] != "post-gen" {
		t.Errorf("second call should be post-generate, got %v", fr.Calls[1].Args)
	}

	// Routes registry must already exist when post-generate fires
	routesPath := filepath.Join(tmpDir, "internal", "router", "routes.go")
	routesContent, err := os.ReadFile(routesPath)
	if err != nil {
		t.Fatalf("cannot read routes.go: %v", err)
	}
	if !strings.Contains(string(routesContent), "Product") {
		t.Errorf("expected 'Product' in routes.go after CRUD generation (post-generate should see it)\nGot: %s", routesContent)
	}
}

func TestScaffolder_StopOnFirst_FailsPreNew(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "hooks-stopfail-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	fr := &hooks.FakeRunner{
		Responses: []hooks.FakeResponse{
			{ExitCode: 1, RunErr: nil}, // pre-new fails → stop
		},
	}
	runner := hooks.NewRunner(&hooks.Config{
		Hooks: map[hooks.Type][]hooks.Entry{
			hooks.PreNew:  {{Command: "false"}},
			hooks.PostNew: {{Command: "should-not-run"}},
		},
	}, fr, &bytes.Buffer{})

	config := &ui.ProjectConfig{
		ProjectName:  "FailApp",
		ModuleName:   "github.com/test/fail",
		Architecture: "Minimalist",
	}
	scaffolder := NewScaffolder(config, WithRunner(runner), WithVersion("1.0.0"))

	err = scaffolder.Execute()
	if err == nil {
		t.Fatal("expected error from pre-new failure, got nil")
	}

	// Only pre-new should have fired; post-new never reached
	if len(fr.Calls) != 1 {
		t.Errorf("expected 1 call (only pre-new), got %d", len(fr.Calls))
	}
}

func TestScaffolder_PostNew_FailureNonAtomic(t *testing.T) {
	// Req 10, Scenario 2: a failing post-new hook leaves generated files on
	// disk (non-atomic by design) and returns an error.
	tmpDir, err := os.MkdirTemp("", "hooks-postfail-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	fr := &hooks.FakeRunner{
		Responses: []hooks.FakeResponse{
			{ExitCode: 0}, // pre-new succeeds
			{ExitCode: 1}, // post-new fails (exit 1)
		},
	}
	runner := hooks.NewRunner(&hooks.Config{
		Hooks: map[hooks.Type][]hooks.Entry{
			hooks.PreNew:  {{Command: "echo", Args: []string{"pre"}}},
			hooks.PostNew: {{Command: "false"}},
		},
	}, fr, &bytes.Buffer{})

	config := &ui.ProjectConfig{
		ProjectName:  "NonAtomicApp",
		ModuleName:   "github.com/test/nonatomic",
		Architecture: "Minimalist",
	}
	scaffolder := NewScaffolder(config, WithRunner(runner), WithVersion("1.0.0"))

	err = scaffolder.Execute()
	if err == nil {
		t.Fatal("expected error from post-new failure, got nil")
	}

	// Project files must remain on disk (non-atomic by design).
	projDir := filepath.Join(tmpDir, "NonAtomicApp")
	for _, f := range []string{"main.go", "go.mod", ".go-arch.yaml"} {
		p := filepath.Join(projDir, f)
		if _, statErr := os.Stat(p); os.IsNotExist(statErr) {
			t.Errorf("expected file %s to remain on disk after post-new failure", p)
		}
	}
	// Verify pre-new and post-new both fired (pre-new succeeded, post-new failed).
	if len(fr.Calls) != 2 {
		t.Errorf("expected 2 calls (pre-new + post-new), got %d", len(fr.Calls))
	}
}

func TestScaffolder_NilRunner_IsNoop(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "hooks-nil-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	// No WithRunner — runner is nil, no hooks should fire
	config := &ui.ProjectConfig{
		ProjectName:  "NoHookApp",
		ModuleName:   "github.com/test/nohook",
		Architecture: "Minimalist",
	}
	scaffolder := NewScaffolder(config, WithVersion("1.0.0"))

	if err := scaffolder.Execute(); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// .go-arch.yaml should still have go_arch_version (WriteVersionField moved in)
	configPath := filepath.Join(tmpDir, "NoHookApp", ".go-arch.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("cannot read .go-arch.yaml: %v", err)
	}
	if !strings.Contains(string(data), "go_arch_version: 1.0.0") {
		t.Error("expected go_arch_version in .go-arch.yaml even without hooks runner")
	}
}

// --- Slice 4: real-tool smoke, silent, empty-config noop ---

func TestHooks_RealTool_Gofmt(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real-tool hook smoke in short mode")
	}
	if _, err := exec.LookPath("gofmt"); err != nil {
		t.Skip("gofmt not installed — skipping real-tool hook smoke")
	}

	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	config := &ui.ProjectConfig{
		ProjectName:  "SmokeApp",
		ModuleName:   "github.com/test/smoke",
		Architecture: "Minimalist",
	}

	// Post-new hooks: gofmt formats the scaffolded files (smoke-tests the
	// RealRunner integration), go mod tidy cleans up go.sum.
	hooksCfg := &hooks.Config{
		Hooks: map[hooks.Type][]hooks.Entry{
			hooks.PostNew: {
				{Command: "gofmt", Args: []string{"-w", "."}},
				{Command: "go", Args: []string{"mod", "tidy"}, IgnoreFailure: true},
			},
		},
	}
	runner := hooks.NewRunner(hooksCfg, hooks.RealRunner{}, os.Stderr)

	scaffolder := NewScaffolder(config, WithRunner(runner), WithVersion("1.0.0"))
	if err := scaffolder.Execute(); err != nil {
		t.Fatalf("Execute with real-tool hooks failed: %v", err)
	}

	// gofmt should have run in the project dir (post-new CWD = project dir).
	projectDir := filepath.Join(tmpDir, "SmokeApp")
	if _, err := os.Stat(filepath.Join(projectDir, "main.go")); err != nil {
		t.Fatalf("main.go missing after scaffold: %v", err)
	}
}

func TestHooks_Silent_SuppressesOutput(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real-echo silent test in short mode")
	}

	var outBuf bytes.Buffer
	runner := hooks.NewRunner(&hooks.Config{
		Hooks: map[hooks.Type][]hooks.Entry{
			hooks.PostGenerate: {
				{Command: "echo", Args: []string{"SHOULD_NOT_APPEAR"}, Silent: true},
			},
		},
	}, hooks.RealRunner{}, &outBuf)

	if err := runner.Fire(hooks.PostGenerate, hooks.EnvContext{}, "."); err != nil {
		t.Fatalf("Fire with silent hook failed: %v", err)
	}

	if got := outBuf.String(); got != "" {
		t.Errorf("silent hook should produce no output, got %q", got)
	}
}

func TestHooks_EmptyConfig_IsNoop(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	// Non-nil runner but empty Hooks map — should be a silent no-op.
	runner := hooks.NewRunner(&hooks.Config{
		Hooks: map[hooks.Type][]hooks.Entry{},
	}, hooks.RealRunner{}, os.Stderr)

	config := &ui.ProjectConfig{
		ProjectName:  "EmptyHooksApp",
		ModuleName:   "github.com/test/emptyhooks",
		Architecture: "Minimalist",
	}
	scaffolder := NewScaffolder(config, WithRunner(runner), WithVersion("1.0.0"))

	if err := scaffolder.Execute(); err != nil {
		t.Fatalf("Execute with empty hooks config should succeed, got: %v", err)
	}
}
