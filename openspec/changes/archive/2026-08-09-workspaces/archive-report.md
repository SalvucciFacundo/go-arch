# Archive Report — Workspaces (Multi-Project Support)

**Status**: ARCHIVED
**Change**: workspaces
**Merged**: PR #49 → main (commit 8f2307b, + lint fix 4649d30)
**Date**: 2026-08-09

## Executive Summary

Closed the workspaces change — multi-project (monorepo) support. A `go-arch.workspace.yaml` at the repository root maps service names to paths; `workspace upgrade`/`check` operate across the set, and `--service <name>` targets a single service from anywhere in the monorepo. Single-project behavior is byte-identical — every workspace feature is opt-in (ADR-7 untouched). Verify: full suite green (9 packages), live smoke exercised all command paths.

## What Ships

- **`internal/pkg/workspace/`**: strict workspace file loader (`yaml.v3 KnownFields(true)` — unknown keys rejected), upward discovery (`--workspace` flag wins), error taxonomy (`workspace_not_found`, `workspace_invalid`, `service_not_found`, `service_path_missing`, `service_duplicate`, `service_no_manifest`).
- **`workspace upgrade`**: full standard upgrade logic per service — chdir, service config reload, `Upgrade` with `WithResolver` + `WithRoot`, plan display, `--yes` → `plan.Apply()` + surgical `WriteVersionField`. Dry-run by default; continue-on-error with per-service summary; non-zero exit if any failed.
- **`workspace check`**: architecture check across all services, same continue-on-error semantics.
- **`--service <name>`** on `generate`/`check`/`upgrade`: chdir + config reload + restore CWD/viper; unknown service → `service_not_found`; no workspace → instructive error.
- **`Upgrade(cfg, WithRoot(root))`**: backward-compatible variadic option (ADR-7 default "." preserved).
- **Hooks CWD**: hooks run inside the service directory (integration test: marker lands in the service, `PROJECT_PATH` points there).
- **Docs**: `docs/workspaces.md`, COMMANDS.md (new section), README.

## Verification Summary

- Full suite green: `go test ./...` (9 packages), `go vet ./...` clean, `gofmt -l .` clean.
- Integration tests (production paths): workspace upgrade dry-run/apply, workspace check, `--service` lands files in the service + CWD restore, unknown service, no-workspace error, hooks CWD marker, config isolation (service config used, restored after).
- Live smoke: `workspace upgrade` processed both services + summary (exit 0); `generate service X --service orders` wrote `Order_service.go` inside the service and restored CWD; `--service billing` → `service_not_found` exit 1; `workspace check` reported per-service violations + non-zero exit.
- Post-merge lint fix: `4649d30` removed unused `resolveServicePath`/`dirExists` helpers (golangci-lint `unused`).

## Process Notes

- The sdd-* sub-agent channel failed repeatedly under context pressure (`sdd_task_result_empty`: 2× verify, 1× archive, 1× explore, 1× propose). With user authorization, the orchestrator executed phases directly: exploration, proposal, spec, design (corrected after fresh-context validation — 2 HIGH fixes: `plan.Apply()` missing + `WithResolver` omitted), tasks, apply (5 slices), and archive.
- The fresh-context validator (general agent) worked throughout — only the sdd-* channel failed.
- Design correction highlights: workspace upgrade must run the FULL standard logic (plan + Apply + WriteVersionField + WithResolver); viper snapshot/restore contract specified; `service_duplicate` vs `workspace_invalid` resolved; `KnownFields(true)` strict loader.

## Follow-Ups (non-blocking)

- MCP workspace tools (chdir precedent exists; future version may add them).
- `workspace new` (add service to workspace file).
- Cross-service template sharing.
- Nested workspaces, concurrent service ops — explicitly out of scope for v1.

## Artifacts

| Artifact | Path |
|----------|------|
| Proposal | `openspec/changes/archive/2026-08-09-workspaces/proposal.md` |
| Exploration | `openspec/changes/archive/2026-08-09-workspaces/exploration.md` |
| Design | `openspec/changes/archive/2026-08-09-workspaces/design.md` |
| Tasks | `openspec/changes/archive/2026-08-09-workspaces/tasks.md` |
| Spec (delta) | `openspec/changes/archive/2026-08-09-workspaces/specs/workspaces/spec.md` |
| Spec (synced) | `openspec/specs/workspaces/spec.md` (11 requirements, byte-identical) |

## Delivery Note

Receipt-driven review disabled at clone scope (user decision after escalating upstream #2743). Delivery under ordinary policy — CI gates (test/lint) are the authority. No review receipt exists; none fabricated.
