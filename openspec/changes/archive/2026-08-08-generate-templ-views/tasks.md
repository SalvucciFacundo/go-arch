# Tasks: Generate Templ Views

> **Status: ARCHIVED** — SDD cycle complete on 2026-08-08. All 14/14 tasks complete; verify verdict PASS (16/16 scenarios). Moved to `openspec/changes/archive/2026-08-08-generate-templ-views/`.

## Review Workload Forecast

Estimated changed lines: ~380-430 | Suggested split: single PR, 4 work-unit commits | Delivery strategy: auto-chain

Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: feature-branch-chain
400-line budget risk: Medium

### Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|------|------|-----------|----------------------|-----------------|-------------------|
| 1 | Templates + scaffold gen | PR 1 (base: tracker) | `go test ./internal/pkg/scaffold/` | tempdir web proj: `go run . generate page Dashboard` → file exists | Revert templates + switch/helper |
| 2 | CLI gate/help/hint | PR 2 (base: PR 1) | `go test ./cmd/` | `go run . generate --help` shows 6 types | Revert cmd/generate.go |
| 3 | MCP enum + dispatch test | PR 3 (base: PR 2) | `go test ./internal/pkg/mcp/` | JSON-RPC `generate_component` page → file created | Revert server.go |
| 4 | Full verification | Merge gate | `go test ./... && go vet ./...` | `go build ./...` | N/A (read-only) |

## Phase 1: Templates (Foundation)

- [x] 1.1 Create `internal/pkg/template/templates/web/page_generated.tmpl`: `package pages`, `import "{{ .ModuleName }}/views/layouts"`, `templ {{ title .EntityName }}()` wrapping `@layouts.Base(0)` 
- [x] 1.2 Create `internal/pkg/template/templates/web/component_generated.tmpl`: `package components`, `templ {{ title .EntityName }}()`, div with `hx-get/hx-target/hx-swap` + `{{ lower .EntityName }}` ids

Commit: `feat(scaffold): add templ page and component generation templates`

## Phase 2: Scaffold Core (RED→GREEN)

- [x] 2.1 RED `scaffold_test.go`: `TestIsValidGoIdentifier` table — true `Dashboard`/`UserCard`/`dashboard`; false `user-card`/`123Name`/empty/`if`
- [x] 2.2 RED `scaffold_test.go`: page gen — tempdir web proj, `page Dashboard` + `page dashboard` → `views/pages/dashboard.templ` has `package pages`, `templ Dashboard()`, `@layouts.Base(0)`
- [x] 2.3 RED `scaffold_test.go`: component gen — `component UserCard` → `views/components/usercard.templ` has `package components`, `templ UserCard()`, `hx-get=`
- [x] 2.4 RED `scaffold_test.go`: guards — flag off → `web_scaffold_required` (no file); `user-card` → `invalid_component_name`; pre-created file → `component_already_exists` (unchanged); web scaffold `page Home` → `component_already_exists` (home.templ byte-identical); backend `service` flag-off succeeds
- [x] 2.5 GREEN `scaffold.go`: `page`/`component` cases (line 243) — three guards: `!UseTemplHTMX`→`web_scaffold_required`, `isValidGoIdentifier`→`invalid_component_name`, `os.Stat`→`component_already_exists` (scoped); target `views/pages|components/{ToLower(name)}.templ`; add `isValidGoIdentifier` via `go/token.IsIdentifier`

Commit: `feat(scaffold): generate templ page and component files with guards`

## Phase 3: CLI Wiring (RED→GREEN)

- [x] 3.1 RED: create `cmd/generate_test.go` (NEW) — `templHint` unit (`page`/`component` contain `templ generate`); help via `SetOut(buf)` + `SetArgs(["generate","--help"])` asserting 6 types; success smoke (page run → buf contains `templ generate`)
- [x] 3.2 GREEN `cmd/generate.go`: add `UseTemplHTMX: viper.GetBool("use_templ_htmx")` (lines 37-43); reword `Use:`/`Short:`/`Long:` listing six types; add `templHint` helper; call after `ui.Success` for page/component

Commit: `feat(cli): wire use_templ_htmx gate and success hint into generate`

## Phase 4: MCP Wiring (RED→GREEN)

- [x] 4.1 RED: create `internal/pkg/mcp/server_test.go` (NEW) — dispatch exercise: invoke `generate_component` handler `type: page` + `type: component` against tempdir web proj; assert file created (not just enum)
- [x] 4.2 GREEN `server.go`: extend enum (line 168) + description (line 162); add `UseTemplHTMX` to config mapping (lines 292-298)

Commit: `feat(mcp): expose page and component types in generate_component`

## Phase 5: Verification

- [x] 5.1 `go test ./...` passes
- [x] 5.2 `go vet ./...` clean
- [x] 5.3 `gofmt -l .` empty
