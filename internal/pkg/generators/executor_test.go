package generators

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go-arch/internal/pkg/hooks"
	"go-arch/internal/pkg/template"
)

// --------------- Test Fakes ---------------

// fakeFirer records calls to FireEntries and can optionally return errors.
type fakeFirer struct {
	called       int
	lastEntries  []hooks.Entry
	lastCtx      hooks.EnvContext
	lastCwd      string
	failWith     error
	failOnCall   int // 1-indexed
	returnedErrs []error
}

func (f *fakeFirer) FireEntries(entries []hooks.Entry, ctx hooks.EnvContext, cwd string) error {
	f.called++
	f.lastEntries = entries
	f.lastCtx = ctx
	f.lastCwd = cwd
	if f.failOnCall > 0 && f.called == f.failOnCall {
		return f.failWith
	}
	if len(f.returnedErrs) >= f.called {
		return f.returnedErrs[f.called-1]
	}
	return nil
}

// fakePromptResolver resolves prompts from a static map.
type fakePromptResolver struct {
	values map[string]string
	err    error
}

func (r *fakePromptResolver) Resolve(name, message, def string, required bool) (string, error) {
	if r.err != nil {
		return "", r.err
	}
	if v, ok := r.values[name]; ok {
		return v, nil
	}
	if def != "" {
		return def, nil
	}
	if required {
		return "", fmt.Errorf("missing required prompt: %s", name)
	}
	return "", nil
}

// --------------- Helpers ---------------

// setupPackDir creates a fixture pack directory with templates/ and assets/.
// Returns the packDir path.
func setupPackDir(t *testing.T) string {
	t.Helper()
	packDir := t.TempDir()

	tmplDir := filepath.Join(packDir, "templates", "common")
	mustMkdirT(t, tmplDir)
	mustWriteFile(t, filepath.Join(tmplDir, "handler.tmpl"), "package {{ .ModuleName }}")

	assetsDir := filepath.Join(packDir, "assets")
	mustMkdirT(t, assetsDir)
	mustWriteFile(t, filepath.Join(assetsDir, "logo.png"), "fake-png-data")

	return packDir
}

