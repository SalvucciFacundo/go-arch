# Design: Generate Routes

## Technical Approach

Auto-register routes for templ+HTMX web projects via a generated registry file (`internal/router/routes.go`) driven by a manifest-held route list. CRUD registers its 5 routes by default; plain handlers opt in via `--route`. The registry is re-rendered deterministically from the manifest route list, preserving main.go's byte-identity with its template and upgrade's PROTECTED semantics. Non-web projects keep the manual hint. The manifest path resolution is unified via a single `manifestDir()` helper that returns CWD in generate context and ProjectName during `new`. The empty-routes compile issue is solved via conditional imports in routes.tmpl. Fresh web projects compile immediately via empty-list routes.go created during `new`.

## Architecture Decisions

### Decision: Registry Over AST or Main.go Regeneration

**Choice**: Generated routes registry file (`internal/router/routes.go`) re-rendered from manifest route list.

**Alternatives considered**:
- (a) Template-regenerate main.go wholesale
- (b) AST-based insertion (go/ast, go/printer)

**Rationale**: Only the registry approach keeps main.go byte-identical to its template (manifest hash stable → upgrade healthy). Approach (a) clobbers user edits and makes main.go PROTECTED forever. Approach (b) is fragile (import rewriting, mux var rename, printer comment drift) and still makes main.go diverge from manifest. The registry file is a dedicated generated artifact whose state is the manifest — exactly upgrade's model.

### Decision: Route List in Manifest (Additive Field)

**Choice**: Additive `routes:` top-level field in manifest schema, keyed by entity for upsert dedupe.

**Alternatives considered**:
- Per-file metadata (routes embedded in manifest entry metadata)
- Separate routes.yaml file

**Rationale**: Additive field preserves backward compatibility (old manifests load with empty routes). Entity-keyed upsert provides natural idempotency (CRUD twice → one entry). Separate file adds I/O complexity without benefit. The route list is small, project-wide state, not per-file state.

### Decision: CRUD Default-On, Handler Opt-In

**Choice**: CRUD registers routes by default in web projects; plain handlers opt in via `--route "METHOD /path"`.

**Alternatives considered**:
- CRUD opt-in via `--register` flag
- Handler auto-detect pattern

**Rationale**: CRUD's `Register(mux)` method already exists in `crud_handler.tmpl` (lines 15-21) with 5 well-defined routes — registration is the natural completion. Plain handlers have no `Register` method and no inherent pattern; auto-detection would require parsing user code. Opt-in via flag keeps the explicit contract.

### Decision: Manifest Path Resolution via Single Helper

**Choice**: Introduce `manifestDir()` helper on Scaffolder that returns CWD in generate context (manifest exists in CWD) and ProjectName during `new` (no manifest yet). Use it consistently in `ensureManifest`, `recordManifest`, `createBinaryFile`, `createFile`, and `GenerateComponent`'s existence checks.

**Alternatives considered**:
- Fix each method individually (split-brain risk)
- Separate code paths for `new` vs `generate`

**Rationale**: The bug is that generate context (CWD = project root, project_name: realapp) incorrectly treats ProjectName as a directory. Multiple methods (`recordManifest` line 51, `createBinaryFile` line 121, `ensureManifest` line 33, `GenerateComponent` existence checks lines 312/330) all use `filepath.Join(s.config.ProjectName, ...)`. A single helper eliminates split-brain: one place to get the project root, used everywhere. Detection: manifest exists in CWD → use CWD; otherwise use ProjectName (new context).

### Decision: Empty-Routes Compile via Conditional Import

**Choice**: Guard the import block in routes.tmpl with `{{ if .Routes }}` so the import vanishes when the route list is empty.

**Alternatives considered**:
- Always emit import (compile error when empty)
- Dummy import with blank identifier `_`

**Rationale**: Go doesn't allow unused imports. When routes list is empty, the `{{ range .Routes }}` emits nothing, making the handler/adapters import unused. Conditional import (`{{ if .Routes }}`) makes the import vanish with the usage. The template compiles with zero routes AND with routes.

### Decision: Fresh Web Project Compiles Immediately

**Choice**: `scaffoldWeb()` creates an empty-list `internal/router/routes.go` during `new` so the project compiles immediately.

**Alternatives considered**:
- Lazy creation on first `generate`
- Guard main.tmpl with conditional

