# Hooks Execution

## Purpose

Defines the runner semantics: fire sites, execution order, CWD resolution, failure policy, timeout behavior, and shell-vs-argv dispatch.

## Requirements

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

## Scenarios

### Scenario: Happy path all four hooks

- GIVEN `.go-arch.yaml` with all four hook types set to `["echo <type>"]`
- WHEN `go-arch new myproj` runs
- THEN `pre-new` fires, project is scaffolded, `post-new` fires in project dir, command succeeds

### Scenario: post-new failure after writes is non-atomic

- GIVEN `post-new: ["false"]` and a successful scaffold
- WHEN `go-arch new myproj` runs
- THEN the project files remain on disk (non-atomic)
- AND the command exits non-zero with `hook_failed`

### Scenario: Empty hook list is silent

- GIVEN `hooks: { pre-generate: [] }`
- WHEN `go-arch generate service X` runs
- THEN no hook process is spawned and the command succeeds
