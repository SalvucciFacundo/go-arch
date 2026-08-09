# Delta for hooks

## ADDED Requirements

### Requirement: Hook Types Are Fixed

The `hooks:` map SHALL accept exactly four keys: `pre-new`, `post-new`, `pre-generate`, `post-generate`. Each value is a YAML list of hook entries.

#### Scenario: All four types accepted

- GIVEN a `.go-arch.yaml` with `hooks:` containing all four keys, each mapping to a list
- WHEN the config is loaded
- THEN the runner parses each key without error

#### Scenario: Unknown hook type rejected

- GIVEN a `.go-arch.yaml` with `hooks: { bogus-hook: ["echo hi"] }`
- WHEN the config is loaded
- THEN the loader returns an error with oops code `unknown_hook_type`

#### Scenario: Non-list value rejected

- GIVEN `hooks: { pre-new: "echo hi" }` (scalar instead of list)
- WHEN parsed
- THEN the loader returns `invalid_hook_config`

### Requirement: Hybrid String And Object Entries

Each hook list entry SHALL be either:
- a **string** (shell shorthand), OR
- an **object** with `command` (required string) plus optional `args` (list of strings), `cwd` (string), `env` (map of string→string), `timeout` (duration string, e.g. `60s`), `silent` (bool), `ignore_failure` (bool).

#### Scenario: String shorthand accepted

- GIVEN `hooks: { post-generate: ["gofmt -w ."] }`
- WHEN parsed
- THEN one entry is produced with command `gofmt`, args `[-w, .]`, shell mode true

#### Scenario: Object form accepted

- GIVEN `hooks: { post-generate: [{command: "go", args: ["mod", "tidy"], timeout: "60s"}] }`
- WHEN parsed
- THEN one entry is produced with command `go`, args `[mod, tidy]`, timeout 60s, shell mode false

#### Scenario: Mixed list accepted

- GIVEN a list containing both a string and an object
- WHEN parsed
- THEN both entries are produced in order

#### Scenario: Full hybrid example

- GIVEN `.go-arch.yaml` containing:
  ```yaml
  hooks:
    pre-new:
      - echo "about to scaffold"
    post-new:
      - command: git
        args: [init]
        cwd: "."
        timeout: 30s
    pre-generate: []
    post-generate:
      - gofmt -w .
      - command: go
        args: [mod, tidy]
        ignore_failure: true
  ```
- WHEN parsed
- THEN 4 hook types are registered with 1, 1, 0, and 2 entries respectively

### Requirement: Unknown Object Keys Rejected

Object-form entries MUST NOT contain keys beyond the allowed set listed above. Unknown keys MUST produce a parse error.

#### Scenario: Unknown key in object rejected

- GIVEN `hooks: { post-generate: [{command: "gofmt", unknown_field: true}] }`
- WHEN parsed
- THEN the loader returns an error with oops code `invalid_hook_config`

### Requirement: Command Field Required In Object Form

Object entries without a `command` key MUST fail validation with oops code `invalid_hook_config`.

#### Scenario: Missing command rejected

- GIVEN `hooks: { post-generate: [{args: ["-w", "."]}] }`
- WHEN parsed
- THEN the loader returns `invalid_hook_config`

### Requirement: Timeout Parsing

`timeout` values MUST parse as Go duration strings (`30s`, `2m`, `500ms`). The literal `0` MUST be accepted as "disable timeout". Invalid strings MUST fail with `invalid_hook_config`.

#### Scenario: Valid duration accepted

- GIVEN `timeout: "90s"`
- WHEN parsed
- THEN the timeout is 90 seconds

#### Scenario: Zero disables timeout

- GIVEN `timeout: 0`
- WHEN parsed
- THEN the timeout is disabled (infinite)

#### Scenario: Invalid string rejected

- GIVEN `timeout: "forever"`
- WHEN parsed
- THEN the loader returns `invalid_hook_config`

### Requirement: Empty Or Missing Hooks Key Is No-op

