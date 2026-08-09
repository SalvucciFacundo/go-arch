# Hooks Environment And Output

## Purpose

Defines what environment variables are passed to hook processes, how process I/O is handled, and the hard rule that all hook output MUST flow through `ui.Out`.

## Requirements

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

## Scenarios

### Scenario: Env precedence

- GIVEN parent env has `FOO=parent`, hook has `env: { FOO: "hook" }`
- WHEN the hook runs
- THEN `FOO=hook` in the hook process

### Scenario: Standard vars always present even without per-hook env

- GIVEN a hook entry with no `env:` key
- WHEN the hook runs
- THEN `PROJECT_NAME`, `PROJECT_PATH`, `ARCHITECTURE`, `HOOK_TYPE` are all set
