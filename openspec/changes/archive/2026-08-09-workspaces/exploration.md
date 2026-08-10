# Exploration: Workspaces / Multi-Project Support (roadmap candidate)

Status: **feasible — and LESS risky than the roadmap's reputation suggests.** The generate/upgrade core is already path-parameterized via `manifestDir()` and the `--project-path` chdir precedent. The real blast radius is the **viper config loading** and the **hooks CWD env**, not the scaffolder. A `WithRoot` option on Upgrade + a workspace file resolver covers most of the surface without breaking ADR-7 for single projects.

## Executive Summary

Multi-project support is feasible with a bounded blast radius. Three load-bearing facts: (1) `manifestDir()` (scaffold.go:79) already resolves "." when a manifest exists in CWD — generate's file writes are ALL relative to it, so generating inside a service just works if the CWD (or an injected root) points there; (2) Upgrade's root is a single line `root := "."` (upgrade.go:73, ADR-7) — a `WithRoot` UpgradeOption makes it workspace-aware with ZERO impact on existing callers (same pattern as `WithResolver`); (3) the MCP `--project-path` os.Chdir + defer-restore precedent (server.go:429-436) already proves the chdir-into-service pattern. The genuinely risky areas are viper's `AddConfigPath(".")` (root.go:41-61 — reads the WRONG .go-arch.yaml from the monorepo root) and the hooks env `ProjectPath` from `os.Getwd()` (scaffold.go:129, 425). Both are contained and testable. The user's branch-isolation concern maps cleanly: slices 1-3 are safe additive work; only the viper/hooks slice touches shared behavior, and even that is opt-in (workspace commands only).

## Findings

### 1. CWD-as-root assumption — where it lives

| Site | File:Line | Mechanism | Workspace impact |
|------|-----------|-----------|------------------|
| `manifestDir()` | scaffold.go:79-85 | "." if manifest in CWD, else ProjectName | Central. Generates are already manifest-relative. |
| Upgrade root | upgrade.go:69-73 | `root := "."` (ADR-7, documented) | Single line — `WithRoot` option. |
| viper config | root.go:41-61 | `AddConfigPath(home)` + `AddConfigPath(".")`, `SetConfigName(".go-arch")` | **HIGH RISK**: reads `./.go-arch.yaml` — at monorepo root that's the wrong config. Needs `--service`-scoped reload. |
| os.Chdir + defer | mcp/server.go:429-436, 577-584, 656-663 | chdir into ProjectPath, restore after | Proven precedent; reuse for workspace service ops. |
| os.Chdir CLI | cmd/upgrade.go:47 | chdir(projectPath) before Upgrade | Same precedent. |
| hooks env CWD | scaffold.go:129, 139, 425, 428 | `os.Getwd()` → `ProjectPath`, Fire cwd | **RISK**: hooks run with service CWD when chdir'd — actually correct if chdir happens first. |
| WriteVersionField | scaffold.go:179-181 | `filepath.Join(cwd, s.manifestDir())` | Manifest-relative; fine with right CWD. |
| Engine resolution | template/engine.go:54-76 | path-keyed, `.go-arch/templates/` relative to CWD | Pack/embedded steps unaffected; local overrides resolve from service CWD. |

### 2. The `--project-path`/MCP precedent

- `cmd/upgrade.go:47`: `os.Chdir(projectPath)` then `Upgrade(cfg)` — the CLI already has the chdir-into-project pattern.
- `mcp/server.go:429-436` (new_project), `:577-584` (upgrade), `:656-663`: `os.Chdir(args.ProjectPath)` + `defer os.Chdir(oldWd)`.
- Verdict: **reusable**. Workspace commands can chdir into each service exactly like `--project-path` does. The pattern is proven (though global-state — the known hazard — it's already the codebase's established approach; an injected root parameter is cleaner but a bigger refactor).

### 3. Manifest & upgrade per-service

- `manifest.go` records paths relative to project root (targetPath keys) — service-relative, so loading a service's manifest from the workspace root requires the service root, not rework.
- `Upgrade()` (upgrade.go:72+) iterates `m.Files` with `filepath.Join(root, path)` where root is `.` — a `WithRoot(root string)` UpgradeOption (mirroring `WithResolver`) makes it work from any CWD. **Zero impact** on existing `Upgrade(cfg)` callers (variadic option).
- Legacy fallback `upgradeLegacy(cfg, root)` already takes root as a parameter — workspace-aware by construction.

### 4. Generate/hooks in a sub-service

- `GenerateComponent`/`GenerateCRUD` write via `filepath.Join(s.manifestDir(), targetPath)` (scaffold.go:201, 231, 259...) — **already manifest-relative**, not CWD-absolute. With `manifestDir()` returning "." for a service (manifest present), generating inside the service CWD works unchanged.
- Hooks fire with `cwd` from `os.Getwd()` (scaffold.go:129, 425) — if the command chdir'd into the service first (workspace pattern), hooks get the right CWD automatically.
- Verdict: `generate crud X --service orders` needs **chdir into the service + config reload** — the dispatch itself is path-correct already.

### 5. Config surface

- `ProjectConfig` (ui/prompts.go:9-20) is flat; `configFromViper` (upgrade.go:133-145) maps viper → struct.
- Workspace file (`go-arch.workspace.yaml`) is a NEW config type — needs its own loader (yaml.v3, packs/manifest.go precedent), NOT viper (viper is single-config).
- Precedence question: service `.go-arch.yaml` (inside each service) vs workspace file (monorepo root). Services stay self-contained; the workspace file only ADDS service→path mapping.