When the `hooks:` key is absent, or a specific hook type has an empty list, the runner MUST NOT execute anything and MUST NOT error.

#### Scenario: Missing hooks key

- GIVEN a `.go-arch.yaml` without a `hooks:` key
- WHEN `new` or `generate` runs
- THEN no hooks run and the command succeeds

#### Scenario: Empty list

- GIVEN `hooks: { pre-new: [] }`
- WHEN `new` runs
- THEN no pre-new hooks run and the command succeeds

### Requirement: Backward Compatibility

Older CLI versions (without hooks support) MUST silently ignore the `hooks:` key when loading `.go-arch.yaml`, because Viper skips unknown top-level keys by default.

#### Scenario: Older CLI ignores hooks key

- GIVEN a `.go-arch.yaml` with a `hooks:` block, loaded by a CLI binary that predates hooks support
- WHEN any command reads the config
- THEN the `hooks:` key is silently ignored and no error is raised

### Requirement: Fire Sites

The runner MUST fire hooks at four sites:
- `pre-new` — after the `new` wizard succeeds, before `Scaffolder.Execute()`
- `post-new` — after `Execute()` and `WriteVersionField` complete successfully, before the final success message
- `pre-generate` — after `generate` config validation succeeds, before scaffolder dispatch
- `post-generate` — after `GenerateComponent`/`GenerateCRUD` returns nil (including the routes-registry re-render), before the final success message

#### Scenario: pre-new fires before scaffold

- GIVEN a project with `pre-new: ["touch /tmp/pre-new-fired"]`
- WHEN `go-arch new myproj` completes the wizard
- THEN the marker file exists before any project directory is created

#### Scenario: post-new fires after scaffold

- GIVEN a project with `post-new: ["ls -la"]`
- WHEN `go-arch new myproj` completes
- THEN the post-new hook runs only after all project files are written

#### Scenario: pre-generate fires before dispatch

- GIVEN a project with `pre-generate: ["echo pre-gen"]`
- WHEN `go-arch generate service Order` runs
- THEN the hook runs before `GenerateComponent` is called

#### Scenario: post-generate fires after routes registry render

- GIVEN a web project with `post-generate: ["cat internal/router/routes.go"]`
- WHEN `go-arch generate crud User` runs
- THEN the hook runs after `renderRoutesRegistry()` completes and sees the updated routes.go

#### Scenario: Happy path all four hooks

- GIVEN `.go-arch.yaml` with all four hook types set to `["echo <type>"]`
- WHEN `go-arch new myproj` runs
- THEN `pre-new` fires, project is scaffolded, `post-new` fires in project dir, command succeeds

#### Scenario: Empty hook list is silent

- GIVEN `hooks: { pre-generate: [] }`
- WHEN `go-arch generate service X` runs
- THEN no hook process is spawned and the command succeeds

### Requirement: CWD Rules

Each hook process MUST run with its working directory set as follows:
- `pre-new` — the invocation directory (the CWD the user ran the CLI from)
- `post-new` — the **newly created project directory** (resolved absolute via `cmd.Dir`, never `os.Chdir`)
- `pre-generate`, `post-generate` — the project root (already CWD per ADR-7)

Per-hook `cwd:` overrides MAY further refine the directory (resolved relative to the default above).

#### Scenario: post-new CWD is project dir

- GIVEN `post-new: ["pwd"]`
- WHEN `go-arch new myproj` runs from `/tmp`
- THEN the hook process CWD is `/tmp/myproj`

#### Scenario: pre-new CWD is invocation dir

- GIVEN `pre-new: ["pwd"]`
- WHEN `go-arch new myproj` runs from `/tmp`
- THEN the hook process CWD is `/tmp` (project dir does not exist yet)

#### Scenario: per-hook cwd override

- GIVEN `post-new: [{command: "ls", cwd: "internal"}]`
- WHEN `go-arch new myproj` runs
- THEN the hook runs with CWD `/tmp/myproj/internal`

