# Archive Report — MCP Workspace Tools

**Status**: ARCHIVED
**Change**: mcp-workspaces
**Merged**: PR #50 (slice) + PR #51 (tracker) → main (commit 93b9199)
**Date**: 2026-08-10

## Executive Summary

Closed the last framework MCP gap: workspace operations exposed over MCP. `workspace_list`, `workspace_upgrade`, `workspace_check` + `service`/`workspacePath` params on `upgrade_project`. The upgrade path is **chdir-free** via `scaffold.WithRoot` injection — the key design decision that avoids the os.Chdir hazard in the long-running MCP server. Tool count: 11 → 14. All additions optional → zero backward-compat risk.

## What Ships

- **`workspace_list`** — services in a workspace (name, path, template); empty → `[]`.
- **`workspace_upgrade`** — batch or single service; dry-run default / `apply: true` commits; chdir-free (WithRoot); structured per-service result (status, upgradable/protected/absent, files_changed); legacy no-manifest → `skipped` (deliberate spec divergence from CLI: non-fatal, batch continues); continue-on-error; top-level ok/partial/failed.
- **`workspace_check`** — architecture check per service (chdir+defer, the proven single-threaded-safe pattern); no-manifest → per-service `error` entry.
- **`upgrade_project` extension** — `service`+`workspacePath` → chdir-free single-service upgrade; no params → byte-identical backward compat (regression test).
- **Empty-workspace refinement** — `workspace.Load` accepts empty services list (verified no CLI breakage); workspace_list → `[]`, upgrade/check no-op.
- **Inline resolver** in mcp (`resolveMCWorkspace`/`findMCService`) — avoids the cmd→mcp import cycle; viper isolated per request (`Reset()` at handler entry).
- **Docs**: COMMANDS.md MCP table (11→14), workspaces.md out-of-scope note → done, ROADMAP follow-up.

## Verification Summary

- Full suite green (`go test ./...`, 9 packages), vet + gofmt clean.
- 9 integration tests via `handleToolCall` on temp monorepos: list (found/not-found), upgrade (batch dry-run, single, service-not-found, legacy-skipped), check (all/single), upgrade_project (with service, no-params backward compat).
- Live smoke on the real MCP server: 14 tools registered; `workspace_list` → 2 services; `workspace_upgrade` → structured per-service result.
- SDD: explore → propose (8 decisions) → spec (12 req / 32 scenarios) → design (fresh-context validator: 10 fixes — legacy-skipped mechanism, shared helpers upgradeMCService/checkMCService, ROADMAP, nil→[] normalization) → tasks (16) → apply → verify/smoke.

## Follow-Ups (non-blocking)

- `service` on `generate_component`/`check_architecture` — deferred (require chdir; not justified yet; would need root-param refactor of validator/GenerateComponent).
- `StartServer` panic `recover()` hardening — separate change (orthogonal).
- `files_changed` in check results (upgrade has it; check shape has violations count only).

## Artifacts

| Artifact | Path |
|----------|------|
| Proposal | `openspec/changes/archive/2026-08-10-mcp-workspaces/proposal.md` |
| Exploration | `openspec/changes/archive/2026-08-10-mcp-workspaces/exploration.md` |
| Design | `openspec/changes/archive/2026-08-10-mcp-workspaces/design.md` |
| Tasks | `openspec/changes/archive/2026-08-10-mcp-workspaces/tasks.md` |
| Spec (delta) | `openspec/changes/archive/2026-08-10-mcp-workspaces/specs/mcp-workspaces/spec.md` |
| Spec (synced) | `openspec/specs/mcp-workspaces/spec.md` (12 requirements, byte-identical) |

## Delivery Note

Receipt-driven review disabled at clone scope (user decision after escalating upstream #2743). Delivery under ordinary policy — CI gates (test/lint) are the authority. No review receipt exists; none fabricated.
