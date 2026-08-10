# Command Reference Guide 📖

This guide provides a detailed technical explanation of every command available in **Go-Arch**.

---

## 1. `setup` ✨
**Usage**: `go-arch setup`

Prepares your environment. It is designed to be the first command a new Go developer runs.
- **OS Detection**: Identifies if you are on Linux, macOS, or Windows.
- **Toolchain**: Verifies if `go` is installed and available in the PATH.
- **Utilities**: Suggests and assists with the installation of `Air` (Live Reloading).

---

## 2. `new` 🏗️
**Usage**: `go-arch new [project-name]`

The main entry point for scaffolding. It triggers an interactive wizard.
### Options provided by the wizard:
- **Module Name**: The Go namespace (e.g., `github.com/user/repo`).
- **Architecture**: Choice between **Minimalist**, **Standard**, or **Hexagonal** (can be overridden via **External Templates**).
- **DB Driver**: Pre-configures specific repository boilerplate (PostgreSQL, MySQL, MongoDB).
- **Use Docker**: Optional generation of `Dockerfile` and `docker-compose.yaml`.
- **Telemetry**: Optional integration of **OpenTelemetry** with support for multiple backends (Jaeger, Zipkin, SigNoz, etc.).
- **gRPC Support**: Scaffolds a complete **gRPC server** including `.proto` contracts and automation through a **Makefile**.

---

## 3. `generate` (or `g`) 🧬
**Usage**: `go-arch generate [type] [Name]`

Injects new components into an existing project. It is **Context-Aware**: it reads your `.go-arch.yaml` to know which folder structure to follow.

### Resolution Order

`go-arch generate <name>` resolves through a three-tier lookup, first match wins:

1. **Pack generator** — if your project uses a template pack (`.go-arch.yaml` `template:` field) and the pack declares a generator named `<name>`
2. **Builtin generator** — CLI-registered builtin generators
3. **Component types** — the built-in code generators below

### Listing generators: `--list`

```bash
go-arch generate --list
# express:
#   docker  Scaffold Docker setup for the project
#   auth    Add authentication middleware
# builtin:
#   no builtin generators registered
# component types:
#   service, repository, handler, crud, page, component
```

Output is grouped by source and sorted within each group.

### Component Types:
- **`service`**: Creates the business logic layer.
- **`repository`**: Creates the interface and the implementation (SQL/NoSQL).
- **`handler`**: Creates the HTTP/API entry point.
- **`crud`**: **Full Automation**.
  - Generates Model, Service, Repository, and Handler.
  - In a Hexagonal project, it correctly places items in `domain`, `ports`, and `adapters`.
  - In a Standard project, it uses `model`, `service`, `repository`, and `handler`.

---

## 4. `serve` 🚀
**Usage**: `go-arch serve`

Runs your application with a developer-first approach.
- **Air Integration**: It looks for an `.air.toml` in the root. If found (and `air` is installed), it runs with **Hot-Reload**.
- **Native Fallback**: If `air` is not available, it executes `go run main.go` or `go run cmd/api/main.go` depending on the layout.

---

## 5. `template` 📦
**Usage**: `go-arch template <install|list|remove|update>`

Manages installable template packs from Go module proxies. Packs provide a self-contained set of templates that define the project layout, files, and optional hooks.

### 5a. `template install`
**Usage**: `go-arch template install <module>[@<version>]`

Fetches a pack module via `go mod download -json`, validates the manifest, and materializes it under `~/.go-arch/packs/<name>@<version>/`. If no `@version` is provided, `@latest` is used.

```bash
go-arch template install github.com/org/go-arch-express@1.0.0
```

If the pack declares hooks, a trust warning is printed and the CLI prompts for explicit confirmation before enabling them.

### 5b. `template list`
**Usage**: `go-arch template list`

Lists all installed packs sorted by name:
```bash
go-arch template list
#   echo@0.5.0
#   express@1.0.0
```

### 5c. `template remove`
**Usage**: `go-arch template remove <name>[@<version>]`

Removes an installed pack. Without `@version`, the latest installed version is removed. With a specific version, only that version is removed.

### 5d. `template update`
**Usage**: `go-arch template update <name>`

