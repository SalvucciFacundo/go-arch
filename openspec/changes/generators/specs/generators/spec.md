# Delta for generators

## ADDED Requirements

### Requirement: Generators Manifest Key (generator-recipes)

A v2 pack manifest MAY declare a top-level `generators:` key (type: map of generator-name → recipe). The key MUST be registered in `knownManifestKeys` alongside the existing v1 keys. A pack declaring `contract_version: 2` with `generators:` MUST pass strict parsing; a v1 pack (`contract_version: 1`) declaring `generators:` MUST be rejected as `invalid_pack_manifest` (the key is unknown to v1).

#### Scenario: v2 manifest with generators key accepted

- GIVEN a manifest with `contract_version: 2` and `generators: { docker: { steps: [...] } }`
- WHEN loaded by a v2 CLI
- THEN the manifest parses and exposes the generators map

#### Scenario: v1 manifest with generators key rejected

- GIVEN a manifest with `contract_version: 1` and `generators: { ... }`
- WHEN loaded
- THEN the loader returns `invalid_pack_manifest` naming `generators` as unknown key

#### Scenario: Empty generators map accepted

- GIVEN `contract_version: 2` with `generators: {}`
- WHEN loaded
- THEN the manifest parses; the generators map is empty but valid

### Requirement: Recipe Step Vocabulary (generator-recipes)

A recipe is an ordered list of steps. Each step MUST declare a `type` field. The supported step types in contract v2 are: `template`, `binary`, `run`, `prompt`, and `use`. Steps execute in declared order (linear — no conditionals or branching in v2). Each step type has its own required fields:

- `template`: `from` (path relative to pack `templates/`), `to` (path relative to project root).
- `binary`: `from` (path relative to pack root), `to` (path relative to project root), optional `mode` (file permission octal, default `0644`).
- `run`: reuses the `hooks.Entry` shape — `command`, `args`, optional `cwd`, `env`, `timeout`, `silent`, `ignore_failure`.
- `prompt`: `name`, `message`, optional `default`, `required` (boolean, default `false`).
- `use`: value is `builtin/<name>` referencing a CLI-registered builtin generator.

#### Scenario: Recipe with all step types

- GIVEN a generator recipe with steps: `template`, `binary`, `run`, `prompt`, `use: builtin/lint`
- WHEN the recipe is validated
- THEN all steps parse and the generator is valid

#### Scenario: Template step fields

- GIVEN a step `{type: template, from: "common/handler.tmpl", to: "internal/handler/handler.go"}`
- WHEN validated
- THEN the step is accepted with from and to populated

#### Scenario: Run step reuses hooks.Entry shape

- GIVEN a step `{type: run, command: "go", args: ["generate", "./..."], timeout: "30s"}`
- WHEN validated
- THEN the step parses with the same field semantics as `hooks.Entry`

#### Scenario: Prompt step with default

- GIVEN a step `{type: prompt, name: "db_driver", message: "Database driver?", default: "postgres"}`
- WHEN validated
- THEN the step parses with name, message, and default populated

### Requirement: Recipe Validation Rules (generator-recipes)

The recipe parser MUST reject invalid recipes at manifest load time with `invalid_pack_manifest` naming the generator and the failing step index. Validation rules:

- Unknown step `type` → error naming the unknown type.
- `template` or `binary` step missing `from` or `to` → error naming the missing field.
- `prompt` step with unknown fields (anything beyond `name`, `message`, `default`, `required`) → error.
- `use` value not matching `builtin/<name>` pattern → error.
- Duplicate step `name` across `prompt` steps within one recipe → error.
- `to` path that is absolute or contains `..` → deferred to runtime sandbox (see path-sandbox requirement); parse-time validation rejects only structurally invalid paths (empty string).

#### Scenario: Unknown step type rejected

- GIVEN a recipe step `{type: "conditional", ...}`
- WHEN validated
- THEN error `invalid_pack_manifest` names step type `conditional` and step index

