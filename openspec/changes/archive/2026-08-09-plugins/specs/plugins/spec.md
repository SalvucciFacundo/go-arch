# Delta for plugins

This delta introduces **installable template packs** — a versioned, contract-bound template ecosystem — plus a `template` command group, a pack step in the engine lookup chain, provenance on the manifest for upgrade, opt-in pack-declared hooks, and a pack-aware binary-file copy path. It also adds two environment variables (`PACK_NAME`, `PACK_VERSION`) to the hooks env contract.

## ADDED Requirements

### Requirement: Pack Manifest File Shape (pack-contract)

A pack is a directory whose root contains a `go-arch.yaml` manifest. The manifest MUST declare the fields `contract_version` (positive integer), `name` (non-empty slug), `version` (semver string), and MAY declare `hooks` (map of hook-type → list of hook entries) and `layout` (list of directory paths). Unknown top-level keys MUST be rejected.

#### Scenario: Valid minimal manifest

- GIVEN a pack root containing `go-arch.yaml` with `contract_version: 1`, `name: express`, `version: 1.2.0`
- WHEN the manifest is loaded
- THEN the manifest parses without error and exposes those three fields

#### Scenario: Valid manifest with hooks and layout

- GIVEN a manifest with all optional fields present
- WHEN the manifest is loaded
- THEN hooks and layout are exposed as declared

#### Scenario: Unknown top-level key rejected

- GIVEN a manifest containing `bogus_key: "x"` alongside required fields
- WHEN loaded
- THEN the loader returns `invalid_pack_manifest` naming the unknown key

### Requirement: contract_version Enforcement

The CLI MUST declare a single supported `contract_version` constant. When loading a pack manifest, if `contract_version` differs from the CLI's supported value, the CLI MUST abort with a hard error naming both the pack's required version and the CLI's supported version, e.g. `pack "X" requires contract vN; this CLI supports vM. Upgrade go-arch.`

#### Scenario: contract_version match accepted

- GIVEN CLI supports v1 and the manifest declares `contract_version: 1`
- WHEN loaded
- THEN the manifest is accepted

#### Scenario: contract_version mismatch rejected

- GIVEN CLI supports v1 and the manifest declares `contract_version: 99`
- WHEN loaded
- THEN the CLI aborts with an error containing `contract v99` and `v1`

#### Scenario: Missing contract_version rejected

- GIVEN a manifest with no `contract_version` field
- WHEN loaded
- THEN the loader returns `invalid_pack_manifest`

### Requirement: Manifest Required Fields Validation

`name` and `version` are REQUIRED. Missing either MUST return `invalid_pack_manifest`. `name` MUST be a non-empty slug matching `^[a-z0-9-]+$`. `version` MUST be a valid semver string.

#### Scenario: Missing name rejected

- GIVEN `contract_version: 1`, `version: 1.0.0`, no `name`
- WHEN loaded
- THEN `invalid_pack_manifest` with `name` in the error

#### Scenario: Invalid name slug rejected

- GIVEN `name: "Bad Name!"`
- WHEN loaded
- THEN `invalid_pack_manifest`

#### Scenario: Invalid semver rejected

- GIVEN `version: "not-a-version"`
- WHEN loaded
- THEN `invalid_pack_manifest` with `version` in the error

### Requirement: Pack Directory Layout

A pack MUST contain a `templates/` subdirectory. The `templates/` tree MAY be empty, but MUST exist. Binary assets MAY live anywhere outside `templates/` (e.g. `assets/`). Packs without a `templates/` directory MUST fail validation at install.

#### Scenario: Pack with templates/ accepted

- GIVEN a pack with `go-arch.yaml` and `templates/common/handler.tmpl`
- WHEN installed
- THEN the pack is accepted

#### Scenario: Pack without templates/ rejected

- GIVEN a pack with only `go-arch.yaml` at the root
- WHEN install is attempted
- THEN install fails with `pack "X" has no templates directory`