### Requirement: Stop On First Failure

By default, if any hook in a list exits with non-zero status, the runner MUST stop processing further hooks in that list and return a failure error with oops code `hook_failed`.

#### Scenario: First failure stops list

- GIVEN `post-generate: ["false", "echo should-not-run"]`
- WHEN `go-arch generate service X` runs
- THEN `echo should-not-run` does NOT execute
- AND the command fails with code `hook_failed`

#### Scenario: post-new failure after writes is non-atomic

- GIVEN `post-new: ["false"]` and a successful scaffold
- WHEN `go-arch new myproj` runs
- THEN the project files remain on disk (non-atomic)
- AND the command exits non-zero with `hook_failed`

### Requirement: ignore_failure Continues

A hook entry with `ignore_failure: true` that exits non-zero MUST NOT abort the remaining hooks. The runner SHOULD log a warning and continue.

#### Scenario: ignore_failure skips and continues

- GIVEN `post-generate: [{command: "false", ignore_failure: true}, "echo continued"]`
- WHEN the runner executes
- THEN `echo continued` runs
- AND a warning is logged about the ignored failure

### Requirement: Default Timeout 30s

Each hook has a default timeout of 30 seconds. If the process exceeds this, the runner MUST kill it (via `exec.CommandContext` cancellation) and return `hook_timeout`. Per-hook `timeout:` overrides the default. `timeout: 0` disables the timeout.

#### Scenario: Default 30s timeout

- GIVEN `pre-new: ["sleep 60"]`
- WHEN the runner executes
- THEN after 30s the process is killed and error code is `hook_timeout`

#### Scenario: Per-hook override

- GIVEN `pre-new: [{command: "sleep", args: ["5"], timeout: "10s"}]`
- WHEN the runner executes
- THEN the hook completes successfully within 10s

#### Scenario: Zero disables timeout

- GIVEN `pre-new: [{command: "sleep", args: ["2"], timeout: 0}]`
- WHEN the runner executes
- THEN the hook completes without timeout enforcement

### Requirement: Shell Vs Argv Dispatch

String-form hooks MUST be executed via the system shell: `sh -c "<string>"` on unix (`runtime.GOOS != "windows"`), `cmd /c "<string>"` on Windows. Object-form hooks MUST be executed argv-direct (no shell interpolation).

#### Scenario: String form via sh on linux

- GIVEN `pre-new: ["echo $HOME"]` on linux
- WHEN the runner executes
- THEN the process is `sh -c "echo $HOME"` and `$HOME` is expanded

#### Scenario: String form via cmd on windows

- GIVEN `pre-new: ["echo %USERNAME%"]` on Windows
- WHEN the runner executes
- THEN the process is `cmd /c "echo %USERNAME%"` and `%USERNAME%` is expanded

#### Scenario: Object form argv-direct

- GIVEN `pre-new: [{command: "echo", args: ["$HOME"]}]`
- WHEN the runner executes on any OS
- THEN argv is `["echo", "$HOME"]` — the literal `$HOME` is printed, not expanded

### Requirement: Command Not Found

If the command binary cannot be resolved (not in PATH), the runner MUST return oops code `hook_command_not_found` without aborting the entire operation with a panic.

#### Scenario: Missing binary

- GIVEN `pre-new: ["nonexistent-binary-xyz arg1"]`
- WHEN the runner executes
- THEN the error carries code `hook_command_not_found`

### Requirement: Standard Environment Variables

Every hook process MUST receive these environment variables:
- `PROJECT_NAME` — the project name from config
- `PROJECT_PATH` — the absolute path of the project directory (for `pre-new`, the invocation-dir absolute path, since the project dir does not exist yet)
- `ARCHITECTURE` — the architecture value from config (e.g. `Minimalist`, `Standard`, `Hexagonal`)
- `HOOK_TYPE` — one of `pre-new`, `post-new`, `pre-generate`, `post-generate`

#### Scenario: All four vars set