#### Scenario: Missing to on template step

- GIVEN `{type: template, from: "x.tmpl"}` with no `to`
- WHEN validated
- THEN error names missing field `to`

#### Scenario: Unknown prompt field rejected

- GIVEN `{type: prompt, name: "x", message: "y", choices: [...]}`
- WHEN validated
- THEN error names unknown field `choices` on the prompt step

#### Scenario: Malformed use reference rejected

- GIVEN `{type: use, value: "external/docker"}`
- WHEN validated
- THEN error names invalid builtin reference

### Requirement: Per-Generator Hooks (generator-recipes)

A generator recipe MAY declare `pre:` and `post:` hook lists (each a list of `hooks.Entry` shapes). These hooks fire around the step-execution phase: `pre` runs before any step, `post` runs after all steps succeed. They are fired through the existing `hooks.Runner`, gated by the same `HooksEnabled` sidecar flag that gates pack-level hooks. `EnvContext` for these hook invocations MUST include `GeneratorName`.

#### Scenario: Pre and post hooks fire around steps

- GIVEN a generator with `pre: [{command: "echo", args: ["start"]}]` and `post: [{command: "echo", args: ["done"]}]`
- WHEN the generator runs with HooksEnabled
- THEN the pre hook fires before step 1 and the post hook fires after the last step

#### Scenario: Post hook skipped on step failure

- GIVEN a generator with a `post` hook and a recipe that fails at step 2
- WHEN the generator runs
- THEN step 1 executes, step 2 fails, and the `post` hook does NOT fire

#### Scenario: Generator hooks require HooksEnabled

- GIVEN a generator with `pre:` hooks and `HooksEnabled: false`
- WHEN the generator runs
- THEN the pre hooks are skipped with a `generator_run_skipped_trust` warning

### Requirement: Linear Step Execution (generator-recipes)

Recipe steps MUST execute in declared declaration order. v2 MUST NOT support conditionals, loops, or branching within a recipe. A recipe with no steps MUST be rejected at validation time (`invalid_pack_manifest`: generator has no steps). The execution engine MUST process each step atomically — a step either completes fully or reports failure.

#### Scenario: Steps execute in order

- GIVEN a recipe with steps [template A, binary B, run C]
- WHEN the generator runs
- THEN A completes, then B, then C

#### Scenario: Empty recipe rejected

- GIVEN a generator with `steps: []`
- WHEN validated
- THEN error `invalid_pack_manifest` names generator with no steps

### Requirement: Step Failure And Partial State (generator-recipes)

When a step fails mid-execution, subsequent steps MUST NOT execute. Files already written by prior completed steps are NOT rolled back — partial state is documented and accepted for v2. The error MUST name the failing step type and index (e.g. `generator_step_failed: step 2 (run) failed: exit code 1`). If the failing step has `ignore_failure: true` (run steps only), execution continues to the next step.

#### Scenario: Step failure stops execution

- GIVEN a recipe with steps [template A, run B, template C] and step B fails
- WHEN the generator runs
- THEN A's file is written, B fails, C is NOT executed
- AND the error names step 2 (run)

#### Scenario: ignore_failure continues execution

- GIVEN a run step with `ignore_failure: true` that returns exit code 1
- WHEN the step executes
- THEN the failure is logged as a warning and execution continues to the next step

#### Scenario: Partial state preserved on failure

- GIVEN a 3-step recipe where step 2 fails
- WHEN the generator fails
- THEN step 1's output files remain on disk (no rollback)
- AND the manifest entry for step 1 (if template-type) is still recorded

### Requirement: Generate Command Resolution Order (generator-dispatch)

`go-arch generate <name>` MUST resolve the name through a three-tier lookup: (1) pack generator — if the project's `.go-arch.yaml` declares `template:` and that pack has a generator named `<name>`; (2) builtin generator — if a CLI-registered builtin generator matches `<name>`; (3) component type — the existing six-type switch in `GenerateComponent`. The first match wins. Resolution MUST NOT cross pack boundaries (a project using pack X only sees pack X's generators).

