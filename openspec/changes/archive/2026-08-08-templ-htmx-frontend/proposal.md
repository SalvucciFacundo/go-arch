# Proposal: templ + HTMX Frontend Flag (`UseTemplHTMX`)

**Status**: draft · **Author**: sdd-propose · **Input**: `openspec/changes/templ-htmx-frontend/exploration.md`

## Executive Summary

Add a `UseTemplHTMX` flag to `go-arch new` that scaffolds a full-stack, server-rendered Go project: backend (existing) + templ views + HTMX + static CSS + web-aware main. Simultaneously fix the pre-existing Hexagonal build break so freshly generated Hexagonal (without web) projects compile.

## Problem Statement

1. **No frontend path**: `go-arch new` produces API-only projects. Users wanting a server-rendered Go web app must hand-wire templ, HTMX, mux, static serving, and `go.mod` edits on top of the scaffold.
2. **Hexagonal build bug (pre-existing, blocking)**: `templates/hexagonal/main.tmpl:7-8` imports `internal/adapters` and `internal/domain`, but a fresh scaffold creates those as empty directories → `go build ./...` fails with "no required module provides package".
3. **Main overwrite collision**: Standard/Hexagonal write `cmd/api/main.go` *before* `createCommonFiles()` runs; a naïve web main added inside `createCommonFiles()` would silently clobber telemetry/gRPC init.

## Intent / Goal

- Ship a single flag that, when ON, turns the scaffold into a full-stack server-rendered template (same binary, no SPA, no Node).
- Restore clean `go build ./...` for fresh Hexagonal projects (with AND without the web flag).
- Leave Minimalist / Standard / Hexagonal behavior identical when the flag is OFF.

## Non-Goals

- SPA / JS framework / bundler integration (Vite, webpack, esbuild).
- templ hot-reload (`templ lsp`, Air wiring for `.templ`).
- Auth, sessions, CSRF, database.
- Generated-project CI (GitHub Actions, golangci-lint).
- Multiple pages beyond the counter demo.

## Scope

### In Scope

- `UseTemplHTMX bool` in `ProjectConfig` (`internal/ui/prompts.go`) + wizard `survey.Confirm`.
- `use_templ_htmx:` line in `common/config.tmpl` round-trip.
- `{{ if .UseTemplHTMX }}` require block in `common/go.mod.tmpl` pinning `github.com/a-h/templ v0.3.906` (re-verify at design).
- New `templates/web/` embedded assets:
  - `base.templ.tmpl`, `page.templ.tmpl`, `counter.templ.tmpl`, `style.css.tmpl`, `handler.tmpl` (`internal/handler/page.go`), `main.tmpl` (architecture-agnostic superset).
  - `htmx.min.js` vendored as **binary** (BSD-2-Clause htmx v1.9.12), copied via `TemplatesFS.ReadFile` + `os.WriteFile` — deliberately bypasses `engine.Render` and the local/global override chain.
- Guard architecture mains with `if !s.config.UseTemplHTMX { createFile(...) }` (Minimalist root `main.go`; Standard/Hexagonal `cmd/api/main.go`).
- `scaffoldWeb()` in `createCommonFiles()` writes web main to `cmd/api/main.go` (Standard/Hexagonal) or root `main.go` (Minimalist) — single superset template with `net/http` mux, static FileServer, conditional telemetry + gRPC blocks. **Does NOT import `internal/adapters` or `internal/domain`** (avoids hex bug in web path too).
- **Hexagonal build fix**: rewrite `templates/hexagonal/main.tmpl` to drop the empty-package imports (or gate them behind a future feature flag); acceptance = `go-arch new` → Hexagonal → `go build ./...` passes with no web flag.
- **Functional counter demo**: `internal/handler/page.go` serves GET `/` (renders page) and POST `/counter` (re-renders fragment with incremented in-memory state, `sync.Mutex`); hx-post/hx-target/hx-swap actually swap — no 404.
- **Generated README**: `templates/web/readme.tmpl` rendered as `README.md` in the project root documenting `go install templ`, `templ generate`, `go run ./cmd/api`, plus htmx attribution.
- Table-driven tests covering: flag OFF leaves existing behavior; flag ON produces all expected files; Hexagonal+OFF builds; Standard+Hexagonal+ON produces web main containing `http.ListenAndServe` + `internal/handler`.

