# templ-htmx-frontend Specification

## Purpose

Scaffold a full-stack, server-rendered Go web project (templ + HTMX + static assets + web-aware main) when the `UseTemplHTMX` flag is ON. Behavior when OFF MUST remain identical to today.

## Requirements

### Requirement: TemplHTMX Flag Activation

The system SHALL produce a full-stack web scaffold when `UseTemplHTMX` is true, and SHALL leave all architecture scaffolds unchanged when false.

#### Scenario: Flag OFF preserves existing behavior

- GIVEN a user runs `go-arch new` with `UseTemplHTMX=false` for any architecture
- WHEN the scaffolder executes
- THEN the generated project matches current behavior exactly (no web files, no templ deps, no handler package)
- AND no `views/`, `static/`, or `internal/handler/` directories exist

#### Scenario: Flag ON produces web scaffold

- GIVEN a user runs `go-arch new` with `UseTemplHTMX=true` for Minimalist, Standard, or Hexagonal
- WHEN the scaffolder executes
- THEN the generated project contains all templ views, static assets, `internal/handler/page.go`, and a web-aware main

### Requirement: Templ Views File Set

The system SHALL generate the following templ view files at the documented paths when the flag is ON.

#### Scenario: Expected view files exist

- GIVEN `UseTemplHTMX=true` and scaffolding completes
- WHEN the project tree is inspected
- THEN `views/layouts/base.templ` exists
- AND `views/pages/home.templ` exists
- AND `views/components/counter.templ` exists
- AND each contains valid templ markup referencing HTMX attributes

### Requirement: Static Asset Vendoring

The system SHALL vendor htmx.min.js as a binary-identical copy of the embedded source, bypassing `engine.Render`.

#### Scenario: htmx.min.js binary copy

- GIVEN `UseTemplHTMX=true`
- WHEN scaffolding completes
- THEN `static/js/htmx.min.js` exists and is byte-identical to the embedded `templates/web/htmx.min.js`
- AND `static/css/style.css` exists with base styles

#### Scenario: htmx is not templ-rendered

- GIVEN the htmx source contains `{{`/`}}` tokens
- WHEN the copy is written
- THEN the file is produced via `TemplatesFS.ReadFile` + `os.WriteFile` (not `engine.Render`)

### Requirement: Web-Aware Main Generation

The system SHALL write a single architecture-agnostic web main (net/http mux, static FileServer, conditional telemetry + gRPC blocks) to the architecture-appropriate path.

#### Scenario: Standard / Hexagonal web main path

- GIVEN `UseTemplHTMX=true` with Standard or Hexagonal architecture
- WHEN scaffolding completes
- THEN `cmd/api/main.go` exists and contains `http.ListenAndServe`
- AND it does NOT import `internal/adapters` or `internal/domain`

#### Scenario: Minimalist web main path

- GIVEN `UseTemplHTMX=true` with Minimalist architecture
- WHEN scaffolding completes
- THEN root `main.go` is the web-aware main (replaces the default minimalist main)

#### Scenario: Architecture main not duplicated

- GIVEN `UseTemplHTMX=true`
- WHEN scaffolding completes
- THEN no second `main.go` conflicts with the web main (guarded arch-mains)

### Requirement: Functional Counter Handler

The system SHALL generate `internal/handler/page.go` serving GET `/` (page render) and POST `/counter` (fragment re-render with incremented in-memory state) so HTMX swap works without 404.

#### Scenario: Counter functional end-to-end

- GIVEN `UseTemplHTMX=true`, project built, `templ generate` run
- WHEN a POST is sent to `/counter`
- THEN the response is the counter fragment with the incremented value
- AND the state is guarded by `sync.Mutex`

#### Scenario: GET / renders the page

- GIVEN the generated project is running
- WHEN GET `/` is requested
- THEN the response contains the base layout + counter component

### Requirement: go.mod Templ Require

The system SHALL add a `require` block for `github.com/a-h/templ` in `go.mod.tmpl` when the flag is ON, and omit it when OFF.

#### Scenario: Templ dep present when flag ON

- GIVEN `UseTemplHTMX=true`
- WHEN `go.mod` is generated
- THEN a `require` block for `github.com/a-h/templ` is present

#### Scenario: No templ dep when flag OFF

- GIVEN `UseTemplHTMX=false`
- WHEN `go.mod` is generated
- THEN no `github.com/a-h/templ` require block exists

### Requirement: Generated README

The system SHALL render `README.md` at the project root when the flag is ON, documenting post-scaffold steps and htmx attribution.

#### Scenario: README present with instructions

- GIVEN `UseTemplHTMX=true`
- WHEN scaffolding completes
- THEN `README.md` exists at the project root
- AND it contains instructions for `go install` of templ, `templ generate`, and `go run`
- AND it includes BSD-2-Clause htmx attribution
