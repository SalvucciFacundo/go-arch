package scaffold

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/SalvucciFacundo/go-arch/v2/internal/pkg/generators"
	"github.com/SalvucciFacundo/go-arch/v2/internal/pkg/hooks"
	"github.com/SalvucciFacundo/go-arch/v2/internal/pkg/packs"
	"github.com/SalvucciFacundo/go-arch/v2/internal/pkg/template"
	"github.com/SalvucciFacundo/go-arch/v2/internal/ui"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/samber/oops"
)

type Scaffolder struct {
	engine   *template.Engine
	config   *ui.ProjectConfig
	manifest *Manifest       // lazy-loaded via ensureManifest
	runner   *hooks.Runner   // lifecycle hooks runner (nil = no hooks)
	version  string          // CLI/MCP version — written to .go-arch.yaml during Execute
	packInfo *packs.PackInfo // pre-resolved pack info (nil when no --template)
}

// ScaffoldOption configures optional behaviour for a Scaffolder.
type ScaffoldOption func(*Scaffolder)

// WithRunner injects a hooks runner into the Scaffolder.
// When nil, hooks are silently skipped (no-op default).
func WithRunner(r *hooks.Runner) ScaffoldOption {
	return func(s *Scaffolder) { s.runner = r }
}

// WithVersion sets the version string written to .go-arch.yaml during Execute.
func WithVersion(v string) ScaffoldOption {
	return func(s *Scaffolder) { s.version = v }
}

// WithPackInfo injects a pre-resolved pack for pack-scoped scaffolding.
// When non-nil, Execute dispatches to scaffoldPack instead of the arch switch.
func WithPackInfo(p packs.PackInfo) ScaffoldOption {
	return func(s *Scaffolder) { s.packInfo = &p }
}

func NewScaffolder(config *ui.ProjectConfig, opts ...ScaffoldOption) *Scaffolder {
	s := &Scaffolder{config: config}

	// Apply options first so packInfo is known before engine construction.
	for _, opt := range opts {
		opt(s)
	}

	// Build engine options: pack info drives pack-scoped template resolution.
	var engineOpts []template.EngineOption
	if s.packInfo != nil {
		// Use the parent of the pack dir as the packs base so the engine
		// can resolve templates via the pack's templates/ subtree.
		// Engine expects: <packsDir>/<name>@<version>/templates/<path>
		packsBase := filepath.Dir(s.packInfo.Dir) // parent of express@1.0.0/
		engineOpts = append(engineOpts,
			template.WithPacksDir(packsBase),
			template.WithPack(s.packInfo.Manifest.Name, s.packInfo.Manifest.Version),
		)
	}
	s.engine = template.NewEngine(engineOpts...)

	return s
}

// manifestDir returns the project root for manifest operations.
// In generate context (manifest exists in CWD), returns ".".
// In new context (no manifest yet), returns s.config.ProjectName.
func (s *Scaffolder) manifestDir() string {
	if ManifestExists(".") {
		return "."
	}
	return s.config.ProjectName
}

// ensureManifest opens the manifest once, cached for the Scaffolder's lifetime.
func (s *Scaffolder) ensureManifest() (*Manifest, error) {
	if s.manifest != nil {
		return s.manifest, nil
	}
	m, err := LoadManifest(s.manifestDir())
	if err != nil {
		return nil, err
	}
	s.manifest = m
	return m, nil
}

// recordManifest hashes the just-written file and upserts the entry.
// Called AFTER successful write in createFile / createBinaryFile.
// Manifest save failure is NON-FATAL: log to stderr and continue.
// The scaffold write already succeeded; the manifest is a secondary index.
func (s *Scaffolder) recordManifest(targetPath, templatePath string, origin Origin, metadata map[string]string, source string) {
	m, err := s.ensureManifest()
	if err != nil {
		recordManifestWarning("manifest load failed: %v", err)
		return
	}
	fullPath := filepath.Join(s.manifestDir(), targetPath)
	hash, err := hashFile(fullPath)
	if err != nil {
		recordManifestWarning("manifest hash failed for %s: %v", targetPath, err)
		return
	}
	m.Upsert(ManifestEntry{
		Path:         targetPath,
		SHA256:       hash,
		Origin:       origin,
		TemplatePath: templatePath,
		Source:       source,
		Metadata:     metadata,
	})
	if err := m.Save(); err != nil {
		recordManifestWarning("manifest save failed: %v", err)
	}
}

