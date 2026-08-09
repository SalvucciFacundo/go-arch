# Archive Report — upgrade-project

**Status**: ARCHIVED
**Change**: upgrade-project
**Archived on**: 2026-08-09
**Branch**: `feat/upgrade-project-4` (4 slices, 9 commits)
**Archived to**: `openspec/changes/archive/2026-08-09-upgrade-project/`
**Review authority**: Review (`receipt-driven development`) is disabled at clone scope for this repository (user decision after escalating upstream #2743). Delivery under ordinary policy — no receipt required, nothing silently approved.

## Final-State Facts (at close)

- **Tasks**: 19/19 complete across 4 slices (Phases 1–5). No unchecked implementation tasks in the archived `tasks.md`.
- **Verify verdict**: **PASS 29/29** — base `upgrade-project` spec 21/21 + `cli` delta 8/8, after the `--project-path` viper re-read fix (commit `576317c`, "fix(cli): re-read viper config after --project-path chdir"). The `verify-report.md` intermediate snapshot recorded FAIL 28/29; the failing scenario (`--project-path overrides root`) was fixed in a later commit, so the final count supersedes that snapshot.
- **Build/tests**: `go test ./...` green, `go vet` clean, `gofmt` clean, `golangci-lint v1.64.8 run ./...` exit 0 (per verify report; static gates remained green).
- **Non-blocking WARNINGS recorded at close**:
  1. Design §8 specifies MCP `new_project` calls `WriteVersionField` after scaffolding; the `server.go` handler does not, so MCP-scaffolded projects keep `go_arch_version:` empty until first upgrade apply (`go_arch_version` is populated post-hoc only on `new` via `cmd/new.go`, not via MCP). Spec R4 tolerates absence, so spec compliance holds; design intent unmet.
  2. Pre-existing MCP stdout pollution: `initConfig`'s "Using config file" notice prints to stdout for all MCP tools (not introduced by this change).

## Specs Synced

| Domain | Action | Details |
|--------|--------|---------|
| `cli` | Updated | 3 ADDED requirements merged into `openspec/specs/cli/spec.md` (Upgrade Subcommand Registered Under RootCmd, Upgrade Command Flags, Upgrade Missing Config Error). Also added `upgrade` to the Commands list. |
| `upgrade-project` | Verified (no copy) | Capability spec `openspec/specs/upgrade-project/spec.md` already in place; verified, not copied. |

## Artifacts

- `openspec/changes/archive/2026-08-09-upgrade-project/exploration.md`
- `openspec/changes/archive/2026-08-09-upgrade-project/proposal.md`
- `openspec/changes/archive/2026-08-09-upgrade-project/specs/cli/spec.md` (delta)
- `openspec/changes/archive/2026-08-09-upgrade-project/design.md`
- `openspec/changes/archive/2026-08-09-upgrade-project/tasks.md` (19/19 complete; marked ARCHIVED)
- `openspec/changes/archive/2026-08-09-upgrade-project/verify-report.md`
- `openspec/changes/archive/2026-08-09-upgrade-project/archive-report.md` (this file, additive)

Source of truth updated: `openspec/specs/cli/spec.md`; capability spec `openspec/specs/upgrade-project/spec.md` (already in place).

## Engram Persistence

Archive report persisted to Engram with `topic_key: sdd/upgrade-project/archive-report`, project `go-arch-cli`.

## Rollback Note

The archived change folder is an immutable audit trail — do not modify it. Rollback of the delivered behavior (if ever needed) is handled at the delivery layer: the 4 chained PRs are sequenced on the feature-branch chain and can be reverted independently.

## Next Steps (delivery)

Deliver the archived work via 4 chained PRs (feature-branch-chain): PR #1 targets the tracker branch, later PRs target the immediate previous PR branch. Sequence per the tasks forecast work units:
1. Manifest + RenderTo + scaffold seam
2. upgrade.go core + tests
3. Legacy + CLI + tests
4. MCP + version wiring

Then the feature branch chain merges to tracker → main.

## Mechanical Copy Verification

The archive move was performed with `git mv`/`mv` (mechanical shell). `diff -r` of the pre-move snapshot vs the archived tree produced **empty output (PASS)** — byte-identity confirmed. The archive-report is additive-only and excluded from the comparison.
