# Exploration: `generate-templ-views`

**Status**: complete
**Branch**: `main` (includes merged templ+HTMX frontend feature, HEAD e3c6bf9)
**Test runner**: `go test ./...` (strict TDD)
**Artifacts**: this file + Engram `sdd/generate-templ-views/explore`

## Executive Summary

Extending `go-arch generate` with `page` (view) and `component` types is **feasible, low-effort, and additive**. It reuses the existing `GenerateComponent` dispatch pattern (scaffold.go:231-270), requires **two new embedded templates** (existing `web/page.templ.tmpl` / `web/component.templ.tmpl` hardcode `templ Home`/`templ Counter` and cannot be reused — they would produce duplicate symbols), and the `views/` target tree is **architecture-agnostic** (no Minimalist/Standard/Hexagonal branching needed — only the backend components branch). The one real prerequisite: **`generate` does not currently read `use_templ_htmx` from `.go-arch.yaml`** (cmd/generate.go:37-43 and mcp/server.go:292-298 both map only a subset of fields), so config gating requires adding a single `viper.GetBool("use_templ_htmx")` read in **both** entry points. MCP `generate_component` needs its `type` enum extended (server.go:168) plus the same viper read; its dispatch (server.go:302-306) already routes non-crud types to `GenerateComponent` unchanged.

## Current State (verified, file:line)

1. **CLI dispatch** — `cmd/generate.go`:
   - `Args: cobra.ExactArgs(2)` (line 21); `compType := args[0]`, `name := args[1]` (lines 24-25).
   - Viper → `ui.ProjectConfig` maps ONLY `project_name`, `module_name`, `architecture`, `db_driver`, `use_docker` (lines 37-43). **`use_templ_htmx` is NOT read** — `generate` cannot know whether the project has the web scaffold.
   - Dispatch (lines 49-53): `if compType == "crud" { GenerateCRUD(name) } else { GenerateComponent(compType, name) }` — new types plug into `GenerateComponent`'s switch with **zero dispatch changes**.
   - Errors: `oops.Code("missing_config")` (lines 30-34), `oops.Code("generation_failed")` with `type`/`name` context (lines 56-61).

2. **Generation pattern** — `internal/pkg/scaffold/scaffold.go` `GenerateComponent` (lines 231-270):
   - Data struct `{ui.ProjectConfig; EntityName}` (lines 235-241) — sufficient for templ templates; no new struct needed.
   - `switch compType` (line 243): `service` (244-250), `repository` (251-257), `handler` (258-264), `default: unsupported component type` (265-267). Each case sets `templatePath` + `targetPath`, branching on `Architecture == "Hexagonal"` (internal/domain|ports|adapters) vs else (internal/service|repository|handler).
   - Rendered via `createFile` (line 269) — `os.MkdirAll` on parent dirs + `os.Create` (scaffold.go:44-63). **`os.Create` silently overwrites existing files** (line 52) — name collisions are silently clobbered today (consistent with backend components).

3. **Web scaffold is architecture-agnostic** — `scaffoldWeb()` (scaffold.go:84-121): creates `views/{layouts,pages,components}` + `static/{css,js}` (lines 85-96) identically for Minimalist/Standard/Hexagonal; only the main-file target differs (root `main.go` for Minimalist, `cmd/api/main.go` otherwise, lines 116-120). Files: `views/layouts/base.templ`, `views/pages/home.templ`, `views/components/counter.templ`, `static/css/style.css`, `internal/handler/page.go`, `README.md` (lines 98-110), binary `static/js/htmx.min.js` (112-114). **Conclusion: `generate page|component` targets are the SAME paths for all three architectures** — no architecture branch, unlike service/repository/handler.