func (s *Scaffolder) Execute() error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	// Fire pre-new hook BEFORE any filesystem operations.
	// The project directory does NOT exist yet at this point.
	if s.runner != nil {
		envCtx := hooks.EnvContext{
			ProjectName: s.config.ProjectName,
			ProjectPath: cwd,
			Arch:        s.config.Architecture,
			HookType:    hooks.PreNew,
			PackName:    s.packName(),
			PackVersion: s.packVersion(),
		}
		if err := s.runner.Fire(hooks.PreNew, envCtx, cwd); err != nil {
			return err
		}
	}

	// PACK BRANCH — dispatch before arch switch (G2).
	if s.packInfo != nil {
		return s.executePack(cwd)
	}

	fmt.Printf("🏗️ Creating project '%s' with %s architecture...\n", s.config.ProjectName, s.config.Architecture)

	// 1. Create base directory
	if err := os.MkdirAll(s.manifestDir(), 0755); err != nil {
		return err
	}

	// 2. Generate structure according to the layout
	var err2 error
	switch s.config.Architecture {
	case "Minimalist":
		err2 = s.scaffoldMinimalist()
	case "Standard":
		err2 = s.scaffoldStandard()
	case "Hexagonal":
		err2 = s.scaffoldHexagonal()
	default:
		return fmt.Errorf("unsupported architecture: %s", s.config.Architecture)
	}
	if err2 != nil {
		return err2
	}

	// 3. Write version field (non-fatal — post-new fires regardless)
	projDir := filepath.Join(cwd, s.manifestDir())
	if s.version != "" {
		_ = WriteVersionField(filepath.Join(projDir, ".go-arch.yaml"), s.version)
	}

	// 4. Fire post-new hook AFTER all files written AND version field set.
	if s.runner != nil {
		envCtx := hooks.EnvContext{
			ProjectName: s.config.ProjectName,
			ProjectPath: projDir,
			Arch:        s.config.Architecture,
			HookType:    hooks.PostNew,
		}
		if err := s.runner.Fire(hooks.PostNew, envCtx, projDir); err != nil {
			return err
		}
	}

	return nil
}

func (s *Scaffolder) createFile(path string, templatePath string, data interface{}) error {
	fullPath := filepath.Join(s.manifestDir(), path)

	// Crear directorios intermedios
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return err
	}

	f, err := os.Create(fullPath)
	if err != nil {
		return err
	}
	defer f.Close()

	if data == nil {
		data = s.config
	}

	if err := s.engine.Render(f, templatePath, data); err != nil {
		return err
	}

	s.recordManifest(path, templatePath, OriginScaffold, nil, "")
	return nil
}

// createBinaryFile copies a file from the embedded templates FS to the target
// path using ReadFile + WriteFile, bypassing engine.Render. This is deliberate:
// binary assets and files containing template delimiters must not pass through
// the text/template engine or the local/global override chain.
func (s *Scaffolder) createBinaryFile(targetPath, embeddedPath string) error {
	full := filepath.Join(s.manifestDir(), targetPath)
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		return err
	}
	data, err := template.TemplatesFS.ReadFile("templates/" + embeddedPath)
	if err != nil {
		return err
	}
	if err := os.WriteFile(full, data, 0644); err != nil {
		return err
	}
	s.recordManifest(targetPath, embeddedPath, OriginBinary, nil, "")
	return nil
}

// scaffoldWeb generates the templ + HTMX web scaffold: templ views, static
// assets, handler, README, and the architecture-agnostic web main. Called from
// createCommonFiles when UseTemplHTMX is true.
func (s *Scaffolder) scaffoldWeb() error {
	dirs := []string{
		"views/layouts",
		"views/pages",
		"views/components",
		"static/css",
		"static/js",
		"internal/router",
	}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(s.manifestDir(), d), 0755); err != nil {
			return err
		}
	}

	views := []struct{ target, tmpl string }{
		{"views/layouts/base.templ", "web/base.templ.tmpl"},
		{"views/pages/home.templ", "web/page.templ.tmpl"},
		{"views/components/counter.templ", "web/component.templ.tmpl"},
		{"static/css/style.css", "web/style.css.tmpl"},
		{"internal/handler/page.go", "web/handler.tmpl"},
		{"README.md", "web/readme.tmpl"},
	}
	for _, v := range views {
		if err := s.createFile(v.target, v.tmpl, nil); err != nil {
			return err
		}
	}

	if err := s.createBinaryFile("static/js/htmx.min.js", "web/htmx.min.js"); err != nil {
		return err
	}

	// Create empty-list routes.go so project compiles immediately.
	data := RoutesData{
		ModuleName:   s.config.ModuleName,
		Architecture: s.config.Architecture,
		Routes:       []RouteEntry{},
	}
	if err := s.createFile("internal/router/routes.go", "common/routes.tmpl", data); err != nil {
		return err
	}

	target := "cmd/api/main.go"
	if s.config.Architecture == "Minimalist" {
		target = "main.go"
	}
	return s.createFile(target, "web/main.tmpl", nil)
}