**Rationale**: main.tmpl gets unconditional `router.Register(mux)` + import. Without routes.go, `go build` fails. Lazy creation leaves existing projects broken until first generate. Empty-list registry is harmless, idempotent, and ensures byte-identity (main.go hash stable).

### Decision: Upgrade Re-Renders routes.go from Manifest

**Choice**: Extend `buildRenderData` to detect routes.go entries and inject the manifest's Routes list. routes.go is never PROTECTED (always regenerated from manifest state).

**Alternatives considered**:
- Store routes in per-file metadata (duplicates manifest.Routes)
- Skip routes.go in upgrade (leaves stale registry)

**Rationale**: routes.go state is the manifest route list. Upgrade must re-render it deterministically. Special-casing routes.go in `buildRenderData` (detect path == "internal/router/routes.go" → inject manifest.Routes) keeps the render data reconstruction clean. Since routes.go is always regenerated from manifest.Routes, it's never PROTECTED (disk hash irrelevant — manifest is source of truth).

### Decision: GenerateComponent Variadic Options

**Choice**: Use variadic `GenerateComponent(compType, name string, opts ...GenerateOption)` with `WithRoute(pattern string)` option to avoid breaking callers.

**Alternatives considered**:
- Add explicit routePattern parameter (breaks 10 call sites)
- Separate method for route-aware generation

**Rationale**: Variadic options minimize churn: only callers that need route registration pass the option. Existing callers (cmd/generate.go:58, mcp/server.go:358, 8 test sites) continue working unchanged. Only cmd/generate.go and mcp/server.go add the option when --route is provided. This is the standard Go idiom for optional parameters.

### Decision: Pattern Validation in Scaffold Layer

**Choice**: Move `isValidRoutePattern` + `web_scaffold_required` checks into the scaffold layer (GenerateComponent) so MCP and CLI paths emit identical codes.

**Alternatives considered**:
- Validate in cmd layer only (MCP duplicates logic)
- No validation (let template fail)

**Rationale**: MCP server.go:358 calls GenerateComponent directly. If validation lives in cmd/generate.go, MCP path bypasses it. Moving validation into GenerateComponent (handler case) ensures both paths emit `web_scaffold_required` and `invalid_route_pattern` codes identically. The cmd layer becomes a thin adapter.

## Data Flow

```
generate crud User (web project)
  ├─→ GenerateCRUD creates 5 files (model, service, repository, handler)
  ├─→ Upsert RouteEntry{entity: "User", handler: "User", origin: "crud"} in manifest
  ├─→ Re-render internal/router/routes.go from manifest.Routes
  │     └─→ routes.tmpl ranges over Routes, emits:
  │           handler.NewUserHandler().Register(mux)  // Standard/Minimalist
  │           adapters.NewUserHandler().Register(mux) // Hexagonal
  └─→ Record routes.go in manifest (origin: component)

generate handler Stats --route "GET /stats" (web project)
  ├─→ Validate pattern in scaffold layer (METHOD + path)
  ├─→ GenerateComponent creates handler file
  ├─→ Upsert RouteEntry{entity: "Stats", handler: "Stats", origin: "handler", route_pattern: "GET /stats"}
  ├─→ Re-render internal/router/routes.go from manifest.Routes
  │     └─→ routes.tmpl emits:
  │           mux.HandleFunc("GET /stats", handler.NewStatsHandler().ServeHTTP)
  └─→ Record routes.go in manifest

upgrade --yes (existing web project)
  ├─→ Re-render main.go with router.Register(mux) call (template changed)
  ├─→ Detect web project (UseTemplHTMX=true) + routes.go absent
  ├─→ Create routes.go with empty route list (func Register(mux) {})
  └─→ Record routes.go in manifest

new (web project)
  ├─→ scaffoldWeb creates views, static, handler, README
  ├─→ Create empty-list internal/router/routes.go (func Register(mux) {})
  └─→ Render main.go with router.Register(mux) call

main.go (after generate or upgrade)
  └─→ Calls router.Register(mux) after demo routes
        └─→ routes.go iterates manifest routes, registers all handlers
```

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/pkg/template/templates/common/routes.tmpl` | Create | Registry template: `package router`, conditional architecture-aware import, `func Register(mux *http.ServeMux)` ranging over manifest routes |
| `internal/pkg/template/templates/web/main.tmpl` | Modify | Add `router.Register(mux)` call after demo routes (line 44) + import `"{{ .ModuleName }}/internal/router"` (line 8) |
| `internal/pkg/scaffold/manifest.go` | Modify | Add `Routes []RouteEntry` field to Manifest struct; add `RouteEntry` type (Entity, Handler, Origin, RoutePattern); add `UpsertRoute` method |
| `internal/pkg/scaffold/scaffold.go` | Modify | (1) Add `manifestDir()` helper; (2) Use it in `ensureManifest`, `recordManifest`, `createBinaryFile`, `createFile`, `GenerateComponent` existence checks; (3) In `GenerateCRUD`, call `UpsertRoute` for web projects; (4) In `GenerateComponent` handler case, accept route option, validate pattern, call `UpsertRoute`; (5) Add `WithRoute` option; (6) Move pattern validation here |
| `cmd/generate.go` | Modify | Add `--route` flag to `generateCmd`; pass to `GenerateComponent` via `WithRoute`; update help text |
| `internal/pkg/scaffold/upgrade.go` | Modify | (1) Extend `buildRenderData` to inject manifest.Routes for routes.go entries; (2) Add absent-routes.go creation logic in Upgrade loop |
| `internal/pkg/mcp/server.go` | Modify | Add optional `route` property to `generate_component` schema; pass to scaffolder via `WithRoute` |

## Interfaces / Contracts

### Manifest Schema Extension

```go
// RouteEntry represents one route registration in the manifest.
type RouteEntry struct {
    Entity       string `yaml:"entity"`                 // e.g. "User"
    Handler      string `yaml:"handler"`                // e.g. "User" (for NewUserHandler)
    Origin       string `yaml:"origin"`                 // "crud" or "handler"
    RoutePattern string `yaml:"route_pattern,omitempty"` // e.g. "GET /stats" (handler only)
}