- GIVEN a hook entry `env: { DEBUG: "1" }` configured in `post-generate`
- WHEN the hook runs
- THEN its process environment includes `PROJECT_NAME`, `PROJECT_PATH`, `ARCHITECTURE`, `HOOK_TYPE=post-generate`, and `DEBUG=1`

#### Scenario: PROJECT_PATH is absolute

- GIVEN `go-arch new myproj` runs from `/tmp`
- WHEN `post-new` fires
- THEN `PROJECT_PATH` in the hook env is `/tmp/myproj` (absolute)

#### Scenario: Standard vars always present even without per-hook env

- GIVEN a hook entry with no `env:` key
- WHEN the hook runs
- THEN `PROJECT_NAME`, `PROJECT_PATH`, `ARCHITECTURE`, `HOOK_TYPE` are all set

### Requirement: Process Environment Inherited

Hook processes MUST inherit the parent CLI process's environment, with the four standard variables above merged on top (overriding any same-named parent variable). Per-hook `env:` map entries are merged last (winning over both).

#### Scenario: Parent PATH inherited

- GIVEN the CLI process has `PATH=/usr/bin:/usr/local/bin`
- WHEN a hook runs
- THEN the hook process sees that same PATH (plus the four standard vars)

#### Scenario: Per-hook env overrides standard

- GIVEN a hook with `env: { PROJECT_NAME: "override" }`
- WHEN the hook runs
- THEN `PROJECT_NAME` in the process env is `override`

#### Scenario: Env precedence

- GIVEN parent env has `FOO=parent`, hook has `env: { FOO: "hook" }`
- WHEN the hook runs
- THEN `FOO=hook` in the hook process

### Requirement: Stdin Closed

The runner MUST set the hook process's stdin to `io.Discard` (or equivalent closed/empty reader). Hooks MUST NOT be able to read from the CLI's stdin.

#### Scenario: Stdin empty

- GIVEN a hook entry that tries to read stdin (`cat`)
- WHEN the hook runs
- THEN it reads EOF immediately and exits 0 (or fails, but does not block waiting for input)

### Requirement: Output Via ui.Out Only

All hook stdout and stderr output MUST be routed through `ui.Out`. The runner MUST NOT write hook output directly to `os.Stdout`. In MCP mode (`ui.Out` is redirected to `os.Stderr` by `mcp/server.go`), hook output MUST appear on stderr, never stdout.

#### Scenario: CLI mode output via ui.Out

- GIVEN a hook `pre-new: ["echo hello-from-hook"]` run in CLI mode
- WHEN the hook runs
- THEN `hello-from-hook` appears on the same writer as other `ui.*` output

#### Scenario: MCP mode output on stderr

- GIVEN the same hook run via MCP `new_project`
- WHEN the hook runs
- THEN `hello-from-hook` appears on stderr
- AND nothing is written to stdout (JSON-RPC stream is preserved)

#### Scenario: Runner rejects direct os.Stdout usage

- GIVEN a hook implementation that calls `os.Stdout.WriteString` (bypassing `ui.Out`)
- WHEN the test suite runs
- THEN the runner's integration test asserts no bytes reached `os.Stdout`

### Requirement: Silent Flag Suppresses Output

A hook entry with `silent: true` MUST NOT have its stdout/stderr echoed through `ui.Out`. Exit status and errors are still reported normally.

#### Scenario: silent suppresses output

- GIVEN `post-generate: [{command: "echo", args: ["quiet"], silent: true}]`
- WHEN the hook runs
- THEN `quiet` does NOT appear in `ui.Out`
- AND the command succeeds

### Requirement: MCP Parity By Scaffold-Layer Wiring

Hooks MUST fire for MCP `new_project` and `generate_component` calls because the runner is wired in the scaffold layer (`Scaffolder.Execute`, `GenerateComponent`, `GenerateCRUD`), which MCP already invokes. No new MCP-specific code is required for parity.