func (s *Scaffolder) scaffoldMinimalist() error {
	// Only main.go and go.mod
	if !s.config.UseTemplHTMX {
		if err := s.createFile("main.go", "minimalist/main.tmpl", nil); err != nil {
			return err
		}
	}
	return s.createCommonFiles()
}

func (s *Scaffolder) scaffoldStandard() error {
	dirs := []string{
		"cmd/api",
		"internal/handler",
		"internal/service",
		"internal/repository",
	}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(s.manifestDir(), d), 0755); err != nil {
			return err
		}
	}

	if !s.config.UseTemplHTMX {
		if err := s.createFile("cmd/api/main.go", "standard/main.tmpl", nil); err != nil {
			return err
		}
	}
	return s.createCommonFiles()
}

func (s *Scaffolder) scaffoldHexagonal() error {
	dirs := []string{
		"cmd/api",
		"internal/domain",
		"internal/ports",
		"internal/adapters",
	}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(s.manifestDir(), d), 0755); err != nil {
			return err
		}
	}

	if !s.config.UseTemplHTMX {
		if err := s.createFile("cmd/api/main.go", "hexagonal/main.tmpl", nil); err != nil {
			return err
		}
	}
	return s.createCommonFiles()
}

func (s *Scaffolder) createCommonFiles() error {
	// go.mod, .go-arch.yaml
	if err := s.createFile("go.mod", "common/go.mod.tmpl", nil); err != nil {
		return err
	}
	if err := s.createFile(".go-arch.yaml", "common/config.tmpl", nil); err != nil {
		return err
	}

	// .env.example (committed template; real .env is gitignored)
	if err := s.createFile(".env.example", "common/env.tmpl", nil); err != nil {
		return err
	}

	// .gitignore (excludes .env and build artifacts)
	if err := s.createFile(".gitignore", "common/gitignore.tmpl", nil); err != nil {
		return err
	}

	// Docker Files (Optional)
	if s.config.UseDocker {
		if err := s.createFile("Dockerfile", "common/Dockerfile.tmpl", nil); err != nil {
			return err
		}
		if err := s.createFile("docker-compose.yaml", "common/docker-compose.yaml.tmpl", nil); err != nil {
			return err
		}
	}

	// Observability (Optional)
	if s.config.UseObservability {
		if err := s.createFile("internal/telemetry/telemetry.go", "common/telemetry.tmpl", nil); err != nil {
			return err
		}
		if err := s.createFile("internal/telemetry/middleware.go", "common/telemetry_middleware.tmpl", nil); err != nil {
			return err
		}
	}

	// gRPC / Microservices (Optional)
	if s.config.UseGRPC {
		if err := s.createFile("api/proto/service.proto", "common/service.proto.tmpl", nil); err != nil {
			return err
		}
		if err := s.createFile("internal/adapters/grpc/server.go", "common/grpc_server.tmpl", nil); err != nil {
			return err
		}
		if err := s.createFile("Makefile", "common/Makefile.tmpl", nil); err != nil {
			return err
		}
	}

	// templ + HTMX Frontend (Optional)
	if s.config.UseTemplHTMX {
		return s.scaffoldWeb()
	}

	return nil
}

// GenerateOption configures optional behavior for GenerateComponent.
type GenerateOption func(*generateConfig)

type generateConfig struct {
	routePattern string
}

// WithRoute sets the route pattern for handler generation.
func WithRoute(pattern string) GenerateOption {
	return func(cfg *generateConfig) {
		cfg.routePattern = pattern
	}
}