// Manifest (extended)
type Manifest struct {
    Version int                      `yaml:"version"`
    Files   map[string]ManifestEntry `yaml:"files"`
    Routes  []RouteEntry             `yaml:"routes,omitempty"` // ADDITIVE
    dir     string                   `yaml:"-"`
}

// UpsertRoute upserts a route entry by entity (dedupe) and re-renders routes.go.
func (m *Manifest) UpsertRoute(entry RouteEntry) error {
    // Find existing by entity, replace if found, else append
    for i, r := range m.Routes {
        if r.Entity == entry.Entity {
            m.Routes[i] = entry
            return m.Save()
        }
    }
    m.Routes = append(m.Routes, entry)
    return m.Save()
}
```

### Route Template Data

```go
// routes.tmpl receives:
type RoutesData struct {
    ModuleName   string
    Architecture string
    Routes       []RouteEntry
}
```

### Template Content (routes.tmpl)

```go
package router

{{ if .Routes }}
import (
    "net/http"
    {{ if eq .Architecture "Hexagonal" }}
    "{{ .ModuleName }}/internal/adapters"
    {{ else }}
    "{{ .ModuleName }}/internal/handler"
    {{ end }}
)
{{ end }}

func Register(mux *http.ServeMux) {
{{ range .Routes }}
{{ if eq .Origin "crud" }}
    {{ if eq $.Architecture "Hexagonal" }}adapters{{ else }}handler{{ end }}.New{{ .Handler }}Handler().Register(mux)
{{ else }}
    mux.HandleFunc("{{ .RoutePattern }}", {{ if eq $.Architecture "Hexagonal" }}adapters{{ else }}handler{{ end }}.New{{ .Handler }}Handler().ServeHTTP)
{{ end }}
{{ end }}
}
```

### web/main.tmpl Changes

```go
import (
    "fmt"
    "log"
    "net/http"

    "{{ .ModuleName }}/internal/handler"
    "{{ .ModuleName }}/internal/router"  // ADD
    ...
)

func main() {
    ...
    mux := http.NewServeMux()

    mux.HandleFunc("GET /", handler.PageHandler)
    mux.HandleFunc("POST /counter", handler.CounterHandler)
    mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

    router.Register(mux)  // ADD

    fmt.Println("Listening on :8080")
    ...
}
```

### Manifest Path Resolution (manifestDir Helper)

```go
// manifestDir returns the project root for manifest operations.
// In generate context (manifest exists in CWD), returns ".".
// In new context (no manifest yet), returns s.config.ProjectName.
func (s *Scaffolder) manifestDir() string {
    if ManifestExists(".") {
        return "."
    }
    return s.config.ProjectName
}