func mustMkdirT(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", path, err)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}

// --------------- Executor Tests ---------------

func TestRun_StepsExecuteInOrder(t *testing.T) {
	packDir := setupPackDir(t)
	projectRoot := t.TempDir()
	ctx := context.Background()
	firer := &fakeFirer{}

	gen := Generator{
		Steps: []Step{
			{Type: "template", From: "common/handler.tmpl", To: "internal/handler.go", Index: 0},
			{Type: "binary", From: "assets/logo.png", To: "static/logo.png", Mode: 0644, Index: 1},
			{Type: "run", Command: "echo", Args: []string{"hello"}, Index: 2},
		},
	}

	opts := RunOptions{
		ProjectRoot:  projectRoot,
		PackDir:      packDir,
		PackName:     "test-pack",
		PackVersion:  "1.0.0",
		HooksEnabled: true,
		Out:          io.Discard,
		Firer:        firer,
	}

	records, err := Run(ctx, gen, opts)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	// Only template and binary steps produce records (run step does not).
	if len(records) != 2 {
		t.Fatalf("expected 2 records (template + binary), got %d", len(records))
	}

	// Step 0: template → handler.go
	r0 := records[0]
	if r0.Path != "internal/handler.go" {
		t.Errorf("record[0].Path = %q, want %q", r0.Path, "internal/handler.go")
	}
	if r0.Origin != "template" {
		t.Errorf("record[0].Origin = %q, want %q", r0.Origin, "template")
	}
	if r0.TemplatePath != "common/handler.tmpl" {
		t.Errorf("record[0].TemplatePath = %q, want %q", r0.TemplatePath, "common/handler.tmpl")
	}
	// File must exist on disk.
	handlerPath := filepath.Join(projectRoot, "internal/handler.go")
	if _, err := os.Stat(handlerPath); os.IsNotExist(err) {
		t.Errorf("template output file %q does not exist", handlerPath)
	}

	// Step 1: binary → logo.png
	r1 := records[1]
	if r1.Path != "static/logo.png" {
		t.Errorf("record[1].Path = %q, want %q", r1.Path, "static/logo.png")
	}
	if r1.Origin != "generator" {
		t.Errorf("record[1].Origin = %q, want %q", r1.Origin, "generator")
	}
	logoPath := filepath.Join(projectRoot, "static/logo.png")
	info, err := os.Stat(logoPath)
	if err != nil {
		t.Fatalf("binary output file: %v", err)
	}
	if info.Mode().Perm() != 0644 {
		t.Errorf("binary mode = %o, want %o", info.Mode(), 0644)
	}

	// Step 2: run → firer called
	if firer.called != 1 {
		t.Errorf("firer called %d times, want 1", firer.called)
	}
	if len(firer.lastEntries) != 1 {
		t.Errorf("firer entries count = %d, want 1", len(firer.lastEntries))
	}
}

func TestRun_FailFast(t *testing.T) {
	packDir := setupPackDir(t)
	projectRoot := t.TempDir()
	ctx := context.Background()
	firer := &fakeFirer{
		failOnCall: 1,
		failWith:   errors.New("boom"),
	}

	gen := Generator{
		Steps: []Step{
			{Type: "template", From: "common/handler.tmpl", To: "internal/handler.go", Index: 0},
			{Type: "run", Command: "echo", Args: []string{"x"}, Index: 1},
			{Type: "template", From: "common/handler.tmpl", To: "internal/other.go", Index: 2},
		},
	}

	opts := RunOptions{
		ProjectRoot:  projectRoot,
		PackDir:      packDir,
		PackName:     "test-pack",
		PackVersion:  "1.0.0",
		HooksEnabled: true,
		Out:          io.Discard,
		Firer:        firer,
	}

	records, err := Run(ctx, gen, opts)
	if err == nil {
		t.Fatal("expected error from failing step, got nil")
	}
	// Step 1's file should exist (it completed), step 2 should NOT.
	handlerPath := filepath.Join(projectRoot, "internal/handler.go")
	if _, err := os.Stat(handlerPath); os.IsNotExist(err) {
		t.Error("step 1's file should exist (completed before failure)")
	}
	otherPath := filepath.Join(projectRoot, "internal/other.go")
	if _, err := os.Stat(otherPath); err == nil {
		t.Error("step 2's file should NOT exist (step failed)")
	}

	// Partial state: we may or may not have records before failure.
	// The key property is files are partially written.
	_ = records
}

func TestRun_IgnoreFailure(t *testing.T) {
	packDir := setupPackDir(t)
	projectRoot := t.TempDir()
	ctx := context.Background()

	firer := &fakeFirer{
		failOnCall: 1,
		failWith:   errors.New("non-fatal-boom"),
	}

	gen := Generator{
		Steps: []Step{
			{Type: "template", From: "common/handler.tmpl", To: "internal/a.go", Index: 0},
			{Type: "run", Command: "echo", Args: []string{"x"}, IgnoreFailure: true, Index: 1},
			{Type: "template", From: "common/handler.tmpl", To: "internal/b.go", Index: 2},
		},
	}

	var buf bytes.Buffer
	opts := RunOptions{
		ProjectRoot:  projectRoot,
		PackDir:      packDir,
		PackName:     "test-pack",
		PackVersion:  "1.0.0",
		HooksEnabled: true,
		Out:          &buf,
		Firer:        firer,
	}

	records, err := Run(ctx, gen, opts)
	if err != nil {
		t.Fatalf("Run() should not fail with ignore_failure: %v", err)
	}
	// 2 template records (run step does not produce a record).
	if len(records) != 2 {
		t.Errorf("expected 2 template records, got %d", len(records))
	}
	// Both files should exist.
	for _, p := range []string{"internal/a.go", "internal/b.go"} {
		if _, err := os.Stat(filepath.Join(projectRoot, p)); os.IsNotExist(err) {
			t.Errorf("file %q should exist after ignore_failure", p)
		}
	}
	// Warning should be logged.
	if !strings.Contains(buf.String(), "ignored") {
		t.Errorf("expected warning in output, got: %q", buf.String())
	}
}

func TestRun_HooksEnabledFalse_SkipsRun(t *testing.T) {
	packDir := setupPackDir(t)
	projectRoot := t.TempDir()
	ctx := context.Background()
	firer := &fakeFirer{}

	gen := Generator{
		Steps: []Step{
			{Type: "template", From: "common/handler.tmpl", To: "internal/tpl.go", Index: 0},
			{Type: "run", Command: "echo", Args: []string{"skip-me"}, Index: 1},
		},
	}

	var buf bytes.Buffer
	opts := RunOptions{
		ProjectRoot:  projectRoot,
		PackDir:      packDir,
		PackName:     "test-pack",
		PackVersion:  "1.0.0",
		HooksEnabled: false,
		Out:          &buf,
		Firer:        firer,
	}

	records, err := Run(ctx, gen, opts)
	if err != nil {
		t.Fatalf("Run() error with HooksEnabled=false: %v", err)
	}
	if len(records) != 1 {
		t.Errorf("expected 1 record (template), got %d: %v", len(records), records)
	}
	// Template file must exist.
	if _, err := os.Stat(filepath.Join(projectRoot, "internal/tpl.go")); os.IsNotExist(err) {
		t.Error("template file should exist even with HooksEnabled=false")
	}
	// Firer must NOT have been called.
	if firer.called != 0 {
		t.Errorf("firer called with HooksEnabled=false (called %d times)", firer.called)
	}
	// Warning must appear.
	if !strings.Contains(buf.String(), CodeGeneratorRunSkippedTrust) {
		t.Errorf("expected %q warning, got: %q", CodeGeneratorRunSkippedTrust, buf.String())
	}
}

func TestRun_UnknownBuiltin(t *testing.T) {
	packDir := setupPackDir(t)
	projectRoot := t.TempDir()
	ctx := context.Background()
	firer := &fakeFirer{}

	gen := Generator{
		Steps: []Step{
			{Type: "use", Value: "builtin/nonexistent", Index: 0},
		},
	}

	opts := RunOptions{
		ProjectRoot: projectRoot,
		PackDir:     packDir,
		PackName:    "test-pack",
		PackVersion: "1.0.0",
		Out:         io.Discard,
		Firer:       firer,
	}

	_, err := Run(ctx, gen, opts)
	if err == nil {
		t.Fatal("expected error for unknown builtin")
	}
	code := oopsCode(err)
	if code != CodeUnknownBuiltin {
		t.Errorf("error code = %q, want %q", code, CodeUnknownBuiltin)
	}
}

func TestRun_TemplateMissing(t *testing.T) {
	packDir := setupPackDir(t)
	projectRoot := t.TempDir()
	ctx := context.Background()
	firer := &fakeFirer{}

	gen := Generator{
		Steps: []Step{
			{Type: "template", From: "nonexistent.tmpl", To: "internal/x.go", Index: 0},
		},
	}

	opts := RunOptions{
		ProjectRoot: projectRoot,
		PackDir:     packDir,
		PackName:    "test-pack",
		PackVersion: "1.0.0",
		Out:         io.Discard,
		Firer:       firer,
	}

	_, err := Run(ctx, gen, opts)
	if err == nil {
		t.Fatal("expected error for missing template")
	}
	code := oopsCode(err)
	if code != template.CodeGeneratorTemplateNotFound {
		t.Errorf("error code = %q, want %q", code, template.CodeGeneratorTemplateNotFound)
	}
}

func TestRun_PreFlightPromptUnresolvable(t *testing.T) {
	packDir := setupPackDir(t)
	projectRoot := t.TempDir()
	ctx := context.Background()
	firer := &fakeFirer{}
	resolver := &fakePromptResolver{values: map[string]string{}} // no "db" value

	gen := Generator{
		Steps: []Step{
			{Type: "prompt", Name: "db", Message: "DB?", Required: true, Index: 0},
			{Type: "template", From: "common/handler.tmpl", To: "internal/h.go", Index: 1},
		},
	}

	opts := RunOptions{
		ProjectRoot:    projectRoot,
		PackDir:        packDir,
		PackName:       "test-pack",
		PackVersion:    "1.0.0",
		HooksEnabled:   true,
		Out:            io.Discard,
		Firer:          firer,
		PromptResolver: resolver,
		ResolvedArgs:   make(map[string]any),
	}

	_, err := Run(ctx, gen, opts)
	if err == nil {
		t.Fatal("expected error for missing required prompt value")
	}
	// Zero writes: template file must NOT exist.
	if _, err := os.Stat(filepath.Join(projectRoot, "internal/h.go")); err == nil {
		t.Error("template file must NOT exist after pre-flight prompt failure")
	}
	code := oopsCode(err)
	if code != CodeGeneratorPromptUnresolvable && code != CodeMissingGeneratorArgument {
		t.Errorf("error code = %q, want generator_prompt_unresolvable or missing_generator_argument", code)
	}
}

func TestRun_PreFlightPromptWithDefault(t *testing.T) {
	packDir := setupPackDir(t)
	projectRoot := t.TempDir()
	ctx := context.Background()
	firer := &fakeFirer{}
	resolver := &fakePromptResolver{values: map[string]string{}}

	gen := Generator{
		Steps: []Step{
			{Type: "prompt", Name: "driver", Message: "Driver?", Default: "postgres", Required: false, Index: 0},
			{Type: "template", From: "common/handler.tmpl", To: "internal/h.go", Index: 1},
		},
	}

	opts := RunOptions{
		ProjectRoot:    projectRoot,
		PackDir:        packDir,
		PackName:       "test-pack",
		PackVersion:    "1.0.0",
		HooksEnabled:   true,
		Out:            io.Discard,
		Firer:          firer,
		PromptResolver: resolver,
		ResolvedArgs:   make(map[string]any),
	}

	records, err := Run(ctx, gen, opts)
	if err != nil {
		t.Fatalf("Run() with default prompt should not error: %v", err)
	}
	if len(records) < 1 {
		t.Fatal("expected at least 1 record")
	}
	// Args map should have the resolved value.
	if v, ok := opts.ResolvedArgs["driver"]; !ok || v != "postgres" {
		t.Errorf("ResolvedArgs[driver] = %v, want 'postgres'", v)
	}
}

func TestRun_PreFlightZeroWritesOnEscape(t *testing.T) {
	packDir := setupPackDir(t)
	projectRoot := t.TempDir()
	ctx := context.Background()
	firer := &fakeFirer{}
	resolver := &fakePromptResolver{values: map[string]string{}}

	gen := Generator{
		Steps: []Step{
			{Type: "template", From: "common/handler.tmpl", To: "ok/file.go", Index: 0},
			{Type: "template", From: "common/handler.tmpl", To: "/etc/passwd", Index: 1}, // escape!
		},
	}

	opts := RunOptions{
		ProjectRoot:    projectRoot,
		PackDir:        packDir,
		PackName:       "test-pack",
		PackVersion:    "1.0.0",
		HooksEnabled:   true,
		Out:            io.Discard,
		Firer:          firer,
		PromptResolver: resolver,
	}

	_, err := Run(ctx, gen, opts)
	if err == nil {
		t.Fatal("expected error for path escape")
	}
	// Zero writes: step 1's target must NOT exist.
	if _, statErr := os.Stat(filepath.Join(projectRoot, "ok/file.go")); statErr == nil {
		t.Error("step 1's target must NOT exist — zero writes on escape")
	}
}

func TestRun_PrePostHooks(t *testing.T) {
	packDir := setupPackDir(t)
	projectRoot := t.TempDir()
	ctx := context.Background()
	firer := &fakeFirer{}

	gen := Generator{
		Pre: []hooks.Entry{
			{Command: "echo", Args: []string{"pre-start"}},
		},
		Post: []hooks.Entry{
			{Command: "echo", Args: []string{"post-done"}},
		},
		Steps: []Step{
			{Type: "template", From: "common/handler.tmpl", To: "internal/h.go", Index: 0},
		},
	}

	opts := RunOptions{
		ProjectRoot:   projectRoot,
		PackDir:       packDir,
		PackName:      "test-pack",
		PackVersion:   "1.0.0",
		HooksEnabled:  true,
		Out:           io.Discard,
		Firer:         firer,
		GeneratorName: "docker",
	}

	_, err := Run(ctx, gen, opts)
	if err != nil {
		t.Fatalf("Run() with pre/post hooks: %v", err)
	}

	// Pre and post should each have been called once.
	// The firer might be called for pre, steps (none are run:), and post.
	if firer.called < 2 {
		t.Errorf("firer called %d times, want at least 2 (pre + post)", firer.called)
	}
	// GeneratorName must be passed through to the EnvContext for hook env injection.
	if firer.lastCtx.GeneratorName != "docker" {
		t.Errorf("lastCtx.GeneratorName: want docker, got %q", firer.lastCtx.GeneratorName)
	}
}

func TestRun_PostHookSkippedOnFailure(t *testing.T) {
	packDir := setupPackDir(t)
	projectRoot := t.TempDir()
	ctx := context.Background()
	firer := &fakeFirer{
		failOnCall: 2, // step fails (run: fails at call 2, which is the step execution — call 1 is pre)
		failWith:   errors.New("step-boom"),
	}

	gen := Generator{
		Pre: []hooks.Entry{
			{Command: "echo", Args: []string{"pre"}},
		},
		Post: []hooks.Entry{
			{Command: "echo", Args: []string{"post"}},
		},
		Steps: []Step{
			{Type: "run", Command: "echo", Args: []string{"step"}, Index: 0},
		},
	}

	var buf bytes.Buffer
	opts := RunOptions{
		ProjectRoot:   projectRoot,
		PackDir:       packDir,
		PackName:      "test-pack",
		PackVersion:   "1.0.0",
		HooksEnabled:  true,
		Out:           &buf,
		Firer:         firer,
		GeneratorName: "test-gen",
	}

	_, err := Run(ctx, gen, opts)
	if err == nil {
		t.Fatal("expected error from failing run step")
	}
	// Pre was called (call 1), step was called (call 2), post should NOT be called.
	if firer.called != 2 {
		t.Errorf("firer called %d times, want 2 (pre + step, NOT post)", firer.called)
	}
	// GeneratorName must be passed to pre hook even on step failure.
	if firer.lastCtx.GeneratorName != "test-gen" {
		t.Errorf("lastCtx.GeneratorName: want test-gen, got %q", firer.lastCtx.GeneratorName)
	}
}

func TestRun_TemplateDataIsolation(t *testing.T) {
	// Template steps render with standard ProjectConfig data only.
	// Prompt values do NOT enter template data.
	packDir := t.TempDir()
	tmplDir := filepath.Join(packDir, "templates", "test")
	mustMkdirT(t, tmplDir)
	// Template that references .ProjectName only — no prompt vars.
	mustWriteFile(t, filepath.Join(tmplDir, "file.tmpl"), "name: {{ .ProjectName }}")

	projectRoot := t.TempDir()
	ctx := context.Background()
	firer := &fakeFirer{}
	resolver := &fakePromptResolver{values: map[string]string{"db": "mysql"}}

	gen := Generator{
		Steps: []Step{
			{Type: "template", From: "test/file.tmpl", To: "output.txt", Index: 0},
			{Type: "prompt", Name: "db", Message: "DB?", Required: true, Index: 1},
		},
	}

	var buf bytes.Buffer
	opts := RunOptions{
		ProjectRoot:    projectRoot,
		PackDir:        packDir,
		PackName:       "test-pack",
		PackVersion:    "1.0.0",
		Out:            &buf,
		Firer:          firer,
		PromptResolver: resolver,
		TemplateData: map[string]interface{}{
			"ProjectName": "TestProject",
		},
	}

	records, err := Run(ctx, gen, opts)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	_ = records

	// Read the rendered template output.
	content, err := os.ReadFile(filepath.Join(projectRoot, "output.txt"))
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	got := strings.TrimSpace(string(content))
	if got != "name: TestProject" {
		t.Errorf("template output = %q, want %q", got, "name: TestProject")
	}
	// db=mysql must NOT appear in template output (prompt values don't leak).
	if strings.Contains(got, "mysql") {
		t.Error("prompt value 'mysql' leaked into template output — data isolation broken")
	}
}