// GenerateComponent generates a specific component (service, repository, handler).
// Accepts variadic GenerateOption for optional route registration.
func (s *Scaffolder) GenerateComponent(compType, name string, opts ...GenerateOption) error {
	// Fire pre-generate hook before any work.
	if s.runner != nil {
		cwd, _ := os.Getwd()
		envCtx := hooks.EnvContext{
			ProjectName: s.config.ProjectName,
			ProjectPath: cwd,
			Arch:        s.config.Architecture,
			HookType:    hooks.PreGenerate,
		}
		if err := s.runner.Fire(hooks.PreGenerate, envCtx, cwd); err != nil {
			return err
		}
	}

	cfg := &generateConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	var targetPath string
	var templatePath string

	data := struct {
		ui.ProjectConfig
		EntityName string
	}{
		ProjectConfig: *s.config,
		EntityName:    name,
	}

	switch compType {
	case "page":
		if !s.config.UseTemplHTMX {
			return oops.Code("web_scaffold_required").
				Hint("Set `use_templ_htmx: true` in .go-arch.yaml or re-run `go-arch new` with the flag").
				Errorf("page generation requires the web scaffold")
		}
		if !isValidGoIdentifier(name) {
			return oops.Code("invalid_component_name").
				Hint("Name must be a valid Go identifier (e.g. Dashboard, UserCard)").
				Errorf("invalid component name: %s", name)
		}
		targetPath = filepath.Join("views/pages", strings.ToLower(name)+".templ")
		templatePath = "web/page_generated.tmpl"
		if _, err := os.Stat(filepath.Join(s.manifestDir(), targetPath)); err == nil {
			return oops.Code("component_already_exists").
				Hint("Choose a different name or delete the existing file").
				Errorf("target file already exists: %s", targetPath)
		}
	case "component":
		if !s.config.UseTemplHTMX {
			return oops.Code("web_scaffold_required").
				Hint("Set `use_templ_htmx: true` in .go-arch.yaml or re-run `go-arch new` with the flag").
				Errorf("component generation requires the web scaffold")
		}
		if !isValidGoIdentifier(name) {
			return oops.Code("invalid_component_name").
				Hint("Name must be a valid Go identifier (e.g. Dashboard, UserCard)").
				Errorf("invalid component name: %s", name)
		}
		targetPath = filepath.Join("views/components", strings.ToLower(name)+".templ")
		templatePath = "web/component_generated.tmpl"
		if _, err := os.Stat(filepath.Join(s.manifestDir(), targetPath)); err == nil {
			return oops.Code("component_already_exists").
				Hint("Choose a different name or delete the existing file").
				Errorf("target file already exists: %s", targetPath)
		}
	case "service":
		templatePath = "common/service.tmpl"
		if s.config.Architecture == "Hexagonal" {
			targetPath = filepath.Join("internal/domain", name+"_service.go")
		} else {
			targetPath = filepath.Join("internal/service", name+"_service.go")
		}
	case "repository":
		templatePath = "common/repository.tmpl"
		if s.config.Architecture == "Hexagonal" {
			targetPath = filepath.Join("internal/ports", name+"_repository.go")
		} else {
			targetPath = filepath.Join("internal/repository", name+"_repository.go")
		}
	case "handler":
		// Validate route pattern if provided
		if cfg.routePattern != "" {
			if !s.config.UseTemplHTMX {
				return oops.Code("web_scaffold_required").
					Hint("Set `use_templ_htmx: true` in .go-arch.yaml or re-run `go-arch new` with the flag").
					Errorf("--route requires the web scaffold")
			}
			if !isValidRoutePattern(cfg.routePattern) {
				return oops.Code("invalid_route_pattern").
					Hint("Pattern must be 'METHOD /path' (e.g. 'GET /stats')").
					Errorf("invalid route pattern: %s", cfg.routePattern)
			}
		}
		templatePath = "common/handler.tmpl"
		if s.config.Architecture == "Hexagonal" {
			targetPath = filepath.Join("internal/adapters", name+"_handler.go")
		} else {
			targetPath = filepath.Join("internal/handler", name+"_handler.go")
		}
	default:
		return fmt.Errorf("unsupported component type: %s", compType)
	}

	if err := s.createFile(targetPath, templatePath, data); err != nil {
		return err
	}

	// Re-record with correct origin (upsert wins over the OriginScaffold
	// that createFile wrote). Non-fatal if manifest operations fail.
	meta := map[string]string{"entity_name": name}
	s.recordManifest(targetPath, templatePath, OriginComponent, meta, "")

	// Upsert route if provided and web project
	if cfg.routePattern != "" && s.config.UseTemplHTMX {
		m, err := s.ensureManifest()
		if err == nil {
			_ = m.UpsertRoute(RouteEntry{
				Entity:       name,
				Handler:      name,
				Origin:       "handler",
				RoutePattern: cfg.routePattern,
			})
			// Re-render routes.go from manifest
			_ = s.renderRoutesRegistry()
		}
	}

	// Fire post-generate hook after all work completes (including routes registry).
	if s.runner != nil {
		cwd, _ := os.Getwd()
		envCtx := hooks.EnvContext{
			ProjectName: s.config.ProjectName,
			ProjectPath: cwd,
			Arch:        s.config.Architecture,
			HookType:    hooks.PostGenerate,
		}
		if err := s.runner.Fire(hooks.PostGenerate, envCtx, cwd); err != nil {
			return err
		}
	}

	return nil
}

