# Exploration: `go-arch upgrade` (roadmap item 2)

## Status

**Feasible.** The scaffolder's render plumbing is directly reusable, and the template engine's
local override chain gives us a deterministic way to simulate "template changed" in tests.
The core design decision is *how to know whether a generated file was user-modified* — pure
content diff cannot answer that; a generation-time fingerprint manifest can.

## Executive Summary

`go-arch upgrade` propagates template changes from the current embedded templates to projects
the CLI generated earlier. The scaffolder already renders every file through
`template.Engine.Render` (scaffold.go:48-67), so "apply an update" is a *re-render + compare +
conditional write* — new plumbing, no rewrite. The hard problem is clobbering: when a file on
disk differs from a fresh re-render, it is ambiguous whether that is a template change or a user
edit. Recommendation: record a **fingerprint manifest** (sha256 of every generated file +
`go_arch_version`) at generation time; upgrade then classifies each file deterministically
(untouched → upgradable, modified → protected, absent from manifest → user-owned). Legacy
projects without a manifest fall back to a whitelist + explicit confirmation. `.go-arch.yaml`
stores no CLI version today (config.tmpl:1-13) — add `go_arch_version` going forward as
reporting/gating, while the manifest is the operative mechanism. Add an `upgrade_project` MCP
tool for 6/6 tools parity, dry-run by default (MCP cannot prompt).

## Verified Current-State Facts (file:line)

### Template engine — `internal/pkg/template/engine.go`
- `TemplatesFS embed.FS` — `//go:embed all:templates/*` (engine.go:18-19).
- `Render(wr io.Writer, templatePath, data)` renders to any writer (engine.go:31-42); prints
  `Using custom template (...)` for non-embedded sources (engine.go:37-39).
- `getTemplate` resolution order **local → global → embedded**:
  1. `.go-arch/templates/<path>` (engine.go:46-50)
  2. `~/.go-arch/templates/<path>` (engine.go:53-60)
  3. `templates/<path>` in `TemplatesFS` (engine.go:62-65)
- FuncMap: `now`, `lower`, `upper`, `plural`, `title` (engine.go:68-83).
- **No comparison/diff capability exists** — Render only writes; nothing reads back or compares.

### Scaffolder — `internal/pkg/scaffold/scaffold.go`
- `Scaffolder{engine, config}` (scaffold.go:15-18); `NewScaffolder` (20-25).
- `Execute()` switches on architecture → Minimalist/Standard/Hexagonal (scaffold.go:36-45).
- `createFile(path, templatePath, data)` — `os.MkdirAll` + **`os.Create` (unconditional
  truncate)** + `engine.Render` (scaffold.go:48-67). No "skip if unchanged" logic exists.
- `createBinaryFile` — ReadFile from `TemplatesFS` + WriteFile, bypasses engine and the
  override chain (scaffold.go:73-83) — used for `static/js/htmx.min.js` (scaffold.go:116).
- `scaffoldWeb()` writes: `views/layouts/base.templ`, `views/pages/home.templ`,
  `views/components/counter.templ`, `static/css/style.css`, `internal/handler/page.go`,
  `README.md` (scaffold.go:102-114); web main at `cmd/api/main.go` or `main.go` for Minimalist
  (scaffold.go:120-124).
- Architecture mains: `minimalist/main.tmpl` (scaffold.go:130), `standard/main.tmpl`
  (scaffold.go:151), `hexagonal/main.tmpl` (scaffold.go:172) — each only when `!UseTemplHTMX`.
- `createCommonFiles()`: always `go.mod`, `.go-arch.yaml`, `.env` (scaffold.go:181-191); Docker
  (194-201); telemetry (204-211); gRPC + `Makefile` (214-224); web (227-229).
- `GenerateComponent` target paths per arch: Hexagonal → `internal/domain|ports|adapters`,
  else `internal/service|repository|handler` (scaffold.go:284-307); page/component guarded +
  collision check (scaffold.go:261-265, 279-283).
- `GenerateCRUD` per-arch file map — Hexagonal: `internal/domain/<name>.go`,
  `internal/domain/<name>_service.go`, `internal/ports/<name>_repository.go` (crud_port.tmpl),
  `internal/adapters/<name>_repository.go`, `internal/adapters/<name>_handler.go`; else
  `internal/model|service|repository|handler` (scaffold.go:325-340).

