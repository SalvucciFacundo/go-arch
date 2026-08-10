# Design: Expose Workspace Features over MCP

## Technical Approach

Map proposal approach (Option C hybrid) onto `handleToolCall`'s switch-case pattern. Each new tool registers a schema in the `tools/list` literal (server.go:124-336) and adds a `case "workspace_*":` branch with a local args struct + `json.Unmarshal`. Existing tools keep byte-identical schemas. Tool count: 11 → 14.

## Architecture Decisions

### Decision: Inline workspace resolver in mcp

| Option | Tradeoff | Decision |
|--------|----------|----------|
| Import `cmd.resolveWorkspace` / `cmd.chdirService` | Import cycle (cmd→mcp via cmd/mcp.go) | ❌ Rejected |
| Inline thin resolver (~15 lines) in `server.go` | ~15 LOC duplicated; no cycle | ✅ Chosen |
| New shared package `internal/pkg/wshelpers` | New package, more surface, same logic | ❌ Overkill |

**Naming deviation from spec**: the spec's `resolveWorkspace(path)` / `resolveService(w, name)` contract is fulfilled by `resolveMCWorkspace(flagPath string) (*workspace.Workspace, error)` and `findMCService(ws, name) (*workspace.Service, error)` respectively. The suffix `MC` prevents collisions with `cmd`-level names during development. `findMCService` returns `*workspace.Service`; callers invoke `ws.ResolvePath(svc)` + `os.Stat` inside — preserving the spec's "returns absolute path" contract at the call site. `resolveMCWorkspace` uses `workspace.Load` / `workspace.Discover(os.Getwd())` internally.

### Decision: chdir policy per handler

| Handler | chdir? | Mechanism |
|---------|--------|-----------|
| `workspace_list` | no | pure `workspace.Load` + enumerate |
| `workspace_upgrade` | no | `scaffold.Upgrade(cfg, WithRoot(absPath), WithResolver(DefaultResolver{}))`; per-service viper via `SetConfigFile(filepath.Join(absPath, ".go-arch.yaml"))` |
| `workspace_check` | **yes, per service** | validator is CWD-bound (`filepath.Walk("internal")` — validator.go:78); byte-for-byte chdir+defer pattern from server.go:722-731 |
| `upgrade_project` with `service` | no | same `WithRoot` path as `workspace_upgrade` single-service |

### Decision: Shared per-service helpers

Both `workspace_upgrade(service:)` and `upgrade_project(service:+workspacePath:)` require identical per-service upgrade logic. To avoid duplication, the design defines two shared helpers in `server.go`:

```go
// upgradeMCService upgrades one service (chdir-free).
// Returns per-service result map with keys: name, status, plan|error, files_changed.
// On missing manifest (legacy): returns {status:"skipped", error:{code:"service_no_manifest",...}} — batch continues.
func upgradeMCService(ws *workspace.Workspace, svc *workspace.Service, apply bool) (map[string]any, error)

// checkMCService runs the architecture check for one service (chdir+defer).
// Returns per-service result map with keys: name, status, violations|error.
// On missing manifest: returns {status:"failed", error:{code:"service_no_manifest",...}} — batch continues.
func checkMCService(ws *workspace.Workspace, svc *workspace.Service) (map[string]any, error)
```

Both handlers iterate services and call the appropriate helper. The helper owns the viper isolation, config build, error mapping, and (for upgrade) plan/apply + `WriteVersionField`.

### Decision: Structured per-service result shape

```json
{
  "status": "ok" | "partial" | "failed",
  "services": [
    {"name": "...", "status": "success"|"failed"|"skipped",
     "plan": [...] | "error": {"code":"...","message":"..."},
     "files_changed": <int>}
  ]
}
```

**Per-service status taxonomy**:
- `success`: upgrade/check completed.
- `failed`: error during upgrade/check.
- `skipped`: legacy service with no manifest (upgrade only). The CLI FAILS here (`upgradeOneService` returns `service_no_manifest` → non-zero exit); the spec DELIBERATELY diverges — the MCP tool treats this as a non-fatal skip so the batch continues. `workspace-no-manifest` spec requires this.

**Top-level status rules**:
- All success (or all skipped, since skipped is non-failure) → `ok`
- Any fail + any success/skipped → `partial`
- All fail → `failed`

**`files_changed`**: populated on success (count of applied file writes; 0 for dry-run). Proposal shape included this; retained for parity.

### Decision: Viper isolation + no-manifest branches