// GenerateCRUD generates the full structure for a CRUD entity
func (s *Scaffolder) GenerateCRUD(name string) error {
	// Fire pre-generate hook before any work.
	if s.runner != nil {
		cwd, _ := os.Getwd()
		envCtx := hooks.EnvContext{
			ProjectName: s.config.ProjectName,
			ProjectPath: cwd,
			Arch:        s.config.Architecture,
			HookType:    hooks.PreGenerate,
		}
		if err := s.runner.Fire(hooks.PreGenerate, envCtx, cwd); err != nil {
			return err
		}
	}

	data := struct {
		ui.ProjectConfig
		EntityName string
	}{
		ProjectConfig: *s.config,
		EntityName:    name,
	}

	fmt.Printf("🚀 Generating full CRUD for '%s'...\n", name)

	var files map[string]string
	if s.config.Architecture == "Hexagonal" {
		files = map[string]string{
			filepath.Join("internal/domain", name+".go"):              "common/model.tmpl",
			filepath.Join("internal/domain", name+"_service.go"):      "common/crud_service.tmpl",
			filepath.Join("internal/ports", name+"_repository.go"):    "common/crud_port.tmpl", // Port interface lives in internal/ports
			filepath.Join("internal/adapters", name+"_repository.go"): "common/crud_repository.tmpl",
			filepath.Join("internal/adapters", name+"_handler.go"):    "common/crud_handler.tmpl",
		}
	} else {
		files = map[string]string{
			filepath.Join("internal/model", name+".go"):                 "common/model.tmpl",
			filepath.Join("internal/service", name+"_service.go"):       "common/crud_service.tmpl",
			filepath.Join("internal/repository", name+"_repository.go"): "common/crud_repository.tmpl",
			filepath.Join("internal/handler", name+"_handler.go"):       "common/crud_handler.tmpl",
		}
	}

	for path, tmpl := range files {
		if err := s.createFile(path, tmpl, data); err != nil {
			return err
		}
		// Re-record with OriginCrud (upsert wins over OriginScaffold from createFile).
		// Non-fatal if manifest operations fail.
		meta := map[string]string{"entity_name": name}
		s.recordManifest(path, tmpl, OriginCrud, meta, "")
	}

	fmt.Println("\n✅ CRUD generated successfully.")

	if s.config.UseTemplHTMX {
		// Upsert route in manifest and re-render routes.go
		m, err := s.ensureManifest()
		if err == nil {
			_ = m.UpsertRoute(RouteEntry{
				Entity:  name,
				Handler: name,
				Origin:  "crud",
			})
			_ = s.renderRoutesRegistry()
			fmt.Println("🔗 Routes registered in internal/router/routes.go")
		}
	} else {
		fmt.Println("📍 Remember to register the routes in your main router.")
	}

	// Fire post-generate hook after all work completes (including routes registry).
	if s.runner != nil {
		cwd, _ := os.Getwd()
		envCtx := hooks.EnvContext{
			ProjectName: s.config.ProjectName,
			ProjectPath: cwd,
			Arch:        s.config.Architecture,
			HookType:    hooks.PostGenerate,
		}
		if err := s.runner.Fire(hooks.PostGenerate, envCtx, cwd); err != nil {
			return err
		}
	}

	return nil
}

// isValidGoIdentifier returns true if name is a valid Go identifier.
// Uses go/token.IsIdentifier which rejects keywords, empty strings,
// strings with hyphens, and strings with leading digits.
func isValidGoIdentifier(name string) bool {
	return token.IsIdentifier(name)
}

// GeneratePackConfig holds optional configuration for GeneratePackGenerator.
type GeneratePackConfig struct {
	PromptErrorCode string
}

// GeneratePackOption configures optional behaviour for GeneratePackGenerator.
type GeneratePackOption func(*GeneratePackConfig)

// WithPromptErrorCode sets the oops error code used when a required prompt
// cannot be resolved from args. MCP callers should use
// CodeMissingGeneratorArgument; CLI non-interactive callers should use
// CodeGeneratorPromptUnresolvable (the default).
func WithPromptErrorCode(code string) GeneratePackOption {
	return func(cfg *GeneratePackConfig) { cfg.PromptErrorCode = code }
}

