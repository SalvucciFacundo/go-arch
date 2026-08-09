# Exploration: Route Auto-Registration in `go-arch generate`

## Status

**Feasible, with scope carved out.** Route auto-registration should be **default for CRUD**, **opt-in via `--route` for plain handlers**, and **only active when the project has a router** (templ+HTMX web scaffold — the only layout whose main file creates an `http.ServeMux`). Recommended mechanism: a **generated routes registry file** (`internal/router/routes.go`) with a `Register(mux *http.ServeMux)` func that `main` calls — `generate` re-renders that registry from a **manifest-held route list**, never touching `main.go`. This preserves upgrade's PROTECTED contract (user-owned main.go) and avoids AST surgery.

**Critical discovery:** `go-arch generate` has a **pre-existing path-resolution inconsistency** with ADR-7. Scaffold's `createFile` joins `ProjectName` as a *directory* (`filepath.Join(s.config.ProjectName, path)`), so running `go-arch generate handler User` from *inside* a real project (`.go-arch.yaml` has `project_name: realapp`) writes to `realapp/realapp/internal/handler/User_handler.go` — a NESTED directory. All existing tests dodge this by setting `project_name: "."`. Route registration must NOT inherit this bug: the router target must be resolved against CWD (like upgrade's ADR-7), or the feature will edit the wrong file. **This must be resolved (and regression-tested) as part of this change or explicitly carved out with a documented limitation.**

## Executive Summary

The design already anticipated route registration: `common/crud_handler.tmpl` ships a `Register(mux *http.ServeMux)` method with the 5 CRUD routes, and `GenerateCRUD` prints "📍 Remember to register the routes in your main router." (scaffold.go:412). The missing piece is the plumbing: who calls `Register`, and where the route lines live.

Three mechanisms were explored:

| Approach | Verdict | Why |
|----------|---------|-----|
| (a) Template-regenerate main.go wholesale | **Rejected** | Clobbers user edits; manifest hash stale → PROTECTED loop; main.go is user-owned territory |
| (b) AST-based insertion (go/ast, go/printer) | **Rejected for default; viable fallback** | Surgical but fragile: import rewriting, mux var rename, non-gofmt files, `printer` comment drift; still makes main.go diverge from manifest |
| (c) **Generated routes registry + manifest-held route list** | **RECOMMENDED** | `internal/router/routes.go` with `Register(mux)`; `generate` re-renders it from a route list persisted in the manifest; `main.go` gets ONE template change (`router.Register(mux)`) propagated by upgrade |

The registry approach is the only one that keeps `main.go` byte-identical to its template (manifest hash stable → upgrade keeps working) while making route additions deterministic, idempotent, and testable without AST.

## Verified Current-State Facts

### 1. Generate dispatch (cmd/generate.go)
- `generate [type] [name]`, `cobra.ExactArgs(2)` (generate.go:26).
- Reads `.go-arch.yaml` via viper, maps a subset of fields into `ui.ProjectConfig` (generate.go:33-49): `project_name`, `module_name`, `architecture`, `db_driver`, `use_docker`, `use_templ_htmx`. **Does NOT read `use_observability`, `observability_backend`, `use_grpc`.**
- Dispatch: `crud` → `scaffolder.GenerateCRUD(name)`, else `scaffolder.GenerateComponent(compType, name)` (generate.go:55-59).
- Runs INSIDE an existing project (config required, generate.go:33-39).

### 2. Component/CRUD generation (internal/pkg/scaffold/scaffold.go)
- `GenerateComponent` switch (scaffold.go:286-358):
  - `handler` (scaffold.go:349-355): Hexagonal → `internal/adapters/{name}_handler.go`; Standard/other → `internal/handler/{name}_handler.go`. Template `common/handler.tmpl`.
  - `service` (335-341): Hexagonal → `internal/domain/`, else `internal/service/`.
  - `repository` (342-348): Hexagonal → `internal/ports/`, else `internal/repository/`.
  - `page`/`component` (299-334): web-only, templ views.
- `GenerateCRUD` (scaffold.go:372-414): Hexagonal → model in `internal/domain`, service in `internal/domain`, port in `internal/ports`, repo + handler in `internal/adapters`; Standard → model/service/repository/handler under `internal/…`. Handler always uses `common/crud_handler.tmpl`.
- **`createFile` (scaffold.go:90-114) writes to `filepath.Join(s.config.ProjectName, path)`** — path resolution bug described above (verified empirically: real project with `project_name: realapp` → nested `realapp/realapp/internal/handler/User_handler.go`).
- `recordManifest` (scaffold.go:45-67) hashes the written file and upserts the manifest entry after every write (origin scaffold/component/crud).
- GenerateCRUD prints the manual-wiring hint (scaffold.go:412): `"📍 Remember to register the routes in your main router."`

