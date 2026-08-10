# Proposal: Expose Workspace Features over MCP

**Status**: proposed
**Change**: `mcp-workspaces`
**Risk Level**: Medium (chdir precedent reused; upgrade path is chdir-free)

## Executive Summary

Expose workspace operations to MCP agents through three new tools (`workspace_list`, `workspace_upgrade`, `workspace_check`) and optional `service`+`workspacePath` parameters on `upgrade_project`. `workspace_upgrade` is fully chdir-free via the existing `WithRoot` injection on `scaffold.Upgrade`; `workspace_check` reuses the proven chdir+defer pattern already used by all five project-scoped handlers. Tool count grows 11 → 14.

## Intent

Agents working inside monorepos currently have no way to discover, upgrade, or check the services defined in `go-arch.workspace.yaml` without dropping to a shell. This change makes workspace operations first-class MCP citizens so agents can orchestrate multi-service upgrades and architecture checks with the same structured JSON contracts as single-project tools.

## Scope

### In Scope
- New tool `workspace_list(workspacePath?)` — pure read, returns services array.
- New tool `workspace_upgrade(workspacePath?, service?, apply?)` — chdir-free via `WithRoot(absServicePath)`. Batch-all by default; optional `service` filters to one. Returns structured per-service JSON `{name, status, plan|error, files_changed}`. Dry-run default (matches `upgrade_project`).
- New tool `workspace_check(workspacePath?, service?)` — chdir+defer per service (validator is CWD-bound). Returns per-service JSON `{name, status, violations|error}`. Continue-on-error like the CLI.
- `upgrade_project` gains optional `service`+`workspacePath` (chdir-free, `WithRoot` to resolved service path).
- Inline thin workspace resolver in `mcp` (~15 lines: `Load`/`Discover`/`Find`/`ResolvePath` + stat check) — cannot import `cmd` helpers (cycle).
- Tool-result error strings reuse CLI codes: `workspace_not_found`, `service_not_found`, `service_path_missing`.
- Tests following existing chdir-into-tempdir convention; temp monorepo fixture with workspace file + 1-2 services.

### Out of Scope
- `service`+`workspacePath` on `generate_component` / `check_architecture` — deferred (chdir-required, higher risk; land standalone workspace tools first).
- `StartServer` `recover()` panic hardening — separate change (orthogonal to workspaces).
- Refactor `validator`/`manifestDir`/hooks to be root-injectable (large; unrelated).
- `go-arch new` workspace-aware creation (documented out-of-scope in `docs/workspaces.md`).
- Nested workspaces (already out-of-scope for CLI).
- Docs changes beyond `docs/COMMANDS.md` MCP table + removal of `docs/workspaces.md:104` out-of-scope note.

## Capabilities

### New Capabilities
None.

### Modified Capabilities
- `workspaces`: add MCP tool surface — `workspace_list`, `workspace_upgrade`, `workspace_check` requirements and scenarios; optional `service`+`workspacePath` parameters on `upgrade_project`.

## Approach

**Tool surface — Option C (hybrid)**. Standalone workspace tools for batch operations, `service`+`workspacePath` on `upgrade_project` for single-service parity with the standalone tool. `service` on generate/check deferred to a future change after the workspace pattern is established.

**workspace resolution UX**: optional `workspacePath` parameter on every workspace tool. When omitted, fall back to `workspace.Discover(os.Getwd())`. Explicit flag wins over discovery (mirrors CLI `--workspace` semantics). MCP CWD = agent's launch directory; discovery handles subdir launches; explicit path is the escape hatch when the agent is outside the monorepo.

**chdir policy**:
- `workspace_list`: no chdir (pure Load + enumerate).
- `workspace_upgrade`: **no chdir**. Call `scaffold.Upgrade(cfg, WithRoot(resolvedServicePath), WithResolver(DefaultResolver{}))`. Read service config via `viper.Reset()` + `SetConfigFile(filepath.Join(resolvedServicePath, ".go-arch.yaml"))` + `ReadInConfig()` — no chdir needed because `Upgrade` is fully root-injectable.
- `workspace_check`: **chdir+defer per service** (reuses the proven byte-for-byte pattern from `generate_component`, `check_architecture`, etc.). Documented constraint: handlers stay sequential; never `os.Exit` in a handler.
- `upgrade_project` with `service`: **no chdir** (same `WithRoot` path as `workspace_upgrade`).

**batch vs single**: `workspace_upgrade` batches all services by default (matches CLI `workspace upgrade`); optional `service` parameter selects a single service for targeted use. Result is always a JSON array of per-service outcomes (`{name, status: success|failed|skipped, plan|error, files_changed}`). Single-service `service` filter fails fast if the name is unknown.

**workspace_list as tool**: kept as a standalone tool. Agents need a cheap read to discover what services exist before calling upgrade/check. Inline listing inside upgrade/check responses would force agents to call a mutating tool just to enumerate.

**error codes**: reuse `workspace_not_found`, `service_not_found`, `service_path_missing` as tool-result error strings (precedent from `docs/workspaces.md:113-115`). Tool results carry errors as structured `{error: {code, message}}` in the JSON result body, not JSON-RPC errors (tool ran; business outcome was failure).

**viper isolation**: every workspace handler calls `viper.Reset()` at entry (matching existing project handlers). For `workspace_upgrade`'s per-service config read, use `SetConfigFile(filepath.Join(resolvedServicePath, ".go-arch.yaml"))` + `ReadInConfig()` — no chdir, no `loadServiceConfig` import (cycle).