### Requirement: Pack Install Location

Installed packs MUST materialize under `~/.go-arch/packs/<name>@<version>/`. The path is namespaced by both name and version so different versions of the same pack coexist.

#### Scenario: Install creates versioned directory

- GIVEN `go-arch template install github.com/org/go-arch-express@1.0.0`
- WHEN install completes
- THEN `~/.go-arch/packs/express@1.0.0/` contains `go-arch.yaml` and `templates/`

#### Scenario: Two versions of same pack coexist

- GIVEN `express@1.0.0` already installed, then `express@1.1.0` is installed
- WHEN both complete
- THEN both `express@1.0.0/` and `express@1.1.0/` directories exist

### Requirement: template install Command

The CLI MUST expose `template install <module>[@<version>]`. The default version is `@latest`. The fetch MUST run via `go mod download -json <module>@<version>` and copy the resulting module directory into the pack install location. The manifest is re-validated after copy.

#### Scenario: Install latest

- GIVEN no explicit version
- WHEN `template install github.com/org/go-arch-express`
- THEN `@latest` is used and the pack is installed under `express@<resolved>/`

#### Scenario: Install pinned version

- GIVEN `@1.2.0`
- WHEN `template install github.com/org/go-arch-express@1.2.0`
- THEN the pack is installed under `express@1.2.0/`

#### Scenario: Install is idempotent for same version

- GIVEN `express@1.0.0` already installed
- WHEN `template install github.com/org/go-arch-express@1.0.0`
- THEN install succeeds without error (replaces existing directory atomically)

### Requirement: template install Errors

Install MUST surface clear errors and MUST NOT leave partial state:
- Module not found → `pack "X" not found in module proxy`
- Network failure → surface `go mod download` error unchanged
- Corrupted module (ziphash mismatch) → abort before copy
- Post-copy manifest validation failure → remove the newly-copied directory

#### Scenario: Pack not found

- GIVEN a non-existent module path
- WHEN `template install github.com/org/no-such-pack@1.0.0`
- THEN the error contains `not found in module proxy`
- AND no directory is created under `~/.go-arch/packs/`

#### Scenario: Offline install with network failure

- GIVEN the network is unavailable and the module is not in `GOMODCACHE`
- WHEN install is attempted
- THEN the `go mod download` error is surfaced and no directory is created

#### Scenario: Offline install with cached module succeeds

- GIVEN `express@1.0.0` is already in `GOMODCACHE` from a prior install
- WHEN the user removes and re-installs while offline
- THEN install succeeds from the cached module

#### Scenario: Corrupted download aborts before copy

- GIVEN the module cache is truncated/invalid
- WHEN install is attempted
- THEN `go mod download` fails ziphash verification
- AND no directory is created under `~/.go-arch/packs/`

### Requirement: template list Command

`template list` MUST print the installed packs: name, version, install path. Output MUST be deterministic (sorted by name).

#### Scenario: Empty list

- GIVEN no packs installed
- WHEN `template list`
- THEN output contains `no packs installed` (or equivalent) and exit code is 0

#### Scenario: Multiple packs listed

- GIVEN `express@1.0.0` and `echo@0.5.0` installed
- WHEN `template list`
- THEN both are listed, sorted alphabetically by name

### Requirement: template remove Command

`template remove <name>[@<version>]` MUST delete the installed pack directory. Without `@version`, the latest installed version of that name is removed. Without any pack installed, the command MUST error `pack "X" is not installed`.

#### Scenario: Remove latest

- GIVEN `express@1.0.0` and `express@1.1.0` installed
- WHEN `template remove express`
- THEN `express@1.1.0/` is removed and `express@1.0.0/` remains

#### Scenario: Remove specific version

- GIVEN both versions installed
- WHEN `template remove express@1.0.0`
- THEN only `express@1.0.0/` is removed

#### Scenario: Remove non-existent pack

