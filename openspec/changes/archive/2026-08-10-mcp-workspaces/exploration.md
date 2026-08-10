# Exploration: Expose workspace features over MCP

Status: **feasible — and the risk profile is better than the archived workspaces exploration assumed.** The key correction: `Upgrade` is fully root-injectable (`WithRoot`), so the workspace upgrade path can be implemented in MCP with **zero `os.Chdir`**. Generate and check remain CWD-bound (`manifestDir()` and `validator.Walk("internal")`) and must reuse the existing chdir+defer pattern, which is provably safe in the current single-threaded server (the defer is registered before any error path and runs on every return, including panic unwinding).

## Current State

- `internal/pkg/mcp/server.go` registers **11 tools** in `tools/list` (server.go:124-336): `new_project`, `list_generators`, `generate_component`, `check_architecture`, `serve_project`, `setup_environment`, `upgrade_project`, `install_template`, `list_packs`, `remove_pack`, `update_pack`. (The task background said 9 — stale; `remove_pack`/`update_pack` landed in `0e85cb5`.) **No workspace tools exist**; the `workspace` package is not imported (server.go:9-14).
- Workspace CLI surface (archived `2026-08-09-workspaces`): `workspace upgrade`/`check` (cmd/workspace.go, cmd/workspace_upgrade.go), `--service` on generate/check/upgrade, `go-arch.workspace.yaml` loader + upward discovery, `Upgrade(..., WithRoot(root))`, opt-in viper config isolation (`loadServiceConfig`).
- MCP project-scoped handlers are chdir-based via `projectPath`; machine-scoped `setup_environment` and pack tools are not.

## Findings

### 1. Current MCP workspace absence + the chdir pattern

- No workspace tool registered; `internal/pkg/workspace` not imported in server.go.
- **All five projectPath handlers use the identical chdir pattern** (server.go):

```go
if args.ProjectPath != "" {
    oldWd, err := os.Getwd()
    if err == nil {
        if chdirErr := os.Chdir(args.ProjectPath); chdirErr != nil {
            sendError(id, -32602, "Cannot change to project directory", chdirErr.Error())
            return
        }
        defer func() { _ = os.Chdir(oldWd) }()
    }
}
```

| Handler | Lines |
|---|---|
| generate_component | server.go:488-497 |
| list_generators | server.go:636-645 |
| check_architecture | server.go:722-731 |
| serve_project | server.go:776-785 |
| upgrade_project | server.go:896-905 |

- Workspace CLI resolution: `resolveWorkspace` (cmd/workspace.go:35-48) — `--workspace` flag wins → `workspace.Load(flagPath)`; else `os.Getwd()` → `workspace.Discover(cwd)` → `Load`. `workspace.Load` (loader.go:22-76) strict yaml.v3 `KnownFields(true)`; `Discover` (discover.go:18-40) walks upward for `go-arch.workspace.yaml`; `ResolvePath` (workspace.go:37-38) = `filepath.Join(w.Dir, FromSlash(s.Path))`; `chdirService` stats the dir first (cmd/workspace.go:77-94). `withService` (cmd/workspace.go:97-112) = chdir + `loadServiceConfig` snapshot/restore (cmd/workspace_helpers.go:27-51) + fn.
- `Upgrade` in workspace CLI: cmd/workspace_upgrade.go:71-74 — `scaffold.Upgrade(cfg, WithResolver(DefaultResolver{}), WithRoot("."))` **after** chdir into the service.

### 2. Chdir pattern safety today — and the WithRoot escape

- **No error path skips the restore in the current code.** The defer is registered immediately after the successful `Chdir`, before any `sendError`/`sendToolResult` early returns (e.g. server.go:503, 929). `defer` runs on every return, and Go runs defers during panic unwinding — so the "error mid-handler leaves the next request in the wrong directory" scenario **cannot happen in the current single-threaded design**.
- Real residual hazards:
  1. `os.Exit`/`log.Fatal` inside the call chain skips defers — but kills the process, so no next request (no `os.Exit` in the mcp package; hooks run subprocesses, bounded).
  2. Panic: defers restore CWD, then the panic propagates through `handleToolCall` → `handleRequest` → `StartServer` loop (server.go:55-78) — **no `recover()` anywhere** → the whole stdio server dies. Availability hazard, not cwd-corruption.
  3. Restore failure is swallowed (`_ = os.Chdir(oldWd)`, server.go:495 etc.) — if the original CWD was deleted mid-handler, the process stays in the project dir and the *next* request starts in the wrong directory. Pathological but real.
  4. **Future concurrency**: `StartServer` is strictly sequential today; if handlers ever become concurrent, `os.Chdir` is a global data race. This is the design reason to prefer root injection where the underlying API allows it.