// GeneratePackGenerator executes a named generator recipe from the
// currently active pack (set via WithPackInfo). It performs pre-flight
// prompt resolution from args, pre-flight sandbox validation, runs the
// recipe via generators.Run, and records the resulting files in the
// project manifest with generator provenance.
func (s *Scaffolder) GeneratePackGenerator(name string, args map[string]any, opts ...GeneratePackOption) error {
	cfg := &GeneratePackConfig{PromptErrorCode: generators.CodeGeneratorPromptUnresolvable}
	for _, o := range opts {
		o(cfg)
	}

	if s.packInfo == nil {
		return oops.
			Code(generators.CodePackNotInstalled).
			Errorf("no pack configured; cannot run generator %q", name)
	}

	gen, ok := s.packInfo.Manifest.Generators[name]
	if !ok {
		// Collect available generator names for the error message.
		names := make([]string, 0, len(s.packInfo.Manifest.Generators))
		for k := range s.packInfo.Manifest.Generators {
			names = append(names, k)
		}
		return oops.
			Code(generators.CodeUnknownGenerator).
			With("generator", name).
			With("available", names).
			Errorf("unknown generator %q for pack %q (available: %v)", name, s.packInfo.Manifest.Name, names)
	}

	// --- Pre-flight: prompt resolution ---
	// Build ResolvedArgs from the caller-provided map, filling defaults
	// for prompt steps that aren't in args.
	resolvedArgs := make(map[string]any)
	for k, v := range args {
		resolvedArgs[k] = v
	}
	failedPrompt, err := s.resolveGeneratorPrompts(gen, args, resolvedArgs)
	if err != nil {
		return err
	}
	if failedPrompt != "" {
		return oops.
			Code(cfg.PromptErrorCode).
			Errorf("required prompt %q not provided for generator %q", failedPrompt, name)
	}

	// Provide a PromptResolver to the executor so it doesn't overwrite
	// our resolved values with defaults. It reads from the resolvedArgs
	// that we already populated with args + defaults.
	promptResolver := &mapPromptResolver{values: resolvedArgs, errorCode: cfg.PromptErrorCode}

	// --- Pre-flight: sandbox validation ---
	projectRoot, err := os.Getwd()
	if err != nil {
		return err
	}
	for i, step := range gen.Steps {
		if step.Type == "template" || step.Type == "binary" {
			if err := generators.ValidateTarget(projectRoot, step.To); err != nil {
				return oops.
					Code(generators.CodeRecipePathEscape).
					Wrapf(err, "step %d (%s): target %q escapes project root", i, step.Type, step.To)
			}
		}
	}

	// --- HooksEnabled from sidecar ---
	hooksEnabled := false
	if sc, scErr := packs.ReadSidecar(s.packInfo.Dir); scErr == nil {
		hooksEnabled = sc.HooksEnabled
	}

	// --- Build and execute ---
	source := fmt.Sprintf("pack:%s@%s", s.packInfo.Manifest.Name, s.packInfo.Manifest.Version)

	var firer *generators.Runner
	if s.runner != nil {
		genRunner := generators.NewRunner(s.runner.CommandRunner(), ui.Out)
		firer = genRunner
	}

	runOpts := generators.RunOptions{
		ProjectRoot:    projectRoot,
		PackDir:        s.packInfo.Dir,
		PackName:       s.packInfo.Manifest.Name,
		PackVersion:    s.packInfo.Manifest.Version,
		GeneratorName:  name,
		HooksEnabled:   hooksEnabled,
		CmdRunner:      hooks.RealRunner{},
		Out:            ui.Out,
		Firer:          firer,
		TemplateData:   s.config,
		ResolvedArgs:   resolvedArgs,
		PromptResolver: promptResolver,
	}

	if s.runner != nil {
		runOpts.CmdRunner = s.runner.CommandRunner()
	}

	ctx := s.buildContext()
	records, runErr := generators.Run(ctx, gen, runOpts)
	if runErr != nil {
		return runErr
	}

	// --- Record manifest entries ---
	argsJSON, err := json.Marshal(resolvedArgs)
	if err != nil {
		// Non-fatal: log warning but continue.
		fmt.Fprintf(ui.Out, "warning: failed to marshal generator args: %v\n", err)
		argsJSON = []byte("{}")
	}
	argsStr := string(argsJSON)

	for _, rec := range records {
		meta := map[string]string{
			"generator": name,
			"args":      argsStr,
		}
		// Merge any existing metadata from the record.
		for k, v := range rec.Metadata {
			if k != "generator" && k != "args" {
				meta[k] = v
			}
		}

		var origin Origin
		switch rec.Origin {
		case "template":
			origin = OriginTemplate
		case "generator":
			origin = OriginGenerator
		default:
			origin = OriginGenerator
		}

		s.recordManifest(rec.Path, rec.TemplatePath, origin, meta, source)
	}

	return nil
}

