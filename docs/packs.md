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

## 📐 Contract Evolution (v2)

The `contract_version` field lets the CLI reject future schemas it can't handle. If a pack declares `contract_version: 99`, the CLI returns a `contract_version_mismatch` error and does not install the pack.

v2 is explicitly deferred — the v1 surface is intentionally minimal:

- One manifest schema
- One template convention (`.tmpl` suffix)
- One install layout (`<name>@<version>`)
- One sidecar file (`pack.json`)

When v2 is needed, it will carry a new `contract_version` and the CLI will reject packs that require it until the upgrade ships.
