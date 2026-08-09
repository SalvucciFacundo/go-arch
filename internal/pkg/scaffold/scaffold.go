package scaffold

import (
	"bytes"
	"fmt"
	"go-arch/internal/pkg/hooks"
	"go-arch/internal/pkg/packs"
	"go-arch/internal/pkg/template"
	"go-arch/internal/ui"
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

	// .env (Always useful)
	if err := s.createFile(".env", "common/env.tmpl", nil); err != nil {
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
