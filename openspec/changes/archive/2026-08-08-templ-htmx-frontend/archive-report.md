# Archive Report: templ-htmx-frontend

**Status**: archived
**Archived**: 2026-08-08
**Branch**: `feat/templ-htmx-frontend-3` (3 slices, 6 commits)
**Author**: sdd-archive
**Input**: proposal, specs, design, tasks, verify-report

## Executive Summary

The `templ + HTMX Frontend Flag (UseTemplHTMX)` change is fully planned, implemented, verified, and now archived. `go-arch new` gains a `UseTemplHTMX` flag that scaffolds a full-stack, server-rendered Go web project (templ views + HTMX + static assets + web-aware main) when ON, and leaves all architecture scaffolds byte-identical when OFF. The pre-existing Hexagonal build break (empty `internal/adapters` / `internal/domain` imports) was also fixed. All delta specs have been synced into the main spec tree (source of truth).

## Final-State Facts (state at close — supersedes intermediate snapshots)

- **Tasks**: 21/21 complete (Phases 1–5). Persisted `tasks.md` shows every implementation task checked; no stale unchecked tasks. Tasks artifact is the completion-visibility source of truth per the Task Completion Gate.
- **Verification**: `verify-report` verdict **PASS** — 22/22 spec scenarios compliant, 0 CRITICAL, 0 blockers, `go test ./... -count=1` exit 0, `go vet ./...` exit 0, gofmt clean on changed files. Live runtime evidence: builds + serve + counter HTTP 1→2 under `sync.Mutex`. Warnings are informational only (WARNING-1: changed-file coverage <80%, mitigated by live verification).
- **No post-verify remediation**: verify was green directly; no fixes landed after the report.
- **Capability specs in place** (verified present, no copy needed):
  - `openspec/specs/templ-htmx-frontend/spec.md` (7 requirements, 13 scenarios)
  - `openspec/specs/hexagonal-build-fix/spec.md` (2 requirements, 3 scenarios)
- **cli spec synced**: `openspec/specs/cli/spec.md` merged the cli delta (4 requirements, 6 scenarios) into a new `## Requirements` section, preserving all existing Core Standards and Commands content.

## Native Review Gate

No native review infrastructure governs this project (no `review/` artifacts, no status-contract, no review kill switch configured). The archive proceeded on orchestrator-provided final-state facts and the PASS verification report. No review receipt was required; this is recorded here for traceability.

## Task Completion Gate

Inspected `openspec/changes/templ-htmx-frontend/tasks.md` (now archived) — all 21 implementation tasks are `[x]`. No exceptional stale-checkbox reconciliation was required. Change closed via status header added to `tasks.md`.

## Specs Synced

| Domain | Action | Details |
|--------|--------|---------|
| cli | Updated (MODIFIED) | 4 requirements, 6 scenarios merged from delta into `## Requirements` section; existing Core Standards + Commands preserved unchanged. Requirements: UseTemplHTMX ProjectConfig Field; Wizard Confirm Prompt for templ + HTMX; Config YAML Round-Trip; Conditional Templ Require in go.mod.tmpl. |
| templ-htmx-frontend | Already in place (new capability) | No change; verified present. |
| hexagonal-build-fix | Already in place (new capability) | No change; verified present. |

## Archive Contents

- `exploration.md` ✅
- `proposal.md` ✅
- `specs/cli/spec.md` (delta) ✅
- `design.md` ✅
- `tasks.md` ✅ (21/21 tasks complete, status CLOSED)
- `verify-report.md` ✅ (verdict PASS)
- `archive-report.md` ✅ (this file)

## Rollback Note

The change is additive and flag-gated. To roll back: drop `UseTemplHTMX` from `ProjectConfig`/wizard/`go.mod.tmpl`/`config.tmpl`, remove `templates/web/*` and the `scaffoldWeb`/`createBinaryFile` wiring, and revert the guarded arch-main lines. The Hexagonal build fix (dropped empty imports) is an independent, correct bug fix that should be retained even on rollback. No data migration; no breaking changes to existing generated projects. On rollback, the synced main specs (`cli`, `templ-htmx-frontend`, `hexagonal-build-fix`) would need corresponding reverts.

## Next Steps

Delivery (owned by the orchestrator — NOT performed here): create 3 chained PRs via `gh` against feature-branch chain `feat/templ-htmx-frontend`:
- **PR #1** (base: `feat/templ-htmx-frontend`): flag plumbing + hex fix — `internal/ui/prompts.go`, `internal/pkg/template/templates/common/go.mod.tmpl`, `common/config.tmpl`, `hexagonal/main.tmpl`, `cmd/new.go`.
- **PR #2** (base: PR #1): web templates + `scaffoldWeb` — `templates/web/*`, `scaffold.go`.
- **PR #3** (base: PR #2): functional + build verification — `scaffold_test.go` + verification evidence.

Do NOT commit the OpenSpec archive changes to a delivery PR unless the orchestrator directs otherwise. `briefing-go-arch-templ-htmx.md` remains untracked and is never pushed.