#### Scenario: Pack generator wins

- GIVEN project with `template: express` and pack `express` declares generator `docker`
- WHEN `go-arch generate docker myservice`
- THEN the pack's `docker` generator runs

#### Scenario: Fallback to builtin

- GIVEN project with `template: express` but the pack has no generator named `auth`
- AND a builtin generator `auth` is registered
- WHEN `go-arch generate auth`
- THEN the builtin `auth` generator runs

#### Scenario: Fallback to component type

- GIVEN project with no generator named `service`
- WHEN `go-arch generate service Order`
- THEN the existing `GenerateComponent("service", "Order")` runs

#### Scenario: No project template skips pack tier

- GIVEN a project without `template:` in `.go-arch.yaml`
- WHEN `go-arch generate <name>`
- THEN resolution starts at the builtin tier (pack tier skipped)

### Requirement: Pack Generator Resolution From Project Config (generator-dispatch)

The pack tier resolves the generator from the project's `.go-arch.yaml` `template:` field. The CLI MUST load the installed pack at the recorded name (latest installed version if no pinned version), verify `HooksEnabled` sidecar, and look up the generator by name in the pack manifest's `generators:` map.

#### Scenario: Resolves pack from template field

- GIVEN `.go-arch.yaml` with `template: express` and `express@1.2.0` installed
- WHEN `go-arch generate docker`
- THEN the CLI loads `express@1.2.0` and finds generator `docker`

#### Scenario: Pinned template version

- GIVEN `.go-arch.yaml` with `template: express@1.0.0`
- WHEN generate runs
- THEN the CLI uses `express@1.0.0` specifically

### Requirement: Name Collision Policy (generator-dispatch)

When a pack generator has the same name as a builtin generator or a built-in component type (`page`, `component`, `service`, `repository`, `handler`, or the default CRUD types), the pack generator MUST win. The builtin/component is silently shadowed — no warning needed because the user explicitly chose the pack via `template:`.

#### Scenario: Pack generator shadows builtin component type

- GIVEN a pack declaring generator `service` and project using that pack
- WHEN `go-arch generate service Order`
- THEN the pack's `service` generator runs (not `GenerateComponent("service")`)

#### Scenario: No pack means no shadow

- GIVEN the same pack installed but project has no `template:` set
- WHEN `go-arch generate service Order`
- THEN `GenerateComponent("service", "Order")` runs (pack tier skipped)

### Requirement: Unknown Generator Error (generator-dispatch)

When no generator matches at any tier, the CLI MUST exit with an `unknown_generator` error that lists all available generator names grouped by source: pack generators (if applicable), builtin generators, and built-in component types. This helps the user discover what's available.

#### Scenario: Unknown generator lists available names

- GIVEN project with `template: express`, pack has generators `docker`, `auth`
- AND builtin generators: `lint`
- WHEN `go-arch generate bogus`
- THEN error `unknown_generator: "bogus"` lists: pack generators (`docker`, `auth`), builtins (`lint`), component types (`page`, `component`, `service`, `repository`, `handler`)

#### Scenario: Unknown generator without pack

- GIVEN no project template
- WHEN `go-arch generate bogus`
- THEN error lists builtins and component types only

### Requirement: Generate --list Output (generator-dispatch)

`go-arch generate --list` MUST print available generators grouped by source. Output MUST include: pack generators (if project uses a pack) with their pack name, builtin generators, and built-in component types. Output MUST be deterministic (sorted within each group).

#### Scenario: List with pack generators

- GIVEN project with `template: express`, pack declares `docker` and `auth`
- WHEN `go-arch generate --list`
- THEN output lists pack generators under `express:` and builtins + component types

#### Scenario: List without pack

- GIVEN no project template
- WHEN `go-arch generate --list`
- THEN output lists builtins and component types only

#### Scenario: Empty builtin registry

