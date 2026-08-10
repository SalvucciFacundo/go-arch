# Delta for mcp-workspaces

## ADDED Requirements

### Requirement: Workspace List Tool (workspace-list-mcp)

The MCP server MUST expose a `workspace_list` tool with an optional `workspacePath` parameter. When called, the tool MUST resolve the workspace (explicit path wins, else `workspace.Discover(os.Getwd())`), enumerate every service in declaration order, and return a JSON array where each entry contains `name`, `path`, and optional `template`. The tool MUST NOT mutate any files. A workspace that cannot be found or loaded MUST produce a tool-result error (not a JSON-RPC error) with code `workspace_not_found` or `workspace_invalid`.

#### Scenario: List services from explicit path

- GIVEN a valid workspace file at `/repo/go-arch.workspace.yaml` with services `orders` and `users`
- WHEN `workspace_list(workspacePath: "/repo/go-arch.workspace.yaml")` is called
- THEN the result contains two entries with `name`, `path` for each service in declaration order

#### Scenario: List via discovery from subdirectory

- GIVEN a workspace file at `/repo/go-arch.workspace.yaml` and CWD `/repo/services/orders`
- WHEN `workspace_list()` is called with no `workspacePath`
- THEN discovery walks upward and returns the same two services

#### Scenario: No workspace found

- GIVEN a CWD with no `go-arch.workspace.yaml` in it or any parent
- WHEN `workspace_list()` is called
- THEN the tool result contains `{"error": {"code": "workspace_not_found"}}`

#### Scenario: Invalid workspace file

- GIVEN a file at the given `workspacePath` that is not valid workspace YAML (unknown top-level key)
- WHEN `workspace_list(workspacePath)` is called
- THEN the tool result contains `{"error": {"code": "workspace_invalid"}}` naming the offending field

### Requirement: Workspace Upgrade Tool (workspace-upgrade-mcp)

The MCP server MUST expose a `workspace_upgrade` tool with optional parameters `workspacePath`, `service`, and `apply` (default `false`). The tool MUST resolve the workspace, then upgrade services using `scaffold.Upgrade` with `WithRoot(resolvedServicePath)` — no `os.Chdir`. When `service` is omitted, ALL services MUST be upgraded in declaration order. When `service` is set, only that service MUST be upgraded. `apply: false` (dry-run default) MUST return the plan without writing any file; `apply: true` MUST commit per-service and call `WriteVersionField` at each resolved root. The tool MUST continue to the next service when one fails and MUST return a structured per-service JSON result.

#### Scenario: Batch dry-run returns per-service plans

- GIVEN a workspace with services `orders` and `users`, each with upgradable files
- WHEN `workspace_upgrade()` is called with no params
- THEN each service entry in the result contains a `plan` with per-file classifications
- AND no files on disk are modified

#### Scenario: Apply commits per-service

- GIVEN the same workspace and `apply: true`
- WHEN `workspace_upgrade(apply: true)` is called
- THEN each service's upgradable files are written
- AND each service's `.go-arch.yaml` has its version field updated via `WriteVersionField`

#### Scenario: Single-service filter

- GIVEN a workspace with services `orders` and `users`
- WHEN `workspace_upgrade(service: "orders")` is called
- THEN only `orders` is upgraded; `users` is untouched

#### Scenario: One service fails, others continue

- GIVEN a workspace where `orders` upgrade fails but `users` succeeds
- WHEN `workspace_upgrade(apply: true)` is called
- THEN `users` is still upgraded
- AND the result status is `partial` with per-service outcomes

### Requirement: Workspace Check Tool (workspace-check-mcp)

The MCP server MUST expose a `workspace_check` tool with optional parameters `workspacePath` and `service`. The tool MUST resolve the workspace, then for each targeted service: change into the resolved service directory, run the architecture validator, and restore the previous working directory via `defer`. The tool MUST process all targeted services even when one fails (continue-on-error) and MUST return a structured per-service JSON result with `name`, `status`, and either `violations` or `error`.

#### Scenario: Check all services reports per-service violations

- GIVEN a workspace with two services, one with violations
- WHEN `workspace_check()` is called
- THEN both services appear in the result
- AND the violating service lists its violations; the clean service has status `ok`

#### Scenario: Single-service filter

- GIVEN a workspace with services `orders` and `users`
- WHEN `workspace_check(service: "users")` is called
- THEN only `users` is checked

#### Scenario: Continue-on-error preserves all outcomes

- GIVEN a workspace where `orders` check fails but `users` succeeds
- WHEN `workspace_check()` is called
- THEN both services are processed
- AND the top-level status reflects the partial failure

### Requirement: Workspace Resolution in MCP (workspace-resolution-mcp)