- GIVEN no `express` pack installed
- WHEN `template remove express`
- THEN error `pack "express" is not installed`

### Requirement: template update Command

`template update <name>` MUST re-fetch the `@latest` version of the named pack, replacing the previously installed latest. Previously pinned older versions are preserved.

#### Scenario: Update refreshes latest

- GIVEN `express@1.0.0` installed
- WHEN `template update express` and upstream `@latest` resolves to `1.1.0`
- THEN `express@1.1.0/` is installed
- AND `express@1.0.0/` remains (older pin preserved)

#### Scenario: Update with no upstream change

- GIVEN `express@1.0.0` installed and `@latest` still resolves to `1.0.0`
- WHEN `template update express`
- THEN the directory is re-fetched and the install succeeds (no-op in content)

### Requirement: Go Module Proxy Fetch (pack-contract)

The CLI MUST NOT bundle its own HTTP client or git binary for pack fetch. The fetch path MUST delegate entirely to `go mod download -json` so module integrity (go.sum / ziphash) is enforced by the Go toolchain. The CLI MUST read the `Dir` field of the JSON output and copy from that directory.

#### Scenario: Fetch via go mod download

- GIVEN `go-arch template install github.com/org/go-arch-express@1.0.0`
- WHEN the fetch runs
- THEN the executed command is `go mod download -json github.com/org/go-arch-express@1.0.0`
- AND the `Dir` field of stdout is used as the copy source

#### Scenario: Missing Go toolchain surfaces error

- GIVEN `go` is not on PATH
- WHEN install is attempted
- THEN the error is the exec-not-found error from `go mod download`

### Requirement: Engine Chain With Pack Step (pack-dispatch)

The engine's template lookup chain MUST become: `local > global > pack > embedded`. The pack step resolves `filepath.Join(packsDir, packName, "templates", templatePath)` using the pack's installed directory. The chain returns the source label `"pack:<name>@<version>"` when the pack step wins.

#### Scenario: Pack overrides embedded

- GIVEN pack `express@1.0.0` installed and it contains `templates/common/handler.tmpl`
- WHEN `getTemplate("common/handler.tmpl")` runs with pack scope
- THEN the pack version is returned with source `pack:express@1.0.0`

#### Scenario: Local still overrides pack

- GIVEN `.go-arch/templates/common/handler.tmpl` exists AND the pack has the same file
- WHEN getTemplate runs
- THEN local wins and source is `"local"`

#### Scenario: Global still overrides pack

- GIVEN `~/.go-arch/templates/common/handler.tmpl` exists AND the pack has the same file
- WHEN getTemplate runs
- THEN global wins

#### Scenario: Pack miss falls back to embedded

- GIVEN pack installed but it does NOT contain `common/unknown.tmpl`
- WHEN getTemplate runs
- THEN the embedded copy is returned with source `"embedded"`

#### Scenario: No pack configured

- GIVEN no pack scope active
- WHEN getTemplate runs
- THEN the pack step is skipped and behavior is identical to today's 3-step chain

### Requirement: Precedence And Namespacing (pack-dispatch)

Each pack is looked up by its own `packName` — packs cannot collide. When two packs both provide the same template path, the currently active pack (per `new --template`) wins; there is no cross-pack fallback.

#### Scenario: Two packs installed, only one active

- GIVEN `express@1.0.0` and `echo@0.5.0` both installed
- WHEN `new --template express` runs
- THEN only `express` templates are consulted; `echo` is invisible to this run

#### Scenario: Same template path in two packs — no collision

- GIVEN both packs declare `templates/common/handler.tmpl`
- WHEN `new --template express` runs
- THEN the pack step resolves against `express`, never `echo`

### Requirement: new --template Flag (pack-dispatch)

The `new` command MUST accept a `--template <pack>` flag. When set, the wizard MUST be bypassed entirely — the CLI takes the project name as the only argument, uses the pack manifest's `layout` and `templates/` to scaffold, and records `ProjectConfig.Template = <pack>`. `--template` MUST NOT combine with interactive wizard prompts.