### Out of Scope

- Node tooling, Tailwind, DaisyUI, React/Vue.
- `templ generate --watch`, Air integration for `.templ`.
- Database, ORM, auth, migrations.
- Generated-project CI.

## Capabilities

### New Capabilities
- `templ-htmx-frontend`: templ views + HTMX + web main + static assets scaffold, gated on `UseTemplHTMX`.
- `hexagonal-build-fix`: fresh Hexagonal projects compile without web flag.

### Modified Capabilities
- `cli`: `ProjectConfig` gains `use_templ_htmx`; wizard adds a Confirm; `go.mod.tmpl` and `config.tmpl` gain conditional blocks.

## Approach

**Option A from exploration (guarded arch mains + single architecture-agnostic web-main superset).** Chosen over Option B (duplicated HTTP blocks per arch template) because: single source of truth for the web main, no duplication across three arch templates, cleanly dodges the Hexagonal empty-package imports, and is easier to test.

## Affected Areas

| Area | Impact | Description |
|---|---|---|
| `internal/ui/prompts.go` | Modified | Add `UseTemplHTMX` field + Confirm question. |
| `internal/pkg/scaffold/scaffold.go` | Modified | Guard three arch mains; add `scaffoldWeb()` call in `createCommonFiles()`. |
| `internal/pkg/scaffold/scaffold_test.go` | Modified | New table cases for flag ON/OFF, hex build, web main content. |
| `internal/pkg/template/templates/common/go.mod.tmpl` | Modified | Conditional templ require block. |
| `internal/pkg/template/templates/common/config.tmpl` | Modified | `use_templ_htmx:` line. |
| `internal/pkg/template/templates/hexagonal/main.tmpl` | Modified | Drop empty-package imports (hex build fix). |
| `internal/pkg/template/templates/web/*` | New | 7 templates + 1 binary asset (htmx.min.js). |

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| templ version drift / breakage | Low | Pin `v0.3.906` (re-verify at design); `go mod tidy` in generated project is safety net. |
| htmx BSD-2-Clause attribution missed | Low | Generated `README.md` includes attribution + LICENSE pointer. |
| Users forget `templ generate` post-step | Med | `cmd/new.go` success message (conditional) + generated `README.md` front-and-center. |
| Web main silently overwrites telemetry/gRPC | Low | Guard `if !UseTemplHTMX` on arch mains; test asserts `http.ListenAndServe` present when flag ON. |
| Hexagonal fix regresses future features | Low | Fix is additive (drop unused imports); new imports re-added by feature PRs with tests. |

## Rollback Plan

The change is additive and gated. Revert = drop the `web/` templates, revert the three guarded arch-mains lines, drop `UseTemplHTMX` from `ProjectConfig`/wizard/go.mod.tmpl/config.tmpl, and keep the Hexagonal fix as a standalone commit (it's a correct, independent bug fix). No data migration, no breaking changes to existing projects.

## Dependencies

- `github.com/a-h/templ` v0.3.906 (re-verify latest stable at design phase).
- htmx v1.9.12 (BSD-2-Clause) as a vendored binary asset in go-arch repo.
- End user must run `templ generate` post-scaffold (documented, not automated).

## Success Criteria

- [ ] `go-arch new` → Minimalist + web ON → `templ generate && go build ./... && go run .` serves page with working counter.
- [ ] `go-arch new` → Standard + web ON → same, via `./cmd/api`.
- [ ] `go-arch new` → Hexagonal + web ON → same.
- [ ] `go-arch new` → Hexagonal + web OFF → `go build ./...` passes (hex fix).
- [ ] Minimalist/Standard/Hexagonal + web OFF → behavior identical to today (regression-free).
- [ ] Generated project contains `README.md` with `templ generate` instructions and htmx attribution.
- [ ] `go test ./...` on go-arch repo passes; new cases cover flag ON/OFF, hex build, web-main content.

## Next Recommended Step

**spec** — write delta specs for `templ-htmx-frontend` (new capability) and `hexagonal-build-fix` (new capability), plus a `cli` delta for the new wizard field / config round-trip.
