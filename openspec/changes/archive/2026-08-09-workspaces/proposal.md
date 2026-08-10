# Proposal: Workspaces — Multi-Project Support

**Status**: proposed
**Next recommended**: spec

## Intent

Teams running multiple go-arch-generated services in a monorepo can't operate across them today — every command assumes the CWD is a single project root (ADR-7). This change adds a `go-arch.workspace.yaml` at the monorepo root that maps service names to paths, plus workspace-aware commands (`workspace upgrade`, `workspace check`, `--service` flag on generate) so operations run across the set. Single-project behavior stays **byte-identical** — every new surface is opt-in.

## Scope

**In scope**
- Workspace file loader + schema: `go-arch.workspace.yaml` at monorepo root, `services: [{name, path, template?}]`, validation (missing path, duplicate name, nonexistent path).
- `workspace upgrade` + `workspace check`: chdir into each service in workspace order (sequential), run existing Upgrade/Check, defer-restore CWD (MCP precedent).
- `--service <name>` flag on `generate` (and check/upgrade when run from a workspace): resolve path from workspace file, chdir, run, restore.
- `Upgrade(cfg, WithRoot(root))` UpgradeOption — makes Upgrade workspace-aware without touching existing callers (mirrors `WithResolver`).
- Workspace file discovery: explicit `--workspace <path>` flag; auto-discover upward from CWD (find `go-arch.workspace.yaml` walking up).
- Error taxonomy: `workspace_not_found`, `workspace_invalid`, `service_not_found`, `service_path_missing`, `service_duplicate`, `service_no_manifest` (legacy fallback), `service_missing`.
- Docs: `docs/workspaces.md` + COMMANDS/README updates.

**Out of scope (v1)**
- Nested workspaces (workspace inside workspace).
- Concurrent service operations (sequential only).
- Cross-service template sharing.
- MCP workspace support (deferred to v1.1; the chdir precedent already exists).
- `new` workspace-aware creation (deferred; existing `new` unchanged).
- Root-injection refactor of generate/hooks (the chdir-hybrid model is the v1 mechanism).

## Approach

### Path resolution model — chdir-hybrid

- **Primary mechanism: chdir** (the codebase's established pattern — `cmd/upgrade.go:47`, `mcp/server.go:429-436`). Workspace commands resolve the service path, `os.Chdir(path)` + defer restore, then run the existing logic unchanged. Generate/hooks see the correct CWD automatically (manifestDir() returns "." with a manifest present; hooks env `ProjectPath` from `os.Getwd()` is correct under chdir).
- **`Upgrade(cfg, WithRoot(root))`** lands as an upgrade option (cheap, backward-compatible variadic, mirrors `WithResolver`) — used by workspace-aware call paths that prefer injection.
- Single-project behavior (no `--workspace`, no `--service`): untouched, byte-identical.

### Workspace file schema

```yaml
# go-arch.workspace.yaml (monorepo root)
services:
  - name: orders
    path: services/orders
    template: express          # optional — pack used at generation
  - name: users
    path: services/users
    template: express
```

- `name`: slug (reuse packs slug regex).
- `path`: relative to the workspace file's directory; must exist at command time.
- `template`: optional metadata; not consumed in v1 (future: workspace-wide pack ops).
- Unknown keys → `workspace_invalid` (strict yaml.v3, packs/manifest.go precedent).

### Command surface

```text
go-arch workspace upgrade                       # upgrade all services in file order
go-arch workspace check                         # check all services
go-arch generate crud User --service orders     # chdir into services/orders, generate, restore
go-arch check --service orders                  # single service from workspace context
go-arch upgrade --service orders
```

- `workspace` parent command: no positional args; subcommands `upgrade`, `check`.
- `--service <name>` on generate/check/upgrade: requires a resolvable workspace (flag `--workspace` or auto-discovered upward); error `service_not_found` if unknown.
- Sequential only; each service operation is independent; a failing service reports and continues to the next (report summary at end) — decided: **continue-on-error for workspace upgrade/check**, with per-service exit reporting. (Single-service `--service` preserves fail-fast.)
- chdir + `defer os.Chdir(oldWd)` per service (MCP precedent).

### viper reload strategy (the HIGH-risk slice — opt-in)

- When a workspace command targets a service, before running: `viper.SetConfigFile(<service>/.go-arch.yaml)` + `viper.ReadInConfig()` (if present), then restore viper state after (defer) so subsequent commands see the original config.
- This is opt-in: only workspace/`--service` paths trigger it. Single-project flow (initConfig at startup, no workspace) is untouched.
- Hooks env: under chdir, `os.Getwd()` already yields the service dir — no hooks refactor needed in v1. Verified by live smoke.

### Branch isolation (user's concern)

Maps to the exploration's blast-radius slices, feature-branch-chain (main untouched until tracker):

| Slice | Risk | Content |
|-------|------|---------|
| 1 | Low | Workspace file loader + types + validation |
| 2 | Medium | `workspace upgrade` + `workspace check` (chdir + WithRoot) |
| 3 | Medium | `--service` flag on generate/check/upgrade |
| 4 | High (opt-in) | viper per-service reload + hooks env verification |
| 5 | Low | Docs + workspace new (if in scope) |

## Implications

| Area | Impact |
|------|--------|
| New package | `internal/pkg/workspace/` — loader, schema, path resolution, errors |
| `internal/pkg/scaffold/upgrade.go` | `WithRoot` UpgradeOption (backward-compatible) |
| `cmd/workspace.go` | New parent command + upgrade/check subcommands |
| `cmd/generate.go`, `cmd/check.go`, `cmd/upgrade.go` | `--service` flag + chdir + viper reload |
| `cmd/root.go` | Workspace file discovery helper (upward walk) |
| `internal/pkg/mcp/server.go` | Untouched in v1 (deferred) |
| `docs/workspaces.md` | New reference |

## Edge Cases

| Case | Resolution |
|------|------------|
| Workspace file missing | `workspace_not_found` (or, for `--service`, `service_not_found` naming the flag) |
| Service path missing on disk | `service_path_missing` |
| Duplicate service names | `service_duplicate` at load |
| Service without manifest | Legacy fallback (upgradeLegacy already parameterized) |
| `--service` without workspace context | Error instructing `--workspace` or running inside a monorepo |
| Workspace without `--service` on generate | Error: generate needs a service target |
| Windows paths | filepath.Join semantics; forward-slash paths in YAML normalized |
| Failing service mid-chain | workspace upgrade/check: report + continue; `--service`: fail-fast |

## Product Tradeoffs

- **Monorepo value vs ADR-7 stability**: the chdir-hybrid keeps ADR-7 for single projects; workspaces opt in. The only shared-code touch is viper reload, which is opt-in too.
- **Sequential vs concurrent**: v1 sequential keeps the os.Chdir global-state hazard bounded (defer restore, no parallel CWD races).
- **Backward compatibility**: zero — every new surface is additive; existing commands and single-project flows are byte-identical.

## Open Questions

None blocking — all 10 exploration questions resolved above.

## Risks

- viper global-state reload is the trickiest slice (initConfig runs once at startup); opt-in + defer-restore bounds it, live smoke verifies.
- os.Chdir global state: sequential-only + defer restore (MCP precedent).
- Windows path/chdir semantics need CI coverage.
- Workspace file schema is a new public contract — strict validation + small v1 scope mitigate.
