# generate-routes Specification

## Purpose

Auto-register routes for templ+HTMX web projects via a generated registry file (`internal/router/routes.go`) driven by a manifest-held route list. CRUD registers its 5 routes by default; plain handlers opt in via `--route`. Non-web projects keep the manual hint. Main.go stays byte-identical to its template.

## Requirements

### Requirement: Routes Registry File

The system MUST generate `internal/router/routes.go` exposing `Register(mux *http.ServeMux)`. The registry MUST be architecture-aware: Hexagonal imports `internal/adapters`; Standard/Minimalist import `internal/handler`. The registry content MUST be a deterministic re-render of the manifest-held route list.

#### Scenario: Registry rendered after CRUD in Standard web project

- GIVEN a web project (templ+HTMX, Standard architecture) with a valid manifest
- WHEN `generate crud User` completes
- THEN `internal/router/routes.go` exists
- AND contains `func Register(mux *http.ServeMux)`
- AND calls `handler.NewUserHandler().Register(mux)`

#### Scenario: Registry architecture-aware for Hexagonal

- GIVEN a web project with Hexagonal architecture
- WHEN `generate crud User` completes
- THEN the registry imports `{{ .ModuleName }}/internal/adapters`
- AND calls `adapters.NewUserHandler().Register(mux)`

### Requirement: Manifest Route List

The manifest MUST hold an additive `routes:` list keyed by entity. Upserting the same entity MUST dedupe (idempotent). Re-rendering the registry from the list MUST produce byte-identical output on repeated renders with the same list.

#### Scenario: CRUD idempotent on duplicate entity

- GIVEN `generate crud User` has already run
- WHEN `generate crud User` runs again
- THEN the route list contains exactly one entry for `User`
- AND `routes.go` has exactly one `NewUserHandler().Register(mux)` call

#### Scenario: Registry deterministic under upgrade

- GIVEN a manifest with a route list of `[User, Order]`
- WHEN the registry is re-rendered twice
- THEN both renders produce byte-identical output

### Requirement: CRUD Default-On Registration (Web)

In a templ+HTMX web project, `generate crud X` MUST append `NewXHandler().Register(mux)` to the route list and re-render the registry. The 5 CRUD routes MUST be derivable from the registration line.

#### Scenario: CRUD registers 5 routes

- GIVEN a web project with no prior registrations
- WHEN `generate crud User` completes
- THEN the registry registers `NewUserHandler().Register(mux)`
- AND the 5 CRUD route patterns are reachable via the handler's `Register` method

### Requirement: Handler Opt-In via --route

`generate handler X --route "METHOD /path"` MUST append a route entry to the route list and re-render the registry. Without `--route`, the route list and registry MUST remain unchanged, and the existing manual hint MUST be preserved.

#### Scenario: Handler with --route registers

- GIVEN a web project
- WHEN `generate handler Stats --route "GET /stats"` runs
- THEN the route list gains an entry for `Stats` with pattern `"GET /stats"`
- AND the registry is re-rendered

#### Scenario: Handler without --route leaves registry untouched

- GIVEN a web project with an existing registry
- WHEN `generate handler Stats` runs (no `--route`)
- THEN `routes.go` is byte-identical to its pre-generate state
- AND the manual registration hint is printed

### Requirement: Web-Only Scope

Route registration MUST only apply when the project has `use_templ_htmx: true`. For non-web projects, `generate crud` MUST print the manual hint and MUST NOT create or update `routes.go`.

#### Scenario: Non-web project gets hint only

- GIVEN a project with `use_templ_htmx: false`
- WHEN `generate crud User` runs
- THEN no `internal/router/routes.go` is created or modified
- AND the manual hint is printed

### Requirement: main.go Byte-Identity

`web/main.tmpl` MUST include a single `router.Register(mux)` call after the demo routes. After any `generate` invocation, `main.go` on disk MUST remain byte-identical to the template render (PROTECTED semantics preserved).

#### Scenario: main.go unchanged after generate

- GIVEN a freshly scaffolded web project
- WHEN `generate crud User` completes
- THEN `main.go` (or `cmd/api/main.go`) has the same sha256 as a fresh template render

### Requirement: Upgrade Interaction

For existing web projects, `upgrade` MUST propagate the `router.Register(mux)` line in the main template AND create `internal/router/routes.go` when absent. The registry re-rendered from the manifest route list MUST NOT be classified PROTECTED on subsequent upgrades.

#### Scenario: Upgrade creates routes.go in existing web project

- GIVEN an existing web project without `internal/router/routes.go`
- WHEN `upgrade --yes` runs
- THEN `internal/router/routes.go` is created with `func Register(mux *http.ServeMux)`
- AND the main template is updated with `router.Register(mux)`

#### Scenario: Upgrade does not mark routes.go PROTECTED

- GIVEN a web project with a registered route and a prior successful upgrade
- WHEN `upgrade` classifies
- THEN `routes.go` is classified `up-to-date` (not PROTECTED)

### Requirement: Nested-Dir Path Fix

`generate` inside a real project (`.go-arch.yaml` with `project_name: realapp`) MUST resolve target paths against CWD. The `new` command path resolution MUST remain unchanged.

#### Scenario: Generate resolves paths at CWD

- GIVEN a directory `realapp/` with `.go-arch.yaml` containing `project_name: realapp`
- WHEN the user runs `generate handler User` from inside `realapp/`
- THEN the handler is written to `./internal/handler/User_handler.go` (no `realapp/realapp/` nesting)

#### Scenario: new command path resolution unchanged

- GIVEN `go-arch new` with `project_name: myapp`
- WHEN the wizard completes
- THEN files are written under `myapp/` as before

### Requirement: MCP Mirror

The `generate_component` MCP tool MUST accept an optional `route` string property for `handler` type. CRUD via MCP MUST update the route registry identically to the CLI path.

#### Scenario: MCP handler with route

- GIVEN the MCP server running
- WHEN `generate_component(type=handler, name=Stats, route="GET /stats")` is called in a web project
- THEN the registry is updated with the new route entry

#### Scenario: MCP crud updates registry

- GIVEN the MCP server running in a web project
- WHEN `generate_component(type=crud, name=User)` is called
- THEN `internal/router/routes.go` is re-rendered with `NewUserHandler().Register(mux)`

### Requirement: Validator Compatibility

`go-arch check` MUST pass in projects containing `internal/router/`. The directory MUST NOT trigger a validator failure.

#### Scenario: Check passes with router dir

- GIVEN a project with `internal/router/routes.go`
- WHEN `go-arch check` runs
- THEN the check exits 0 with no router-related failure