4. **Templ conventions to follow** — `internal/pkg/template/templates/web/`:
   - `base.templ.tmpl`: `package layouts`, `templ Base(count int)`, imports `{{ .ModuleName }}/views/components`, renders `@components.Counter(count)` (line 18).
   - `page.templ.tmpl`: `package pages`, `templ Home(count int)`, `@layouts.Base(count)` (lines 1-7).
   - `component.templ.tmpl`: `package components`, `templ Counter(count int)`, `div#counter` with `hx-post="/counter" hx-target="#counter" hx-swap="innerHTML"` (lines 1-8).
   - `handler.tmpl` → `internal/handler/page.go`: `PageHandler` renders `pages.Home(c)`, `CounterHandler` renders `components.Counter(c)` (lines 16-31).
   - `web/main.tmpl`: `mux.HandleFunc("GET /", handler.PageHandler)`, `POST /counter`, `GET /static/` file server (lines 40-44).
   - **Package names are fixed per directory** (`layouts`/`pages`/`components`) — generated files must hardcode them, not derive from the name.
   - **Reuse of existing templates is impossible**: `page.templ.tmpl` and `component.templ.tmpl` hardcode `templ Home`/`templ Counter`; rendering them a second time (e.g. `generate page dashboard`) would emit a second `Home`/`Counter` symbol in the same package → compile error, and the name would not match the requested entity. **New parametrized templates are required.**

5. **Config round-trip / gating feasibility** — `internal/ui/prompts.go:18` `UseTemplHTMX bool \`mapstructure:"use_templ_htmx"\``; `common/config.tmpl:12` emits `use_templ_htmx: {{ .UseTemplHTMX }}`; `common/go.mod.tmpl:23-27` pins `github.com/a-h/templ v0.3.906` when ON. **The value IS in `.go-arch.yaml`**, so `viper.GetBool("use_templ_htmx")` is available — it is simply not mapped today at either generation entry point (cmd/generate.go:37-43, mcp/server.go:292-298). Pre-merge projects lack the key → `GetBool` returns `false` → gate rejects (correct: they have no `views/` tree).

6. **MCP server** — `internal/pkg/mcp/server.go`:
   - `generate_component` tool: description line 162, `type` enum `["service", "repository", "handler", "crud"]` line 168, `name` line 171-174, `projectPath` 175-178.
   - Dispatch (lines 302-306): `if args.Type == "crud" { GenerateCRUD } else { GenerateComponent }` — **new types flow through automatically** once the enum + `GenerateComponent` accept them.
   - Config mapping (lines 292-298) also lacks `UseTemplHTMX` — same fix needed.
   - `new_project` already accepts `useTemplHTMX` (line 235, schema 152-155).

7. **Test patterns** — `internal/pkg/scaffold/scaffold_test.go`:
   - Scaffold tests: tempdir + `os.Chdir` + `NewScaffolder(config)` + `Execute()`, assert file existence (e.g. `TestScaffolder_Layouts` 14-86, `TestScaffolder_FlagONWebFiles` 306-362).
   - Generation-level pattern: `TestScaffolder_CRUD` (718-752) — `ProjectName: "."`, chdir tempdir, call generator directly, assert expected files exist.
   - Content assertions: `TestScaffolder_ConfigAndContentRoundTrip` (479-550) — read generated file, `strings.Contains` checks (e.g. hx attributes 545-549).
   - Build tests skip when `templ` binary is absent (`exec.LookPath`, lines 556-558, 610-612). **File/content/gating tests need no templ binary** — strict-TDD-safe.

## Affected Areas

