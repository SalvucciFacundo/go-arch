# Packs — Installable Template Packs 📦

Packs ship a complete project scaffold as a Go module. Install one from a module proxy, scaffold with `new --template`, and upgrade safely — the CLI tracks which pack version generated every file.

---

## Quick path

1. `go-arch template install github.com/org/go-arch-express@1.0.0`
2. `go-arch new myapp --template express`
3. `go-arch upgrade` (re-renders from the recorded pack version)

---

## ⚠️ Trust Warning

**Packs can declare hooks** — shell commands that run during `new`. When you install a pack that declares hooks, the CLI shows a trust prompt and asks you to explicitly opt in. Hooks are disabled by default; your choice is recorded in a sidecar file (`pack.json`) next to the installed pack.

Treat pack hooks like npm scripts or `Makefile` targets: review the pack's `go-arch.yaml` before enabling them.

---

## 📦 Pack Contract (v1)

A pack is a Go module whose root contains a `go-arch.yaml` manifest and a `templates/` directory.

### Manifest schema

| Field | Type | Required | Description |
|---|---|---|---|
| `contract_version` | int | **yes** | Must be `1` |
| `name` | string | **yes** | Lowercase slug: `^[a-z0-9]+(-[a-z0-9]+)*$` |
| `version` | string | **yes** | Semantic version (e.g. `1.0.0`, `2.1.0-alpha`) |
| `layout` | []string | no | Directory paths to create under the project root |
| `hooks` | map | no | Hook entries per hook type (`pre-new`, `post-new`). Same schema as `.go-arch.yaml` hooks |
| `binary_assets` | []BinaryAsset | no | Files copied verbatim (no template engine) |

### BinaryAsset

| Field | Type | Required | Description |
|---|---|---|---|
| `source` | string | **yes** | Path relative to pack root (e.g. `assets/htmx.min.js`) |
| `target` | string | **yes** | Path relative to project root (e.g. `static/js/htmx.min.js`) |

### Example manifest

```yaml
# go-arch.yaml (pack root)
contract_version: 1
name: express
version: 1.0.0
layout:
  - cmd/api
  - internal/handler
  - static/js

binary_assets:
  - source: assets/htmx.min.js
    target: static/js/htmx.min.js

hooks:
  post-new:
    - go mod tidy
```

### Template convention

Pack templates live under `templates/` and follow the **`.tmpl` suffix convention**:

```
pack/
├── go.mod
├── go-arch.yaml
└── templates/
    ├── .go-arch.yaml.tmpl    → .go-arch.yaml
    ├── main.go.tmpl           → main.go
    └── common/
        └── env.tmpl           → common/env.tmpl  (NOT a target — resolved as lookup key)
```

- `templates/<path>.tmpl` → project target `<path>`
- The lookup key is the full relative path WITH the `.tmpl` suffix (e.g. `common/env.tmpl`)
- Pack supplies a `templates/.go-arch.yaml.tmpl` so the scaffolded project's config records `template: <name>` naturally

---

## 📁 Install Location

Packs are installed to `~/.go-arch/packs/<name>@<version>/`.

```
~/.go-arch/packs/
├── express@1.0.0/
│   ├── go.mod
│   ├── go-arch.yaml
│   ├── pack.json           ← sidecar (hooks_enabled, installed_at)
│   └── templates/...
└── express@1.1.0/
    └── ...
```

Set `GO_ARCH_PACKS_DIR` to override the base directory (useful for CI and testing).

---

## 🧭 Template Lookup Precedence

When the engine resolves a template, it searches in this order:

| Step | Source | Location |
|---|---|---|
| 1 | **Local** | `<project>/.go-arch/templates/` |
| 2 | **Global** | `~/.go-arch/templates/` |
| 3 | **Pack** | `~/.go-arch/packs/<name>@<version>/templates/` |
| 4 | **Embedded** | Built-in templates shipped with the CLI binary |

First match wins. Local overrides pack, pack overrides embedded. This applies to both `new --template` and `upgrade` (for entries without a pack source — see below).

---

## 🔄 Upgrade Interaction

Pack-scaffolded projects record the generating pack in each manifest entry:

```yaml
# .go-arch/manifest.yaml
files:
  main.go:
    sha256: abc123...
    origin: scaffold
    template: web/main.tmpl
    source: pack:express@1.0.0      # ← provenance field
```

During `go-arch upgrade`:

- **Pack installed at recorded version** → re-renders directly from the pack's `templates/` directory, bypassing the local/global/embedded chain entirely.
- **Pack not installed** → entry is classified as **PROTECTED** and never overwritten. A warning names the missing pack and version.
- **Pack version bump** (e.g. recorded `1.0.0` but only `1.1.0` is installed) → entries are PROTECTED. The CLI does NOT auto-substitute a newer version. Upgrade with `template update` first, then re-scaffold.

This guarantees that what generated the file is what re-renders it — no silent fallback, no surprise template substitutions.

---

## 🪟 Windows Notes

