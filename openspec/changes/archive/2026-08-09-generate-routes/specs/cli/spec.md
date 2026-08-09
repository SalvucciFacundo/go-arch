# Delta for cli

## ADDED Requirements

### Requirement: Generate Handler --route Flag

The `generate handler` command MUST accept an optional `--route "METHOD /path"` flag. When provided in a web project (templ+HTMX), the flag MUST append a route entry to the manifest route list and trigger a registry re-render. When omitted, the route list and registry MUST remain unchanged, and the existing manual hint MUST be preserved. In non-web projects the flag MUST be rejected with a usage error.

#### Scenario: Handler with --route in web project

- GIVEN a web project (templ+HTMX) with a valid manifest
- WHEN `go-arch generate handler Stats --route "GET /stats"` runs
- THEN the manifest route list gains an entry for `Stats` with pattern `"GET /stats"`
- AND `internal/router/routes.go` is re-rendered
- AND the command exits 0

#### Scenario: Handler without --route unchanged

- GIVEN a web project with a registry containing one entry
- WHEN `go-arch generate handler Stats` runs (no `--route`)
- THEN the registry is byte-identical to its pre-generate state
- AND the manual registration hint is printed

#### Scenario: --route rejected in non-web project

- GIVEN a project with `use_templ_htmx: false`
- WHEN `go-arch generate handler Stats --route "GET /stats"` runs
- THEN the command fails with a usage error
- AND no registry or manifest changes occur

### Requirement: Generate CRUD Registers Routes in Web Projects

In a web project, `generate crud X` MUST append `NewXHandler().Register(mux)` to the manifest route list and re-render the registry. The operation MUST be idempotent: running `generate crud X` twice MUST produce the same registry as running it once (dedupe by entity name). In non-web projects, behavior MUST remain unchanged (manual hint, no registry writes).

#### Scenario: CRUD registers routes in web project

- GIVEN a web project (templ+HTMX) with no prior registrations
- WHEN `go-arch generate crud User` runs
- THEN `internal/router/routes.go` contains `NewUserHandler().Register(mux)`
- AND the command exits 0

#### Scenario: CRUD idempotent in web project

- GIVEN `go-arch generate crud User` has already run in a web project
- WHEN `go-arch generate crud User` runs again
- THEN `routes.go` contains exactly one `NewUserHandler().Register(mux)` call

#### Scenario: CRUD in non-web project unchanged

- GIVEN a project with `use_templ_htmx: false`
- WHEN `go-arch generate crud User` runs
- THEN no `internal/router/` file is created or modified
- AND the manual hint is printed

### Requirement: Generate Help Updated

The `generate` command help MUST document `--route` for `handler` type and note that `crud` registers routes by default in web projects.

#### Scenario: Help documents route behavior

- WHEN `go-arch generate --help` runs
- THEN the output mentions `--route` for handler type
- AND the output notes CRUD auto-registers in web projects

## MODIFIED Requirements

### Requirement: Generate Oops Codes for Web Generation

The `generate` command MUST emit four new oops codes for web-scaffold generation failures:
- `web_scaffold_required` when `use_templ_htmx` is false/missing for `page`/`component` types, or when `--route` is supplied in a non-web project.
- `invalid_component_name` when the name argument is not a valid Go identifier.
- `component_already_exists` when the target file already exists (scoped to `page`/`component` only).
- `invalid_route_pattern` when `--route` is supplied with a malformed pattern (missing METHOD or path).

(Previously: `generate` emitted `web_scaffold_required`, `invalid_component_name`, and `component_already_exists`.)

#### Scenario: web_scaffold_required emitted when flag off

- GIVEN `use_templ_htmx: false`
- WHEN `go-arch generate page Dashboard` runs
- THEN the error returned carries oops code `web_scaffold_required`

#### Scenario: web_scaffold_required emitted for --route in non-web

- GIVEN `use_templ_htmx: false`
- WHEN `go-arch generate handler Stats --route "GET /stats"` runs
- THEN the error returned carries oops code `web_scaffold_required`

#### Scenario: invalid_component_name emitted for bad name

- GIVEN a web-scaffold project
- WHEN `go-arch generate component user-card` runs
- THEN the error returned carries oops code `invalid_component_name`

#### Scenario: component_already_exists emitted on collision

- GIVEN the target file already exists
- WHEN `go-arch generate page <Name>` runs
- THEN the error returned carries oops code `component_already_exists`

#### Scenario: invalid_route_pattern emitted for bad pattern

- GIVEN a web project
- WHEN `go-arch generate handler X --route "BADPATTERN"` runs
- THEN the error returned carries oops code `invalid_route_pattern`