**import cycle**: inline thin resolver in `internal/pkg/mcp/server.go` (~15 lines): `resolveWorkspace(path)` returns `*workspace.Workspace` or typed error; `resolveService(w, name)` returns absolute service path or typed error. Mirrors `cmd.resolveWorkspace`/`chdirService` without the chdir.

**StartServer recover()**: separate change. Document the constraint in this change's docs; do not conflate hardening with feature work.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/pkg/mcp/server.go` | Modified | Add `workspace_list`/`workspace_upgrade`/`workspace_check` to `tools/list` + `handleToolCall` cases; add optional `service`+`workspacePath` to `upgrade_project` schema + handler; add inline workspace resolver (~15 lines). |
| `internal/pkg/mcp/server_test.go` | Modified | New tests: workspace file discovery, service resolution, upgrade dry-run/apply per-service results, check continue-on-error, error codes. Follow existing chdir-into-tempdir convention (server_test.go:16-100). |
| `internal/pkg/scaffold/upgrade_opts.go` | Verified | Confirm `WithRoot` + `Apply` + `WriteVersionField` usage paths; no change expected. |
| `internal/pkg/workspace/` | Read-only | Used via `Load`, `Discover`, `Find`, `ResolvePath`. No changes. |
| `docs/COMMANDS.md` | Modified | MCP tools table (line 161-174): add the three new workspace tools + note `service`/`workspacePath` params on `upgrade_project`. |
| `docs/workspaces.md` | Modified | Remove line 104 ("MCP workspace tools ... out-of-scope") and add a short section documenting the MCP tools and the chdir constraint for `workspace_check`. |
| `ROADMAP.md` | Modified | Follow-up entry: `service` on generate_component/check_architecture deferred to a later change; StartServer `recover()` hardening as a separate item. |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| chdir in `workspace_check` corrupts CWD on restore failure | Low (same as existing handlers) | Reuse byte-for-byte proven pattern; document sequential-only constraint; restore failure swallowed (`_ = os.Chdir`) matches existing handlers; `StartServer` `recover()` is a separate hardening change. |
| Import cycle mcp↔cmd | Prevented | Inline thin resolver; do not import `cmd` from `mcp`. |
| Workspace discovery returns unrelated workspace | Low | `workspacePath` parameter is the escape hatch; explicit wins over discovery (CLI parity). |
| Viper cross-request contamination | None | `viper.Reset()` at handler entry; `SetConfigFile` scoped to resolved path. |
| Tool surface bloat 11 → 14 | Medium | Document in ROADMAP; keep `service` on generate/check deferred until pattern is stable. |
| Batch upgrade with one service failing hides per-service detail | Medium | Structured JSON result with per-service outcomes; top-level `status: partial` when any fail (matches CLI exit-code semantics). |

## Rollback Plan

Revert the PR. No schema migration, no data changes — purely additive tool registrations and handler cases. Existing tools are unchanged when new optional parameters are absent, so revert has no compatibility impact. Tests are additive; revert deletes them.

## Dependencies

- `internal/pkg/workspace` (Load, Discover, Find, ResolvePath) — stable, archived feature.
- `scaffold.Upgrade` with `WithRoot`, `WithResolver` — stable, verified chdir-free.
- Existing chdir+defer pattern in `server.go` — reused, not refactored.

## Success Criteria

- [ ] `workspace_list` returns services array for a valid workspace file; returns `workspace_not_found` when discovery fails.
- [ ] `workspace_upgrade` (dry-run) returns per-service plan JSON without mutating any service files; `apply: true` commits per-service and calls `WriteVersionField` at each resolved root.
- [ ] `workspace_upgrade` with unknown `service` returns `service_not_found`; with missing service path returns `service_path_missing`.
- [ ] `workspace_check` returns per-service violations JSON; continue-on-error behavior matches CLI (processes all services; top-level status reflects any failure).
- [ ] `upgrade_project` with `service`+`workspacePath` upgrades only the named service at the resolved root — no chdir.
- [ ] Tests pass with `go test ./...`; no new imports cross the `mcp ↔ cmd` boundary.
- [ ] `docs/COMMANDS.md` and `docs/workspaces.md` reflect the new tools; `docs/workspaces.md:104` out-of-scope note removed.

## Edge Cases

- Workspace file not found by flag or discovery → `workspace_not_found` error in tool result.
- Named service not in workspace → `service_not_found`.
- Service path does not exist on disk → `service_path_missing`.
- Service directory has no `.go-arch.yaml` (legacy project) → treat as no-op with warning in result; do not fail the batch.
- Workspace with zero services → `workspace_list` returns `[]`; `workspace_upgrade`/`workspace_check` return empty result with `status: ok`.
- `apply: true` without dry-run on batch → applies all services sequentially, continues on per-service failure, final status reflects partial success.
- Concurrent requests → documented as sequential-only (matches existing chdir handlers); no concurrency added in this change.
- `workspacePath` points to a regular file, not a workspace → `workspace_invalid` from loader.
- `WriteVersionField` on a service without `.go-arch.yaml` version field → skip version bump, report in result.

## Open Questions

None. All exploration open questions are resolved by the decisions above.

## Next Recommended

`spec` (sdd-spec): write the delta spec modifying the `workspaces` capability with MCP tool requirements, scenarios, and the `upgrade_project` parameter extension.