Every workspace handler entry: `viper.Reset()`. For `workspace_upgrade`'s per-service config read:
```
viper.Reset()
viper.SetConfigFile(filepath.Join(absServicePath, ".go-arch.yaml"))
err := viper.ReadInConfig()  // best-effort; missing manifest = legacy path
if viper.GetString("project_name") == "" {
    // DELIBERATE CLI DIVERGENCE: CLI upgradeOneService (workspace_upgrade.go:62-68)
    // returns service_no_manifest → batch eventually exits non-zero.
    // Spec workspace-no-manifest REQUIRES status:"skipped" + warning, batch continues.
    return entry{status:"skipped", error:{code:"service_no_manifest", message:"..."}}
}
```

For `workspace_check` after chdir+ReadInConfig: mirror `cmd/check.go:58-63` and `cmd/workspace.go:148-152`:
```
if viper.GetString("project_name") == "" {
    return entry{status:"failed", error:{code:"service_no_manifest", message:"..."}}
}
```

Without this guard, empty `Architecture` → `validator.NewValidator(cfg)` computes `requiredDirs = nil` → zero violations → silent ok for a non-project directory. The CLI guards this; the MCP tool must mirror it.

### Decision: dryRun parameter reconciliation

The modified spec lists `dryRun (default true)` as a parameter alongside `apply (default false)`. The current `upgrade_project` schema only has `apply` (bool, default false). **Resolution**: `dryRun` is the existing implicit behavior (`apply: false`); no new schema parameter is introduced. The description string will clarify: "Dry-run by default — set apply: true to commit." This preserves backward compatibility.

### Decision: Empty workspace handling — REFINEMENT ⚑

The spec requires empty workspace → `[]` / `status: ok`. Current `workspace.Load` rejects empty services (loader.go:48-53 returns `workspace_invalid`). Design requires a one-line change to remove that rejection. CLI impact is benign: `range ws.Services` no-ops; `findService` still returns `service_not_found`.

### Decision: nil → `[]` normalization

Go marshals `nil []T` to `null`, not `[]`. The `list_packs` handler already solves this (server.go:972-973: `if out == nil { out = []PackInfo{} }`). The same normalization is applied to:
- `workspace_list`: `services` array
- `workspace_upgrade`: `services` array
- `workspace_check`: `services` array
- Per-service `plan` / `violations` arrays (when empty)

### Decision: Error taxonomy

Business errors flow inside the tool-result `content` JSON body — NOT as JSON-RPC `-326xx` errors. Shape: `{"error": {"code": "...", "message": "..."}}`. Codes reused from `workspace/errors.go`: `workspace_not_found`, `workspace_invalid`, `service_not_found`, `service_path_missing`, `service_no_manifest`. Extract code via `oops.OopsError.Code()` (see `formatMCGeneratorError` precedent at server.go:1115-1126).

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/pkg/workspace/loader.go` | Modify | Remove `len(raw.Services) == 0` rejection (loader.go:48-53). |
| `internal/pkg/workspace/loader_test.go` | Modify | Update `TestLoad_NoServices` to expect success + empty `Services` slice. |
| `internal/pkg/mcp/server.go` | Modify | Add 3 new schemas to `tools/list`; extend `upgrade_project` schema with `service`+`workspacePath`; add 3 new `case "workspace_*":` branches + extend `case "upgrade_project":` with workspace dispatch; add `resolveMCWorkspace` + `findMCService` helpers (~25 lines); add shared `upgradeMCService` / `checkMCService` helpers; add `toolResultError(code, msg)` JSON helper. Apply nil→`[]` normalization on all result arrays. |
| `internal/pkg/mcp/server_test.go` | Modify | Tests: workspace discovery happy/error, service resolution, upgrade dry-run/apply, legacy-skip (no-manifest → skipped with warning, batch continues), check continue-on-error + no-manifest failed entry, error codes, `upgrade_project` with service, backward-compat, empty workspace. |
| `docs/COMMANDS.md` | Modify | MCP tools table: add 3 new workspace tools + document `service`/`workspacePath` on `upgrade_project`. |
| `docs/workspaces.md` | Modify | Remove line 104 out-of-scope note; add short section on MCP workspace tools + chdir constraint for `workspace_check`. |
| `ROADMAP.md` | Modify | Follow-up entry: `service` on generate_component/check_architecture deferred to later change; StartServer `recover()` hardening as separate item. |

## Sequence Diagrams

```
workspace_upgrade (chdir-free):
  client → server.tools/call(workspace_upgrade)
         → handleToolCall → resolveMCWorkspace(flagPath)
            ├─ flagPath set   → workspace.Load(flagPath)
            └─ else           → workspace.Discover(os.Getwd()) → Load
         → for each service (or single via Find):
              absPath := ws.ResolvePath(svc); os.Stat(absPath)
              → upgradeMCService(ws, svc, apply):
                  viper.Reset(); viper.SetConfigFile(absPath/.go-arch.yaml); ReadInConfig()
                  if viper.GetString("project_name") == "":
                      → entry{status:"skipped", error:{code:"service_no_manifest"}}; continue
                  cfg := build from viper
                  plan, err := scaffold.Upgrade(cfg, WithRoot(absPath), WithResolver(DefaultResolver{}))
                  if apply && err==nil: plan.Apply(); WriteVersionField(); files_changed := plan.CountBy(Upgradable)
                  append per-service result{name, status, plan|error, files_changed}
         → normalize nil → []; sendToolResult JSON

