<p align="center">
  <img src="./docs/img/banner.png" alt="Go-Arch CLI Banner" width="100%">
</p>

<p align="center">
  <img src="https://img.shields.io/github/v/release/SalvucciFacundo/go-arch-cli?style=for-the-badge&color=00ADD8" alt="Release">
  <img src="https://img.shields.io/github/license/SalvucciFacundo/go-arch-cli?style=for-the-badge&color=00ADD8" alt="License">
  <img src="https://img.shields.io/github/go-mod/go-version/SalvucciFacundo/go-arch-cli?style=for-the-badge&color=00ADD8" alt="Go Version">
  <img src="https://img.shields.io/badge/OS-Linux%20|%20macOS%20|%20Windows-00ADD8?style=for-the-badge" alt="OS Support">
</p>

---

# Go-Architect CLI (go-arch) 🚀

**Go-Architect CLI** is a professional, agnostic, and multi-platform scaffolding tool designed to standardize Go project initialization. Inspired by the performance and modularity of the Angular CLI, it empowers developers to bootstrap production-ready applications with clean architecture patterns in seconds.

## ✨ Key Features

- 🏗️ **Architecture Layouts**: Native support for **Minimalist**, **Standard**, and **Hexagonal** (Ports & Adapters).
- 🔌 **Agnostic & Decoupled**: Data-driver independent (PostgreSQL, MySQL, MongoDB) and IDE-agnostic.
- ⚡ **Built-in Hot-Reload**: Seamless integration with `Air` for a high-performance development loop.
- 🛠️ **Component Generators**: Scaffold Services, Repositories, Handlers, CRUD, and more mapped to your layout.
- 🌐 **Server-Rendered Frontend**: Optional **templ + HTMX** frontend living in the same binary — no SPA, no Node.
- 🧩 **Frontend Generators**: `generate page` and `generate component` for templ views with HTMX attributes.
- 🐚 **Infrastructure Ready**: Optional **Docker** & **Docker Compose** generation for the app and DB.
- 🧪 **QA & TDD Oriented**: Automatic test file generation with manual mocking patterns.
- 🎨 **Deep Customization**: High-level template system (Global/Local) to override any generated code.
- 🧠 **Smart Pluralization**: Linguistically aware generation (e.g., `Category` -> `Categories`).
- 🛡️ **Living Architecture**: Built-in validation to ensure project integrity over time.
- 🔭 **Multi-Backend Observability**: Built-in OpenTelemetry support for **Jaeger**, **Zipkin**, **Prometheus**, and **SigNoz**.
- 🛰️ **Microservices Ready**: Native **gRPC & Protocol Buffers** integration with automated code generation.
- 🤖 **MCP Server**: Exposes every CLI command as an MCP tool for coding agents (OpenCode, Claude Desktop, etc.).
- 🧊 **Multi-Platform**: Native packages for **Linux (Arch, Debian, Alpine, Fedora)**, **macOS**, and **Windows**.

## 🚀 Installation

### ⚡ Single Command (Recommended)

Install the latest release binary with one command — no build tools required.

**Linux / macOS:**
```bash
curl -fsSL https://raw.githubusercontent.com/SalvucciFacundo/go-arch-cli/main/install.sh | bash
```
Installs to `/usr/local/bin` (or `~/.local/bin` when you don't have write permission, with PATH guidance). Verifies the SHA-256 checksum before installing.

**Windows (PowerShell):**
```powershell
irm https://raw.githubusercontent.com/SalvucciFacundo/go-arch-cli/main/install.ps1 | iex
```
Installs to `~\.go-arch\bin` and adds it to your user PATH.

### 📦 Binary Downloads
Download the latest pre-compiled binaries from the [Releases](https://github.com/SalvucciFacundo/go-arch-cli/releases) page.

### 🐧 Linux (Native Packages)
| Distribution | Install Command |
| :--- | :--- |
| **Arch Linux** | `sudo pacman -U go-arch_*.pkg.tar.zst` |
| **Debian/Ubuntu** | `sudo dpkg -i go-arch_*.deb` |
| **Fedora/RHEL** | `sudo rpm -i go-arch_*.rpm` |
| **Alpine** | `apk add --allow-untrusted go-arch_*.apk` |

### 🍏 macOS / 🪟 Windows
1. Download the latest version from [Releases](https://github.com/SalvucciFacundo/go-arch-cli/releases).
2. **macOS**: Move the binary to `/usr/local/bin/go-arch`.
3. **Windows**: Add the folder containing `go-arch.exe` to your system `PATH`.

### 🛠️ Manual Build (Any OS)
```bash
go install github.com/SalvucciFacundo/go-arch-cli@latest
```

## 📐 Usage Guide

### 1. Project Initialization
Launches an interactive wizard to configure Module Name, Layout, and Database Drivers.
```bash
go-arch new my-project
```

### 2. Development Server
Runs the application. Automatically detects `Air` for hot-reload capabilities.
```bash
go-arch serve
```

### 3. Architecture Health Check
Validates that the project structure and imports follow the selected layout rules.
```bash
go-arch check
```

### 4. Smart Generators
Generates patterns based on your project metadata (detects layout and namespace).
```bash
go-arch generate service Product
go-arch generate repository User
go-arch generate crud Category # Complete CRUD implementation
```

In projects scaffolded with the **templ + HTMX frontend**, you can also generate frontend parts:
```bash
go-arch generate page Dashboard        # views/pages/dashboard.templ
go-arch generate component UserCard    # views/components/usercard.templ (with HTMX attributes)
```

### 5. Model Context Protocol (MCP) Server
Starts a native MCP server communicating over standard input/output (stdio), allowing coding agents (like OpenCode, Claude Desktop, or Gemini) to interact with the CLI tools. Every CLI command has a corresponding tool:
```bash
go-arch mcp
```
- `new_project` → `go-arch new`
- `generate_component` → `go-arch generate` (incl. `page` / `component`)
- `check_architecture` → `go-arch check`
- `serve_project` → `go-arch serve` (returns the exact run command)
- `setup_environment` → `go-arch setup` (detects, and can install `air` with consent)

### 6. Version
Prints the build version. Local builds print `dev`; GoReleaser releases inject the tag automatically.
```bash
go-arch version
```

## 🏗️ Supported Architectures

- **Minimalist**: Thin structure for microservices or single-file scripts.
- **Standard**: Conventional Go layout for mid-sized projects and CLI tools.
- **Hexagonal**: Domain-Centric design for enterprise-grade applications requiring high decoupling.

## 🎨 Customization (External Templates)

You can override any built-in template with your own. The CLI follows this lookup order:
1. **Local**: `./.go-arch/templates/<path>`
2. **Global**: `~/.go-arch/templates/<path>`
3. **Embedded**: Built-in defaults.

Check the [**Architecture Guide**](./docs/ARCHITECTURE.md) for detailed mapping and customization instructions.

## 🐚 Infrastructure & Docker

If **Docker Support** is enabled, the CLI generates:
- **Dockerfile**: Optimized multi-stage build.
- **docker-compose.yaml**: Application + Database + **Observability Backend** (Jaeger, Zipkin, etc.) orchestration.
- **Makefile**: Automation for gRPC code generation (`make proto`) and environment setup.

## 📚 Resources

- [**Architecture Guide**](./docs/ARCHITECTURE.md)
- [**Command Reference**](./docs/COMMANDS.md)

---
Built with ❤️ for the Go Community by [SalvucciFacundo](https://github.com/SalvucciFacundo).