### 3. The generated project's router
- **Only the templ+HTMX web main creates a ServeMux.** `internal/pkg/template/templates/web/main.tmpl` (main.tmpl:40-44):
  ```go
  mux := http.NewServeMux()
  mux.HandleFunc("GET /", handler.PageHandler)
  mux.HandleFunc("POST /counter", handler.CounterHandler)
  mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
  ```
- Standard main (`standard/main.tmpl:37-41`) has **no ServeMux and no HTTP server** — just a "// Handler logic here" comment. Hexagonal main (`hexagonal/main.tmpl:41-44`) also has **no ServeMux**. Minimalist main (`minimalist/main.tmpl:12-23`) has **no ServeMux**.
- Web main target path: Minimalist → `main.go`, Standard/Hexagonal → `cmd/api/main.go` (scaffold.go:171-175).
- Web main imports `{{ .ModuleName }}/internal/handler` (web/main.tmpl:8) — the templ handlers (`page.go` from `web/handler.tmpl`).
- **Implication: route auto-registration is only meaningful for templ+HTMX projects.** Plain Standard/Hexagonal/Minimalist projects have no router to register into. CRUD in a non-web project still prints the manual hint — correct, keep it.

### 4. The Register method (already in the templates)
- `common/crud_handler.tmpl:15-21`: `func (h *{{ .EntityName }}Handler) Register(mux *http.ServeMux)` registers the 5 CRUD routes at `/{{ .EntityName | lower | plural }}` (+ `/{id}` variants), using the `plural` inflection funcmap (engine.go:85).
- `common/handler.tmpl` (plain handler): **no Register method** — only `ServeHTTP` (handler.tmpl:16-17). A plain handler has no route pattern; registration for plain handlers needs a user-supplied pattern → `--route` flag.
- Both templates ship `New{{ .EntityName }}Handler()` constructors (crud_handler.tmpl:23, handler.tmpl:12) — the natural callable for a registry.

### 5. Manifest & upgrade interaction (internal/pkg/scaffold/manifest.go, upgrade.go)
- Manifest: `manifest.yaml` at `.go-arch/.go-arch.yaml`… actually `ManifestPath` = `.go-arch/manifest.yaml` (manifest.go:40-42). Entry = path + sha256 + origin + template + metadata (manifest.go:24-30).
- Upgrade classifies (upgrade.go:22-27): `upgradable` (disk matches manifest, template re-render differs), `protected` (disk hash ≠ manifest hash → user-modified, NEVER overwrite, upgrade.go:131-136), `absent`, `up_to_date` (omitted).
- **If route registration edited main.go (approaches a/b), the manifest sha256 for main.go would immediately be stale → next `upgrade` classifies it PROTECTED forever.** That's actually *correct* semantics (the router became user-owned), but it means upgrade would never again apply template improvements to main.go. The registry approach avoids this: main.go stays byte-identical to its template; the only main.go change is a one-time template update propagated by upgrade.
- `renderEntry`/`buildRenderData` (upgrade.go:171-197) already reconstruct template data from manifest metadata — a route list persisted in the manifest can be re-rendered the same way, keeping the registry idempotent under upgrade.

### 6. MCP impact (internal/pkg/mcp/server.go)
- `generate_component` tool (server.go:166-188): schema `type` enum `[service, repository, handler, crud, page, component]`, `name`, `projectPath`. Dispatch mirrors CLI (server.go:314-365).
- Schema change needed ONLY if a `--route`-style param is added for plain handlers (e.g. optional `route` string property). CRUD default-on needs no schema change, only a description update. `route` must be an **optional** property to stay backward-compatible with existing agent callers.

### 7. Validator
- `validator.go:53-62` checks **required** dirs per architecture but does NOT forbid extra dirs — adding `internal/router/` passes `go-arch check` for all three layouts.

### 8. Existing tests / conventions
- scaffold_test.go, upgrade_test.go, manifest_test.go all chdir to a temp dir and use `ProjectName: "."` (e.g. scaffold_test.go:828, 988, 1030) — the nested-dir bug is untested/unseen.
- TestHandlerFunctional (scaffold_test.go:613-720) runs `templ generate` + `go test` in a generated project — the pattern to follow for router integration tests.
- TestUpgrade_ClassProtected (upgrade_test.go:124-169) proves a disk/manifest hash mismatch → PROTECTED → Apply skips it.

