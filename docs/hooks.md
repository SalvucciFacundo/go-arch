# Hooks — Lifecycle Extensibility 🔧

Hooks let you run shell commands automatically when the CLI scaffolds or generates code. They live in `.go-arch.yaml` under the optional `hooks:` key.

---

## ⚠️ Trust Warning

**`.go-arch.yaml` is an executable surface.** Hooks run arbitrary commands with your shell and inherit your environment. Treat the config file like npm scripts, a `Makefile`, or any other code you would review before committing to source control.

---

## 🪝 Fire Sites

| Hook type | Runs when | CWD |
|---|---|---|
| `pre-new` | After the `new` wizard completes, before any file is created | Invocation directory (project dir does not exist yet) |
| `post-new` | After all project files are scaffolded and `go_arch_version` is written to `.go-arch.yaml` | New project directory (absolute) |
| `pre-generate` | After `generate` config validation, before component/crud dispatch | Project root |
| `post-generate` | After `generate` completes (including routes-registry re-render) | Project root |

`upgrade` never fires hooks.

---

## 📋 Schema

Each hook type accepts a YAML list. Every entry can be either a **string** (shell shorthand) or an **object**:

```yaml
hooks:
  pre-new:
    - echo "Starting scaffold…"          # string = shell shorthand

  post-new:
    # Object form (all fields)
    - command: go                        # required
      args: [mod, tidy]                  # optional
      cwd: "."                           # optional — relative to default CWD
      env:                               # optional — merged on top of standard vars
        GOWORK: off
      timeout: 60s                       # optional — default 30s; 0 disables
      silent: false                      # optional — suppress stdout/stderr
      ignore_failure: false              # optional — continue on non-zero exit

    # Mixed list: string + object entries execute in order
    - gofmt -w .
```

### Object fields

| Field | Type | Default | Description |
|---|---|---|---|
| `command` | string | *(required)* | Binary name or path |
| `args` | []string | `[]` | Argv for direct execution |
| `cwd` | string | `""` | Working directory, resolved relative to the default fire-site CWD |
| `env` | map | `{}` | Extra env vars — wins over standard vars |
| `timeout` | duration | `30s` | Per-entry timeout; `0` disables |
| `silent` | bool | `false` | When true, stdout and stderr are discarded |
| `ignore_failure` | bool | `false` | When true, a non-zero exit logs a warning and continues |

Unknown object keys are rejected at config load time.

---

## 🔤 Shell vs Argv

| Form | Dispatch | Env variable expansion |
|---|---|---|
| **String** | `sh -c "<string>"` (unix) / `cmd /c "<string>"` (Windows) | Shell expands `$VAR`, `%VAR%`, etc. |
| **Object** | Argv-direct (`command` + `args`) | Literal strings — no shell interpolation |

Prefer object form for cross-platform portability. String form is convenient for quick one-liners.

---

## 🌍 Standard Environment Variables

Every hook process receives these env vars (inherits parent, then merged):

| Variable | Value |
|---|---|
| `PROJECT_NAME` | From `.go-arch.yaml` `project_name` |
| `PROJECT_PATH` | Absolute project directory (
`pre-new`: invocation dir; `post-new`: new project dir) |
| `ARCHITECTURE` | `Minimalist`, `Standard`, or `Hexagonal` |
| `HOOK_TYPE` | `pre-new`, `post-new`, `pre-generate`, or `post-generate` |

Precedence: parent env → standard vars → per-hook `env:` overrides (last wins).

---

## ⏱️ Timeout Policy

- Default timeout: **30 seconds** per hook.
- Per-hook `timeout:` overrides the default.
- `timeout: 0` disables the timeout (process runs indefinitely).
- On timeout, the process receives a kill signal via `exec.CommandContext` and the error carries oops code `hook_timeout`.
- The timeout is per-entry, not cumulative across the list.

---

## 🛑 Failure Semantics

- **Stop on first failure** — if a hook exits non-zero and does not have `ignore_failure: true`, remaining hooks in the list are skipped and the CLI exits with code 1 (oops code `hook_failed`).
- **`ignore_failure: true`** — a warning is printed and execution continues to the next entry.
- **Command not found** (`exec.ErrNotFound`) exits with `hook_command_not_found`.

---

## 🔌 MCP Behaviour

When hooks run under an MCP tool call (`new_project`, `generate_component`):

- Hooks fire identically to CLI mode — the runner is wired at the scaffold layer.
- **Hook output goes to stderr** (`ui.Out` is redirected to `os.Stderr` in MCP mode). The JSON-RPC stdout stream is never corrupted.
- **Hooks are non-interactive**: stdin is closed (reads EOF immediately). A hook that tries to read stdin will not block.
- MCP does NOT skip hooks, suppress them, or detect MCP mode — they run the same as in CLI.

---

## 🧩 Non-Atomic `post-*` Failure

A failing `post-new` or `post-generate` hook leaves the scaffolded/generated files on disk. The CLI exits non-zero, but the project directory is not rolled back. Set `ignore_failure: true` on cleanup hooks if you want to avoid blocking the workflow.

---

## 📝 Example: Full Hybrid Config

```yaml
hooks:
  pre-new:
    - echo "Scaffolding $(date)"
  post-new:
    - command: go
      args: [mod, tidy]
      timeout: 60s
    - command: git
      args: [init]
      ignore_failure: true
  pre-generate: []
  post-generate:
    - gofmt -w .
    - command: go
      args: [vet, ./...]
      silent: true
      ignore_failure: true
```
