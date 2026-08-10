# Tasks: Expose Workspace Features over MCP

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~650–750 total (S1 ~300–350, S2 ~350–400) |
| 400-line budget risk | Medium (total >400; each slice <400) |
| Chained PRs recommended | Yes |
| Suggested split | PR 1 → PR 2 (feature-branch-chain) |
| Delivery strategy | auto-chain |
| Chain strategy | feature-branch-chain |

Decision needed before apply: No
Chained PRs recommended: Yes
Chain strategy: feature-branch-chain
400-line budget risk: Medium

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|------|------|-----------|----------------------|-----------------|-------------------|
| S1 | Loader empty-services relaxation + inline resolver + shared upgrade/check helpers + tool schemas | PR 1 (base = tracker `feat/mcp-workspaces`) | `go test ./internal/pkg/mcp/ ./internal/pkg/workspace/ -count=1` | `handleToolCall` dispatch on `t.TempDir()` monorepo (same-package harness, real handler path) | Revert loader.go change + server.go helpers/schemas; no handler wiring depends |
| S2 | 3 handlers + `upgrade_project` workspace wiring + integration tests + docs | PR 2 (base = PR 1 branch) | `go test ./... -count=1` | `handleToolCall` on temp monorepo; `go run . mcp` stdio smoke | Revert handler cases + docs; S1 schemas are inert without cases |

## Slice 1 — Foundation (PR 1 → tracker `feat/mcp-workspaces`, ~300–350 lines)

- [ ] 1.1 Modify `internal/pkg/workspace/loader.go`: delete empty-services rejection (loader.go:48-53); rewrite `TestLoad_NoServices` (loader_test.go:165-176) to expect success + empty `Services` slice
- [ ] 1.2 Add `resolveMCWorkspace(flagPath)` (Load wins over Discover) + `findMCService(ws, name)` + `toolResultError(code, msg)` to `server.go`; table-driven tests: explicit path, discovery, `workspace_not_found`, `workspace_invalid`, `service_not_found`
- [ ] 1.3 Add `upgradeMCService(ws, svc, apply)` to `server.go`: viper.Reset + `SetConfigFile(absPath/.go-arch.yaml)` + `ReadInConfig`; no-manifest → `skipped` + `service_no_manifest` warning; `scaffold.Upgrade(cfg, WithRoot(absPath), WithResolver(DefaultResolver{}))`; apply → `plan.Apply` + `WriteVersionField` + `files_changed`; tests: legacy-skip, `service_path_missing`, dry-run plan, apply commit
- [ ] 1.4 Add `checkMCService(ws, svc)` to `server.go`: chdir+defer (server.go:722-731 pattern), viper.Reset + `ReadInConfig`, no-manifest → `failed` + `service_no_manifest`; `validator.NewValidator(cfg).Validate()`; tests: clean/violations, no-manifest failed entry
- [ ] 1.5 Register 3 new schemas in `tools/list` (workspace_list/upgrade/check) + extend `upgrade_project` schema with optional `service` + `workspacePath`; test: `tools/list` returns 14 tools
- [ ] 1.6 Verify slice 1: `go test ./internal/pkg/mcp/ ./internal/pkg/workspace/ -count=1` + `go vet ./...` + gofmt green

## Slice 2 — Handlers, Integration Tests, Docs (PR 2 → `feat/mcp-workspaces-1`, ~350–400 lines)

- [ ] 2.1 Add `case "workspace_list":` handler: `resolveMCWorkspace` → enumerate `{name, path, template}` in declaration order → nil→`[]` → `sendToolResult`; no mutation
- [ ] 2.2 Tests via `handleToolCall`: explicit path (orders+users order), discovery from subdir, `workspace_not_found`, `workspace_invalid` naming field, empty workspace → `[]`
- [ ] 2.3 Add `case "workspace_upgrade":` handler: batch-all or `findMCService` single filter, loop `upgradeMCService`, top-level status ok/partial/failed, nil→`[]`
- [ ] 2.4 Tests: batch dry-run plans + no files mutated; apply commits + version field per service; single-service filter; one-fails→`partial` others continue; `service_path_missing`; legacy-skip batch continues; all-skipped→`ok`; empty→`ok`+`[]`
- [ ] 2.5 Add `case "workspace_check":` handler: loop `checkMCService`, continue-on-error, top-level status, nil→`[]`
- [ ] 2.6 Tests: all processed, mixed violations→`partial`, single filter, no-manifest failed entry, empty→`ok`+`[]`
- [ ] 2.7 Extend `case "upgrade_project":` to dispatch to `upgradeMCService` when `service`/`workspacePath` provided (chdir-free); no params → byte-identical existing path
- [ ] 2.8 Tests: service-scoped upgrade touches only named service, monorepo root untouched; no-params backward-compat (existing `TestUpgradeProjectDryRun/ApplyTrue` regression)
- [ ] 2.9 Docs: `docs/COMMANDS.md` MCP table +3 tools + `upgrade_project` params; `docs/workspaces.md` remove line 104 note + add MCP tools section (chdir constraint for workspace_check); `ROADMAP.md` follow-ups (service on generate/check deferred; StartServer recover() hardening)
- [ ] 2.10 Verify slice 2: `go test ./... -count=1` + `go vet ./...` + gofmt green

## Commit Plan (conventional)

### Slice 1
- `feat(workspace): allow empty service lists in workspace loader` (1.1)
- `feat(mcp): add inline workspace resolver helpers` (1.2)
- `feat(mcp): add shared workspace upgrade/check service helpers` (1.3–1.4)
- `feat(mcp): register workspace tool schemas and upgrade_project workspace params` (1.5)

### Slice 2
- `feat(mcp): add workspace_list handler` (2.1–2.2)
- `feat(mcp): add workspace_upgrade handler` (2.3–2.4)
- `feat(mcp): add workspace_check handler` (2.5–2.6)
- `feat(mcp): wire service and workspacePath into upgrade_project` (2.7–2.8)
- `docs: document MCP workspace tools and upgrade_project workspace params` (2.9)
- `chore(roadmap): track deferred workspace MCP follow-ups` (2.9)
