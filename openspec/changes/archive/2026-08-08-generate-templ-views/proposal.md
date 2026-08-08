# Proposal: Generate Templ Views

**Status**: draft | **next_recommended**: spec

## Executive Summary

Add `page` and `component` types to `go-arch generate` for templ+HTMX projects, producing parametrized `.templ` files under `views/pages/` and `views/components/`. Requires a web-scaffold gate (`use_templ_htmx: true`), CamelCase name validation, and existence-before-write collision checks.

## Problem Statement

`go-arch generate` today creates backend components only (service, repository, handler, crud). Projects scaffolded with `use_templ_htmx: true` get a one-shot `views/` tree (home page + counter component) but no CLI path to generate additional pages or components — developers must hand-author `.templ` files. Additionally, `generate` does not read `use_templ_htmx` from `.go-arch.yaml` (cmd/generate.go:37-43, mcp/server.go:292-298), so it cannot distinguish web projects from non-web ones.

## Intent

Provide Angular-CLI-style `ng generate component` parity: `go-arch generate page <Name>` → `views/pages/{name}.templ`; `go-arch generate component <Name>` → `views/components/{name}.templ`. Gate both on the web-scaffold flag, enforce CamelCase names (valid Go identifiers), and fail on file collisions instead of silently overwriting.

## Scope

### In Scope
- Two new embedded templates: `web/page_generated.tmpl`, `web/component_generated.tmpl`
- `page` and `component` switch cases in `GenerateComponent` (scaffold.go:243)
- Web-scaffold gate: `viper.GetBool("use_templ_htmx")` read in both cmd/generate.go:37-43 and mcp/server.go:292-298; oops code `web_scaffold_required` on failure
- CamelCase validation: reject names that are not valid Go identifiers with oops code `invalid_component_name`
- Collision check: `os.Stat` before `createFile` for `page`/`component` types only; oops code `component_already_exists`
- MCP `generate_component` enum extension (server.go:168) + description update (server.go:162)
- Help-text reword: disambiguate `component` as a concrete type vs. umbrella term
- Tests: generation (file exists + content), gating, collision, name validation — no `templ` binary required

### Out of Scope
- Handler/route auto-registration (Approach 2 — High effort, rejected)
- `view` alias for `page` (defer unless trivial during implementation)
- Kebab-case/hyphenated name support or `pascal` template func
- Generating into non-web projects
- Documentation beyond CLI help text
- Changes to existing backend generation types (service/repository/handler/crud)

## Capabilities

### New Capabilities
- `templ-view-generation`: parametrized page/component generation with web-scaffold gate, CamelCase validation, and collision detection

### Modified Capabilities
- `cli`: `generate` command gains `page`/`component` types; help text reworded; new oops codes (`web_scaffold_required`, `invalid_component_name`, `component_already_exists`)

## Approach

Minimal reuse of existing `GenerateComponent` dispatch (Approach 1 from exploration). Add two switch cases mapping to architecture-agnostic `views/pages/` and `views/components/` paths. Existing `{ui.ProjectConfig; EntityName}` data struct is sufficient. New templates use existing `title`/`lower` funcMap. Gate checked before dispatch; collision check scoped to new types only (preserves existing backend behavior).

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/pkg/scaffold/scaffold.go` | Modified | Add `page`/`component` cases + existence check before `createFile` |
| `cmd/generate.go` | Modified | Add `UseTemplHTMX` viper read, name validation, gate, help reword |
| `internal/pkg/mcp/server.go` | Modified | Extend enum + description, add `UseTemplHTMX` to config mapping |
| `internal/pkg/template/templates/web/page_generated.tmpl` | New | Parametrized page template |
| `internal/pkg/template/templates/web/component_generated.tmpl` | New | Parametrized component template |
| `internal/pkg/scaffold/scaffold_test.go` | Modified | Generation, gating, collision, name-validation tests |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Collision check scope creep (applying to existing types) | Low | Explicitly limit `os.Stat` guard to `page`/`component` cases only |
| Help-text ambiguity between `component` type and umbrella term | Med | Reword `Use:` and `Short:` to clarify; test wording in help output |
| MCP enum drift from CLI types | Low | Single source of truth: both must list identical types; test enum contents |

## Rollback Plan

Revert the commit. All changes are additive (new templates, new switch cases, new config read). No existing behavior changes except the collision check (scoped to new types) and the help text. A revert restores prior `generate` behavior exactly.

## Dependencies

- None external. Existing `title`/`lower` template funcs (engine.go:68-82) are sufficient.

## Success Criteria

- [ ] `go-arch generate page Dashboard` creates `views/pages/dashboard.templ` with `templ Dashboard()` wrapping `@layouts.Base(0)`
- [ ] `go-arch generate component UserCard` creates `views/components/usercard.templ` with `templ UserCard()`
- [ ] Both commands fail with `web_scaffold_required` when `use_templ_htmx: false`
- [ ] Both commands fail with `invalid_component_name` for non-CamelCase input (e.g. `user-card`)
- [ ] Both commands fail with `component_already_exists` when target file exists
- [ ] MCP `generate_component` accepts `page`/`component` in its type enum
- [ ] All existing `go test ./...` tests continue to pass
- [ ] No `templ` binary required for any new test
