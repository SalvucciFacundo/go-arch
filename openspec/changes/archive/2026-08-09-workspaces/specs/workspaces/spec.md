# Delta for workspaces

## ADDED Requirements

### Requirement: Workspace File Schema (workspace-file)

The CLI MUST accept an optional `go-arch.workspace.yaml` file at the monorepo root defining a workspace. The file MUST contain a `services:` list; each service entry MUST have a `name` (slug matching `^[a-z0-9]+(-[a-z0-9]+)*$`) and a `path` (relative to the workspace file's directory). A service entry MAY have an optional `template` string. The workspace loader MUST reject unknown top-level keys, unknown service keys, duplicate service names, and missing/invalid name or path with a `workspace_invalid` error naming the offending field.

#### Scenario: Valid workspace file loads

- GIVEN a `go-arch.workspace.yaml` with two services `orders` and `users` with valid slugs and existing relative paths
- WHEN the workspace file is loaded
- THEN the workspace has two services in declaration order
- AND each service has the declared name and path

#### Scenario: Duplicate service name rejected

- GIVEN a workspace file with two services both named `orders`
- WHEN the workspace file is loaded
- THEN an error `workspace_invalid` names the duplicate `orders`

#### Scenario: Unknown service key rejected

- GIVEN a service entry with an unknown key `frobnicate: true`
- WHEN the workspace file is loaded
- THEN an error `workspace_invalid` names the unknown key

### Requirement: Workspace File Discovery (workspace-discovery)

The CLI MUST support two ways to locate a workspace file: an explicit `--workspace <path>` flag pointing at a `go-arch.workspace.yaml`, and automatic discovery walking upward from the current directory (each parent directory, stopping at filesystem root) for a file named `go-arch.workspace.yaml`. An explicit flag wins over discovery. When no workspace file is found by either mechanism and a command requires workspace context, the CLI MUST error with `workspace_not_found`.

#### Scenario: Explicit flag wins over discovery

- GIVEN a monorepo with a root workspace file and a CWD deep inside `services/orders`
- WHEN `go-arch generate crud X --service orders --workspace /repo/go-arch.workspace.yaml` runs
- THEN the explicit workspace file is used

#### Scenario: Auto-discovery from subdirectory

- GIVEN a workspace file at `/repo/go-arch.workspace.yaml` and CWD `/repo/services/orders`
- WHEN a workspace command runs without `--workspace`
- THEN discovery walks up from `/repo/services/orders` to `/repo` and finds the workspace file

#### Scenario: No workspace found

- GIVEN a CWD with no `go-arch.workspace.yaml` in it or any parent
- WHEN a command requires workspace context
- THEN the CLI errors with `workspace_not_found`

### Requirement: Workspace Upgrade (workspace-upgrade)

The `workspace upgrade` command MUST upgrade every service in the workspace file in declaration order, sequentially. For each service, the CLI MUST: resolve the service path relative to the workspace file's directory, verify the path exists, change into the service directory, run the standard upgrade logic, and restore the previous working directory before the next service. The upgrade MUST continue to the next service when one service fails, and MUST print a per-service status summary (success or failure) at the end. The command MUST exit non-zero if any service failed. Each service MUST be upgraded with the same behavior as a standalone `go-arch upgrade` in that directory, including pack-source re-rendering and PROTECTED classification.

#### Scenario: Upgrades all services in order

- GIVEN a workspace with services `orders` and `users`
- WHEN `go-arch workspace upgrade` runs
- THEN both services are upgraded sequentially in declaration order
- AND a per-service status summary is printed
- AND the command exits 0 when both succeed

#### Scenario: One service fails, others continue

- GIVEN a workspace where `orders` upgrade fails but `users` upgrade succeeds
- WHEN `go-arch workspace upgrade` runs
- THEN `users` is still upgraded
- AND the summary reports `orders` failed
- AND the command exits non-zero

#### Scenario: Missing service path

- GIVEN a service whose `path` does not exist on disk
- WHEN `go-arch workspace upgrade` runs
- THEN the CLI errors `service_path_missing` for that service
- AND other services are still processed

### Requirement: Workspace Check (workspace-check)

The `workspace check` command MUST run the architecture check for every service in the workspace file in declaration order, sequentially, with the same chdir-and-restore mechanics and continue-on-error semantics as `workspace upgrade`.

#### Scenario: Checks all services

- GIVEN a workspace with two services
- WHEN `go-arch workspace check` runs
- THEN both services are checked
- AND a per-service summary is printed

### Requirement: Service Flag on Commands (service-flag)

The `generate`, `check`, and `upgrade` commands MUST accept an optional `--service <name>` flag when run from a workspace context. When set, the CLI MUST resolve the named service's path from the workspace file, change into that directory, run the command's normal logic, and restore the previous working directory. A `--service` value not present in the workspace MUST error with `service_not_found` naming the value. A `--service` flag without a resolvable workspace (no `--workspace`, no discovered file) MUST error explaining the flag needs workspace context. Single-service `--service` invocations MUST preserve fail-fast behavior (unlike multi-service workspace commands).

#### Scenario: Generate into a service

- GIVEN a workspace with service `orders` at `services/orders`
- WHEN `go-arch generate crud User --service orders` runs
- THEN the component is generated inside `services/orders`
- AND the previous working directory is restored

#### Scenario: Unknown service name

- GIVEN a workspace without service `billing`
- WHEN `go-arch generate crud User --service billing` runs
- THEN an error `service_not_found` names `billing`

#### Scenario: Service flag without workspace

- GIVEN no workspace file discoverable from CWD and no `--workspace` flag
- WHEN `go-arch generate crud User --service orders` runs
- THEN the CLI errors explaining the flag needs workspace context

### Requirement: Upgrade Root Option (upgrade-root-option)

The `Upgrade` function MUST accept a `WithRoot(root string)` option that sets the project root used for manifest loading and file operations instead of the default `"."` (ADR-7). Existing callers that do not pass the option MUST behave exactly as before. The option MUST mirror the existing `WithResolver` variadic pattern.

#### Scenario: WithRoot changes the root

- GIVEN a service at path `services/orders` and a manifest inside it
- WHEN `Upgrade(cfg, WithRoot("services/orders"))` runs from the monorepo root
- THEN the service's manifest is loaded and its files are upgraded
- AND the monorepo root files are untouched

#### Scenario: No option keeps ADR-7

- GIVEN an existing call site `Upgrade(cfg)`
- WHEN it runs inside a single-project directory
- THEN the root is `"."` exactly as before (byte-identical behavior)

### Requirement: Opt-In Config Isolation (config-isolation)

When a workspace command targets a service, the CLI MUST load the service's own `.go-arch.yaml` (if present) for that operation instead of any config that would otherwise be read from the monorepo root. The CLI MUST restore the prior viper config state after the operation. When NO workspace command or `--service` flag is used, config loading MUST remain byte-identical to current single-project behavior.

#### Scenario: Service config used for generate

- GIVEN a monorepo root `.go-arch.yaml` and a service `.go-arch.yaml` with different settings
- WHEN `go-arch generate crud User --service orders` runs
- THEN the service's `.go-arch.yaml` settings are used
- AND the previous config is restored afterwards

#### Scenario: Single project config unchanged

- GIVEN a single-project directory with `.go-arch.yaml`
- WHEN `go-arch generate crud User` runs (no workspace context)
- THEN behavior is identical to current releases

### Requirement: Hooks CWD Under Chdir (hooks-cwd)

When a workspace command changes into a service directory, generator and lifecycle hooks MUST run with the service directory as their working directory, so hook-created files and `PROJECT_PATH` environment values target the service. This MUST be verified by a live smoke test.

#### Scenario: Hook file lands in the service

- GIVEN a service with a post-generate hook that writes a marker file
- WHEN `go-arch generate crud User --service orders` runs
- THEN the marker file appears inside `services/orders`
- AND `PROJECT_PATH` in the hook environment points at `services/orders`

### Requirement: Continue-On-Error Semantics (continue-on-error)

Multi-service workspace commands (`workspace upgrade`, `workspace check`) MUST continue to the next service after a service failure and MUST report a final summary. Single-service `--service` invocations MUST preserve fail-fast behavior.

#### Scenario: Workspace command continues on failure

- GIVEN a workspace where the first service fails
- WHEN `go-arch workspace upgrade` runs
- THEN the second service is still processed
- AND the summary lists both outcomes
- AND the exit code is non-zero

#### Scenario: Single service fails fast

- GIVEN a service whose upgrade fails
- WHEN `go-arch upgrade --service orders` runs
- THEN the command fails immediately without processing other services

### Requirement: Error Taxonomy (workspace-errors)

The CLI MUST use these oops codes: `workspace_not_found` (no workspace file), `workspace_invalid` (schema/validation error), `service_not_found` (unknown `--service` name), `service_path_missing` (declared path absent on disk), `service_duplicate` (duplicate names at load), `service_no_manifest` (service lacks a manifest; legacy fallback applies). Each error MUST name the offending value or path.

#### Scenario: Each error names its cause

- GIVEN a missing workspace file
- WHEN a workspace command runs
- THEN the error `workspace_not_found` names the searched locations

### Requirement: Workspace Documentation (workspace-docs)

The CLI MUST document the workspace feature in a `docs/workspaces.md` reference covering: the workspace file schema, discovery rules, `workspace upgrade`/`check`, the `--service` flag, chdir semantics, continue-on-error behavior, and the ADR-7 opt-in note (single projects unaffected).

#### Scenario: Docs cover usage

- GIVEN the shipped `docs/workspaces.md`
- WHEN a user reads it
- THEN it explains the schema, commands, and single-project compatibility