// ensureManifest uses manifestDir()
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

// recordManifest uses manifestDir() for hash path
func (s *Scaffolder) recordManifest(targetPath, templatePath string, origin Origin, metadata map[string]string) {
    m, err := s.ensureManifest()
    if err != nil {
        recordManifestWarning("manifest load failed: %v", err)
        return
    }
    fullPath := filepath.Join(s.manifestDir(), targetPath)  // FIX: use manifestDir()
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
        Metadata:     metadata,
    })
    if err := m.Save(); err != nil {
        recordManifestWarning("manifest save failed: %v", err)
    }
}

// createBinaryFile uses manifestDir()
func (s *Scaffolder) createBinaryFile(targetPath, embeddedPath string) error {
    full := filepath.Join(s.manifestDir(), targetPath)  // FIX: use manifestDir()
    // ... rest unchanged
}

// createFile uses manifestDir()
func (s *Scaffolder) createFile(path string, templatePath string, data interface{}) error {
    fullPath := filepath.Join(s.manifestDir(), path)  // FIX: use manifestDir()
    // ... rest unchanged
}

// GenerateComponent existence checks use manifestDir()
case "page":
    // ...
    if _, err := os.Stat(filepath.Join(s.manifestDir(), targetPath)); err == nil {  // FIX
        return oops.Code("component_already_exists").
            Hint("Choose a different name or delete the existing file").
            Errorf("target file already exists: %s", targetPath)
    }
case "component":
    // ...
    if _, err := os.Stat(filepath.Join(s.manifestDir(), targetPath)); err == nil {  // FIX
        return oops.Code("component_already_exists").
            Hint("Choose a different name or delete the existing file").
            Errorf("target file already exists: %s", targetPath)
    }
```

### GenerateComponent Variadic Options

```go
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

// GenerateComponent generates a specific component (service, repository, handler)
func (s *Scaffolder) GenerateComponent(compType, name string, opts ...GenerateOption) error {
    cfg := &generateConfig{}
    for _, opt := range opts {
        opt(cfg)
    }

    // ... existing switch cases ...

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
    }

    if err := s.createFile(targetPath, templatePath, data); err != nil {
        return err
    }

    // Re-record with correct origin
    meta := map[string]string{"entity_name": name}
    s.recordManifest(targetPath, templatePath, OriginComponent, meta)

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
            // Re-render routes.go
            _ = s.renderRoutesRegistry()
        }
    }

    return nil
}

// isValidRoutePattern validates "METHOD /path" format.
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

// renderRoutesRegistry re-renders internal/router/routes.go from manifest.Routes.
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
    return s.createFile("internal/router/routes.go", "common/routes.tmpl", data)
}
```

### Upgrade Re-Renders routes.go

```go
// buildRenderData extended to inject manifest.Routes for routes.go
func buildRenderData(cfg *ui.ProjectConfig, entry ManifestEntry, m *Manifest) interface{} {
    // Special case: routes.go needs the route list
    if entry.Path == "internal/router/routes.go" {
        return RoutesData{
            ModuleName:   cfg.ModuleName,
            Architecture: cfg.Architecture,
            Routes:       m.Routes,
        }
    }
    if entityName, ok := entry.Metadata["entity_name"]; ok {
        return struct {
            ui.ProjectConfig
            EntityName string
        }{
            ProjectConfig: *cfg,
            EntityName:    entityName,
        }
    }
    return cfg
}