- GIVEN no builtin generators registered (v2 ships with empty registry)
- WHEN `go-arch generate --list`
- THEN the builtin section shows "no builtin generators registered"

### Requirement: Missing Pack On Generate (generator-dispatch)

When the project's `.go-arch.yaml` declares `template: <name>` but the pack is not installed, `go-arch generate <generator>` MUST error with `pack_not_installed` naming the pack and suggesting `go-arch template install`. Built-in component types MUST still work in this state (they don't need the pack).

#### Scenario: Pack not installed, pack generator requested

- GIVEN `.go-arch.yaml` with `template: express` but `express` is not installed
- WHEN `go-arch generate docker`
- THEN error `pack_not_installed: "express"; run "go-arch template install ..."`

#### Scenario: Pack not installed, builtin component type still works

- GIVEN same state as above
- WHEN `go-arch generate service Order`
- THEN the builtin component generation runs normally (no pack needed)

### Requirement: Path Sandbox (generator-trust)

Every recipe step that writes a file (`template`, `binary`) MUST validate that the resolved target path is inside the project root. The validation MUST reject:

- Absolute target paths (starting with `/` on Unix, drive letter on Windows).
- Paths containing `..` segments that resolve outside the project root.
- Symlink escapes: if the resolved real path (after symlink resolution) is outside the project root.

A step with an invalid target path MUST produce a `recipe_path_escape` error naming the step index and the offending path. Validation MUST be pre-flight (see next requirement).

#### Scenario: Relative path inside project accepted

- GIVEN project at `/home/user/myapp` and step `to: "internal/handler/handler.go"`
- WHEN validated
- THEN the path resolves inside the project and is accepted

#### Scenario: Absolute path rejected

- GIVEN step `to: "/etc/passwd"`
- WHEN validated
- THEN `recipe_path_escape` error names the absolute path

#### Scenario: Dot-dot traversal rejected

- GIVEN project at `/home/user/myapp` and step `to: "../../etc/shadow"`
- WHEN validated
- THEN `recipe_path_escape` error

#### Scenario: Symlink escape rejected

- GIVEN a symlink `myapp/link → /tmp` and step `to: "link/evil"`
- WHEN validated with symlink resolution
- THEN `recipe_path_escape` error (real path is `/tmp/evil`, outside project root)

### Requirement: Path Escape Pre-flight Validation (generator-trust)

Before executing ANY step in a recipe, the engine MUST validate ALL file-writing steps' target paths. If ANY path fails sandbox validation, the recipe MUST abort with `recipe_path_escape` BEFORE writing any file — zero files are written on escape attempt. This prevents partial state from a malicious or misconfigured recipe.

#### Scenario: Pre-flight catches escape before any write

- GIVEN a 3-step recipe where step 1 writes to `ok/file.go` and step 3 writes to `../../etc/bad`
- WHEN the recipe is invoked
- THEN validation rejects step 3's path BEFORE step 1 writes
- AND `ok/file.go` is NOT created on disk

#### Scenario: All paths valid, execution proceeds

- GIVEN a recipe where all target paths resolve inside the project root
- WHEN pre-flight validation runs
- THEN validation passes and execution begins

### Requirement: Run Step Trust Gate (generator-trust)

`run:` steps in a recipe MUST be gated by the `HooksEnabled` sidecar flag for the pack. When `HooksEnabled` is `false`, the engine MUST skip the `run:` step, emit a warning `generator_run_skipped_trust: generator "<name>" step <index> (run) skipped; hooks not enabled for pack "<pack>"`, and continue to the next step.

#### Scenario: HooksEnabled true, run step executes

- GIVEN a pack with `HooksEnabled: true` and a recipe with a `run:` step
- WHEN the generator runs
- THEN the `run:` step executes normally

#### Scenario: HooksEnabled false, run step skipped with warning

- GIVEN a pack with `HooksEnabled: false` and a recipe with a `run:` step
- WHEN the generator runs
- THEN the `run:` step is skipped
- AND warning `generator_run_skipped_trust` is emitted naming the generator, step, and pack
- AND non-run steps (template, binary) still execute