// resolveGeneratorPrompts resolves prompt steps from args, filling defaults
// where values are absent. Returns (failedPrompt, error) — failedPrompt is
// the name of a required prompt that could not be resolved; if all prompts
// resolve, both return values are empty.
func (s *Scaffolder) resolveGeneratorPrompts(gen generators.Generator, args map[string]any, out map[string]any) (string, error) {
	for _, step := range gen.Steps {
		if step.Type != "prompt" {
			continue
		}
		// Check args first.
		if v, ok := args[step.Name]; ok {
			out[step.Name] = v
			continue
		}
		// Use default.
		if step.Default != "" {
			out[step.Name] = step.Default
			continue
		}
		// Required with no value.
		if step.Required {
			return step.Name, nil
		}
		// Not required, no default → empty string.
		out[step.Name] = ""
	}
	return "", nil
}

// buildContext returns a background context for generator execution.
func (s *Scaffolder) buildContext() context.Context {
	return context.Background()
}

// mapPromptResolver implements generators.PromptResolver using a
// static map. The caller (GeneratePackGenerator) populates the map
// with args + defaults before the executor runs, so the executor
// sees resolved values instead of overwriting with defaults.
type mapPromptResolver struct {
	values    map[string]any
	errorCode string
}

func (r *mapPromptResolver) Resolve(name, message, def string, required bool) (string, error) {
	if v, ok := r.values[name]; ok {
		switch val := v.(type) {
		case string:
			return val, nil
		default:
			return fmt.Sprint(val), nil
		}
	}
	if def != "" {
		return def, nil
	}
	if required {
		return "", oops.
			Code(r.errorCode).
			Errorf("required prompt %q not provided and has no default", name)
	}
	return "", nil
}

// RoutesData is the template data for routes.tmpl.
type RoutesData struct {
	ModuleName   string
	Architecture string
	Routes       []RouteEntry
}

// renderRoutesRegistry re-renders internal/router/routes.go from manifest.Routes.
// Uses RenderTo with quiet=true to suppress custom-template notices.
// Uses compare-then-write to avoid unnecessary disk writes and manifest churn.
func (s *Scaffolder) renderRoutesRegistry() error {
	m, err := s.ensureManifest()
	if err != nil {
		return err
	}
	data := RoutesData{
		ModuleName:   s.config.ModuleName,
		Architecture: s.config.Architecture,
		Routes:       m.Routes,
	}

	var buf bytes.Buffer
	templatePath := "common/routes.tmpl"
	if err := s.engine.RenderTo(&buf, templatePath, data, true); err != nil {
		return err
	}

	targetPath := "internal/router/routes.go"
	fullPath := filepath.Join(s.manifestDir(), targetPath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return err
	}

	// Compare-then-write: skip write if content is byte-identical
	existing, readErr := os.ReadFile(fullPath)
	if readErr == nil && bytes.Equal(existing, buf.Bytes()) {
		return nil
	}

	if err := os.WriteFile(fullPath, buf.Bytes(), 0644); err != nil {
		return err
	}

	s.recordManifest(targetPath, templatePath, OriginComponent, nil, "")
	return nil
}

// packName returns the pack manifest name or empty string when no pack is active.
func (s *Scaffolder) packName() string {
	if s.packInfo == nil {
		return ""
	}
	return s.packInfo.Manifest.Name
}

// packVersion returns the pack manifest version or empty string when no pack is active.
func (s *Scaffolder) packVersion() string {
	if s.packInfo == nil {
		return ""
	}
	return s.packInfo.Manifest.Version
}

