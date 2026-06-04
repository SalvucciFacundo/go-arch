# Command Reference Guide 📖

This guide provides a detailed technical explanation of every command available in **Go-Architect CLI**.

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

## 5. `check` 🛡️
**Usage**: `go-arch check`

Validates the project's **Architectural Health**. This command is intended to be used during development and in **CI/CD pipelines**.
- **Structural Integrity**: Checks if the required folders for your selected architecture exist.
- **Dependency Guard (Layer Leaking)**: Scans Go imports to ensure layers are correctly decoupled (e.g., Domain must not import Adapters).
- **Exit Codes**: Returns `1` if violations are found, making it compatible with automation tools.

---

## 6. `mcp` 🤖
**Usage**: `go-arch mcp`

Launches the Model Context Protocol (MCP) server over standard I/O (stdio).
- **JSON-RPC 2.0**: Implements the MCP protocol for seamless integration with AI coding agents (such as OpenCode or Claude Desktop).
- **Tool definitions**: Exposes the key capabilities (`new_project`, `generate_component`, `check_architecture`) directly as MCP tools with formal input schemas.
- **Stderr Routing**: Automatically redirects standard UI logs to stderr to protect the integrity of the JSON-RPC stdout communication channel.

---

## 7. Metadata System (`.go-arch.yaml`) 📄

The CLI is stateless, meaning it doesn't store your project data in a database. Instead, it uses this YAML file as the **Source of Truth**.
- **Architecture Locking**: Prevents generating components that don't match the project's initial architecture.
- **Namespace Consistency**: Ensures all new files use the correct `module name` in their imports.

---

## 💡 Pro Tip: Customizing Output
If you want to change how `generate` creates code, remember you can create your own templates in `.go-arch/templates/` (check `ARCHITECTURE.md` for details).