## Affected Areas

- `cmd/generate.go` — add `--route` flag; pass route intent into scaffolder.
- `internal/pkg/scaffold/scaffold.go` — new `GenerateHandler`/`GenerateCRUD` route-list append + registry re-render; fix/normalize path resolution (ADR-7 CWD semantics).
- `internal/pkg/scaffold/manifest.go` — hold the route list (new top-level field or metadata); (maybe) manifest version bump.
- `internal/pkg/template/templates/common/routes.tmpl` (NEW) — registry file template: `package router; func Register(mux *http.ServeMux) { … }`.
- `internal/pkg/template/templates/web/main.tmpl` — add `router.Register(mux)` call (one-line template change; propagates via upgrade).
- `internal/pkg/mcp/server.go` — optional `route` param + description update for `generate_component`.
- `internal/pkg/scaffold/upgrade.go` — routes.go must re-render idempotently from the manifest route list (so upgrade never marks it PROTECTED).
- Docs: `docs/COMMANDS.md`, README, ROADMAP item 3.

## Approaches

### 1. Template-regenerate main.go wholesale
Re-render the entire web main with the new route baked in (main.tmpl gains a `{{ range .Routes }}` block).
- Pros: No AST; routes look native; matches "one source of truth" template philosophy.
- Cons: **Clobbers any user edit to main.go** (ports, middleware, custom routes) — the exact thing upgrade PROTECTED exists to prevent; manifest hash stale → upgrade marks main.go PROTECTED and it diverges forever; needs the full route list in template data anyway (state problem identical to approach 3, but with destructive rewrite).
- Effort: Medium. **Verdict: Reject.**

### 2. AST-based insertion into main.go
Parse user's main.go (go/parser), find `main()` + `http.NewServeMux()`, insert `handler.NewXHandler().Register(mux)` (crud) or `mux.HandleFunc(pattern, …)` (handler) after the mux block, re-emit with go/printer.
- Pros: Surgical; preserves user edits elsewhere; stdlib only; works even if user has custom router wiring.
- Cons: Must add imports for `internal/adapters` (Hexagonal) to main.go; brittle against renamed `mux` var, non-gofmt source, printer comment/format drift; **still makes main.go diverge from manifest** → PROTECTED on next upgrade; idempotency requires detecting an already-registered route; much more test surface.
- Effort: High. **Verdict: Reject as default; keep as documented fallback for "user has no generated registry" edge.**

### 3. Generated routes registry + manifest-held route list (RECOMMENDED)
New generated file `internal/router/routes.go`:
```go
package router

import (
    "net/http"
    "{{ .ModuleName }}/internal/handler"
)

func Register(mux *http.ServeMux) {
    handler.NewUserHandler().Register(mux) // one line per generated route
}
```
- `generate` appends the new route's registration line to a **route list persisted in the manifest** (e.g. top-level `routes:` or per-entity metadata), then **re-renders routes.go from that list** (idempotent, deterministic).
- `web/main.tmpl` gains one call: `router.Register(mux)` after the demo routes (template change; existing projects get it via `go-arch upgrade`).
- Manifest: routes.go recorded like any generated file (origin component/crud); `renderEntry` re-renders it from the route list → upgrade sees it up-to-date, never PROTECTED.
- Pros: main.go stays template-pure (manifest hash stable → upgrade healthy); no AST; idempotent; routes are a manifest-readable, diffable artifact; `go-arch check` passes (extra dir allowed); plain handler can opt-in via `--route`.
- Cons: One-time template change to web main (needs upgrade propagation for existing projects); a new generated file/dir in user projects; legacy projects (no manifest) fall back to the manual hint; plain-handler registration needs a pattern from the user.
- Effort: Medium.

### Recommendation
**Approach 3** (registry + manifest route list). It is the only mechanism consistent with the upgrade manifest's PROTECTED philosophy: the router file the user may edit (`main.go`) is never touched by generate again after scaffold, and all route churn lives in a dedicated generated file whose state is the manifest — exactly the upgrade engine's model.