// executePack handles the pack-scoped scaffold path: scaffoldPack, hooks, version.
func (s *Scaffolder) executePack(cwd string) error {
	fmt.Printf("🏗️ Creating project '%s' from pack '%s'...\n",
		s.config.ProjectName, s.packInfo.Manifest.Name)

	if err := s.scaffoldPack(); err != nil {
		return err
	}

	projDir := filepath.Join(cwd, s.manifestDir())
	if s.version != "" {
		_ = WriteVersionField(filepath.Join(projDir, ".go-arch.yaml"), s.version)
	}

	// Fire post-new hook with pack env vars.
	if s.runner != nil {
		envCtx := hooks.EnvContext{
			ProjectName: s.config.ProjectName,
			ProjectPath: projDir,
			Arch:        s.config.Architecture,
			HookType:    hooks.PostNew,
			PackName:    s.packName(),
			PackVersion: s.packVersion(),
		}
		if err := s.runner.Fire(hooks.PostNew, envCtx, projDir); err != nil {
			return err
		}
	}

	return nil
}

// scaffoldPack walks the pack's templates/ tree, maps templates/<path>.tmpl →
// target <path> (G2 convention), creates files via createFile, copies binary
// assets declared in the manifest, and records manifest entries with
// pack source provenance.
func (s *Scaffolder) scaffoldPack() error {
	p := s.packInfo
	source := fmt.Sprintf("pack:%s@%s", p.Manifest.Name, p.Manifest.Version)

	// 1. Create base directory
	if err := os.MkdirAll(s.manifestDir(), 0755); err != nil {
		return err
	}

	// 2. Create layout directories from manifest
	for _, layoutDir := range p.Manifest.Layout {
		if err := os.MkdirAll(filepath.Join(s.manifestDir(), layoutDir), 0755); err != nil {
			return err
		}
	}

	// 3. Walk templates/ tree — strip .tmpl extension for target path
	templatesRoot := filepath.Join(p.Dir, "templates")
	if err := fs.WalkDir(os.DirFS(templatesRoot), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		// G2 convention: templates/<path>.tmpl → target <path>
		if !strings.HasSuffix(path, ".tmpl") {
			return nil // skip non-template files (e.g. binary assets inside templates)
		}

		targetPath := strings.TrimSuffix(path, ".tmpl")
		// lookup key is the path WITH .tmpl extension within the pack's templates dir
		lookupKey := path

		// createFile records with empty source; we override below
		if err := s.createFile(targetPath, lookupKey, s.config); err != nil {
			return err
		}
		// Override manifest entry with pack source provenance
		s.recordManifest(targetPath, lookupKey, OriginScaffold, nil, source)

		return nil
	}); err != nil {
		return err
	}

	// 4. Copy binary assets declared in manifest (G3)
	for _, asset := range p.Manifest.BinaryAssets {
		if err := s.createPackBinary(asset.Target, asset.Source, p.Dir); err != nil {
			return err
		}
		s.recordManifest(asset.Target, "_binary:"+asset.Source, OriginBinary, nil, source)
	}

	return nil
}

// createPackBinary copies a binary asset from the pack root to the project.
// The asset source is relative to the pack directory (e.g. "assets/htmx.min.js").
// Unlike createFile, this does NOT pass through the template engine — the
// file is copied verbatim.
//
// DESIGN NOTE: Reads directly from the pack directory rather than using
// engine.ResolveBinary. The pack directory is the authoritative source for
// pack-declared assets (G3 resolved: "pack binaries at pack root assets/").
// Using the 4-step chain (local > global > pack > embedded) would add
// unnecessary indirection for assets that are explicitly declared in the
// pack manifest. ResolveBinary remains available for callers that need the
// full chain resolution (e.g. engine-level testing), but the pack-scaffold
// path intentionally bypasses it.
func (s *Scaffolder) createPackBinary(targetPath, relSource, packDir string) error {
	fullTarget := filepath.Join(s.manifestDir(), targetPath)
	if err := os.MkdirAll(filepath.Dir(fullTarget), 0755); err != nil {
		return err
	}

	srcPath := filepath.Join(packDir, relSource)
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return err
	}

	return os.WriteFile(fullTarget, data, 0644)
}

// isValidRoutePattern validates "METHOD /path" format.
// Method must be one of GET, POST, PUT, DELETE, PATCH, HEAD, OPTIONS.
// Path must start with "/".
func isValidRoutePattern(pattern string) bool {
	parts := strings.Fields(pattern)
	if len(parts) != 2 {
		return false
	}
	method := parts[0]
	path := parts[1]
	validMethods := map[string]bool{
		"GET": true, "POST": true, "PUT": true, "DELETE": true, "PATCH": true,
		"HEAD": true, "OPTIONS": true,
	}
	if !validMethods[method] {
		return false
	}
	if !strings.HasPrefix(path, "/") {
		return false
	}
	return true
}