### Requirement: Generator Hooks Trust Gate (generator-trust)

Per-generator `pre:` and `post:` hooks follow the same `HooksEnabled` gate as `run:` steps. When `HooksEnabled` is `false`, both `pre:` and `post:` hook lists MUST be skipped entirely, and a single warning MUST be emitted naming the generator and pack.

#### Scenario: HooksEnabled false, all generator hooks skipped

- GIVEN a generator with `pre:` and `post:` hooks, pack with `HooksEnabled: false`
- WHEN the generator runs
- THEN no hook process is spawned
- AND a single warning names the generator and pack

### Requirement: Install-time Trust Warning Extension (generator-trust)

When `template install` fetches a v2 pack whose manifest declares generators containing `run:` steps or `pre:`/`post:` hooks, the install trust prompt MUST extend the existing hooks warning to explicitly mention command execution from generators. The warning text SHOULD include something like: `⚠ Pack "X" declares generators that may run shell commands. Review before enabling.`

#### Scenario: v2 pack with run steps triggers extended warning

- GIVEN a v2 pack with a generator containing a `run:` step
- WHEN `template install` runs
- THEN the trust warning mentions generator command execution
- AND the existing hooks prompt is shown

#### Scenario: v2 pack with only template steps, no extended warning

- GIVEN a v2 pack whose generators use only `template:` and `prompt:` steps (no `run:`, no hooks)
- WHEN `template install` runs
- THEN the extended command-execution warning is NOT shown (but the standard hooks prompt may still appear if pack-level hooks are declared)

### Requirement: Origin Generator Manifest Entry (generator-provenance)

When a generator produces a file, the scaffolder MUST record a `ManifestEntry` with `origin: generator`, `source: pack:<name>@<version>`, and `metadata: {generator: "<generator-name>", args: <json>}`. The `args` field captures the resolved prompt values and any `generatorArgs` from MCP as a JSON object. For `template:` and `binary:` steps, the entry ALSO records the `template` field (path relative to pack) so the upgrade path can re-render (see dual-recording requirement).

#### Scenario: Generator file records origin and source

- GIVEN generator `docker` in pack `express@1.2.0` creates `docker-compose.yml`
- WHEN the generator runs
- THEN the manifest entry has `origin: "generator"`, `source: "pack:express@1.2.0"`, `metadata.generator: "docker"`

#### Scenario: Args captured in metadata

- GIVEN a generator with prompts and MCP args
- WHEN the generator runs
- THEN `metadata.args` contains the resolved prompt values and generatorArgs as JSON

### Requirement: Protected Default On Upgrade (generator-provenance)

`ManifestEntry` with `origin: generator` MUST be classified as **PROTECTED** during `go-arch upgrade`. PROTECTED entries are never silently overwritten — the upgrade MUST skip them and print a warning per entry explaining that generator output is PROTECTED and the user should re-run the generator manually. This is because generator output is logic output, not template renders, and the v1 re-render guarantee cannot hold.

#### Scenario: Generator entry protected on upgrade

- GIVEN a project with a manifest entry `origin: generator, source: pack:express@1.0.0`
- WHEN `go-arch upgrade` runs
- THEN the entry is classified PROTECTED
- AND a warning is printed: `entry "docker-compose.yml" is PROTECTED (generator output); re-run generator to update`
- AND the file is NOT overwritten

#### Scenario: Pack removed, generator entry still protected

- GIVEN the pack `express@1.0.0` has been removed
- WHEN upgrade runs on the generator entry
- THEN the entry is still PROTECTED (same warning)
- AND no error occurs (PROTECTED is the safe default)

### Requirement: Pure Template Step Dual Recording (generator-provenance)