workspace_check (chdir per service):
  client → server.tools/call(workspace_check)
         → handleToolCall → resolveMCWorkspace
         → for each service:
              → checkMCService(ws, svc):
                  oldWd, _ := os.Getwd(); os.Chdir(ws.ResolvePath(svc))
                  defer func() { _ = os.Chdir(oldWd) }()
                  viper.Reset(); ReadInConfig() from "."
                  if viper.GetString("project_name") == "":
                      → entry{status:"failed", error:{code:"service_no_manifest"}}; continue
                  cfg := from viper; validator.NewValidator(cfg).Validate()
                  append per-service {name, status, violations|error}
         → normalize nil → []; sendToolResult JSON
```

## Interfaces / Contracts

```go
// Resolver (fulfills spec's resolveWorkspace/resolveService contract):
func resolveMCWorkspace(flagPath string) (*workspace.Workspace, error)
func findMCService(ws *workspace.Workspace, name string) (*workspace.Service, error)
// Callers invoke ws.ResolvePath(svc) + os.Stat for the absolute path.

// Shared per-service helpers:
func upgradeMCService(ws *workspace.Workspace, svc *workspace.Service, apply bool) (map[string]any, error)
// → {name, status:"success"|"failed"|"skipped", plan|error, files_changed}

func checkMCService(ws *workspace.Workspace, svc *workspace.Service) (map[string]any, error)
// → {name, status:"ok"|"failed", violations|error}

// JSON helper:
func toolResultError(code, message string) string
```

## Testing Strategy

| Layer | What | Approach |
|-------|------|----------|
| Unit — resolver | `resolveMCWorkspace` / `findMCService` happy + each error code | Table-driven, `t.TempDir`. |
| Unit — `workspace_list` | Explicit path, discovery, not found, invalid, empty | `handleToolCall` + temp monorepo; assert JSON body + nil→`[]`. |
| Unit — `workspace_upgrade` | batch dry-run; apply; single-service; one-fails-continues; `service_path_missing`; **no-manifest → skipped with warning, batch continues**; all-skipped → top-level `ok` | `t.TempDir` monorepo with 2 services; one missing `.go-arch.yaml`. |
| Unit — `workspace_check` | all processed, mixed violations, single filter, **no-manifest → failed entry, batch continues** | chdir-into-tempdir pattern. |
| Unit — `upgrade_project` with `service` | chdir-free, only named service touched; backward-compat | Assert files outside service untouched. |
| Unit — loader | `TestLoad_NoServices` expects success + empty `Services` | Update existing test. |

## Threat Matrix

`N/A — no routing, shell, subprocess, VCS/PR automation, executable-file classification, or process-integration boundary.` The design reuses existing `os.Chdir`+defer precedent (not a new boundary) and no new subprocess is introduced.

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| `workspace.Load` empty-services relaxation affects CLI callers | Low | CLI loops are `range ws.Services` → no-op on empty; `findService` still returns `service_not_found`. Verify with existing workspace tests. |
| chdir restore failure in `workspace_check` corrupts next request | Low (same as existing handlers) | Byte-for-byte reuse of proven pattern; sequential-only; document constraint. `StartServer recover()` is separate hardening. |
| Per-service viper leak across loop iterations | None | `viper.Reset()` at top of each iteration; `SetConfigFile` scoped. |
| Import cycle mcp↔cmd | Prevented | No `cmd` import from `mcp`; inline resolver only. |
| Spec-vs-proposal `files_changed` shape drift | None | Design retains it; explicit in result shape. |

## Open Questions

None.

## Next Recommended

`sdd-tasks`: break into implementation tasks. Suggested split:
1. Relax `workspace.Load` empty-services + update `loader_test.go`.
2. Add `resolveMCWorkspace` / `findMCService` / `toolResultError` helpers in `server.go`.
3. Add shared `upgradeMCService` / `checkMCService` helpers (with no-manifest branches).
4. Add `workspace_list` schema + handler + tests.
5. Add `workspace_upgrade` schema + handler + tests (dry-run, apply, single, errors, legacy-skip, all-skipped).
6. Add `workspace_check` schema + handler + tests (all/single, continue-on-error, no-manifest).
7. Extend `upgrade_project` schema + handler + tests (service+workspacePath; backward-compat).
8. Doc updates: `docs/COMMANDS.md`, `docs/workspaces.md` (remove line 104 + add MCP section).
9. ROADMAP.md follow-up entries (generate/check deferred; StartServer `recover()` hardening).