- **WithRoot actually avoids chdir for upgrade**: `Upgrade` resolves `root := uc.root` (upgrade.go:72-78, ADR-7 default "."), then `ManifestExists(root)` / `LoadManifest(root)` / `filepath.Join(root, path)` (upgrade.go:80-119) and `Apply()` uses `p.ProjectRoot` (upgrade.go:378-379, 394). `WriteVersionField` takes an explicit path (upgrade.go:456). The template engine is embedded-FS — zero CWD reads (grep of template/engine.go for Getwd/Chdir: none). `WithResolver` resolves pack dirs by absolute path. → **`Upgrade(cfg, WithRoot(serviceAbsPath))` is fully CWD-independent; no chdir needed.**

### 3. Workspace resolution in MCP

- `workspace.Load(path)` + `workspace.Discover(cwd)` are both pure functions of their arguments — ideal for MCP. The `workspace` package imports only yaml/filepath/oops → no import cycle with mcp.
- **Import cycle constraint**: `cmd` imports `mcp` (cmd/mcp.go), so mcp **cannot** import the cmd helpers (`resolveWorkspace`, `withService`, `loadServiceConfig`, `chdirService`). MCP must replicate ~15 lines: `Load(workspacePath)` or `Discover(os.Getwd())` → `Find(name)` → `ResolvePath(svc)` → optional stat/IsDir check.
- UX recommendation: **optional `workspacePath` param on each workspace tool, falling back to `workspace.Discover(os.Getwd())`** — mirrors the CLI's "flag wins, else discovery" (cmd/workspace.go:35-48). MCP CWD is the agent's launch directory; upward discovery covers subdir launches, but an explicit path is the predictable escape hatch when the agent is not inside the monorepo.

### 4. Service-relative operations: injectable vs chdir-required

| Operation | CWD-bound? | Mechanism | Evidence |
|---|---|---|---|
| `scaffold.Upgrade` | **No — injectable** | `WithRoot(absPath)` + `WithResolver` | upgrade.go:72-78; Apply: 378-379; engine: no Getwd |
| `UpgradePlan.Apply` | No (uses `ProjectRoot`) | — | upgrade.go:394 |
| `WriteVersionField(path, v)` | No (explicit path) | pass `filepath.Join(root, ".go-arch.yaml")` | upgrade.go:456-474 |
| `validator.Validate` (check) | **Yes** | `filepath.Walk("internal")` + `os.Stat(requiredDirs)` relative to CWD | validator.go:78 |
| `Scaffolder.GenerateComponent/CRUD` | **Yes** | `manifestDir()` returns "." when manifest in CWD (scaffold.go:79-84); writes via `filepath.Join(s.manifestDir(), ...)` (scaffold.go:109, 917, 1060); hooks fire with `os.Getwd()` (425, 554, 573, 643) | scaffold.go |
| `GeneratePackGenerator` | **Yes** | sandbox root = `os.Getwd()` (scaffold.go:735) | scaffold.go:735 |
| hooks runner | Yes | default dir = caller's `os.Getwd()` (runner.go:89, 100-101; ResolveDir 201-206) | runner.go |
| `new_project` (Execute) | Yes (creates `./Name`) | out of scope for workspaces | scaffold.go:129, 158-181 |

### 5. Proposed tool surface

**Option A — standalone workspace tools** (`workspace_upgrade`, `workspace_check`, optional `workspace_list`):
- `workspace_upgrade(workspacePath?, service?, apply?)` — **chdir-free** via `WithRoot(resolvedServicePath)`. Per-service plan (dry-run default); `apply: true` commits + `WriteVersionField`. Safest tool in the whole change.
- `workspace_check(workspacePath?, service?)` — **requires chdir per service** (validator CWD-bound) → reuse the proven chdir+defer pattern.
- `workspace_list(workspacePath?)` — pure read (Load + enumerate services) for agent discovery; cheap, zero risk.
- Pros: isolated additions; upgrade path uses the clean design; matches CLI `workspace upgrade/check`. Cons: 2-3 new tools; check still chdirs.

**Option B — `service` + `workspacePath` params on existing tools**:
- `upgrade_project(+service)` — **no chdir** (WithRoot). ✅
- `generate_component(+service)` — **requires chdir** (manifestDir/hooks/sandbox). ❌
- `check_architecture(+service)` — **requires chdir** (validator). ❌
- Pros: familiar surface. Cons: mixed safety semantics on one param; bigger schema/doc churn.

**Option C — hybrid (recommended)**:
- New: `workspace_list` (read) + `workspace_upgrade` (chdir-free) + `workspace_check` (chdir) — batch surface.
- `upgrade_project` gains optional `service` + `workspacePath` (chdir-free single-service upgrade).
- Optional: `service` params on `generate_component`/`check_architecture` reusing chdir+defer exactly like the CLI `--service` (cmd/generate.go:72-80, cmd/check.go:51-72).
- Rationale: upgrade — the highest-value workspace op and the only chdir-free one — is done right; check/generate follow the proven precedent; zero backward-compat risk (all params optional).

### 6. Risks

