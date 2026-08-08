# Design: templ + HTMX Frontend Flag (`UseTemplHTMX`)

**Status**: draft · **Author**: sdd-design · **Input**: proposal, exploration, specs (`templ-htmx-frontend`, `hexagonal-build-fix`, `cli`)
**templ pin**: `github.com/a-h/templ v0.3.906` (Context7-verified current stable — kept from proposal).

## Executive Summary

Single architecture-agnostic `web/main.tmpl` superset, gated architecture mains (`if !UseTemplHTMX`), binary htmx copy bypass, in-memory mutex counter in `internal/handler/page.go`. Additive, flag-gated, zero regression when OFF.

## Context & Constraints (verified)

- `scaffoldStandard`/`Hexagonal` write `cmd/api/main.go` BEFORE `createCommonFiles()` (scaffold.go:86, :105). Web main inside `createCommonFiles` would silently clobber telemetry/gRPC. **Resolution: guard arch mains + superset web main (Option A).**
- `createFile(path, templatePath, data)` (scaffold.go:44-63) uses `engine.Render` (`text/template`). htmx source contains `{{`/`}}` → engine chokes. **Resolution: new `createBinaryFile` helper bypassing engine.**
- Fresh Hexagonal projects do NOT build today: `hexagonal/main.tmpl:7-8` imports empty `internal/adapters` and `internal/domain` dirs. Web main MUST NOT copy those imports.
- `TemplatesFS` (engine.go:17-19) uses `//go:embed all:templates/*`; any `templates/web/*` lands in the embedded FS with the `templates/...` prefix.
- **scaffoldStandard creates `internal/handler/` by default** (scaffold.go:76) — this is legitimate Standard architecture structure, not a web-flag side effect. Test assertions must not flag this as a regression.

## Architecture Decisions

| ID | Decision | Options | Choice | Rationale |
|---|---|---|---|---|
| AD-1 | Web main placement | A: single superset `web/main.tmpl` B: fork HTTP blocks into each arch main | **A** | Single source of truth; no duplication; dodges hex empty-import bug; easier to test. |
| AD-2 | Architecture main collision | A: guard `if !UseTemplHTMX` + overwrite B: merge into templates | **A** | Guard is one-line per arch; matches existing `if s.config.UseX` block pattern in `createCommonFiles`. |
| AD-3 | htmx copy | A: engine.Render B: `TemplatesFS.ReadFile` + `os.WriteFile` | **B** | htmx has `{{`/`}}` tokens (text/template chokes); binary asset should not participate in local/global override chain. |
| AD-4 | Counter state | A: package-level `sync.Mutex` + int B: struct receiver with DI C: JS-based counter | **A** | Demo scope — no DI needed; mutex satisfies concurrency correctness; keeps handler simple and testable. |
| AD-5 | Hex build fix | A: drop `internal/adapters` + `internal/domain` imports from `hexagonal/main.tmpl` B: generate placeholder Go files | **A** | Additive; future features re-add imports with tests; no dead code shipped. |
| AD-6 | Web main routing | Use Go 1.22+ `http.ServeMux` method patterns (`"GET /"`, `"POST /counter"`) | Chosen | Go 1.24 is the floor (go.mod.tmpl:3); no external router dep; spec explicitly says "no SPA / no router lib". |

## Component Design

### `ProjectConfig` (`internal/ui/prompts.go`)

Add field after `UseGRPC`:

```go
UseTemplHTMX bool `mapstructure:"use_templ_htmx"`
```

`RunWizard`: append `survey.Confirm` question to `mainQs` (message: "Include templ + HTMX frontend?", default false) after the gRPC question.

### `scaffoldWeb()` method (`internal/pkg/scaffold/scaffold.go`)