// Upgrade function extended to pass manifest to buildRenderData
func Upgrade(cfg *ui.ProjectConfig) (*UpgradePlan, error) {
    root := "."
    if !ManifestExists(root) {
        return upgradeLegacy(cfg, root)
    }
    m, err := LoadManifest(root)
    if err != nil {
        return nil, err
    }
    engine := template.NewEngine()
    plan := &UpgradePlan{ProjectRoot: root}

    for path, entry := range m.Files {
        if path == ".go-arch.yaml" {
            continue
        }

        action := FileAction{
            Path:         path,
            Origin:       entry.Origin,
            ManifestHash: entry.SHA256,
        }

        fullPath := filepath.Join(root, path)
        diskBytes, err := os.ReadFile(fullPath)
        if err != nil {
            if os.IsNotExist(err) {
                action.Classification = ClassAbsent
                plan.Files = append(plan.Files, action)
                continue
            }
            return nil, err
        }
        diskHash := hashBytes(diskBytes)
        action.DiskHash = diskHash

        // go.mod is always report-only
        if path == "go.mod" {
            rerender, rerenderErr := renderEntry(engine, entry, cfg, m)
            if rerenderErr == nil {
                rerenderHash := hashBytes(rerender)
                action.RerenderHash = rerenderHash
                action.RerenderBytes = rerender
                if diskHash != rerenderHash {
                    action.Classification = ClassUpgradable
                } else {
                    continue
                }
            } else {
                continue
            }
            plan.Files = append(plan.Files, action)
            continue
        }

        // routes.go: always re-render from manifest.Routes (never PROTECTED)
        if path == "internal/router/routes.go" {
            rerender, err := renderEntry(engine, entry, cfg, m)
            if err != nil {
                continue
            }
            rerenderHash := hashBytes(rerender)
            action.RerenderHash = rerenderHash
            action.RerenderBytes = rerender
            if diskHash != rerenderHash {
                action.Classification = ClassUpgradable
            } else {
                continue
            }
            plan.Files = append(plan.Files, action)
            continue
        }

        // Disk vs manifest: user-modified?
        if diskHash != entry.SHA256 {
            action.Classification = ClassProtected
            plan.Files = append(plan.Files, action)
            continue
        }

        // Disk matches manifest — re-render to check for template changes
        rerender, err := renderEntry(engine, entry, cfg, m)
        if err != nil {
            continue
        }
        rerenderHash := hashBytes(rerender)
        action.RerenderHash = rerenderHash

        if rerenderHash == diskHash {
            continue
        }

        action.Classification = ClassUpgradable
        action.RerenderBytes = rerender

        if strings.HasPrefix(path, "views/") || path == "static/css/style.css" {
            plan.TemplHint = true
        }

        plan.Files = append(plan.Files, action)
    }

    return plan, nil
}

