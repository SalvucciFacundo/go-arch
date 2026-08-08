# Archive Report — generate-templ-views

**Status**: archived
**Change**: generate-templ-views
**Archived to**: `openspec/changes/archive/2026-08-08-generate-templ-views/`
**Archive date**: 2026-08-08
**Branch**: `feat/generate-templ-views`

## Final-State Facts (at close)

- **Tasks**: 14/14 complete (tasks.md fully checked). No stale unchecked implementation tasks.
- **Verify verdict**: PASS — 16/16 spec scenarios compliant (live CLI + MCP + `templ generate` + `go build ./...` evidence).
- **No post-verify remediation**: verify was green directly; no CRITICAL or WARNING findings.
- **Capability spec**: `openspec/specs/templ-view-generation/spec.md` was already in place (verified, not copied).
- **Delta sync**: `cli` delta merged into `openspec/specs/cli/spec.md` (3 requirements added; 4 pre-existing requirements preserved; editorial Commands entry updated).

## Review Delivery Mode (native gate)

Native review delivery (`receipt-driven development`) is **disabled** at clone scope for this repository: `gentle-ai review mode status` reports `clone-local: off` (decided by user after issue #2743, a corrupted 2.2.4-era lineage escalation). With the kill switch off this is `disabled/unmanaged` — **ordinary repository policy governs delivery** (hooks, tests, CI). `reviewGate` is structurally absent from status; nothing was silently approved and no fabricated receipt exists. The earlier archive blocker from the corrupted lineage no longer applies because review delivery is disabled for this repo. Archive proceeded under ordinary repository policy.

## Artifacts

| Artifact | Path | Present |
|----------|------|---------|
| Exploration | `exploration.md` | ✅ |
| Proposal | `proposal.md` | ✅ |
| Delta spec (cli) | `specs/cli/spec.md` | ✅ |
| Design | `design.md` | ✅ |
| Tasks | `tasks.md` | ✅ (14/14 complete) |
| Verify report | `verify-report.md` | ✅ (PASS 16/16) |
| Archive report | `archive-report.md` | ✅ |

## Specs Synced

| Domain | Action | Details |
|--------|--------|---------|
| `cli` | Updated | 3 requirements added (`Generate Command Supports Page and Component Types`, `Generate Oops Codes for Web Generation`, `Backend Generation Unchanged`); editorial Commands entry updated |

Source of truth updated: `openspec/specs/cli/spec.md`.

## Next Steps (delivery)

- **Deliver**: single PR `feat/generate-templ-views` → `main`. The change is ready for the merge gate under ordinary repository policy.
- Merge gate evidence: `go test ./...`, `go vet ./...`, `go build ./...` all green per verify report; 4 work-unit commits on the branch.

## Rollback Note

Rollback is a revert of the single delivery PR (`feat/generate-templ-views` → `main`). The change is additive and isolated:
- Templates: `internal/pkg/template/templates/web/page_generated.tmpl`, `component_generated.tmpl`
- Scaffold: `internal/pkg/scaffold/scaffold.go` (page/component cases + guards + `isValidGoIdentifier`)
- CLI: `cmd/generate.go` (help text, gate, `templHint`)
- MCP: `internal/pkg/mcp/server.go` (enum + description + config mapping)

Backend generation behavior is unchanged; reverting the PR restores prior behavior with no data migration required.

## Mechanical Copy Verification

- Archive move used snapshot → `mv` fallback (`git mv` refused because the change folder was untracked); source directory confirmed removed.
- `diff -r` (snapshot vs. archived tree): **empty** — PASS. Archive report is additive-only and excluded from the comparison.
- `openspec/changes/` active directory no longer contains this change (only `archive/`).
