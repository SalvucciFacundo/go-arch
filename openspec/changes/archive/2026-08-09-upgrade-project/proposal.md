# Proposal: upgrade-project

## Status
**Proposed** — exploration complete (verified facts with file:line refs).

## Executive Summary
`go-arch` creates projects but cannot evolve them. When embedded templates change across releases (Go 1.24 bump, CRUD port fix, templ+HTMX scaffold, hexagonal import fix), generated projects are stranded. Add a fingerprint manifest recorded at generation time, a `go-arch upgrade` command that re-renders and classifies each file deterministically, and an `upgrade_project` MCP tool (6/6 parity).

## Intent
Propagate embedded template changes to projects the CLI generated earlier, without clobbering user edits. Give users a safe, default-dry-run upgrade path and a deterministic answer to "is this file scaffold-owned or user-owned?".

## Scope

### In Scope
- **D1 — Fingerprint manifest**: `.go-arch/manifest.yaml` (path → {sha256, origin}). `new`, `generate`, `crud` record each written file; `createFile`/`createBinaryFile` are the seam.
- **D2 — `go-arch upgrade` command**: cobra subcommand (doctor pattern), `--dry-run` (default) / `--yes` / `--project-path`. Re-renders via the same engine chain (local → global → embedded). Classification: untouched → upgradable, modified → PROTECTED skip, absent → report only. Legacy projects (no manifest): whitelist + per-file confirm. Web updates hint `templ generate` (never run it).
- **D3 — `upgrade_project` MCP tool**: dry-run by default (MCP cannot prompt); `apply: true` commits changes; returns plan as JSON.
- **Bonus**: `go_arch_version` field in `.go-arch.yaml` (written surgically, not by re-render — `generated_at: {{ now }}` makes re-render non-idempotent). Reporting/gating only.

### Out of Scope
- Auto-migrating user code in `internal/handler|service|repository|domain|model|ports|adapters/*_handler.go|adapters/*_repository.go`.
- Overwriting `go.mod` / `go.sum` — dep bumps are **report-only** `go get` hints (`go mod tidy` rewrites go.mod, making fingerprint comparison meaningless).
- Upgrading legacy projects without explicit confirmation.
- Running `templ generate` / `go build` silently (binary may be absent).

## Capabilities

### New Capabilities
- `upgrade-project`: manifest + upgrade command + MCP tool; deterministic ownership classification, dry-run default, legacy fallback.

### Modified Capabilities
- `cli`: register new `upgrade` subcommand under RootCmd (cobra pattern, doctor.go:14-16).

## Approach
- **Manifest** (primary mechanism): recorded by every scaffold write path; `go_arch_version` added to config.tmpl going forward.
- **Upgrade**: re-render each manifest entry; compare sha256 (manifest vs disk vs re-render) → classify. Apply uses compare-then-write, never blind `os.Create`.
- **Legacy fallback**: whitelist (main.go, cmd/api/main.go, .env, Dockerfile, docker-compose.yaml, Makefile, api/proto, telemetry, grpc server, web templates, static assets, README) + per-file prompt. `go.mod` report-only.
- **Non-TTY**: refuse to prompt; require `--yes` (CLI) or `apply: true` (MCP); otherwise print plan.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/pkg/scaffold/manifest.go` | New | Manifest struct, Load/Save/Update, hashFile |
| `internal/pkg/scaffold/scaffold.go` | Modified | `createFile`/`createBinaryFile` record fingerprints; `GenerateCRUD`/`GenerateComponent` likewise |
| `internal/pkg/scaffold/upgrade.go` | New | `Upgrade(cfg) (*Plan, error)` — classify + apply logic |
| `cmd/upgrade.go` | New | cobra command, flags, interactive plan display |
| `internal/pkg/mcp/server.go` | Modified | `upgrade_project` tool in `tools/list` + `handleToolCall` |
| `templates/common/config.tmpl` | Modified | add `go_arch_version: {{ .GoArchVersion }}` |
| `internal/ui/prompts.go` | Modified | `ProjectConfig.GoArchVersion` field |
| Tests | New/Modified | `upgrade_test.go` (scaffold), `cmd/upgrade_test.go`, engine override pattern |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| User-modified generated files clobbered | Med | Fingerprint manifest (untouched vs modified); legacy whitelist + per-file prompt; `--yes` opt-in |
| `go.mod` drift from `go mod tidy` | High | Report-only — never rewrite; print `go get` hints |
| `generated_at: {{ now }}` breaks re-render idempotency | High | NEVER re-render `.go-arch.yaml` wholesale; write version field surgically |
| `templ` binary absent | Med | Hint only, never run; reuse `generate.go:79-81` pattern |
| Legacy projects without manifest | Med | Whitelist + interactive confirm; document one-time opt-in by re-scaffolding or manual manifest bootstrap |
| Uncommitted user work in target files | Low | Warn when git dirty in target paths; `--dry-run` is the review path |
| Custom templates in `.go-arch/templates/` ignored | Low | Re-render uses full engine chain (local → global → embedded) — custom overrides preserved |

## Rollback Plan
- Delete `.go-arch/manifest.yaml` (additive file) — no behavior change to the project itself.
- `git checkout .` reverts any applied upgrade writes.
- `go_arch_version` field in `.go-arch.yaml` is additive; absence is tolerated.
- Upgrade is idempotent: re-running on a clean tree after apply produces no changes.

## Dependencies
- Existing `template.Engine.Render` + override chain (engine.go:31-65).
- Existing cobra/viper/survey/oops stack.
- Existing doctor/check command pattern for project-root commands.

## Success Criteria
- [ ] `go-arch new` + `upgrade` (no-op on fresh project) shows "up to date".
- [ ] Modified embedded template → `upgrade --dry-run` reports update; `--yes` applies; manifest fingerprint refreshed.
- [ ] User-edited generated file is PROTECTED and reported, never overwritten.
- [ ] Legacy project (no manifest) prompts per file, applies only confirmed.
- [ ] `upgrade_project` MCP tool returns plan JSON with `dryRun: true`; applies with `apply: true`.
- [ ] `go test ./...` passes with new upgrade tests (engine override pattern).

## Next Recommended
`sdd-spec` — spec the new `upgrade-project` capability (manifest schema, upgrade classification, MCP contract) and the `cli` delta (new subcommand).