- Template paths and pack directories use `filepath` — forward slashes in manifests are normalized.
- The download step (`go mod download -json`) works identically on Windows via `exec.CommandContext`.
- Hooks use `cmd /c` for string-form entries on Windows (same as `.go-arch.yaml` hooks).
- Pack install uses `os.Rename(tmp, dst)` — atomic on POSIX; on Windows, the window between `RemoveAll` and `Rename` is negligible for local operations.

---

## 📐 Contract v2 — Generators

Pack contract v2 introduces **generators**: declarative recipes that scaffold project files from a pack. A generator is a named YAML recipe DSL executed by `go-arch generate <name>`.

Set `contract_version: 2` in the pack manifest to opt in. The CLI supports v1 and v2 — existing v1 packs continue to work unchanged.

### Manifest schema (v2 additions)

| Field | Type | Required | Description |
|---|---|---|---|
| `contract_version` | int | **yes** | `1` or `2` |
| `generators` | map | no | Map of generator-name → recipe |

```yaml
# go-arch.yaml (pack root, v2)
contract_version: 2
name: express
version: 1.0.0
layout:
  - cmd/api

generators:
  docker:
    description: "Scaffold Docker setup for the project"
    pre:
      - echo "Starting Docker scaffold..."
    steps:
      - type: template
        from: "docker/Dockerfile.tmpl"
        to: "Dockerfile"
      - type: template
        from: "docker/docker-compose.yml.tmpl"
        to: "docker-compose.yml"
      - type: prompt
        name: "compose"
        message: "Include Docker Compose?"
        default: "true"
        required: false
      - type: run
        command: "echo"
        args: ["Docker scaffold complete"]
        silent: true
    post:
      - echo "Docker scaffold finished"
```

### Recipe step types

Each recipe is an ordered list of steps. Steps execute in declaration order — no conditionals or branching in v2.

| Step type | Required fields | Description |
|---|---|---|
| `template` | `from`, `to` | Render a pack template (`templates/<from>`) to `<to>`. No chain fallback. |
| `binary` | `from`, `to` | Copy a file verbatim from the pack. Optional `mode` (file permission octal, default `0644`). |
| `run` | `command` (or shell string) | Execute a command. Reuses `hooks.Entry` shape: `command`, `args`, `cwd`, `env`, `timeout`, `silent`, `ignore_failure`. |
| `prompt` | `name`, `message` | Collect a user value. Optional `default`, `required` (boolean). Resolved values flow into `run:` step env and `use:` builtins. |
| `use` | `value: "builtin/<name>"` | Delegate to a CLI-registered builtin generator. |

### Pre and post hooks

A generator MAY declare `pre:` and `post:` hook lists. These fire around step execution (pre before step 1, post after all steps succeed). Hooks are gated by the same `HooksEnabled` sidecar flag as pack-level hooks.

### Trust model

**run steps and hooks require explicit trust.** When a v2 pack declares generators with `run:` steps or `pre:`/`post:` hooks, the install trust prompt warns about command execution. `HooksEnabled` must be `true` in the sidecar (`pack.json`) for these steps to execute — if disabled, `run:` steps and hooks are skipped with a warning, while `template:` and `binary:` steps still run.

### Path sandbox

All `template` and `binary` target paths are validated **pre-flight** — before ANY file is written. The sandbox rejects:

- Absolute paths (e.g. `/etc/passwd`)
- `..` traversal (e.g. `../../etc/shadow`)
- Symlink escapes (resolved real path outside project root)

If ANY step's path escapes, the entire recipe aborts with zero files written. This prevents partial state from a malicious or misconfigured recipe.

### Provenance and upgrade semantics

Generator-produced files are recorded in the project manifest with full provenance:

- **Template-step files**: `origin: template` with `metadata.generator` and `metadata.args`. These files are **upgradable** — `go-arch upgrade` re-renders them from the pack template (byte-identical to normal pack re-renders).
- **Non-template files** (binary copies, run output): `origin: generator`. These files are **PROTECTED** — never overwritten by upgrade. A per-entry warning tells the user to re-run the generator manually.

Automatic generator re-execution during `go-arch upgrade` is deferred to a future v2.1.

### Lookup order

`go-arch generate <name>` resolves through a three-tier lookup, first match wins:

1. **Pack generator** — if the project's `.go-arch.yaml` declares `template:` and that pack has a generator named `<name>`
2. **Builtin generator** — if a CLI-registered builtin matches
3. **Component type** — existing six-type switch (`service`, `repository`, `handler`, `page`, `component`, `crud`)

A pack generator silently shadows a builtin or component type with the same name.

### Listing generators

`go-arch generate --list` prints available generators grouped by source: pack generators (with pack name and description), builtin generators, and component types.

### MCP integration

The MCP server exposes generators through two additions:

- `generate_component`: relaxed `type` enum (any string), plus optional `generatorArgs` object for passing prompt values
- `list_generators`: returns available generators for the current project context as a structured list

### Contract version compatibility

| CLI version | v1 packs | v2 packs |
|---|---|---|
| pre-generators | ✅ Accepted | ❌ Rejected (`contract_version_mismatch`) |
| with generators | ✅ Accepted | ✅ Accepted |