// renderEntry extended to pass manifest
func renderEntry(engine *template.Engine, entry ManifestEntry, cfg *ui.ProjectConfig, m *Manifest) ([]byte, error) {
    if entry.Origin == OriginBinary {
        data, err := template.TemplatesFS.ReadFile("templates/" + entry.TemplatePath)
        return data, err
    }
    data := buildRenderData(cfg, entry, m)
    var buf bytes.Buffer
    if err := engine.RenderTo(&buf, entry.TemplatePath, data, true); err != nil {
        return nil, err
    }
    return buf.Bytes(), nil
}
```

### scaffoldWeb Creates Empty routes.go

```go
func (s *Scaffolder) scaffoldWeb() error {
    dirs := []string{
        "views/layouts",
        "views/pages",
        "views/components",
        "static/css",
        "static/js",
        "internal/router",  // ADD: ensure router dir exists
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

    // Create empty-list routes.go so project compiles immediately
    data := RoutesData{
        ModuleName:   s.config.ModuleName,
        Architecture: s.config.Architecture,
        Routes:       []RouteEntry{},  // empty
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
```

### MCP Schema Extension

```go
"generate_component": {
    "inputSchema": {
        "properties": {
            ...existing...
            "route": {
                "type":        "string",
                "description": "Route pattern for handler type (e.g. 'GET /stats'). Ignored for other types.",
            },
        },
    },
}

// MCP handler extended
case "generate_component":
    var args struct {
        Type        string `json:"type"`
        Name        string `json:"name"`
        ProjectPath string `json:"projectPath"`
        Route       string `json:"route"`  // ADD
    }
    // ... unmarshal ...
    
    scaffolder := scaffold.NewScaffolder(cfg)
    var err error
    if args.Type == "crud" {
        err = scaffolder.GenerateCRUD(args.Name)
    } else {
        var opts []scaffold.GenerateOption
        if args.Route != "" {
            opts = append(opts, scaffold.WithRoute(args.Route))
        }
        err = scaffolder.GenerateComponent(args.Type, args.Name, opts...)
    }
```

### Call Sites Updated (GenerateComponent Variadic)

**No changes needed** for existing callers (variadic accepts zero options):
- `cmd/generate.go:58` → `scaffolder.GenerateComponent(compType, name)` (unchanged)
- `internal/pkg/mcp/server.go:358` → add `opts` parameter when `args.Route != ""`
- `internal/pkg/scaffold/scaffold_test.go` lines 835, 995, 1037, 1083, 1116, 1160, 1218, 1256 → unchanged (no route option)

**Only cmd/generate.go and mcp/server.go add the option** when --route is provided.

## Testing Strategy

| Layer | What to Test | Approach |
|-------|-------------|----------|
| Unit | Registry content after CRUD (5 routes + Register call) | Generate CRUD in temp web project, assert routes.go contains `NewUserHandler().Register(mux)` and all 5 route patterns |
| Unit | Registry architecture-aware for Hexagonal | Generate CRUD in Hexagonal web project, assert imports `internal/adapters` and calls `adapters.NewUserHandler()` |
| Unit | CRUD idempotent (twice → one registration) | Generate CRUD twice, assert exactly one `NewUserHandler().Register(mux)` call |
| Unit | Registry deterministic under upgrade | Re-render routes.go twice with same manifest, assert byte-identical |
| Unit | Handler with --route registers | Generate handler with `--route "GET /stats"`, assert registry has `mux.HandleFunc("GET /stats", handler.NewStatsHandler().ServeHTTP)` |
| Unit | Handler without --route unchanged | Generate handler without flag, assert routes.go byte-identical to pre-generate state |
| Unit | Non-web project gets hint only | Generate CRUD in non-web project, assert no routes.go created/modified, hint printed |
| Unit | main.go unchanged after generate | Generate CRUD, assert main.go sha256 matches fresh template render |
| Unit | Upgrade creates routes.go in existing web project | Simulate existing web project without routes.go, run upgrade, assert routes.go created with `func Register(mux *http.ServeMux) {}` |
| Unit | Upgrade does not mark routes.go PROTECTED | Register route, run upgrade, assert routes.go classified up-to-date (not PROTECTED) |
| Unit | Generate resolves paths at CWD | Generate handler in project with `project_name: realapp`, assert file written to `./internal/handler/User_handler.go` (no nesting) |
| Unit | new command path resolution unchanged | Scaffold new project with `project_name: myapp`, assert files under `myapp/` |
| Unit | MCP handler with route | Call MCP generate_component(type=handler, route="GET /stats"), assert registry updated |
| Unit | MCP crud updates registry | Call MCP generate_component(type=crud), assert routes.go re-rendered |
| Unit | Check passes with router dir | Create project with `internal/router/routes.go`, run validator, assert exit 0 |
| Unit | --route rejected in non-web | Run `generate handler Stats --route "GET /stats"` in non-web project, assert `web_scaffold_required` error |
| Unit | invalid_route_pattern emitted | Run `generate handler X --route "BADPATTERN"`, assert `invalid_route_pattern` error |
| Unit | Empty routes.go compiles | Scaffold new web project, assert `internal/router/routes.go` exists with empty `func Register(mux)` and compiles |
| Unit | manifestDir() returns CWD in generate context | Create manifest in CWD, call manifestDir(), assert returns "." |
| Unit | manifestDir() returns ProjectName in new context | No manifest in CWD, call manifestDir(), assert returns ProjectName |
| Integration | Full workflow: generate + upgrade + generate again | Scaffold web project, generate CRUD, upgrade, generate handler, assert all registrations present, main.go byte-identical |
| Functional | Generated project compiles and runs | Generate web project, generate CRUD, run `go build`, `go test`, `go-arch check` in generated project |

## Threat Matrix

N/A — no routing, shell, subprocess, VCS/PR automation, executable-file classification, or process-integration boundary. This change generates a router file in user projects but does not change the go-arch CLI's own routing, shell, or process behavior.

## Migration / Rollout

No migration required. The change is additive:
- New template file (`routes.tmpl`) embedded in binary.
- Manifest schema extended with optional `routes:` field (backward compatible).
- Existing projects get routes.go via `upgrade --yes` (creates absent file with empty list).
- New projects get routes.go via `scaffoldWeb` (empty list during `new`, populated on first generate).
- All manifest path operations use `manifestDir()` helper (no split-brain).

## Open Questions

- [ ] None — all scope and approach decisions resolved in proposal and exploration.