When a recipe step of type `template:` writes a file, the scaffolder MUST record a SINGLE manifest entry with `origin: template` and `source: pack:<name>@<version>`. The entry's `metadata` MUST include `generator` (the generator name that produced the file) and `args` (resolved prompt/generatorArgs as JSON). This single-entry approach preserves both upgrade re-renderability (via `origin: template`) and full generator provenance for audit (via `metadata.generator` + `metadata.args`).

> **Refinement (applied during implementation)**: The original spec proposed dual entries (one `origin: generator`, one `origin: template`). During implementation this was refined to a single entry with `origin: template` and `metadata.generator`/`metadata.args` carrying the generator audit trail. This preserves the key guarantee — template steps are re-renderable on upgrade — while avoiding the complexity of duplicate entries per generated file. The design document records this in "Refinements vs Spec."

#### Scenario: Template step records origin template with generator metadata

- GIVEN a recipe with step `{type: template, from: "common/handler.tmpl", to: "internal/handler/handler.go"}`
- WHEN the generator `docker` runs from pack `express@1.2.0`
- THEN the manifest contains ONE entry for `internal/handler/handler.go`:
  - `origin: template, source: pack:express@1.2.0, template: "common/handler.tmpl"`
  - AND `metadata.generator: "docker"` plus `metadata.args: {...}`

#### Scenario: Template step re-renderable on upgrade

- GIVEN the entry from above (single entry with `origin: template`)
- WHEN `go-arch upgrade` runs with `express@1.2.0` still installed
- THEN the entry is re-rendered via `renderPackEntry` (upgradable)
- AND the generator metadata (generator + args) is preserved as-is

### Requirement: Re-run On Upgrade Deferred (generator-provenance)

Re-running generators automatically during `go-arch upgrade` or `template update` is explicitly **out of scope** for contract v2. The CLI MUST NOT attempt to re-execute generator recipes during upgrade. Users who want updated generator output MUST manually re-run `go-arch generate <name>`. This is documented behavior; a future v2.1 MAY add idempotent generator re-execution.

#### Scenario: Upgrade does not re-run generators

- GIVEN a project with generator-produced files and the pack updated to a newer version
- WHEN `go-arch upgrade` runs
- THEN generator entries remain PROTECTED
- AND no generator recipe is re-executed
- AND the CLI does not error (this is expected behavior)

### Requirement: Generate Component Schema Relaxation (generator-mcp)

The MCP `generate_component` tool MUST relax its `type` parameter from a fixed six-value enum to accept any string. The handler validates the resolved type through the same three-tier dispatch as the CLI (pack → builtin → component type). The tool MUST also accept an optional `generatorArgs` object parameter (JSON map of string → any) for passing prompt values and generator-specific arguments.

#### Scenario: MCP call with pack generator type

- GIVEN an MCP client calls `generate_component` with `type: "docker"` and `generatorArgs: {compose: true}`
- AND the project uses a pack with generator `docker`
- WHEN the call completes
- THEN the pack's `docker` generator runs with `compose: true` in its args

#### Scenario: MCP call with builtin component type

- GIVEN an MCP client calls `generate_component` with `type: "service"`
- WHEN the call completes
- THEN the builtin component generation runs (same as CLI)

#### Scenario: MCP call with unknown type

- GIVEN an MCP client calls `generate_component` with `type: "bogus"`
- WHEN the call completes
- THEN the MCP response is an error following the same `unknown_generator` contract as the CLI

### Requirement: List Generators MCP Tool (generator-mcp)

The MCP server MUST expose a new `list_generators` tool. The tool MUST return the available generators for the current project context: pack generators (with pack name and description if declared), builtin generators, and built-in component types. The response shape MUST be a structured list (not plain text) so MCP clients can present it programmatically.

#### Scenario: List generators with pack

- GIVEN project with `template: express`, pack declares generators `docker` and `auth`
- WHEN MCP `list_generators` is called
- THEN the response includes pack generators (with source `pack:express`) and builtins + component types

#### Scenario: List generators without pack

- GIVEN no project template
- WHEN MCP `list_generators` is called
- THEN the response includes builtins and component types only