- `internal/pkg/scaffold/scaffold.go` — add `page`/`component` cases to `GenerateComponent` switch (line 243) + optional `requireWebScaffold()` gate; `scaffoldWeb` (84-121) unchanged.
- `cmd/generate.go` — add `UseTemplHTMX: viper.GetBool("use_templ_htmx")` to config mapping (lines 37-43); reword `Generating %s component` messages (lines 45, 63) since views are not "components"; new oops error code for the gate.
- `internal/pkg/mcp/server.go` — extend enum (line 168) + description (line 162); add `UseTemplHTMX` to config mapping (lines 292-298).
- `internal/pkg/template/templates/web/page_generated.tmpl` (NEW) — `package pages`, `templ {{ title .EntityName }}()` wrapping `@layouts.Base(0)`.
- `internal/pkg/template/templates/web/component_generated.tmpl` (NEW) — `package components`, `templ {{ title .EntityName }}()` with HTMX attribute pattern.
- `internal/pkg/scaffold/scaffold_test.go` — new tests: page/component generation (files exist + content), gating (UseTemplHTMX=false → error), overwrite/collision behavior.
- `internal/pkg/template/engine.go` — unchanged (new templates auto-embedded by `//go:embed all:templates/*` line 18; funcMap lines 68-82 already has `title`/`lower`). Optional: `pascal` func if hyphenated names are supported.
- `openspec/specs/cli/spec.md` — future delta: extend the `generate` command requirement (line 31: "Scaffolds components (services, repos, handlers)").

## Design Questions — Answers

| Question | Answer | Evidence |
|----------|--------|----------|
| What does `generate page <name>` produce? | Just `views/pages/{name}.templ` (package pages, `templ {Title}()` wrapping `@layouts.Base(0)`) + success message with hint to wire a route in `internal/handler/page.go` / `web/main.tmpl` and re-run `templ generate`. Matches the CLI's existing "you register the routes" philosophy (`GenerateCRUD` prints "📍 Remember to register the routes", scaffold.go:309). Handler+route auto-registration would require appending to existing compiled files — not template-friendly; defer. | scaffold.go:273-310 |
| What does `generate component <name>` produce? | `views/components/{name}.templ` (package components, `templ {Title}()` with the same hx-* attribute shape as counter.templ, e.g. `hx-get="/{name}" hx-target="#{name}" hx-swap="innerHTML"`). | component.templ.tmpl:3-8 |
| Architecture mapping? | **None needed** — same `views/` paths for Minimalist/Standard/Hexagonal; only backend targets branch on architecture. Simpler than service/repository/handler. | scaffold.go:84-121 |
| Config gating? | **Yes — require `use_templ_htmx: true`**. Without the gate, `createFile`'s `MkdirAll` silently creates `views/` + a `.templ` file in a project whose `go.mod` has no templ dependency → dead, uncompilable code. Gate must be added as a viper read in BOTH cmd/generate.go and mcp/server.go (neither reads it today). Failure mode without web scaffold: `oops.Code("web_scaffold_required")` + hint. | cmd/generate.go:37-43; mcp/server.go:292-298; scaffold.go:44-63 |
| Template design? | **New** `web/page_generated.tmpl` + `web/component_generated.tmpl`, parametrized on `{{ .EntityName }}` + existing `title`/`lower` funcs + `{{ .ModuleName }}`. Existing templates hardcode `Home`/`Counter` → reuse causes duplicate symbols. Data passed: the existing `{ui.ProjectConfig; EntityName}` struct — no change. | page.templ.tmpl:5; component.templ.tmpl:3; scaffold.go:235-241 |
| Naming: `view` vs `page`? | **`page`** — maps exactly to `views/pages/`, matches templ's own domain language ("pages/layouts/components"), mirrors `ng generate`. `view` is a reasonable alias but less precise. `component` for the second type (matches `views/components/`, ng parity). Minor collision: "component" is also the CLI umbrella term (generate.go:19-20 "Generate a new component") — reword help text. | — |
| MCP impact? | **Yes** — extend `type` enum (line 168) + description (line 162); dispatch needs no change (lines 302-306). Same `UseTemplHTMX` viper read (lines 292-298). | mcp/server.go:160-182, 292-306 |

## Approaches