```go
func (s *Scaffolder) scaffoldWeb() error {
    dirs := []string{"views/layouts","views/pages","views/components","static/css","static/js"}
    for _, d := range dirs { if err := os.MkdirAll(filepath.Join(s.config.ProjectName, d), 0755); err != nil { return err } }

    views := []struct{ target, tmpl string }{
        {"views/layouts/base.templ",      "web/base.templ.tmpl"},
        {"views/pages/home.templ",        "web/page.templ.tmpl"},
        {"views/components/counter.templ","web/component.templ.tmpl"},
        {"static/css/style.css",          "web/style.css.tmpl"},
        {"internal/handler/page.go",      "web/handler.tmpl"},
        {"README.md",                     "web/readme.tmpl"},
    }
    for _, v := range views { if err := s.createFile(v.target, v.tmpl, nil); err != nil { return err } }

    if err := s.createBinaryFile("static/js/htmx.min.js", "web/htmx.min.js"); err != nil { return err }

    target := "cmd/api/main.go"
    if s.config.Architecture == "Minimalist" { target = "main.go" }
    return s.createFile(target, "web/main.tmpl", nil)
}
```

Hook in `createCommonFiles()` after the gRPC block:

```go
if s.config.UseTemplHTMX { return s.scaffoldWeb() }
```

### Guard architecture mains

- `scaffoldMinimalist`: wrap `createFile("main.go", "minimalist/main.tmpl", nil)` with `if !s.config.UseTemplHTMX`.
- `scaffoldStandard`: wrap `createFile("cmd/api/main.go", "standard/main.tmpl", nil)` with `if !s.config.UseTemplHTMX`.
- `scaffoldHexagonal`: wrap `createFile("cmd/api/main.go", "hexagonal/main.tmpl", nil)` with `if !s.config.UseTemplHTMX`.

### `createBinaryFile(targetPath, embeddedPath)` (`scaffold.go`)

```go
func (s *Scaffolder) createBinaryFile(targetPath, embeddedPath string) error {
    full := filepath.Join(s.config.ProjectName, targetPath)
    if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil { return err }
    data, err := template.TemplatesFS.ReadFile("templates/" + embeddedPath)
    if err != nil { return err }
    return os.WriteFile(full, data, 0644)
}
```

Deliberately bypasses engine + local/global override chain (asset, not user template).

### `hexagonal/main.tmpl` fix

Drop lines 7-8 (`{{ .ModuleName }}/internal/adapters`, `{{ .ModuleName }}/internal/domain`) and the "Wire up ports and adapters" comment. Result compiles with `UseTemplHTMX=false`; web path never uses this template (guarded).

### `templates/web/main.tmpl` — superset

Imports: `context`, `fmt`, `log`, `net/http`, `{{ .ModuleName }}/internal/handler`. Conditional: `internal/telemetry` (if UseObservability), `internal/adapters/grpc` (if UseGRPC). Body: `fmt.Println` header → optional telemetry init + defer shutdown → optional gRPC goroutine → `mux := http.NewServeMux()` → `mux.HandleFunc("GET /", handler.PageHandler)` → `mux.HandleFunc("POST /counter", handler.CounterHandler)` → `mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))` → `http.ListenAndServe(":8080", mux)`. Does NOT import `internal/adapters` or `internal/domain`.

### `internal/handler/page.go` (`templates/web/handler.tmpl`)

```go
package handler

import (
    "net/http"
    "sync"
    "{{ .ModuleName }}/views/components"
    "{{ .ModuleName }}/views/pages"
)

var (
    counterMu sync.Mutex
    counter   int
)

func PageHandler(w http.ResponseWriter, r *http.Request) {
    counterMu.Lock(); c := counter; counterMu.Unlock()
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    _ = pages.Home(c).Render(r.Context(), w)
}

func CounterHandler(w http.ResponseWriter, r *http.Request) {
    counterMu.Lock(); counter++; c := counter; counterMu.Unlock()
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    _ = components.Counter(c).Render(r.Context(), w)
}
```

templ signatures (`views/pages/home.templ`, `views/components/counter.templ`):

```
templ Home(count int)        → composes layouts.Base(count)
templ Base(count int)        → HTML shell: <link /static/css/style.css>, <script /static/js/htmx.min.js>, @components.Counter(count)
templ Counter(count int)     → <div id="counter" hx-post="/counter" hx-target="#counter" hx-swap="innerHTML">{ count }</div>
```

POST `/counter` returns only the fragment → `hx-target="#counter"` swap works, no 404.

### `common/go.mod.tmpl` — conditional templ require

```
{{ if .UseTemplHTMX }}
require (
    github.com/a-h/templ v0.3.906
)
{{ end }}
```

### `common/config.tmpl` — round-trip