### Requirement: Prompt Values From GeneratorArgs (generator-mcp)

When a generator with `prompt:` steps is invoked via MCP, the prompt values MUST be sourced from the `generatorArgs` object. Each `prompt` step's `name` field is looked up in `generatorArgs`; if found, the value is used as the prompt response (no interactive prompt shown). If a prompt step has `required: true` and the corresponding value is missing from `generatorArgs`, the call MUST fail with `missing_generator_argument` naming the prompt.

#### Scenario: Prompt value provided in generatorArgs

- GIVEN a generator with prompt step `{name: "db_driver", message: "...", required: true}`
- AND MCP call with `generatorArgs: {db_driver: "mysql"}`
- WHEN the generator runs
- THEN the prompt resolves to `"mysql"` without interactive input

#### Scenario: Prompt with default, not in generatorArgs

- GIVEN a prompt step `{name: "db_driver", default: "postgres", required: false}`
- AND MCP call without `db_driver` in generatorArgs
- WHEN the generator runs
- THEN the prompt resolves to `"postgres"` (the default)

### Requirement: Missing Generator Argument Error (generator-mcp)

When a required prompt value is not provided via `generatorArgs` in an MCP call, the tool MUST return a `missing_generator_argument` error naming the prompt's `name` field. The error message SHOULD suggest adding the value to `generatorArgs`.

#### Scenario: Required prompt missing from generatorArgs

- GIVEN a generator with prompt `{name: "db_driver", required: true}`
- AND MCP call with empty `generatorArgs: {}`
- WHEN the tool runs
- THEN error `missing_generator_argument: "db_driver"` is returned
- AND no generator steps execute

#### Scenario: Non-required prompt missing, no default

- GIVEN a prompt `{name: "optional_note", required: false}` with no `default`
- AND MCP call without `optional_note` in generatorArgs
- WHEN the generator runs
- THEN the prompt resolves to empty string (no error)

### Requirement: Non-Interactive Prompt Error (generator-trust)

When a generator with a required `prompt:` step runs in a non-interactive context (no TTY, no MCP `generatorArgs` value), the engine MUST fail with a clear error naming the unresolved prompt. The engine MUST NOT block waiting for stdin in a non-interactive context.

#### Scenario: Required prompt without TTY and no args

- GIVEN a generator with `{name: "db_driver", required: true}` and no default
- AND the CLI runs in non-interactive mode (piped stdin)
- WHEN the generator runs
- THEN error `generator_prompt_unresolvable: "db_driver"` and no steps execute

#### Scenario: Prompt with default in non-interactive uses default

- GIVEN a prompt `{name: "driver", default: "postgres"}`
- AND non-interactive context
- WHEN the generator runs
- THEN the prompt resolves to `"postgres"` without error

### Requirement: Unknown Builtin Reference (generator-recipes)

A `use: builtin/<name>` step referencing a name not registered in the CLI's builtin generator registry MUST produce an `unknown_builtin` error at recipe execution time, naming the unknown builtin. The error MUST list all registered builtin names.

#### Scenario: Unknown builtin at execution time

- GIVEN a recipe step `{use: "builtin/nonexistent"}`
- AND no builtin named `nonexistent` registered
- WHEN the generator runs
- THEN error `unknown_builtin: "nonexistent"` listing registered builtins

#### Scenario: Empty builtin registry, any use step fails

- GIVEN no builtins registered
- AND a recipe step `{use: "builtin/lint"}`
- WHEN the generator runs
- THEN error `unknown_builtin: "lint"` noting no builtins are registered

### Requirement: Template Step Missing Pack Template (generator-recipes)

When a `template:` step references a `from:` path that does not exist in the pack's `templates/` directory, the engine MUST fail the step with `generator_template_not_found` naming the generator, step index, and the missing template path. The error MUST NOT silently fall through to embedded or local/global templates — the pack template is the only valid source for generator template steps.