### 6. CLI structure

- Commands in cmd/*.go registered on RootCmd (root.go). A `workspace` parent command + `--service` flag on generate/check/upgrade slots in cleanly.
- `--service X` flag on existing commands: parse → resolve path from workspace file → chdir (or inject root) → run existing logic.
- No new infra needed — cobra handles it.

### 7. MCP

- Project-scoped tools (new_project, generate_component, upgrade_project) are already chdir-based via ProjectPath — a `workspacePath`+`service` param could reuse it. Machine-scoped setup_environment unaffected.
- Verdict: MCP workspace support = optional v1.1; the chdir precedent is already there.

### 8. Blast-radius matrix

| Area | Class | Why |
|------|-------|-----|
| Generate dispatch (crud/component) | (a) unchanged | Already manifest-relative via manifestDir() |
| Template engine (pack/embedded) | (a) unchanged | Path-keyed, no CWD dependency for pack/embedded steps |
| Packs/generators contract | (a) unchanged | No path assumptions beyond service-relative |
| Upgrade | (b) WithRoot option | Single-line ADR-7 root; variadic option = backward-compatible |
| check/doctor | (b) resolver only | Manifest-relative already |
| Legacy upgrade | (b) resolver only | root already parameterized |
| MCP chdir tools | (b) resolver only | ProjectPath precedent |
| **viper config loading** | **(c) real rework** | AddConfigPath(".") reads monorepo-root config; needs per-service reload or config injection |
| **hooks env ProjectPath** | **(c) real rework** | os.Getwd() — correct only if chdir happens first; needs service-path injection for non-chdir design |
| Workspace file loader | (c) new (additive) | New config type, yaml.v3 loader |
| Nested workspaces / concurrent ops | (d) unknown/risky | Out of v1 scope — single-level, sequential |

## Path Resolution Model (options)

1. **Chdir-based (recommended v1)** — workspace command resolves service path from workspace file, `os.Chdir(path)` + defer restore (MCP precedent), then runs existing logic unchanged. Pros: zero rework of generate/hooks (they see the right CWD). Cons: global state (the known hazard), sequential only.
2. **Root-injection** — pass `WithRoot`/path-param into each call. Pros: no global state, concurrency-safe. Cons: hooks env + viper still read CWD — needs deep refactor to honor injected root everywhere.
3. **Hybrid (recommended)** — chdir for workspace commands (v1), keep ADR-7 for single projects. The `WithRoot` UpgradeOption lands anyway (cheap, used by chdir-free paths), but the primary mechanism is chdir.

## Command Surface

```text
go-arch workspace upgrade                    # upgrade all services in workspace file order
go-arch workspace check                      # check all services
go-arch generate crud User --service orders  # chdir into services/orders, generate, restore
go-arch generate auth jwt --service users
go-arch new --workspace ./monorepo --name svc # (optional) create a service inside a workspace
```

Workspace file (`go-arch.workspace.yaml`):
```yaml
services:
  - name: orders
    path: services/orders
    template: express
  - name: users
    path: services/users
    template: express
```

## Branch Isolation Assessment (user's concern)

The SDD chained-PR structure maps cleanly to blast radius:

| Slice | Risk | Safe early? |
|-------|------|-------------|
| 1. Workspace file loader + types + validation | Low | ✅ pure additive, no behavior change |
| 2. `workspace upgrade` + `workspace check` (chdir + WithRoot) | Medium | ✅ opt-in command, existing commands untouched |
| 3. `--service` flag on generate (chdir into service) | Medium | ✅ additive flag, default path unchanged |
| 4. viper config reload per-service + hooks env service path | **High** | ⚠️ touches shared initConfig — but opt-in (only when --service/workspace used) |
| 5. Docs + workspace new | Low | ✅ |

Main is never touched until the tracker merges (feature-branch-chain). Even slice 4 is opt-in: single-project behavior (no --service) is byte-identical. **The user's branch-isolation concern is fully satisfiable.**

## Open Questions (for proposal)

1. Chdir-based vs root-injection (recommend hybrid: chdir primary + WithRoot option).
2. Workspace file location: root `go-arch.workspace.yaml` vs `go-arch.workspace.yaml` discovery upward? (recommend: explicit `--workspace` flag + auto-discover upward).
3. viper reload strategy for slice 4: `viper.SetConfigFile(service/.go-arch.yaml)` + re-ReadInConfig vs config injection into calls.
4. Does `new` get workspace-aware creation (add service to workspace file)? v1 scope?
5. MCP workspace support in v1 or v1.1?
6. Sequential-only (v1) vs concurrent service ops (deferred).
7. Nested workspaces — explicitly out of scope.

## Risks

- **viper global state**: single-config assumption; reloading per-service is the trickiest part (root.go initConfig runs once at startup).
- **hooks env CWD**: correct under chdir; must test hook-fired files land in the service (live smoke).
- **os.Chdir global state**: the known hazard; sequential-only + defer restore (MCP precedent) bounds it.
- **Manifest path collisions**: service manifests are relative — verify upgrade doesn't write monorepo-root files when chdir'd.
- **Windows**: path resolution in workspace file (forward slashes), chdir semantics — needs CI coverage.

## Next Recommended

**propose** — the blast radius is bounded (mostly additive, opt-in, branch-safe). The proposal must lock: chdir-hybrid model, workspace file location, viper reload strategy, and v1 scope (sequential-only, no nested).