Sub-decisions:
- **CRUD → default on** (Register method already exists; 5 routes derivable; removes the manual step GenerateCRUD warns about).
- **Plain handler → opt-in `--route "METHOD /path"`** (no Register method, no inherent pattern). Without the flag, current behavior + hint unchanged.
- **page/component/service/repository → never auto-register** (templ views have no routes; the counter demo POST /counter is hardcoded demo content).
- **Non-web projects (no templ+HTMX) → no registration**; keep the manual hint. (Scope guard: plain layouts have no router.)
- **Manifest: routes.go re-rendered from manifest route list** — upgrade stays idempotent; consider a manifest schema extension (version bump or additive field) as the carrier.
- **Path resolution: fix or explicitly bound.** Because of the nested-dir bug, the router file location must be resolved as CWD-relative (upgrade's ADR-7 semantics), not `ProjectName`-joined. Recommendation: within this change, align `generate` path resolution to CWD (test-covered), since routing cannot be correct on top of a nested-dir bug.

## Edge Cases & Risks

- **Nested-dir path bug (CRITICAL, pre-existing):** `createFile` joins `ProjectName` as a dir; real projects get `realapp/realapp/…`. Route registration on top of this would edit/miss the wrong router. Must be fixed or explicitly bounded with a documented limitation before routing lands.
- **User-edited main.go:** if the user already hand-wired routes into main.go, registry approach still works (main.go untouched; registry Register is additive). But if main.go's hash already diverges, upgrade will PROTECT it — acceptable and correct.
- **Existing projects (pre-change):** web main template change (`router.Register(mux)`) propagates via `go-arch upgrade`; routes.go must be created by upgrade too (or generated on first `generate`), else the new main.go references a missing package → compile error. Design must handle "routes.go absent but main.go references it" (e.g. upgrade creates routes.go when web scaffold + new template applied; or main.tmpl guards with a helper that exists regardless).
- **Legacy projects (no manifest):** `upgradeLegacy` whitelist path — routes.go won't exist; generate must fall back to hint-only (no manifest → no route list). Document.
- **templ projects:** `templ generate` must run after route generation? No — routes.go is pure Go, no templ dependency. The existing `templHint` stays for page/component only.
- **Minimalist:** web Minimalist → main.go at root with mux (scaffold.go:172-174); registry works. Plain Minimalist has no router → hint only.
- **Hexagonal:** generated CRUD handler lives in `internal/adapters` — registry imports adapters (NOT handler). Registry template must be architecture-aware (same switch as GenerateComponent/CRUD).
- **Duplicate generation:** `generate crud User` twice → route list must be idempotent (dedupe by entity name) or routes double-register (runtime panic on conflicting patterns). Manifest list naturally dedupes on upsert — verify.
- **MCP:** adding optional `route` property is backward-compatible; do not make it required. `generate_component` chdir+viper.Reset pattern (server.go:325-342) must be preserved; route registration must behave identically under MCP (it will, since it lives in scaffold, not cmd).
- **Validator:** `internal/router` passes check (extra dirs allowed) — verify with a check test.

## Testing Strategy (strict TDD)

1. **Registry rendering unit tests** (scaffold package): temp web project (Standard + templ), `GenerateCRUD("User")` → assert `internal/router/routes.go` exists, contains `NewUserHandler().Register(mux)` and the 5 route patterns; main.go byte-identical to template render.
2. **Path resolution regression:** run generate inside a real project (project_name: realapp) → assert files land at CWD-relative paths (no nesting) — new tests that would catch the ADR-7 mismatch.
3. **Idempotency:** generate crud twice → routes.go has exactly one registration set.
4. **Plain handler opt-in:** `--route "GET /users"` → registry line `mux.HandleFunc("GET /users", …)` or `NewUserHandler()` wrapped; without flag → no registry change, hint preserved.
5. **Upgrade interaction:** after registration, `Upgrade()` → routes.go up-to-date (not PROTECTED); simulate template change via local override (.go-arch/templates) → main.go + routes.go upgradable; Apply keeps routes.
6. **MCP:** `generate_component` with type crud → routes.go updated; optional `route` param accepted.
7. **Functional (optional, templ present):** extend TestHandlerFunctional pattern — generate, `templ generate`, `go test ./...` in the generated project to prove it compiles with the registry.

## Ready for Proposal

**Yes.** Key decisions to hand the orchestrator/user:
1. Scope: CRUD default-on, plain-handler `--route` opt-in, web projects only (non-web keeps manual hint).
2. Mechanism: registry file `internal/router/routes.go` re-rendered from manifest-held route list; main.go touched only via template.
3. The pre-existing nested-dir path bug must be fixed (or explicitly deferred with a documented limitation) before/with routing — recommend fixing it in this change since routing correctness depends on it.