Every workspace tool (`workspace_list`, `workspace_upgrade`, `workspace_check`) MUST accept an optional `workspacePath` parameter. When provided, the tool MUST call `workspace.Load(workspacePath)` directly. When omitted, the tool MUST call `workspace.Discover(os.Getwd())` and then `workspace.Load` on the discovered path. An explicit `workspacePath` MUST win over discovery. The resolver MUST be implemented inline in the `mcp` package (no import of `cmd` — cycle prevention).

#### Scenario: Explicit path wins over discovery

- GIVEN a workspace at `/repo/go-arch.workspace.yaml` and a different workspace at `/other/go-arch.workspace.yaml`
- WHEN `workspace_upgrade(workspacePath: "/other/go-arch.workspace.yaml")` is called with CWD inside `/repo`
- THEN the `/other` workspace is used

#### Scenario: Discovery from launch subdir

- GIVEN a workspace at `/repo/go-arch.workspace.yaml` and CWD `/repo/services/orders`
- WHEN `workspace_list()` is called with no `workspacePath`
- THEN the `/repo` workspace is found via upward discovery

### Requirement: MCP Workspace Error Codes (mcp-workspace-errors)

The MCP workspace tools MUST surface business errors as structured `{"error": {"code": "<code>", "message": "<detail>"}}` inside the tool result body (not as JSON-RPC errors). The following codes MUST be used: `workspace_not_found` (no workspace file by flag or discovery), `workspace_invalid` (schema/validation error naming the field), `service_not_found` (named `service` not in workspace), `service_path_missing` (declared path absent on disk), `service_no_manifest` (service directory lacks `.go-arch/manifest.yaml`). Each error MUST name the offending value or path.

#### Scenario: Unknown service name

- GIVEN a workspace without service `billing`
- WHEN `workspace_upgrade(service: "billing")` is called
- THEN the result body contains `{"error": {"code": "service_not_found", "message": "..."}}` naming `billing`

#### Scenario: Service path missing on disk

- GIVEN a service whose `path` does not exist on disk
- WHEN `workspace_upgrade(service: "orders")` is called for that service
- THEN the per-service entry for `orders` has `status: failed` with `error.code: service_path_missing`

#### Scenario: Error is in result body, not JSON-RPC

- GIVEN any of the above error conditions
- WHEN the tool is called
- THEN the JSON-RPC response is successful (no `-326xx` error)
- AND the error lives in the tool-result `content` body

### Requirement: Viper Isolation for Workspace Handlers (mcp-viper-isolation)

Each MCP workspace handler MUST call `viper.Reset()` at entry before any config read. For `workspace_upgrade`, per-service config MUST be loaded via `viper.SetConfigFile(filepath.Join(resolvedServicePath, ".go-arch.yaml"))` + `ReadInConfig()` — without `os.Chdir`. The handler MUST NOT import `cmd.loadServiceConfig` (cycle). After the handler returns, the next request starts from a clean viper state regardless of what the previous request configured.

#### Scenario: Per-service config loaded without chdir

- GIVEN a service at `/repo/services/orders` with its own `.go-arch.yaml`
- WHEN `workspace_upgrade(service: "orders")` is called
- THEN `viper.SetConfigFile("/repo/services/orders/.go-arch.yaml")` is used
- AND no `os.Chdir` occurs

#### Scenario: Next request starts clean

- GIVEN a prior workspace handler that set viper keys from a service config
- WHEN a subsequent `workspace_list()` is called
- THEN viper starts from a reset state (no leftover keys from the previous request)

### Requirement: Structured Per-Service Result (workspace-per-service-result)

`workspace_upgrade` MUST return a JSON result containing a `services` array where each entry has `name` (string), `status` (`success` | `failed` | `skipped`), and either `plan` (per-file classification array for dry-run) or `error` (structured `{code, message}`). The top-level result MUST include an overall `status` field (`ok` | `partial` | `failed`). `workspace_check` MUST return a `services` array where each entry has `name`, `status`, and either `violations` (array) or `error`.

#### Scenario: Upgrade dry-run result shape

- GIVEN a workspace with two services, both with upgradable files
- WHEN `workspace_upgrade()` is called (dry-run default)
- THEN the result contains `status: "ok"` and `services` array with two entries each having `status: "success"` and a `plan` array

#### Scenario: Check result with mixed outcomes

- GIVEN a workspace where one service has violations and one is clean
- WHEN `workspace_check()` is called
- THEN the result `status` is `"partial"`
- AND the violating service has a `violations` array; the clean service has `status: "ok"`

### Requirement: Backward Compatibility (mcp-workspace-backward-compat)

All new parameters (`workspacePath`, `service`, `apply` on the new tools; `service` and `workspacePath` on `upgrade_project`) MUST be optional. When none of the new parameters are supplied, existing tools MUST behave byte-identically to their current behavior. The existing 11 MCP tools MUST remain registered and functional with no schema or behavioral change. Tool count grows from 11 to 14.

#### Scenario: Existing tools unchanged