- **chdir global state in a long-running server**: bounded today (sequential loop; defer-before-errors; panics unwind defers). Constraints to document: keep handlers sequential; never `os.Exit` in a handler; consider a `recover()` in `StartServer` to keep the server alive across panics (independent hardening, out of scope).
- **Viper global config**: **no conflict** — every project handler already starts with `viper.Reset()` (server.go:499, 647, 733, 787, 907), so per-request isolation exists; a workspace handler chdirs → `viper.Reset()` → `ReadInConfig()` reads the service config and the next handler resets again. `loadServiceConfig` (cmd/workspace_helpers.go) is neither needed nor importable (cmd→mcp cycle).
- **Hooks CWD**: pack hooks are disabled by default in MCP (`allowHooks` opt-in, server.go:288-291); user-global hooks (`hooks.Load`, server.go:516) fire with the handler CWD — correct under the chdir path used by generate/check.
- **Workspace discovery from MCP CWD**: launch dir may be unrelated to the monorepo → `workspacePath` param is the required escape hatch; `Discover` fails cleanly with `workspace_not_found` (discover.go:36-39).
- **Backward compat**: all new params optional; existing tools unchanged when absent. Tool count grows 11 → 13-14 — update ROADMAP/docs.
- **Windows**: `ResolvePath` already `FromSlash`es (workspace.go:38); chdir semantics same as CLI.

## Affected Areas

- `internal/pkg/mcp/server.go` — tools/list entries + handleToolCall cases for workspace tools; optional `service`/`workspacePath` params on upgrade_project (and optionally generate_component/check_architecture); inline workspace resolution (Load/Discover/Find/ResolvePath) + stat check.
- `internal/pkg/mcp/server_test.go` — new tests following the existing chdir-into-tempdir convention (server_test.go:16-100).
- `internal/pkg/scaffold/upgrade_opts.go` — already provides `WithRoot`; no change expected (verify `Apply` + `WriteVersionField` usage).
- `internal/pkg/workspace/` — read-only usage; no change expected.
- `docs/workspaces.md` (line 104 lists MCP tools as out-of-scope) + `docs/COMMANDS.md` / ROADMAP — doc updates.

## Approaches

1. **Option A — standalone tools only** — Effort: Low-Med. Clean upgrade path; check chdirs; no existing-tool churn.
2. **Option B — service param only** — Effort: Low-Med. Familiar; mixed safety; schema churn on 3 tools.
3. **Option C — hybrid (recommended)** — Effort: Med. `workspace_list` + `workspace_upgrade` (chdir-free) + `workspace_check` (chdir) + `service` on `upgrade_project` (chdir-free); optionally `service` on generate/check via chdir.

## Recommendation

**Option C.** Implement `workspace_list` + `workspace_upgrade` (fully chdir-free via `WithRoot`) + `workspace_check` (chdir+defer, proven) as new tools, and add optional `service`+`workspacePath` to `upgrade_project` (chdir-free). This delivers the entire workspace surface while keeping `os.Chdir` usage to exactly the two places where the underlying APIs (`validator`, `manifestDir`/hooks/generator-sandbox) leave no alternative — and those two reuse the already-proven pattern byte-for-byte. The proposal phase must decide whether `generate_component`/`check_architecture` `service` params are in scope (they require chdir) or deferred.

## Open Questions (for proposal)

1. Tool surface: Option C as recommended, or trim to A (standalone only)? Are `service` params on `generate_component`/`check_architecture` in scope (chdir-required) or deferred?
2. `workspace_upgrade` semantics: batch-all (iterate services like CLI `workspace upgrade`) vs single-service via `service` param — both, or one?
3. Does `workspace_upgrade` need `workspace_list` for agent discovery, or is listing services inside `workspace_upgrade`/`workspace_check` responses sufficient?
4. Should `StartServer` gain a `recover()` (server-wide panic hardening) as part of this change or a separate one?
5. Error taxonomy: reuse `workspace_not_found` / `service_not_found` / `service_path_missing` codes as tool-result error strings (CLI precedent)?

## Risks

- chdir remains for check/generate paths — bounded (sequential + defer-before-errors) but must be documented as a permanent constraint; future concurrency would break it.
- Import cycle mcp→cmd: resolution logic must be duplicated inline in mcp (~15 lines) — keep it thin, mirror `resolveWorkspace`.
- Tool-surface growth (11 → 13-14): update ROADMAP + docs/workspaces.md out-of-scope note.
- Testing: workspace MCP tests must chdir into tempdirs (existing convention, server_test.go:24-26) and build a temp monorepo (workspace file + 1-2 services); `go test ./...` currently has no mcp test deps beyond stdlib — keep it that way.

## Ready for Proposal

**Yes.** Feasible with bounded blast radius; the safe design is proven (WithRoot makes upgrade chdir-free; check/generate reuse the verified chdir precedent). Tell the user: the long-running-process concern is real but narrower than feared — upgrade can avoid chdir entirely, and the chdir paths are safe under the current sequential design; the proposal should lock the tool surface (recommend hybrid) and whether the chdir-required `service` params on generate/check are in scope.
