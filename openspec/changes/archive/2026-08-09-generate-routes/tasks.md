# Tasks: Generate Routes (ARCHIVED)

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | 1000–1500 (authored, tests included) |
| Session review budget | 800 |
| Suggested split | 4 chained PRs |

Decision needed before apply: No
Chained PRs recommended: Yes
Chain strategy: feature-branch-chain
400-line budget risk: High

### Work Units

| Unit | Goal / commit | PR (base) | Test cmd | Runtime | Rollback |
|------|---------------|-----------|----------|---------|----------|
| 1 | Manifest route list + routes.tmpl · `feat(scaffold): manifest route list and routes template` | PR1 (tracker) | `go test ./internal/pkg/scaffold/ -run UpsertRoute` | N/A: unwired | Revert manifest.go, routes.tmpl |
| 2 | CWD path fix, registry render, options, CRUD · `fix(scaffold): render route registry from manifest` | PR2 (PR1) | `go test ./internal/pkg/scaffold/` | temp web crud | Revert scaffold.go |
| 3 | Upgrade, main.tmpl, CLI, MCP · `feat(cli): generate --route and upgrade registry` | PR3 (PR2) | `go test ./cmd/ ./internal/pkg/mcp/ ./internal/pkg/scaffold/` | upgrade web proj | Revert upgrade.go, main.tmpl, generate.go, server.go |
| 4 | Verify · `test(scaffold): route registry end-to-end` | PR4 (PR3) | `go test ./... && go vet ./... && golangci-lint run` | new web; crud; upgrade | None (no code) |

## Phase 1: Foundation

- [x] 1.1 Create `internal/pkg/template/templates/common/routes.tmpl`: package router; `{{ if .Routes }}` import (net/http + adapters|handler by Arch); `Register(mux)` ranges: crud → `NewXHandler().Register(mux)`, handler → `mux.HandleFunc(pattern, ...ServeHTTP)`
- [x] 1.2 `internal/pkg/scaffold/manifest.go`: `RouteEntry{Entity,Handler,Origin,RoutePattern}` + `Routes []RouteEntry` (`omitempty`) + `UpsertRoute` (entity dedupe → Save)
- [x] 1.3 RED→GREEN `manifest_test.go`: UpsertRoute dedupe, YAML round-trip, omitempty

## Phase 2: Core Scaffold

- [x] 2.1 RED→GREEN `scaffold.go` `manifestDir()`: "." if manifest in CWD else ProjectName; apply in ensureManifest, recordManifest, createBinaryFile, createFile, GenerateComponent checks
- [x] 2.2 `RoutesData{ModuleName,Architecture,Routes}` + `renderRoutesRegistry()` (routes.go from manifest.Routes)
- [x] 2.3 `GenerateComponent(compType, name, opts ...GenerateOption)`: generateConfig + `WithRoute`; handler: `web_scaffold_required` + `invalid_route_pattern`; upsert + re-render when pattern set
- [x] 2.4 `isValidRoutePattern`: METHOD in GET/POST/PUT/DELETE/PATCH/HEAD/OPTIONS, path starts "/"
- [x] 2.5 `GenerateCRUD`: web → UpsertRoute{origin:"crud"} + render; non-web → hint only
- [x] 2.6 `scaffoldWeb()`: `internal/router` dir + empty-list routes.go
- [x] 2.7 RED→GREEN `scaffold_test.go`: Standard+Hexagonal, idempotent, --route, omit byte-identical, non-web hint, main byte-identity, path fix (realapp), new unchanged, manifestDir, validator, empty compiles

## Phase 3: Wiring

- [x] 3.1 `web/main.tmpl`: import `internal/router` + `router.Register(mux)`
- [x] 3.2 `upgrade.go`: `buildRenderData(cfg, entry, m)` routes.go → RoutesData; `renderEntry(engine, entry, cfg, m)`; loop re-renders routes.go (never PROTECTED); create absent routes.go
- [x] 3.3 `cmd/generate.go`: `--route` flag → `WithRoute`; help documents --route + CRUD default-on
- [x] 3.4 `internal/pkg/mcp/server.go`: optional `route` in generate_component schema; pass WithRoute when set
- [x] 3.5 RED→GREEN: `upgrade_test.go` (creates routes.go, not PROTECTED); `cmd/generate_test.go` (flag, help, bad pattern); `server_test.go` (route, crud registry)

## Phase 4: Verification

- [ ] 4.1 `go test ./...` green; `go vet ./...` clean; `gofmt -l .` empty
- [ ] 4.2 `golangci-lint` v1.64.8 exit 0
- [ ] 4.3 Live: fresh web `new` → `go build` OK; `generate crud User` registers; `upgrade --yes` idempotent