- GIVEN an MCP client calling `upgrade_project(projectPath, dryRun: true)`
- WHEN the call is made
- THEN behavior is identical to the pre-change version

#### Scenario: New tools are additive

- GIVEN the MCP `tools/list` response
- WHEN queried
- THEN it returns 14 tools (the original 11 plus `workspace_list`, `workspace_upgrade`, `workspace_check`)

### Requirement: Empty Workspace Handling (workspace-empty-services)

When a valid workspace file contains zero services, `workspace_list` MUST return an empty array `[]`. `workspace_upgrade` and `workspace_check` MUST return a result with `status: "ok"` and an empty `services` array, performing no work.

#### Scenario: Empty workspace list

- GIVEN a workspace file with `services: []`
- WHEN `workspace_list()` is called
- THEN the result is `[]`

#### Scenario: Empty workspace upgrade is no-op

- GIVEN an empty workspace
- WHEN `workspace_upgrade()` is called
- THEN the result is `{status: "ok", services: []}` with no side effects

### Requirement: No-Manifest Legacy Service Handling (workspace-no-manifest)

When a targeted service directory lacks `.go-arch/manifest.yaml` (legacy project), `workspace_upgrade` MUST treat that service as a no-op and include it in the result with `status: "skipped"` and a warning in the entry. The batch MUST NOT fail because of one legacy service — it MUST continue to the next service.

#### Scenario: Legacy service skipped in batch

- GIVEN a workspace with services `orders` (legacy, no manifest) and `users` (has manifest, upgradable)
- WHEN `workspace_upgrade(apply: true)` is called
- THEN `orders` is reported as `status: "skipped"` with a warning
- AND `users` is still upgraded normally

### Requirement: Inline Workspace Resolver (workspace-inline-resolver)

The `mcp` package MUST implement a thin inline resolver (~15 lines) providing `resolveWorkspace(path)` (returns `*workspace.Workspace` or typed error) and `resolveService(w, name)` (returns absolute service path or typed error). The resolver MUST mirror `cmd.resolveWorkspace`/`cmd.chdirService` semantics without the chdir and MUST NOT import the `cmd` package.

#### Scenario: Resolver finds workspace by path

- GIVEN an explicit `workspacePath`
- WHEN `resolveWorkspace(workspacePath)` is called
- THEN it returns the loaded workspace or a `workspace_not_found` / `workspace_invalid` error

#### Scenario: Resolver finds service by name

- GIVEN a loaded workspace and a valid service name
- WHEN `resolveService(w, "orders")` is called
- THEN it returns the absolute path via `w.ResolvePath(service)`

## MODIFIED Requirements

### Requirement: upgrade_project MCP Tool

The MCP server MUST expose an `upgrade_project` tool with parameters `projectPath` (optional), `dryRun` (default true), `apply` (default false), and new optional parameters `service` (string) and `workspacePath` (string). When `service` and `workspacePath` are both provided, the tool MUST resolve the workspace at `workspacePath`, find the named service, and upgrade ONLY that service using `scaffold.Upgrade` with `WithRoot(resolvedServicePath)` — fully chdir-free. When `service` is provided without `workspacePath`, the tool MUST discover the workspace via `workspace.Discover(os.Getwd())`. When neither `service` nor `workspacePath` is provided, the tool MUST behave identically to the pre-change version (backward compatible). With `dryRun: true` or `apply: false` it MUST return the plan as JSON and mutate nothing. With `apply: true` it MUST perform the classified updates and return the applied plan.

(Previously: `upgrade_project` accepted only `projectPath`, `dryRun`, and `apply` — no workspace awareness.)

#### Scenario: MCP dry-run returns plan JSON

- GIVEN the MCP server is running and the project has upgradable files
- WHEN `upgrade_project` is called with `dryRun: true`
- THEN the result is a JSON plan with per-file classifications
- AND no files on disk are modified

#### Scenario: MCP apply commits changes

- GIVEN the MCP server and `apply: true`
- WHEN `upgrade_project` is called
- THEN upgradable files are written and the returned JSON reflects the applied plan

#### Scenario: MCP default is dry-run

- GIVEN a bare `upgrade_project` call with no flags
- WHEN invoked
- THEN behavior is identical to `dryRun: true`

#### Scenario: Service-scoped upgrade via workspace

- GIVEN a workspace at `/repo/go-arch.workspace.yaml` with service `orders` at `services/orders`
- WHEN `upgrade_project(service: "orders", workspacePath: "/repo/go-arch.workspace.yaml", apply: true)` is called
- THEN only `services/orders` is upgraded using `WithRoot` with no `os.Chdir`
- AND the monorepo root files are untouched

#### Scenario: No workspace params preserves current behavior

- GIVEN no `service` or `workspacePath` provided
- WHEN `upgrade_project(projectPath: "/my/project")` is called
- THEN behavior is byte-identical to the pre-change version