1. **Minimal reuse of `GenerateComponent` (recommended)** — add `page`/`component` cases to the existing switch (scaffold.go:243), two new embedded templates, `UseTemplHTMX` viper read + gate in cmd/generate.go and mcp/server.go, enum extension, tests.
   - Pros: follows the exact existing pattern; ~6 files; additive; no dispatch changes; template engine embeds new files automatically.
   - Cons: silent overwrite on name collision (pre-existing behavior); `generate component` umbrella-term collision needs help-text reword.
   - Effort: **Low** (~100-150 authored lines + tests; within 400-line review budget).

2. **Generation + handler/route auto-registration** — also append a `{Name}Handler` to `internal/handler/page.go` and a `mux.HandleFunc` to the web main.
   - Pros: one-shot wiring, closer to full `ng generate` parity.
   - Cons: appending to existing compiled Go files is not template-friendly (would require full-file re-render or text surgery); risks clobbering user edits; significantly more tests.
   - Effort: **High**.

3. **No gating (generate views into any project)** — skip the `use_templ_htmx` check.
   - Pros: zero config changes.
   - Cons: generates uncompilable code into non-web projects (no templ dep in go.mod); violates the project's clean-failure philosophy (oops codes everywhere).
   - Effort: **Low** but **rejected** — poor UX, violates existing error conventions.

## Recommendation

**Approach 1**: extend `GenerateComponent` with `page` and `component` cases mapping to arch-agnostic `views/pages/` and `views/components/` targets, backed by two new parametrized templates (`web/page_generated.tmpl`, `web/component_generated.tmpl`) using the existing `{ProjectConfig; EntityName}` data + `title`/`lower` funcs. Require `use_templ_htmx: true` in both generation entry points (add `viper.GetBool("use_templ_htmx")` to cmd/generate.go:37-43 and mcp/server.go:292-298), failing with a new oops code when the project lacks the web scaffold. Extend the MCP `generate_component` enum (server.go:168) + description. Command names: `go-arch generate page <name>` and `go-arch generate component <name>` (alias `view` optional). Print a success hint to re-run `templ generate` (mirrors cmd/new.go:43-45).

## Edge Cases and Risks

- **Non-web project / pre-merge config**: `.go-arch.yaml` missing `use_templ_htmx` → `GetBool` = false → gate rejects with clear oops error. Correct behavior; no migration needed.
- **Name collisions**: `os.Create` silently overwrites (scaffold.go:52) — `generate page home` clobbers the scaffold's `home.templ` and would then produce duplicate `Home` symbols → compile error. Pre-existing behavior for backend components; either accept (document) or add an exists-check in the change scope (recommend: document only, keep scope tight).
- **kebab-case names**: filename `user-dashboard.templ` is legal, but `templ {{ title .EntityName }}()` → `User-dashboard` is an **invalid Go identifier**. Existing convention already requires CamelCase (`mcp/server.go:173` "e.g. User, Product"; backend templates use `EntityName` as a Go identifier). Document the constraint; optionally add a `pascal` func to engine.go:68-82 to strip hyphens (scope decision for proposal).
- **Duplicate symbols in one package**: two generated files declaring the same templ component → compile error (same class as collisions above).
- **`component` umbrella-term collision**: help text (generate.go:19-20) must be reworded so `generate component` reads as a concrete type, not the category.
- **`templ generate` must be re-run** after generation — CLI cannot invoke templ; print the hint (new.go:43-45 pattern).
- **Future Datastar**: `briefing-go-arch-datastar.md` suggests `UseTemplDatastar` may supersede `UseTemplHTMX`; gating on the existing flag keeps the extension point reusable for a future sibling flag. Out of scope here; noted only.

## Ready for Proposal

**Yes** — all six verification points confirmed against real code. The proposal phase should decide: (a) `page` vs `view` naming (recommend `page`, optional `view` alias), (b) whether to add a `pascal` template func for hyphenated names, (c) whether to add an exists-check for name collisions (recommend defer), (d) exact generated-template body for page (`@layouts.Base(0)` vs parameterized).