### Generated project shape — `internal/pkg/template/templates/`
- `common/`: config.tmpl, go.mod.tmpl, env.tmpl, Dockerfile.tmpl, docker-compose.yaml.tmpl,
  telemetry.tmpl, telemetry_middleware.tmpl, service.proto.tmpl, grpc_server.tmpl, Makefile.tmpl,
  handler.tmpl, service.tmpl, repository.tmpl, model.tmpl, crud_handler.tmpl, crud_port.tmpl,
  crud_repository.tmpl, crud_service.tmpl.
- `minimalist|standard|hexagonal/main.tmpl` — architecture mains.
- `web/`: base.templ.tmpl, page.templ.tmpl, component.templ.tmpl, page_generated.tmpl,
  component_generated.tmpl, handler.tmpl, main.tmpl (web superset), readme.tmpl,
  style.css.tmpl, htmx.min.js (binary, 1.9.12 per readme attribution).
- Real template changes across versions (git log): crud_port interface fix (a2bc77b), hexagonal
  empty-import fix (ce0f148), templ+HTMX scaffold (0e9e461), page/component templates (84344a9),
  Go 1.24 bump (74821b9) — exactly the "changes can't propagate" problem `upgrade` solves.

### `.go-arch.yaml` — `templates/common/config.tmpl`
- Fields: `project_name`, `module_name`, `architecture`, `db_driver`, `use_docker`,
  `use_observability`, `observability_backend`, `use_grpc`, `use_templ_htmx`, `generated_at`
  (config.tmpl:4-13).
- **No `go-arch` CLI version field** — confirmed (config.tmpl:1-13).
- **`generated_at: {{ now }}`** (config.tmpl:13) — a timestamp makes any wholesale re-render of
  `.go-arch.yaml` always differ. Upgrade must never re-render the whole file; version field must
  be written surgically.

### doctor command — `cmd/doctor.go` (pattern for the new command)
- init() + `RootCmd.AddCommand` (doctor.go:14-16); cobra `RunE` with `ui.*` output and oops
  codes (doctor.go:18-78).
- Config read through viper in pure function `projectConfigStatus()` (doctor.go:83-91),
  extracted for deterministic tests (doctor_test.go:36-92).
- Config loading: root.go `initConfig` — `viper.AddConfigPath(home + ".")`,
  `viper.SetConfigName(".go-arch")` (root.go:41-60).

### Test patterns
- scaffold_test.go: `os.MkdirTemp` + chdir + restore (56-66), `NewScaffolder` + `Execute`
  (76-77), assert files exist/content (82-87); round-trip content assertions (483-554);
  `templ generate` + `go build ./...` integration tests, skipped when `templ` missing (559-607).
- cmd tests: chdir tempdir, call `doctorCmd.RunE`, `viper.Reset` + `AddConfigPath(".")` +
  `ReadInConfig` (doctor_test.go:23-60).
- engine_test.go: render to `bytes.Buffer` (28-29); **custom template override via
  `.go-arch/templates/`** (114-151) — this is the exact mechanism upgrade tests can use to
  simulate a "new template version" without touching the embedded FS.
- Strict TDD active: `test_command: go test ./...` (openspec/config.yaml).

### MCP — `internal/pkg/mcp/server.go`
- 5 tools today: new_project (114), generate_component (163), check_architecture (186),
  serve_project (199), setup_environment (212); dispatch switch (251-521).
- Per-tool pattern: optional `projectPath` chdir (304-313), `viper.Reset` + `ReadInConfig`
  (315-321), action, `sendToolResult` (524-533).
- **UI redirected to stderr** (server.go:44) — interactive survey prompts are impossible in MCP
  mode; an upgrade tool must be non-interactive (dry-run default / explicit apply flag).

### Version — `cmd/version.go`
- `Version = "dev"` injected via ldflags (version.go:5-6; main package version.go:4) — the value
  available for a `go_arch_version` field.

## Affected Areas
- `internal/pkg/template/engine.go` — add a `RenderToString`/byte-render helper (or reuse
  Render into a buffer) and possibly a `TemplateSource` accessor for upgrade's re-render.
- `internal/pkg/scaffold/scaffold.go` — write the manifest in `createFile`/`createBinaryFile`/
  `GenerateCRUD`/`GenerateComponent`; add an `Upgrade(config) (*Plan, error)` method.
- `internal/pkg/scaffold/manifest.go` (new) — manifest read/write, fingerprinting, plan
  classification (unchanged / update-available / user-modified / missing / user-added).