#### Scenario: MCP new_project fires pre-new and post-new

- GIVEN an MCP client calls `new_project` on a project whose `.go-arch.yaml` defines `pre-new` and `post-new`
- WHEN the call completes
- THEN both hooks ran in the correct order

#### Scenario: MCP generate_component fires pre/post-generate

- GIVEN an MCP client calls `generate_component` in a project with all four hooks defined
- WHEN the call completes
- THEN `pre-generate` and `post-generate` both ran

#### Scenario: MCP call with hook that writes stdout

- GIVEN a hook `pre-generate: ["echo noisy"]` running under MCP
- WHEN `generate_component` is called
- THEN `noisy` appears on stderr (via `ui.Out` redirect)
- AND the JSON-RPC stdout stream is not corrupted

### Requirement: MCP Runs Hooks Non-Interactively

Hook processes under MCP MUST be non-interactive: stdin closed (per `environment.md`), and no prompts from hook commands are expected. Hooks MUST NOT detect MCP-mode and skip themselves — they run the same as in CLI mode.

#### Scenario: Hook never prompts in MCP

- GIVEN a hook entry `pre-new: ["cat"]` under MCP
- WHEN the call executes
- THEN the hook reads EOF on stdin and does not block

### Requirement: Upgrade Fires No Hooks

The `upgrade` command MUST NOT invoke any hooks. This is a non-goal: upgrade modifies templates and the manifest, never project runtime state, and ADR-8 already protects `.go-arch.yaml` from upgrade mutation.

#### Scenario: upgrade ignores hooks

- GIVEN a project with `pre-generate` and `post-generate` hooks defined
- WHEN `go-arch upgrade --yes` runs
- THEN no hook process is spawned

#### Scenario: upgrade with hooks defined is silent

- GIVEN a project with all four hook types configured
- WHEN `go-arch upgrade --yes` runs
- THEN no hook output appears and exit code is 0

### Requirement: CLI Exit Codes

Hook failure MUST propagate as CLI exit code 1 (via `ui.Fatal` after oops-wrapped error). The exit codes are:
- `0` — success (including all hooks passing)
- `1` — any hook failed (non-zero exit) and `ignore_failure` is not set, OR any hook timed out

#### Scenario: Successful run exits 0

- GIVEN all hooks pass
- WHEN `go-arch generate service X` completes
- THEN exit code is 0

#### Scenario: Hook failure exits 1

- GIVEN `post-generate: ["false"]`
- WHEN `go-arch generate service X` runs
- THEN exit code is 1 and the error carries `hook_failed`

#### Scenario: Timeout exits 1

- GIVEN `pre-new: ["sleep 60"]` (default 30s timeout)
- WHEN `go-arch new myproj` runs
- THEN exit code is 1 and the error carries `hook_timeout`

### Requirement: No CLI Flags For Hooks

The hooks system MUST NOT introduce new CLI flags on `new` or `generate`. Hook behavior is controlled exclusively by `.go-arch.yaml`.

#### Scenario: No new flags

- WHEN `go-arch new --help` or `go-arch generate --help` runs
- THEN no hook-related flags are listed

### Requirement: config.tmpl Example

The embedded `common/config.tmpl` MUST include a commented example of the `hooks:` section so users discover the feature.

#### Scenario: Config template has commented hooks block

- GIVEN `go-arch new myproj` completes
- WHEN the generated `.go-arch.yaml` is read
- THEN it contains a commented `# hooks:` block showing the hybrid form

### Requirement: docs/hooks.md Reference

A new file `docs/hooks.md` MUST document:
- The four hook types and their fire sites
- The hybrid config schema
- The trust warning (hooks are executable surface — treat `.go-arch.yaml` like npm scripts)
- The MCP behavior (hooks run; no prompts; output on stderr)
- The non-atomic nature of `post-*` failure

#### Scenario: docs file exists

- GIVEN the repo
- WHEN `docs/hooks.md` is read
- THEN it contains a trust warning section and a schema reference