#### Scenario: Flag bypasses wizard

- GIVEN `express@1.0.0` installed
- WHEN `go-arch new myapp --template express` runs
- THEN no wizard prompts are shown
- AND the project is scaffolded from the pack's templates
- AND `ProjectConfig.Template == "express"` in the resulting config

#### Scenario: Flag with explicit version

- GIVEN `express@1.0.0` installed
- WHEN `go-arch new myapp --template express@1.0.0`
- THEN the pinned version is used (no resolution to @latest)

#### Scenario: Template not installed

- GIVEN no `express` pack installed
- WHEN `go-arch new myapp --template express`
- THEN the CLI errors `pack "express" is not installed; run "go-arch template install ..."`

### Requirement: ProjectConfig.Template Field

`ProjectConfig` MUST gain a `Template string` field (mapstructure tag `template,omitempty`). The embedded `common/config.tmpl` MUST document the `template:` field with a commented example. Older `.go-arch.yaml` files (without `template:`) MUST continue to load with the field empty.

#### Scenario: New project records template

- GIVEN `go-arch new myapp --template express`
- WHEN the generated `.go-arch.yaml` is read
- THEN it contains `template: express`

#### Scenario: Old project loads with empty template

- GIVEN a `.go-arch.yaml` without a `template:` key
- WHEN loaded
- THEN `ProjectConfig.Template == ""` and the project behaves as non-pack

#### Scenario: config.tmpl documents the field

- GIVEN a newly scaffolded project
- WHEN `config.tmpl`-derived `.go-arch.yaml` is inspected
- THEN it contains a commented `# template:` example block

### Requirement: MCP new_project.template Param (pack-dispatch)

The MCP `new_project` tool schema MUST accept an optional `template` string parameter mirroring the CLI flag. When set, MCP dispatch MUST bypass the wizard-equivalent defaults and scaffold from the named pack, recording `ProjectConfig.Template`.

#### Scenario: MCP call with template param

- GIVEN an MCP client calls `new_project` with `template: express`
- WHEN the call completes
- THEN the project is scaffolded from the `express` pack
- AND `ProjectConfig.Template == "express"`

#### Scenario: MCP call without template param

- GIVEN an MCP client calls `new_project` without `template`
- WHEN the call completes
- THEN MCP behavior is identical to today (wizard defaults via params)

#### Scenario: MCP call with missing pack

- GIVEN no `express` pack installed
- WHEN MCP `new_project` is called with `template: express`
- THEN the call fails with `pack "express" is not installed`

### Requirement: Engine Output Routes Through ui.Out (pack-dispatch)

The `fmt.Printf` at `engine.go:47` that prints the custom-template notice MUST be replaced with `ui.Out` output so MCP-mode runs preserve the JSON-RPC stdout stream. This applies to ALL non-error engine output (including the pack-render notice).

#### Scenario: CLI mode custom-template notice via ui.Out

- GIVEN a non-embedded template source is used
- WHEN `RenderTo` runs
- THEN the notice is written to `ui.Out`
- AND no `fmt.Printf` to `os.Stdout` occurs

#### Scenario: MCP mode custom-template notice on stderr

- GIVEN the same run under MCP
- WHEN `RenderTo` runs
- THEN the notice appears on stderr (via `ui.Out` redirect)
- AND stdout (JSON-RPC) contains no stray bytes

### Requirement: ManifestEntry.source Field (pack-upgrade)

`ManifestEntry` MUST gain an optional `source` field (`yaml:"source,omitempty"`). For pack-scaffolded files, `source` MUST be `pack:<name>@<version>`. For non-pack files, `source` is omitted (backward compatible with existing manifests).

#### Scenario: Pack-scaffolded file records source

- GIVEN `go-arch new myapp --template express@1.0.0`
- WHEN the manifest is read after scaffold
- THEN each pack-originated entry has `source: "pack:express@1.0.0"`