- `cmd/upgrade.go` (new) — cobra command, `--dry-run`, `--yes`, `--project-path`; doctor pattern.
- `internal/pkg/mcp/server.go` — `upgrade_project` tool in `tools/list` + `handleToolCall`.
- `templates/common/config.tmpl` — add `go_arch_version: {{ .GoArchVersion }}` (default "dev").
- `internal/ui/prompts.go` — `ProjectConfig.GoArchVersion` field (mapstructure).
- Tests: `internal/pkg/scaffold/upgrade_test.go`, `cmd/upgrade_test.go` (new); extend
  `engine_test.go` override pattern.

## Approaches

| # | Approach | Pros | Cons | Effort |
|---|----------|------|------|--------|
| 1 | **Pure content diff** — hardcoded list of template targets; re-render, compare bytes, report/apply | Works for existing projects immediately; zero scaffolder changes; simple | Cannot distinguish template change from user edit (ambiguous diffs in main.go always prompt or clobber); `go mod tidy` rewrites go.mod → noise; hardcoded path list rots as templates evolve; no per-file ownership | Low-Med |
| 2 | **Fingerprint manifest (recommended)** — `new`/`generate`/`crud` record sha256 of every written file + `go_arch_version`; upgrade re-renders and classifies per file | Deterministic ownership: untouched → upgradable, modified → protected, absent → user-owned; solves clobbering at the root; version field adds reporting + future migration hooks (roadmap item 6); works with local/global custom templates (same engine chain); no static whitelist needed for new projects | Requires changing `new`/`generate`/`crud` to write manifest; legacy projects lack it → need fallback mode; a bit more code | Med |
| 3 | **Version-gated diff** — only compare when `go_arch_version < current` | Simple gate; cheap "up to date" answer | Does not solve user-edit detection on its own; insufficient alone | Low (only as part of 2) |

## Recommendation

**Approach 2 — fingerprint manifest as the primary mechanism**, with Approach 1 semantics as
the legacy fallback:

- **Manifest**: `.go-arch/manifest.yaml` (coexists with `.go-arch/templates/`, engine.go:46)
  listing `{path, sha256, template, origin}` where origin ∈ {scaffold, component, crud, binary}.
  Written by `createFile`/`createBinaryFile` (scaffold), `GenerateCRUD` (scaffold.go:313-351)
  and `GenerateComponent` (scaffold.go:235-310) — every write path registers a fingerprint.
- **`go-arch upgrade`** (run in project root, viper config like check/doctor, cmd/check.go:23-35):
  1. Validate `.go-arch.yaml` (project_name present) — else oops `missing_config`
     (check.go:24-29 pattern).
  2. Re-render each manifest file from the current templates via `engine.Render` (same local →
     global → embedded chain, so `.go-arch/templates/` custom overrides keep working).
  3. Classify: disk == fingerprint && re-render != disk → **update available**; disk !=
     fingerprint → **user-modified (protected, reported)**: not in manifest → never touched;
     file missing on disk → report, do not recreate (default).
  4. `--dry-run` prints the plan and exits 0. Default interactive: grouped `survey.Confirm`
     ("Apply N scaffold updates?") — survey already a dependency (prompts.go:6). `--yes`
     applies all proposed. Non-TTY (MCP/CI): refuse to prompt, require `--yes`, else print plan.
  5. Apply = render new bytes to the file (compare-then-write, never blind `os.Create`),
     update fingerprints + `go_arch_version` in the manifest.
  6. Web projects: after view/static updates, print the `templ generate` hint
     (reuse generate.go:79-81 `templHint`); never run `templ generate`/`go mod tidy` silently.
