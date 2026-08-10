# Workspaces — Multi-Project Support

`go-arch` supports monorepos through **workspaces**: a `go-arch.workspace.yaml` file at the repository root that maps service names to paths, plus commands that operate across the set.

> **Single projects are unaffected.** Every workspace feature is opt-in — a project without a workspace file behaves exactly as before (ADR-7: the CWD is the project root).

---

## Workspace File

`go-arch.workspace.yaml` at the monorepo root:

```yaml
services:
  - name: orders
    path: services/orders
    template: express     # optional — pack used at generation
  - name: users
    path: services/users
    template: express
```

- `name` — required, lowercase alphanumeric with internal dashes (`orders`, `api-gateway`).
- `path` — required, relative to the workspace file's directory.
- `template` — optional metadata; not consumed in v1.
- Unknown keys, duplicate names, and invalid slugs are rejected with a clear error.

---

## Discovery

The CLI locates the workspace file two ways:

1. **`--workspace <path>`** — explicit flag (wins).
2. **Auto-discovery** — walks upward from the current directory looking for `go-arch.workspace.yaml`.

```bash
# From anywhere inside the monorepo, without a flag:
go-arch workspace upgrade

# Or explicitly:
go-arch workspace upgrade --workspace /repo/go-arch.workspace.yaml
```

---

## Commands

### `go-arch workspace upgrade`

Upgrades every service in declaration order. Each service runs the standard upgrade logic (plan, dry-run by default; `--yes` applies). A failing service is reported and the remaining services still run; the command exits non-zero if any failed.

```bash
go-arch workspace upgrade            # dry-run: print plans only
go-arch workspace upgrade --yes      # apply all upgradable files
```

### `go-arch workspace check`

Runs the architecture check for every service, with per-service summary and continue-on-error semantics.

```bash
go-arch workspace check
```

### `--service <name>`

Target a single service from anywhere inside the monorepo:

```bash
go-arch generate crud User --service orders
go-arch check --service orders
go-arch upgrade --service orders --yes
```

The CLI changes into the service directory, reloads the service's `.go-arch.yaml`, runs the command, and restores the previous directory and config. Single-service `--service` invocations fail fast (unlike multi-service workspace commands).

---

## Behavior Notes

- **Sequential execution** — services are processed one at a time, in declaration order. Concurrent execution is not supported in v1.
- **Continue-on-error** — `workspace upgrade` and `workspace check` process every service even if one fails; the final summary shows each outcome and the exit code is non-zero if any failed.
- **Hooks** — generator and lifecycle hooks run with the service directory as their working directory, so hook-created files and `PROJECT_PATH` target the service.
- **Config isolation** — each service uses its own `.go-arch.yaml`; the previous config is restored after the operation.
- **Legacy services** — a service without a manifest is reported (`service_no_manifest`) and skipped in batch mode.
- **Pack sources** — workspace upgrade re-renders pack-sourced files via the recorded pack (same as standalone upgrade, including PROTECTED classification).

---

## Batch Apply Semantics

- `workspace upgrade` defaults to **dry-run** (plans only).
- `workspace upgrade --yes` applies all upgradable files per service.
- Legacy per-file interactive prompting is **disabled under batch** — legacy services are reported and skipped unless `--yes` applies them non-interactively.

---

## Out of Scope (v1)

- Nested workspaces (workspace inside workspace).
- Concurrent service operations.
- Cross-service template sharing.
- MCP workspace tools (the chdir precedent exists; a future version may add them).
- `go-arch new` workspace-aware creation.

---

## Error Codes

| Code | Meaning |
|------|---------|
| `workspace_not_found` | No workspace file found by flag or discovery |
| `workspace_invalid` | Workspace file schema/validation error |
| `service_not_found` | `--service` named a service not in the workspace |
| `service_path_missing` | A service's declared path does not exist on disk |
| `service_duplicate` | Two services share a name |
| `service_no_manifest` | A service lacks a manifest; legacy fallback applies |