#### Scenario: Non-pack file omits source

- GIVEN a project scaffolded without `--template`
- WHEN the manifest is read
- THEN entries have no `source` field (omitempty drops it)

#### Scenario: Old manifest without source parses

- GIVEN a pre-existing `.go-arch/manifest.json` with no `source` fields
- WHEN loaded by the new CLI
- THEN the manifest parses successfully and `Source == ""` for all entries

### Requirement: Upgrade Re-Render From Recorded Pack (pack-upgrade)

When `go-arch upgrade` re-renders a `ManifestEntry` with `source: pack:<name>@<version>`, it MUST look up that specific pack@version and render through it. It MUST NOT fall back to any other source (local/global/embedded) when the recorded pack is available.

#### Scenario: Re-render uses recorded pack

- GIVEN a project with entries `source: pack:express@1.0.0`
- WHEN `go-arch upgrade` runs with `express@1.0.0` still installed
- THEN the templates are read from `~/.go-arch/packs/express@1.0.0/templates/`

#### Scenario: Re-render does not substitute embedded

- GIVEN `express@1.0.0` installed and `common/handler.tmpl` exists in both the pack and embedded
- WHEN upgrade re-renders a `source: pack:express@1.0.0` entry
- THEN the pack version is used, not embedded

### Requirement: Missing Pack Marks Entry PROTECTED (pack-upgrade)

If `go-arch upgrade` encounters a `ManifestEntry` whose recorded `source: pack:<name>@<version>` cannot be resolved (pack not installed, or version missing), the entry MUST be classified as **PROTECTED**, a warning MUST be printed per entry naming the missing pack, and the upgrade MUST NOT silently substitute the embedded template.

#### Scenario: Pack removed after scaffold

- GIVEN a project with entries `source: pack:express@1.0.0`
- WHEN `express@1.0.0` has been removed via `template remove`
- AND `go-arch upgrade` runs
- THEN each affected entry is classified PROTECTED
- AND a warning `pack "express@1.0.0" is not installed; entries protected` is printed per entry
- AND no embedded fallback occurs

#### Scenario: Version removed but newer installed

- GIVEN `express@1.0.0` removed, only `express@1.1.0` installed
- WHEN upgrade runs on entries recorded with `@1.0.0`
- THEN those entries are PROTECTED with the missing-pack warning
- AND `@1.1.0` is NOT auto-substituted

### Requirement: Pack Version Bump Behavior (pack-upgrade)

A user MAY opt into a newer pack version via `go-arch template update <name>`. After the new version is installed, a subsequent `go-arch upgrade` on a project that recorded an older version still uses the older recorded version (per `pack-upgrade` above). To switch the project to the new version, the user MUST update `source:` entries (a future command or manual edit — v1 scope: warn that entries still reference the removed version).

#### Scenario: After pack update, old entries still reference old version

- GIVEN project recorded `express@1.0.0`
- AND user runs `template update express` (installing 1.1.0)
- AND `template remove express@1.0.0` (removing 1.0.0)
- WHEN `go-arch upgrade` runs
- THEN entries are PROTECTED with a warning naming `express@1.0.0`

### Requirement: Pack Hooks Opt-In With Trust Warning (pack-hooks)

When `template install` fetches a pack whose manifest declares a non-empty `hooks:` map, the CLI MUST print a trust warning — e.g. `⚠ Pack "X" declares hooks that will run with your shell. Review before enabling.` — and MUST require explicit confirmation. If the user declines, the pack MUST still install but with the `hooks:` section stripped from the in-memory config; the installed `go-arch.yaml` MUST record `hooks_enabled: false` in the pack's metadata sidecar so subsequent operations know hooks were declined.

#### Scenario: Pack with hooks triggers warning

- GIVEN a pack whose manifest contains `hooks: { post-new: [...] }`
- WHEN `template install` runs
- THEN the trust warning is printed
- AND the CLI prompts `Enable hooks from this pack? [y/N]`