- **Version tracking**: add `go_arch_version` to `.go-arch.yaml` (config.tmpl) going forward.
  Operative mechanism is the manifest; the version is informational (report "created with
  v1.8.0, templates at v1.9.0"), a cheap up-to-date gate, and the hook for future versioned
  migrations (ROADMAP.md:60). Do **not** re-render `.go-arch.yaml` wholesale — `generated_at`
  (config.tmpl:13) makes it never-idempotent; write only the version field surgically.
- **MCP**: add `upgrade_project` tool (6/6 parity with CLI commands, ROADMAP.md:14). Params
  `projectPath`, `dryRun` (default true), `apply` (default false). Non-interactive: with
  `apply: false` returns the plan as JSON; `apply: true` performs the classified updates.
- **CLI shape**: `go-arch upgrade [--dry-run] [--yes] [--project-path <dir>]`, mirroring
  doctor's structure (init + cobra.Command + oops codes).

### What is safe to update (legacy whitelist fallback)
- **Propose apply** (scaffold-owned): `main.go` / `cmd/api/main.go` (web main superset,
  web/main.tmpl), `go.mod` (care: see risk 2), `.env`, `Dockerfile`, `docker-compose.yaml`,
  `Makefile`, `api/proto/service.proto`, `internal/telemetry/*`, `internal/adapters/grpc/server.go`,
  `static/js/htmx.min.js` (binary), `static/css/style.css`, `README.md`, scaffold-original
  views (`views/layouts/base.templ`, `views/pages/home.templ`, `views/components/counter.templ`).
- **Never auto-apply** (user-owned): `internal/handler/*.go` (custom logic — incl. scaffolded
  page.go and CRUD handlers once edited), `internal/service/*.go`, `internal/repository/*.go`,
  `internal/domain/*.go`, `internal/model/*.go`, `internal/ports/*.go`,
  `internal/adapters/*_handler.go` / `*_repository.go` (entity files), user-added
  `views/pages|components/*`.
- For manifest projects this static split is unnecessary — fingerprints decide per file.

## Risks & Edge Cases
1. **User-modified generated files** (central risk) — fingerprint solves it for new projects;
   legacy fallback must prompt per file and never auto-overwrite user-owned paths.
2. **`go.mod` / `go.sum`** — `go mod tidy` rewrites go.mod (indirect requires), so its
   fingerprint drifts immediately and dep bumps would look "user-modified". Treat go.mod as
   report-only (print the `go get` commands the new template requires) or special-case it;
   `go.sum` is never scaffold-generated (created by tidy) so it stays out of the manifest.
3. **Deleted generated files** — user deleted e.g. `counter.templ`; default is report + skip
   (do not resurrect), document `--yes`-with-caveat if recreation is ever desired.
4. **Architecture mismatch** — re-render uses the *stored* architecture from `.go-arch.yaml`
   (as serve/check do, serve.go:23, check.go:34); a manual architecture edit flows through.
5. **templ generate staleness** — after `views/*.templ` updates, `*_templ.go` files are stale
   and `go build` breaks; upgrade must hint `templ generate` (binary may be absent → hint only,
   generate.go:79-81 pattern) and never run build tooling silently.
6. **`.go-arch.yaml` timestamp** — `generated_at: {{ now }}` (config.tmpl:13) forbids wholesale
   re-render; only the version field is written.
7. **Custom/global template overrides** — upgrade must use `engine.Render`'s local → global →
   embedded chain (engine.go:44-66) so user overrides in `.go-arch/templates/` are respected;
   comparing against raw embedded bytes would break custom-template projects.
8. **Non-TTY / CI / MCP** — survey prompts impossible; dry-run + explicit `--yes` (CLI) or
   `apply: true` (MCP); MCP UI already on stderr (server.go:44).
9. **Post-adoption generate calls** — `generate`/`crud` must register fingerprints, otherwise
   upgrade treats those files as user-added and never updates them.
10. **Uncommitted work** — overwriting files the user has dirty in git is destructive; upgrade
    should warn when the project is a git repo with uncommitted changes in target files (or
    document that `--dry-run` + `--yes` is the review path).

## Test Strategy (strict TDD, `go test ./...`)
- **Engine injection for "template changed"**: reuse the `.go-arch/templates/` local override
  (engine.go:46-50, proven in engine_test.go:114-151) — scaffold v1, copy a modified template
  into the project's local override, run upgrade, assert file updated + manifest fingerprint
  refreshed. No embedded-FS mutation needed.
- **User-edit protection**: scaffold, hand-edit a generated file, run upgrade → assert file
  unchanged and reported as user-modified; assert exit/plan status.
- **Manifest round-trip**: fingerprint matches after clean re-render; new files registered by
  `GenerateCRUD`/`GenerateComponent`.
- **cmd tests**: `upgradeCmd.RunE` in tempdir with `viper.Reset` (doctor_test.go:16-31, 55-60);
  `--dry-run` writes nothing; `--yes` applies; missing config → `missing_config` error.
- **MCP**: `tools/list` includes upgrade_project; plan call with `dryRun: true` mutates nothing.

## Ready for Proposal

**Yes.** Feasibility confirmed with file:line evidence. Tell the user: the approach is a
generation-time fingerprint manifest (`.go-arch/manifest.yaml`) + `go_arch_version` in
`.go-arch.yaml`, `go-arch upgrade` with dry-run/interactive/`--yes` confirmation, a whitelist
fallback for legacy projects, and an `upgrade_project` MCP tool. Proposal should scope the
manifest write-back into `new`/`generate`/`crud` plus the upgrade command, tests, and MCP tool.
