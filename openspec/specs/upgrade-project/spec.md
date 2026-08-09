# upgrade-project Specification

## Purpose

Propagate embedded template changes to previously generated projects without clobbering user edits, using a generation-time fingerprint manifest, a dry-run-default `upgrade` command, a legacy whitelist fallback, and an `upgrade_project` MCP tool.

## Requirements

### Requirement: Manifest Recording on Scaffold Writes

Every scaffold write path (`new`, `generate component`, `generate crud`, and their MCP equivalents) MUST record each written file in `.go-arch/manifest.yaml` with `{path, sha256, origin}` where `origin ∈ {scaffold, component, crud, binary}`. Binary writes (e.g. `htmx.min.js`) MUST also be recorded. The manifest is created on first write and updated atomically after each successful write.

#### Scenario: `new` records common + architecture files

- GIVEN a fresh project generated via `go-arch new`
- WHEN scaffolding completes
- THEN `.go-arch/manifest.yaml` exists and contains an entry for each file written by `createFile` and `createBinaryFile` with matching sha256 fingerprints and `origin: scaffold` or `origin: binary`

#### Scenario: `generate component` appends an entry

- GIVEN an existing project with a valid manifest
- WHEN `go-arch generate service Order` runs successfully
- THEN the manifest gains a new entry for the generated file with `origin: component`

#### Scenario: `generate crud` appends per-architecture entries

- GIVEN an existing project with a valid manifest
- WHEN `go-arch generate crud Order` runs
- THEN the manifest gains entries for every CRUD file written (per-architecture map) with `origin: crud`

#### Scenario: Manifest survives round-trip

- GIVEN a written manifest
- WHEN loaded via `manifest.Load`
- THEN each entry's sha256 matches the on-disk file hash

### Requirement: Upgrade Classification

`Upgrade(cfg)` MUST re-render each manifest entry via the full engine chain (local → global → embedded) and classify each file as exactly one of:

| Class | Condition | Action |
|-------|-----------|--------|
| upgradable | disk sha256 == manifest fingerprint AND re-render sha256 != disk sha256 | propose overwrite |
| user-modified (PROTECTED) | disk sha256 != manifest fingerprint | report, NEVER overwrite |
| absent | file missing on disk | report, do not recreate by default |
| up-to-date | disk sha256 == manifest fingerprint AND re-render sha256 == disk sha256 | no action |

#### Scenario: Untouched file classified upgradable

- GIVEN a manifest entry where the on-disk file matches the recorded fingerprint
- AND the re-rendered bytes differ from the on-disk bytes
- WHEN `Upgrade` classifies
- THEN the file is reported as `upgradable`

#### Scenario: User-edited file classified PROTECTED

- GIVEN a manifest entry where the on-disk file differs from the recorded fingerprint
- WHEN `Upgrade` classifies
- THEN the file is reported as `PROTECTED` and is NOT in the apply set

#### Scenario: Absent file reported only

- GIVEN a manifest entry whose path does not exist on disk
- WHEN `Upgrade` classifies
- THEN the file is reported as `absent` and is NOT recreated

#### Scenario: Up-to-date file produces no plan entry

- GIVEN a manifest entry where re-render, disk, and fingerprint all match
- WHEN `Upgrade` classifies
- THEN no action is reported for that file

### Requirement: Upgrade Apply is Compare-Then-Write

Applying an upgradable update MUST render new bytes to a buffer, compare against current disk bytes, and write ONLY when they differ. `os.Create` blind truncation MUST NOT be used. After a successful apply the manifest fingerprint for that path MUST be refreshed to the new sha256.

#### Scenario: Apply writes only when different

- GIVEN an upgradable file
- WHEN `--yes` is supplied and apply runs
- THEN the file is overwritten with the re-rendered bytes
- AND the manifest entry is updated with the new sha256

#### Scenario: Apply idempotent on clean tree

- GIVEN a freshly applied upgrade
- WHEN upgrade runs again
- THEN the plan reports zero upgradable files

### Requirement: go-arch_version Field in .go-arch.yaml

`.go-arch.yaml` MUST gain a `go_arch_version` field written surgically (only that key) and never by wholesale re-render (because `generated_at: {{ now }}` makes re-render non-idempotent). The field is reporting/gating only; absence is tolerated.

#### Scenario: Version field written surgically

- GIVEN an upgrade that applies at least one change
- WHEN the apply completes
- THEN `.go-arch.yaml` has an updated `go_arch_version` value
- AND all other keys in the file are byte-identical to their pre-apply values

#### Scenario: Missing version field tolerated

- GIVEN a project with no `go_arch_version` key
- WHEN upgrade runs
- THEN upgrade does not fail on that basis alone

### Requirement: Legacy Whitelist Fallback

For projects without `.go-arch/manifest.yaml`, upgrade MUST use a static whitelist of scaffold-owned paths (main.go / cmd/api/main.go, .env, Dockerfile, docker-compose.yaml, Makefile, api/proto, internal/telemetry/\*, internal/adapters/grpc/server.go, static/js/htmx.min.js, static/css/style.css, README.md, scaffold-original views) and prompt per file. `go.mod` is report-only (prints `go get` hints). User-owned paths (`internal/handler|service|repository|domain|model|ports|adapters/*_handler.go|*_repository.go`, user-added views) MUST NOT be auto-applied.

#### Scenario: Legacy per-file confirm

- GIVEN a project with `.go-arch.yaml` but no manifest
- WHEN `go-arch upgrade` runs interactively
- THEN each whitelisted file is presented individually for confirmation
- AND only confirmed files are written

#### Scenario: go.mod is report-only for legacy

- GIVEN a legacy project with a stale `go.mod`
- WHEN upgrade runs
- THEN the plan prints `go get` hints and does not rewrite `go.mod`

### Requirement: Non-TTY Refuses to Prompt

When stdin is not a TTY (CI, MCP), upgrade MUST refuse to prompt. In CLI mode it MUST require `--yes` to apply and otherwise print the plan. In MCP mode, default is dry-run; `apply: true` commits.

#### Scenario: Non-TTY without --yes prints plan only

- GIVEN a TTY-less invocation of `go-arch upgrade`
- WHEN no `--yes` flag is supplied
- THEN the plan is printed to stdout and no files are written
- AND the exit code is 0

#### Scenario: Non-TTY with --yes applies

- GIVEN a TTY-less invocation with `--yes`
- WHEN upgrade runs
- THEN upgradable files are applied

### Requirement: templ generate Hint

After any view or static file update, upgrade MUST print a hint to run `templ generate` (matching the existing `templHint` pattern) and MUST NOT invoke `templ` or `go build` directly.

#### Scenario: Hint printed after view update

- GIVEN an apply that updated `views/**/*.templ` or `static/css/style.css`
- WHEN the apply completes
- THEN stdout contains a `templ generate` hint
- AND the `templ` binary is not executed by upgrade

### Requirement: upgrade_project MCP Tool

The MCP server MUST expose an `upgrade_project` tool with parameters `projectPath` (optional), `dryRun` (default true), `apply` (default false). With `dryRun: true` or `apply: false` it MUST return the plan as JSON and mutate nothing. With `apply: true` it MUST perform the classified updates and return the applied plan.

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

#### Scenario: MCP UI on stderr

- GIVEN an MCP invocation
- WHEN upgrade runs
- THEN informational output goes to stderr, result to stdout, matching existing MCP conventions