Append: `use_templ_htmx: {{ .UseTemplHTMX }}` after `use_grpc`.

### `cmd/new.go` — conditional success message

After `ui.Success(...)` line 41, add:

```go
if config.UseTemplHTMX {
    ui.Info("Next steps: cd into your project, run `templ generate`, then `go run .` (Minimalist) or `go run ./cmd/api` (Standard/Hexagonal).")
}
```

### `templates/web/readme.tmpl` → project `README.md`

Sections: title, prerequisites (`go install github.com/a-h/templ/cmd/templ@latest`), usage (`templ generate`, `go run .` or `go run ./cmd/api`), architecture notes, **BSD-2-Clause htmx v1.9.12 attribution** with upstream URL.

## Data Flow

```
cmd/new.go ──► RunWizard() ──► config.UseTemplHTMX=true
     │
     ▼
scaffolder.Execute()
     │
     ├── scaffoldMinimalist/Standard/Hexagonal
     │       └── if !UseTemplHTMX → createFile(arch main)
     │       └── createCommonFiles()
     │               ├── go.mod, config, env
     │               ├── if UseDocker → Dockerfile, compose
     │               ├── if UseObservability → telemetry
     │               ├── if UseGRPC → proto, grpc_server, Makefile
     │               └── if UseTemplHTMX → scaffoldWeb()
     │                       ├── mkdir views/*, static/*
     │                       ├── createFile × 6 (templ views, css, handler, readme)
     │                       ├── createBinaryFile htmx.min.js
     │                       └── createFile web main → cmd/api/main.go | main.go
     └── done
```

## File Changes

| File | Action | Description |
|---|---|---|
| `internal/ui/prompts.go` | Modify | +`UseTemplHTMX` field + Confirm wizard question |
| `internal/pkg/scaffold/scaffold.go` | Modify | Guard 3 arch mains + `scaffoldWeb()` + `createBinaryFile` helper + `createCommonFiles` hook |
| `internal/pkg/scaffold/scaffold_test.go` | Modify | Table cases: flag OFF unchanged; flag ON all archs; hex+OFF builds; web main content; htmx byte-identity; handler unit tests |
| `internal/pkg/template/templates/common/go.mod.tmpl` | Modify | Conditional templ require block |
| `internal/pkg/template/templates/common/config.tmpl` | Modify | `use_templ_htmx` line |
| `internal/pkg/template/templates/hexagonal/main.tmpl` | Modify | Drop empty-package imports (hex build fix) |
| `cmd/new.go` | Modify | Conditional templ-generate success hint |
| `internal/pkg/template/templates/web/base.templ.tmpl` | Create | Base HTML shell |
| `internal/pkg/template/templates/web/page.templ.tmpl` | Create | Home page composing base |
| `internal/pkg/template/templates/web/component.templ.tmpl` | Create | Counter fragment with `hx-*` |
| `internal/pkg/template/templates/web/style.css.tmpl` | Create | Dark base styles |
| `internal/pkg/template/templates/web/handler.tmpl` | Create | `internal/handler/page.go` source |
| `internal/pkg/template/templates/web/main.tmpl` | Create | Superset web main |
| `internal/pkg/template/templates/web/readme.tmpl` | Create | Generated README with htmx attribution |
| `internal/pkg/template/templates/web/htmx.min.js` | Create | Binary vendored asset (v1.9.12, BSD-2-Clause) |

## Interfaces / Contracts

- `ProjectConfig.UseTemplHTMX bool` — `mapstructure:"use_templ_htmx"`, default false.
- `(*Scaffolder).scaffoldWeb() error` — new method, unexported.
- `(*Scaffolder).createBinaryFile(targetPath, embeddedPath string) error` — new helper.
- `handler.PageHandler(w, r)` / `handler.CounterHandler(w, r)` — exported HTTP handlers in generated code.
- templ components: `pages.Home(count int)`, `layouts.Base(count int)`, `components.Counter(count int)` — all return `templ.Component` via generated code.

## Testing Strategy