Re-fetches the `@latest` version of a pack. Previously installed older versions are preserved. If the new version declares hooks, the trust warning is re-prompted.

```bash
go-arch template update express
```

---

## 6. `check` 🛡️
**Usage**: `go-arch check`

Validates the project's **Architectural Health**. This command is intended to be used during development and in **CI/CD pipelines**.
- **Structural Integrity**: Checks if the required folders for your selected architecture exist.
- **Dependency Guard (Layer Leaking)**: Scans Go imports to ensure layers are correctly decoupled (e.g., Domain must not import Adapters).
- **Exit Codes**: Returns `1` if violations are found, making it compatible with automation tools.

---

## 7. `workspace` 🗂️

**Usage**: `go-arch workspace <upgrade|check>`

Operates on multiple services defined in a `go-arch.workspace.yaml` at the monorepo root. See [Workspaces](./workspaces.md) for the full reference.

- **`workspace upgrade`** — upgrades every service in declaration order (dry-run by default; `--yes` applies). Continue-on-error with per-service summary.
- **`workspace check`** — runs the architecture check for every service with per-service summary.
- **`--service <name>`** (on `generate`, `check`, `upgrade`) — target a single service from anywhere inside the monorepo.

```bash
go-arch workspace upgrade
go-arch workspace upgrade --yes
go-arch workspace check
go-arch generate crud User --service orders
```

---

## 8. `mcp` 🤖
**Usage**: `go-arch mcp`

Launches the Model Context Protocol (MCP) server over standard I/O (stdio).
- **JSON-RPC 2.0**: Implements the MCP protocol for seamless integration with AI coding agents (such as OpenCode or Claude Desktop).
- **Stderr Routing**: Automatically redirects standard UI logs to stderr to protect the integrity of the JSON-RPC stdout communication channel.

### MCP Tools

Every CLI command has a corresponding MCP tool for agents:

| CLI command | MCP tool | Purpose |
|---|---|---|
| `go-arch new` | `new_project` | Scaffold a new project with layout, DB, Docker, observability, gRPC, and templ+HTMX options |
| `go-arch generate` | `generate_component` | Generate service/repository/handler/crud/page/component, or run pack generators with `generatorArgs` |
| `go-arch generate --list` | `list_generators` | List available generators (pack, builtin, component types) for the current project |
| `go-arch check` | `check_architecture` | Validate the project structure and import rules |
| `go-arch serve` | `serve_project` | Return the exact run command (`air` or `go run <mainPath>`) — agents never start a long-running server over MCP |
| `go-arch setup` | `setup_environment` | Detect Go/air presence; with `install: true` installs only `air` (user-level, no sudo). The Go toolchain itself is never installed by the tool |

`serve_project` is check-only by design: MCP is request/response, so a tool that blocks on a live server would hang the agent. Agents run the returned command themselves and test the project over HTTP.

`setup_environment` follows a consent pattern: the agent asks the human for permission, then calls with `install: true` (installs `air`) or hands the exact install command over.

---

## 9. `doctor` 🩺
**Usage**: `go-arch doctor`

Runs environment diagnostics and reports issues found:
- **Go toolchain**: checks `go version` is available.
- **air (hot-reload)**: checks whether `air` is installed (if not, `go-arch serve` falls back to `go run`).
- **git**: checks whether `git` is installed.
- **Platform**: reports OS/architecture.
- **Project config**: validates the `.go-arch.yaml` (skipped with a finding when not in a go-arch project).

Exits non-zero when any check fails, so it is safe to use in scripts and CI.

---

## 10. `version` 🏷️
**Usage**: `go-arch version`

Prints the build version. When built via GoReleaser (tagged release), the version is injected automatically. Local development builds print `dev`.

---

## 11. Metadata System (`.go-arch.yaml`) 📄

The CLI is stateless, meaning it doesn't store your project data in a database. Instead, it uses this YAML file as the **Source of Truth**.
- **Architecture Locking**: Prevents generating components that don't match the project's initial architecture.
- **Namespace Consistency**: Ensures all new files use the correct `module name` in their imports.

---

## 💡 Pro Tip: Customizing Output
If you want to change how `generate` creates code, remember you can create your own templates in `.go-arch/templates/` (check `ARCHITECTURE.md` for details).
