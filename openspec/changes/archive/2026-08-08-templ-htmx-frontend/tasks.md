# Tasks: templ + HTMX Frontend Flag (`UseTemplHTMX`)

**Status**: CLOSED — archived `2026-08-08`. All 21/21 tasks complete; verify verdict PASS (22/22 spec scenarios). Change moved to `openspec/changes/archive/2026-08-08-templ-htmx-frontend/`.

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~560 authored (vendored htmx.min.js excluded) |
| 400-line budget risk | High (session budget 800 → Medium) |
| Chained PRs recommended | Yes |
| Suggested split | 3 chained PRs (feature branch chain) |
| Delivery strategy | auto-chain |
| Chain strategy | feature-branch-chain |

Decision needed before apply: No
Chained PRs recommended: Yes
Chain strategy: feature-branch-chain
400-line budget risk: High

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|------|------|-----------|----------------------|-----------------|-------------------|
| 1 | Flag plumbing + hex fix | PR #1 (base: tracker `feat/templ-htmx-frontend`) | `go test ./internal/ui/ ./internal/pkg/scaffold/` | Piped survey answers → `go run . new demo`; `cd demo && go build ./...` | Revert prompts.go, go.mod.tmpl, config.tmpl, hexagonal/main.tmpl, cmd/new.go |
| 2 | Web templates + scaffoldWeb | PR #2 (base: PR #1 branch) | `go test ./internal/pkg/scaffold/` | Scaffold Minimalist+ON; `find demo -type f`; `cmp` htmx bytes | Delete `templates/web/*`; revert scaffold.go guards/helpers |
| 3 | Functional + build verification | PR #3 (base: PR #2 branch) | `go test ./internal/pkg/scaffold/ -run TestHandler -v` | Hex+ON → `templ generate && go build ./... && go run ./cmd/api`; `curl -X POST /counter` | Revert test-only additions |

## Phase 1: Foundation — Flag Plumbing & Hex Fix

- [x] 1.1 `internal/ui/prompts.go`: add `UseTemplHTMX bool` (`mapstructure:"use_templ_htmx"`) after `UseGRPC`; append `survey.Confirm` "Include templ + HTMX frontend?" (default false) after gRPC question.
- [x] 1.2 `internal/pkg/template/templates/common/go.mod.tmpl`: add `{{ if .UseTemplHTMX }}` require block pinning `github.com/a-h/templ v0.3.906`.
- [x] 1.3 `internal/pkg/template/templates/common/config.tmpl`: append `use_templ_htmx: {{ .UseTemplHTMX }}` after `use_grpc`.
- [x] 1.4 `internal/pkg/template/templates/hexagonal/main.tmpl`: drop `internal/adapters` + `internal/domain` imports and "Wire up ports" comment (hex build fix).
- [x] 1.5 `cmd/new.go`: after `ui.Success`, add `if config.UseTemplHTMX` → `ui.Info` hint to run `templ generate` then `go run .` / `go run ./cmd/api`.

## Phase 2: Web Templates & Assets

- [x] 2.1 Create `templates/web/main.tmpl` superset: context/fmt/log/net/http + `internal/handler`; conditional telemetry + gRPC blocks; ServeMux `GET /`, `POST /counter`, `GET /static/` FileServer; `ListenAndServe(":8080")`; NO `internal/adapters`/`internal/domain` imports.
- [x] 2.2 Create `templates/web/handler.tmpl` → `internal/handler/page.go`: package `sync.Mutex` counter; `PageHandler` GET `/` renders `pages.Home(c)`; `CounterHandler` POST increments + renders `components.Counter(c)`.
- [x] 2.3 Create `templates/web/base.templ.tmpl` (shell: style.css link, htmx.min.js script, @Counter), `page.templ.tmpl` (`templ Home(count)` → Base), `component.templ.tmpl` (`templ Counter(count)` with hx-post/hx-target/hx-swap).
- [x] 2.4 Create `templates/web/style.css.tmpl` (dark base styles).
- [x] 2.5 Create `templates/web/readme.tmpl` → README.md: prerequisites (`go install github.com/a-h/templ/cmd/templ@latest`), usage (`templ generate`, `go run`), BSD-2-Clause htmx v1.9.12 attribution + URL.
- [x] 2.6 Vendor `templates/web/htmx.min.js` (htmx v1.9.12, binary, license header intact).

## Phase 3: Scaffolder Wiring

- [x] 3.1 `internal/pkg/scaffold/scaffold.go`: add `createBinaryFile(targetPath, embeddedPath)` via `TemplatesFS.ReadFile("templates/"+...)` + `os.WriteFile` (bypasses engine/override chain).
- [x] 3.2 Guard 3 arch mains with `if !s.config.UseTemplHTMX` (Minimalist `main.go`; Standard/Hexagonal `cmd/api/main.go`).
- [x] 3.3 Add `scaffoldWeb()`: mkdir `views/{layouts,pages,components}` + `static/{css,js}`; createFile ×6 (base/page/component/css/handler/readme); createBinaryFile htmx; web main → `main.go` (Minimalist) / `cmd/api/main.go`; hook `if s.config.UseTemplHTMX { return s.scaffoldWeb() }` in `createCommonFiles()` after gRPC block.

## Phase 4: Tests (`internal/pkg/scaffold/scaffold_test.go`)

- [x] 4.1 Extend `TestScaffolder_Layouts` OFF: no `views/`/`static/`; no `internal/handler/page.go` (Minimalist/Hexagonal; Standard keeps handler dir).
- [x] 4.2 Add ON cases ×3 archs: assert views, static, handler/page.go, README, web main exist.
- [x] 4.3 Web main path (Minimalist→`main.go`; Std/Hex→`cmd/api/main.go`) + content: contains `http.ListenAndServe` + `internal/handler`; NOT `internal/adapters`/`internal/domain`.
- [x] 4.4 htmx byte-identity: `bytes.Equal(TemplatesFS.ReadFile(...), os.ReadFile(target))`.
- [x] 4.5 Config round-trip (`use_templ_htmx:` in `.go-arch.yaml`); templ require present/absent via render; README contains `templ generate` + BSD-2-Clause; counter.templ contains hx-post/hx-target/hx-swap.
- [x] 4.6 Hex+OFF build: generated project `go mod tidy && go build ./...` exit 0.
- [x] 4.7 Hex+ON build: `go mod tidy && templ generate` (skip if binary absent) then `go build ./...` exit 0.
- [x] 4.8 Handler functional: httptest GET `/` → 200 + counter markup; POST `/counter` → 200 + "1"; second POST → "2" (mutex state persists).

## Phase 5: Verification

- [x] 5.1 `go test ./...` on go-arch repo — all green (existing + new).
- [x] 5.2 `go vet ./...` + `gofmt -l .` clean on changed files.