| Layer | Case | Approach |
|---|---|---|
| Unit | Flag OFF unchanged (all 3 archs) | Extend `TestScaffolder_Layouts` table; assert no `views/`, `static/` dirs exist for ALL archs; assert no `internal/handler/page.go` file for Minimalist and Hexagonal only (Standard legitimately creates `internal/handler/` dir per scaffold.go:76, but the web `page.go` file must be absent) |
| Unit | Flag ON all files present (×3 archs) | Assert existence of templ views, handler, static, README, web main |
| Unit | Web main path correct | Minimalist→`main.go`; Standard/Hexagonal→`cmd/api/main.go` |
| Unit | Web main content | Read generated main; assert contains `http.ListenAndServe` and `internal/handler`; asserts does NOT contain `internal/adapters` or `internal/domain` |
| Unit | htmx byte-identity | `bytes.Equal(TemplatesFS.ReadFile("templates/web/htmx.min.js"), os.ReadFile(target))` |
| Unit | Hexagonal + OFF builds | Run `go mod tidy && go build ./...` in generated project (fresh module has empty go.sum), assert exit 0 |
| Unit | Hexagonal + ON builds post-templ-generate | Run `go mod tidy && templ generate` (skip if `templ` binary absent → `t.Skip`), then `go build ./...` exit 0 |
| Unit | Config round-trip | Read `.go-arch.yaml`, assert `use_templ_htmx:` matches input |
| Unit | Templ require conditional | Render `go.mod.tmpl` with flag on/off; assert presence/absence of `github.com/a-h/templ` |
| Unit | README content | Read generated `README.md`; assert contains `templ generate` instructions AND `BSD-2-Clause` htmx attribution text |
| Unit | View files contain HTMX attributes | Read `views/components/counter.templ`; assert contains `hx-post`, `hx-target`, `hx-swap` (not just file existence) |
| Unit | Handler functional test — GET / | In generated project (after `templ generate`), construct `handler.PageHandler` and call with `httptest.NewRecorder()` + `httptest.NewRequest("GET", "/", nil)`; assert status 200 and body contains "counter" or page markup |
| Unit | Handler functional test — POST /counter | Call `handler.CounterHandler` with `httptest.NewRecorder()` + `httptest.NewRequest("POST", "/counter", nil)`; assert status 200 and body contains "1" (first increment) |
| Unit | Handler state persistence | Call `CounterHandler` twice; second call returns "2" proving `sync.Mutex`-guarded state persists across invocations |

**Handler test implementation approach**: After `templ generate` runs (skip if binary absent), the generated `.templ` files produce `views/pages/pages_templ.go` and `views/components/components_templ.go`. The handler imports these packages directly. Test constructs HTTP requests with `httptest`, calls handlers directly (no server needed), asserts response status and body content. This validates the functional counter spec requirement without spinning up a full HTTP server.

## Threat Matrix

N/A — no routing (production), shell commands, subprocesses, VCS/PR automation, executable-file classification, or process-integration boundary. The change writes files via `os.WriteFile` and `text/template.Execute` only.

## Migration / Rollout

No migration required. Additive, flag-gated. Hex fix is a standalone correct bug fix (independent commit-friendly).

## Risks & Mitigations

| Risk | Mitigation |
|---|---|
| templ version drift | Pin `v0.3.906`; generated project `go mod tidy` is safety net (runs before `go build` in all build tests). |
| htmx attribution missed | README template includes BSD-2-Clause block; htmx license header preserved in vendored file. |
| User forgets `templ generate` | `cmd/new.go` conditional hint + README front-and-center instructions. |
| Web main silent overwrite of telemetry | Guard `if !UseTemplHTMX` on arch mains + test asserting `http.ListenAndServe` presence when flag ON. |
| Hex fix regresses future features | Additive (drops unused imports); future feature PRs re-add with tests. |
| Go 1.22+ method routing assumed | Go 1.24 is the project floor (`go.mod.tmpl:3`); documented in README. |
| Empty go.sum in fresh module | All build tests run `go mod tidy` before `go build ./...` to populate go.sum. |

## Open Questions

- None blocking. (Optional follow-up: wire `templ generate --watch` via Air in a future change — explicitly out of scope.)

## Next Recommended Step

`sdd-tasks` — break this design into implementation tasks (flag + wizard, go.mod/config tmpl, hex fix, `scaffoldWeb` + `createBinaryFile` + guards, web templates, handler, main, readme, tests, success message).