#### Scenario: Pack template exists

- GIVEN pack `express` with `templates/common/handler.tmpl` and step `{from: "common/handler.tmpl", to: "..."}`
- WHEN the step executes
- THEN the template is read from the pack and written to the target

#### Scenario: Pack template missing

- GIVEN pack `express` without `templates/nonexistent.tmpl` and step `{from: "nonexistent.tmpl", to: "..."}`
- WHEN the step executes
- THEN error `generator_template_not_found` names the generator, step index, and `nonexistent.tmpl`
- AND no fallback to embedded/local/global occurs

### Requirement: Contract Version Mismatch V2 Pack On Old CLI (generator-recipes)

A CLI that only supports contract v1 (pre-generators release) encountering a v2 pack MUST reject it with the existing `contract_version_mismatch` error. The error MUST name the pack's required version (2) and the CLI's supported version (1). This is the existing error path — no new error code is needed, but the message wording MUST remain backward-compatible.

#### Scenario: v1 CLI rejects v2 pack

- GIVEN a CLI supporting only contract v1
- AND a pack with `contract_version: 2`
- WHEN the pack is loaded (install or generate)
- THEN error: `pack "X" requires contract v2; this CLI supports v1. Upgrade go-arch.`

## MODIFIED Requirements

### Requirement: Contract Version Supported Set

The CLI MUST declare a supported set of contract versions `{1, 2}` (not a single constant). When loading a pack manifest, if `contract_version` is not in the supported set, the CLI MUST abort with a hard error naming both the pack's required version and the CLI's supported range, e.g. `pack "X" requires contract vN; this CLI supports v1–v2. Upgrade go-arch.`

(Previously: The CLI declared a single `SupportedContractVersion = 1` constant and compared with `!=`; error message named only one supported version.)

#### Scenario: v1 pack accepted

- GIVEN the CLI supports {1, 2} and the manifest declares `contract_version: 1`
- WHEN loaded
- THEN the manifest is accepted

#### Scenario: v2 pack accepted

- GIVEN the CLI supports {1, 2} and the manifest declares `contract_version: 2`
- WHEN loaded
- THEN the manifest is accepted

#### Scenario: Unsupported version rejected

- GIVEN the CLI supports {1, 2} and the manifest declares `contract_version: 99`
- WHEN loaded
- THEN the CLI aborts with error containing `contract v99` and `v1–v2`

#### Scenario: Missing contract_version rejected

- GIVEN a manifest with no `contract_version` field
- WHEN loaded
- THEN the loader returns `invalid_pack_manifest`

### Requirement: Generator Name Env Var In Hook Context

When a pack generator hook fires (pre or post), the `EnvContext` MUST include a `GeneratorName` field. The hooks runner MUST inject `GENERATOR_NAME=<generator-name>` into the process environment for that hook invocation. When no generator context is active (e.g. project-level hooks or `new --template` hooks), `GENERATOR_NAME` MUST NOT be set.

(Previously: `EnvContext` carried only `PackName` and `PackVersion`; no generator-scoped env var existed.)

#### Scenario: Generator hook receives GENERATOR_NAME

- GIVEN a pack with generator `docker` and `HooksEnabled: true`
- WHEN a pre or post hook fires for the generator
- THEN the process environment includes `GENERATOR_NAME=docker` alongside `PACK_NAME` and `PACK_VERSION`

#### Scenario: Non-generator hook does not receive GENERATOR_NAME

- GIVEN a pack-level `post-new` hook firing during `new --template`
- WHEN the hook runs
- THEN `GENERATOR_NAME` is NOT set in the hook environment

#### Scenario: GENERATOR_NAME not leaked between invocations

- GIVEN two generators `docker` and `auth` running sequentially
- WHEN `docker`'s post hook fires, then `auth`'s pre hook fires
- THEN the first invocation sees `GENERATOR_NAME=docker`
- AND the second sees `GENERATOR_NAME=auth`
- AND the env does not leak between them
