# Proposal: Generate Routes

## Status
**Proposed.** Feasibility verified in `openspec/changes/generate-routes/exploration.md`.

## Executive Summary
Auto-register routes for templ+HTMX projects via a generated registry (`internal/router/routes.go`) driven by a manifest route list; fix the nested-directory bug in `createFile`.

## Intent
CRUD registers its 5 routes by default (`Register(mux)` exists in `crud_handler.tmpl`); plain handler opts in via `--route "METHOD /path"`; registry is one generated file re-rendered from the manifest with no AST on `main.go`; non-web projects keep the hint.

## Scope

### In
- `common/routes.tmpl` → `internal/router/routes.go` with `Register(mux)` (architecture-aware); manifest gains additive `routes:` keyed by entity (upsert = dedupe); `web/main.tmpl` adds one `router.Register(mux)` line (propagated via upgrade, which also creates missing routes.go in existing web projects).
- CLI: `--route` flag on `generate handler`, CRUD default-on, hint-only for non-web; MCP mirrors with optional `route` property.
- Nested-dir fix: `createFile` uses CWD inside real projects; `new` unchanged.

### Out
- AST or wholesale edits to `main.go`.
- Routes for `page`/`component`/`service`/`repository` or non-web layouts.
- Auto-pattern detection for plain handlers; third-party routers.

## Capabilities

### New
- `generate-routes`: registry, manifest route list, main-template wiring, upgrade interaction.

### Modified
- `cli`: `generate handler --route`; CRUD default-on in web projects; `createFile` path fix.

## Approach
Registry + manifest route list + one `router.Register(mux)` call (approach c). Main stays byte-identical to template, preserving PROTECTED semantics.

## Affected Areas
`cmd/generate.go` (`--route` flag) · `internal/pkg/scaffold/scaffold.go` (route-list + registry re-render + path fix) · `internal/pkg/scaffold/manifest.go` (additive `routes:` field) · `internal/pkg/template/templates/common/routes.tmpl` (new) · `internal/pkg/template/templates/web/main.tmpl` (`router.Register(mux)` call) · `internal/pkg/scaffold/upgrade.go` (idempotent re-render + create-if-absent) · `internal/pkg/mcp/server.go` (optional `route`).

## Risks

| Risk | L | Mitigation |
|------|---|------------|
| Compile break on upgrade (no routes.go) | High | Upgrade creates empty-list routes.go |
| Duplicate registrations / wrong imports | Med | Manifest upsert dedupes by entity; registry mirrors GenerateCRUD architecture switch |
| Path fix breaks `new` | Low | Separate code path |

## Rollback Plan
Three independent reverts: registry/manifest/template, CLI/MCP flag, path fix. Existing projects revert via `upgrade --yes` to prior main.tmpl.

## Dependencies
Existing templates + upgrade/manifest infra. No new libraries.

## Success Criteria
- [ ] `generate crud User` (web) creates registry with `NewUserHandler().Register(mux)` + 5 routes; idempotent (`crud X` twice → one registration).
- [ ] `generate handler X --route` adds a line; omit → no change.
- [ ] `web/main.tmpl` byte-identical to template after generate; `upgrade` on existing web project adds Register call + creates routes.go.
- [ ] Path fix: in-project writes at CWD; `new` unchanged.
- [ ] Non-web: hint only.
- [ ] MCP: crud updates registry, optional `route` accepted.
- [ ] `go vet` + `go test` + `go-arch check` pass.

## Next Recommended
`sdd-spec` (new `generate-routes` capability + delta for `cli`).
