# Archive Report: hooks

**Status**: ARCHIVED
**Archived**: 2026-08-09
**Change**: hooks (roadmap item 5 — lifecycle extensibility)
**Verify verdict**: PASS — 26/26 requirements, 57/57 scenarios
**Delivery mode**: Ordinary policy — receipt-driven development (review gate) disabled at clone scope; CI is authority. No receipt exists; nothing silently approved and none fabricated.

## Executive Summary

Added lifecycle hooks (`pre-new`, `post-new`, `pre-generate`, `post-generate`) to `.go-arch.yaml` so generated projects run their own tooling (`gofmt`, `go mod tidy`, `git init`). A new `internal/pkg/hooks/` package owns config parsing (hybrid string/object via `yaml.v3`), the runner (fakeable `CommandRunner`), and env building. The `Scaffolder` receives a `*hooks.Runner` via `ScaffoldOption` (`WithRunner`, `WithVersion`) and fires hooks inside `Execute`/`GenerateComponent`/`GenerateCRUD` — which gives MCP parity for free. Output always routes through `ui.Out` (stderr under MCP); `os.Stdout` is never written by the runner (integration-tested). Fully merged to main via PR #31.

## What Shipped

- **Config**: hybrid `hooks:` map — string shorthand (shell-executed via `sh -c`/`cmd /c`) and object form (`command`+optional `args`/`cwd`/`env`/`timeout`/`silent`/`ignore_failure`), argv-direct. Four fixed types only; unknown types/keys/missing-command/bad-duration/scalar-not-list rejected with `oops` codes.
- **Runner** (`internal/pkg/hooks/`): `CommandRunner`/`RealRunner` (`exec.CommandContext`), `Fire` with default 30s timeout (`0` disables), stop-on-first (`hook_failed`) unless `ignore_failure`, exit 127/9009 → `hook_command_not_found`, stdin closed, `ui.Out`-only output, CWD resolution via `cmd.Dir` (never `os.Chdir`).
- **Env**: standard vars `PROJECT_NAME`, `PROJECT_PATH` (absolute), `ARCHITECTURE`, `HOOK_TYPE`; parent env inherited, per-hook `env:` merged last.
- **Wiring**: 4 fire sites in `scaffold.go`; `WriteVersionField` moved into `Execute()` (non-fatal, version injected via `WithVersion`). CLI (`cmd/new.go`, `cmd/generate.go`) and MCP (`mcp/server.go` new_project + generate_component) construct/inject the runner. Loader errors surfaced at all 4 call sites (`hooks_load_failed` / `sendToolResult(error)`).
- **Surface**: commented `# hooks:` example in `config.tmpl`; `docs/hooks.md` (148 lines) with schema, fire sites, trust warning, MCP behavior, non-atomic `post-*` failure. No new CLI flags. `upgrade` fires no hooks.
- **Rollback**: drop Runner calls + package; older binaries ignore `hooks:` via viper (no migration).

## Verification Summary

Final state at close (highest authority: orchestrator final-state facts + merged commits on main):

- **Verify re-run verdict**: PASS — 26/26 requirements, 57/57 scenarios (48 requirement-level + 9 top-level).
- **Tests**: `go test ./... -count=1` exit 0 (7/7 packages ok); `go vet ./...` clean; `gofmt -l .` empty.
- **All four prior FAIL findings resolved** (per `verify-report`): Req 14 string-form command-not-found → `hook_command_not_found` (unit test + live probe); Req 7 and Req 10 s2 now have covering tests; all 4 loader-error call sites surface errors.
- **Post-verify fix** (landed on tracker/main AFTER verify-report was persisted): commit `b70ea84 fix(hooks): remove unused mustWrap test helper` — golangci-lint flagged unused `mustWrap` in `internal/pkg/hooks/runner_test.go` on the tracker PR's CI; removed 6 lines. No behavior or spec-compliance change; all tests green on main.
- **Merge chain**: PR #27 (slice 1) → #28 (slice 2) → #29 (slice 3) → #30 (slice 4) into tracker `feat/hooks`; tracker merged to main via PR #31 (commit `74217a1`).

### Follow-Ups (non-blocking, none spec-breaking)

1. **Mapping form-agnostic** (`verify-report` WARNING 1): `exitCode == 127 || 9009 → hook_command_not_found` also applies to object-form entries; an object-form binary legitimately exiting 127 would be misclassified as `hook_command_not_found` instead of `hook_failed`. Low likelihood, safe failure mode. Suggestion: scope the 127/9009 mapping to `entry.Shell` entries.
2. **Pre-existing MCP stdout pollution** (design Key Risks #1, out of scope): `fmt.Printf` in `scaffold.go` (121, 452, 494) and `ui.Success` write to `os.Stdout` under MCP. The hooks runner itself is clean.
3. **CLI config shadowing** (pre-existing): when `$HOME/.go-arch.yaml` exists, viper prefers it over the project `.go-arch.yaml` (search order home→cwd); MCP resets viper to CWD-only. Follow-up candidate.

## Artifacts

- exploration.md ✅ · proposal.md ✅ · specs/ (config-schema, environment, execution, mcp-and-upgrade, hooks/spec.md delta) ✅ · design.md ✅ · tasks.md ✅ (23/23 complete) · apply-progress.md ✅ · verify-report.md ✅ · archive-report.md ✅

## Spec Sync

- Delta spec `openspec/changes/hooks/specs/hooks/spec.md` (26 ADDED requirements, no prior main spec) copied verbatim (mechanical `cp` + empty `diff -r` readback) to `openspec/specs/hooks/spec.md`.
- The four aspect delta files (`config-schema.md`, `environment.md`, `execution.md`, `mcp-and-upgrade.md`) are preserved in the archive as the change's delta artifacts; the single `hooks/spec.md` carries the merged requirement set.

## Next Steps

- Change is closed. Follow-ups above are candidates for a future change. ROADMAP.md was already updated locally (untracked; not touched by archive).

## Rollback Note

Independent reverts per slice. Archived folder is an immutable audit trail. `.go-arch.yaml` files already using `hooks:` are backward/forward safe (older binaries ignore the key; newer honor it).

## Delivery-Mode Note

Receipt-driven development (review gate) disabled at clone scope per user decision after escalating upstream #2743. Archive under ordinary delivery policy — no receipt required, nothing silently approved.