#### Scenario: User accepts hooks

- GIVEN the trust warning was printed
- WHEN the user confirms `y`
- THEN the pack installs with hooks enabled

#### Scenario: User declines hooks

- GIVEN the trust warning
- WHEN the user answers `N`
- THEN the pack installs with `hooks_enabled: false` in the pack metadata sidecar
- AND no pack hooks fire on subsequent `new --template`

#### Scenario: Pack without hooks installs silently

- GIVEN a pack whose manifest has no `hooks:` key or it is empty
- WHEN `template install` runs
- THEN no trust warning is printed and no prompt is shown

### Requirement: Pack-Scoped Hook Fire (pack-hooks)

When `new --template <pack>` runs for a pack with hooks enabled, the pack's declared hooks MUST fire at the standard fire sites (pre-new, post-new, etc.) scoped to the pack. They MUST NOT be merged into the project's `.go-arch.yaml` — hooks remain pack-owned.

#### Scenario: Pack post-new hook fires on new --template

- GIVEN a pack with `hooks: { post-new: [{command: "go", args: ["mod", "tidy"]}] }` and hooks enabled
- WHEN `go-arch new myapp --template express` runs
- THEN the post-new hook fires after the scaffold completes
- AND the project `.go-arch.yaml` does NOT contain a `hooks:` block

#### Scenario: Pack hooks do not pollute project config

- GIVEN a project scaffolded from a pack with hooks
- WHEN the generated `.go-arch.yaml` is read
- THEN no `hooks:` block is present

### Requirement: Pack Hooks Never Fire On Project-Level Generate (pack-hooks)

