# Hooks MCP Parity, Upgrade Non-Goal, And Docs

## Purpose

Defines how hooks interact with the MCP transport, confirms the upgrade-command non-goal, and specifies CLI-level integration points (flags, exit codes, docs).

## Requirements

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

## Scenarios

### Scenario: MCP call with hook that writes stdout

- GIVEN a hook `pre-generate: ["echo noisy"]` running under MCP
- WHEN `generate_component` is called
- THEN `noisy` appears on stderr (via `ui.Out` redirect)
- AND the JSON-RPC stdout stream is not corrupted

### Scenario: upgrade with hooks defined is silent

- GIVEN a project with all four hook types configured
- WHEN `go-arch upgrade --yes` runs
- THEN no hook output appears and exit code is 0