`generate component` / `generate crud` in a pack-scaffolded project MUST NOT fire pack-declared hooks. Pack hooks fire ONLY during `new --template` (the pack's lifecycle entry point). Project-level generate fires only the project's own hooks (which are none, because pack hooks are not merged into project config).

#### Scenario: generate in pack project fires no pack hooks

- GIVEN a project scaffolded from a pack with `post-generate` hooks declared
- WHEN `go-arch generate service Order` runs
- THEN no hook process is spawned

### Requirement: PACK_NAME And PACK_VERSION Env Vars (pack-hooks)

When a pack hook fires, the runner MUST inject two additional environment variables: `PACK_NAME` (the pack's `name` field) and `PACK_VERSION` (the pack's `version` field). These are added to the env context for that hook invocation only — they do not persist between invocations.

#### Scenario: Pack hook receives PACK_NAME and PACK_VERSION

- GIVEN a pack with `name: express`, `version: 1.2.0` and hooks enabled
- WHEN a pack hook fires
- THEN the process environment includes `PACK_NAME=express` and `PACK_VERSION=1.2.0`

#### Scenario: Pack hook still receives standard vars

- GIVEN a pack hook firing
- WHEN the hook runs
- THEN `PROJECT_NAME`, `PROJECT_PATH`, `ARCHITECTURE`, `HOOK_TYPE` are also set alongside `PACK_NAME`/`PACK_VERSION`

#### Scenario: Non-pack hooks do not receive PACK_*

- GIVEN a project without a template (standard scaffold)
- WHEN a project-level hook fires
- THEN `PACK_NAME` and `PACK_VERSION` are NOT set in the hook environment

### Requirement: template update Re-Prompts For Hooks (pack-hooks)

When `template update <name>` fetches a new version whose manifest declares hooks, and the prior version had `hooks_enabled: false`, the CLI MUST re-prompt the trust warning for the new version.

#### Scenario: Prior declined, new version has hooks

- GIVEN `express@1.0.0` installed with hooks declined
- WHEN `template update express` fetches `1.1.0` which declares hooks
- THEN the trust warning is printed again and the user is re-prompted

#### Scenario: Prior accepted, new version has hooks

- GIVEN `express@1.0.0` installed with hooks accepted
- WHEN `template update express` fetches `1.1.0` which declares hooks
- THEN the trust warning is printed and the user is re-prompted (prior acceptance does not carry over)

### Requirement: Pack-Aware createBinaryFile (pack-assets)

The scaffolder MUST provide a pack-aware variant of the binary-file copy path: given a source path relative to the pack (e.g. `assets/htmx.min.js`) listed in the pack manifest's `binary_assets`, the scaffolder reads the bytes directly from the installed pack's directory and copies them verbatim to the destination, recording the entry in the manifest with origin `binary` and `source: pack:<name>@<version>`. Pack-declared binary assets are read from the pack directory (not through the engine's 4-step lookup chain) in contract v1 — this is a documented deviation. The engine's `ResolveBinary` chain resolution API remains available for callers that need the full override chain.

#### Scenario: Binary file copied from pack

- GIVEN a pack with `assets/htmx.min.js` and a template that references it
- WHEN `new --template express` runs
- THEN `htmx.min.js` is copied verbatim to the project
- AND the manifest entry has `origin: binary` and `source: pack:express@1.0.0`

#### Scenario: Binary file from embedded fallback

- GIVEN no pack configured and the engine is asked for a binary asset path
- WHEN the copy runs
- THEN the embedded FS is used as source

#### Scenario: Pack binary assets are read from pack directory

- Pack binary assets declared in the manifest's `binary_assets` list are read directly from the installed pack's `assets/` directory by `createPackBinary`, not through the engine's 4-step lookup chain.
- Local and global overrides do NOT apply to pack-declared binary assets in contract v1. This is a documented design deviation: the pack directory is the authoritative source for manifest-declared assets. The engine's `ResolveBinary` API remains the public entry point for chain-based binary resolution when callers need it.

### Requirement: Empty Pack Error (pack-dispatch)

If `new --template <pack>` is invoked for a pack whose `templates/` directory is empty, the CLI MUST error with `pack "X" has no templates` and MUST NOT create any project directory.

#### Scenario: Empty templates dir

- GIVEN a pack installed with an empty `templates/` directory
- WHEN `go-arch new myapp --template express`
- THEN the CLI errors `pack "express" has no templates`
- AND `myapp/` is not created

### Requirement: Windows Portability (pack-contract)

The pack fetch and install path MUST use only stdlib `path/filepath` and the Go toolchain — no shell-specific fetch, no tar extraction, no POSIX assumptions. Tests MUST run on Windows CI.

#### Scenario: Windows install succeeds

- GIVEN the CLI built for Windows
- WHEN `template install github.com/org/go-arch-express@1.0.0`
- THEN the pack materializes under `%USERPROFILE%\.go-arch\packs\express@1.0.0\`

#### Scenario: Windows path separators handled

- GIVEN a pack with nested template paths
- WHEN installed on Windows
- THEN paths use `\` separators and templates resolve correctly

### Requirement: docs/packs.md Reference

A new file `docs/packs.md` MUST document: the pack contract v1 schema, the install location, the `template` command group, the trust warning for hooks, the engine lookup chain update to four steps, and the upgrade interaction with pack-sourced files.

#### Scenario: docs file exists and covers contract

- GIVEN the repo
- WHEN `docs/packs.md` is read
- THEN it contains a contract schema table, install instructions, and a trust warning section

### Requirement: README And ARCHITECTURE Updates

`README.md` and `docs/ARCHITECTURE.md` MUST update the template lookup order documentation from three steps (local > global > embedded) to four steps (local > global > pack > embedded), and `docs/COMMANDS.md` MUST document the new `template` command group.

#### Scenario: README reflects four-step lookup

- GIVEN the repo after the change
- WHEN `README.md` is read
- THEN the override lookup section lists four steps including "installed packs"

#### Scenario: COMMANDS.md documents template group

- GIVEN the repo after the change
- WHEN `docs/COMMANDS.md` is read
- THEN `template install`, `template list`, `template remove`, `template update` are each documented
