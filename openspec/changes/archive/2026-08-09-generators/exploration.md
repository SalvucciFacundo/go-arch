# Exploration: Generators (plugins v2 — executable generation logic in packs)

Status: **success** — feasible. The v1 pack contract (archive `2026-08-09-plugins`) gives us the manifest pipeline, trust sidecar, engine chain, and upgrade provenance to build on. The v2 shape is: `contract_version: 2` + a `generators:` manifest key whose values are **YAML recipe DSLs** executed by a new generator runner inside the CLI (steps: render templates, copy binaries, run hooks-entries, prompt), with an optional registry of well-known builtin generators a recipe can reference. External-binary generators and Go plugins are rejected: Go plugins are unsupported on Windows (dead on arrival for a Windows+Linux CLI) and pack-shipped binaries break the module-proxy distribution model.

## Executive Summary

Executable generation logic in packs is feasible on the current base, but the execution model must be a **declarative recipe DSL** (option c), not arbitrary code. The load-bearing walls from v1 — strict manifest parsing (`internal/pkg/packs/manifest.go`), the sidecar trust flag (`packs/sidecar.go`), the 4-step engine chain (`internal/pkg/template/engine.go`), the pack-aware upgrade resolver (`internal/pkg/scaffold/upgrade.go`), and the scaffold-layer hooks runner (`internal/pkg/hooks/runner.go`) — all extend without rewrites. `generate` today is a hardcoded switch of six built-in types in `GenerateComponent`; a pack generator plugs in as a new branch resolved from the project's `.go-arch.yaml` `template:` field. Contract evolution: bump `contract_version` to 2 (the strict `knownManifestKeys` allow-list already rejects unknown keys, and `docs/packs.md:154-165` promised exactly this). Trust: recipes are a bigger surface than templates (they write files and may run commands) — gate `run:` steps behind the existing `HooksEnabled` sidecar flag and sandbox target paths to the project root. Upgrade: generator-produced files get `origin: generator` + `source: pack:<name>@<ver>` and default to **PROTECTED** (logic output cannot be re-rendered; only pure template-file steps stay upgradable).

## Findings

### 1. Current generate flow — where a pack generator hooks in

- **CLI dispatch** (`cmd/generate.go:19-97`): `generate [type] [name]` with `cobra.ExactArgs(2)` (`:33`); `RunE` validates config (`:40-46`), builds `ProjectConfig` from Viper (`:49-56`), loads hooks (`:62-68`), then dispatches at `:72-79` — `crud` → `GenerateCRUD(name)`, everything else → `GenerateComponent(compType, name, opts...)` with optional `--route` (`:15`, read at `:60`).
- **Validation + dispatch switch** (`internal/pkg/scaffold/scaffold.go`): `GenerateComponent` fires pre-generate (`:421-432`), then switches on `compType` (`:450-523`): `page` (`:451`, requires `use_templ_htmx`, identifier check, collision check), `component` (`:469`), `service` (`:487`, Hexagonal → `internal/domain`), `repository` (`:494`), `handler` (`:501` + route pattern validation `:502-514`), `default` → `unsupported component type` (`:521-523`). `createFile` (`:525`) renders via the engine chain (`:197-221` → `engine.Render`), manifest recorded with `OriginComponent` (`:531-532`).
- **GenerateCRUD** (`scaffold.go:567-653`): pre-generate (`:568-580`), per-architecture file map (`:592-608`), `createFile` loop (`:610-618`), route auto-registration + registry re-render (`:622-636`), post-generate (`:638-650`).
- **Routes registry re-render** (`scaffold.go:669-707`): `renderRoutesRegistry` renders `internal/router/routes.go` from `manifest.Routes` via `RenderTo(..., quiet=true)` (`:685`), compare-then-write (`:695-703`), records manifest (`:705`).
- **MCP parity** (`internal/pkg/mcp/server.go:395-457`): `generate_component` mirrors the same crud-vs-component dispatch (`:443-451`); the tool's `type` enum is the hardcoded six (`:179-183`).
- **Where a pack generator hooks in**: `generate` currently loads NO pack info — only hooks + config (`cmd/generate.go:62-70`). A pack generator needs a new branch: read `.go-arch.yaml` → `template: <name>` (field exists, `internal/ui/prompts.go:19`; recorded on `new --template`), resolve the installed pack via `packs.Path(name, version)` + sidecar, look up the generator in the pack manifest, and dispatch to a new `Scaffolder.GeneratePackGenerator(...)` before the built-in switch. The insertion point is either `cmd/generate.go:72` (dispatch before `GenerateComponent`) or a new `case`-style branch inside `GenerateComponent` — the cmd layer is cleaner because built-in validation (identifier/route/templ-gate) must NOT run for pack generators.

### 2. Pack contract v1 surface — how v2 extends it

- `internal/pkg/packs/manifest.go`: `SupportedContractVersion = 1` (`:15`); `Manifest` struct (`:24-31`) = `contract_version`, `name`, `version`, `layout`, `hooks`, `binary_assets`; `knownManifestKeys` strict allow-list (`:41-48`) — **unknown top-level key → `invalid_pack_manifest`** (`:69-73`); contract mismatch → hard error naming both versions (`:134-139`); slug + semver validation (`:151-155`, `:167-171`).
- `docs/packs.md:23-64` documents v1; `docs/packs.md:154-165` already commits to the v2 strategy: "When v2 is needed, it will carry a new `contract_version` and the CLI will reject packs that require it until the upgrade ships."
- **Extension options**:
  - **Bump to `contract_version: 2`** + add `generators:` to `knownManifestKeys`. Old CLI rejects with the clean existing `contract_version_mismatch` error (`manifest.go:134-139`). Real code change: the v2 CLI must accept the set {1, 2}, not a single constant — `SupportedContractVersion` becomes a supported-range/set check, and the error message must say "supports v1–v2".
  - **Additive key, keep `contract_version: 1`**: a shipped v1 CLI still rejects the pack — but with the confusing `unknown key "generators"` error instead of a version error; and it muddies the v1 meaning ("templates-only"). Worst case, a lenient parser would silently ignore the generators — the exact silent-divergence failure the contract exists to prevent.
  - Recommendation: **bump to 2**. It matches the documented promise, gives the clearest error, and keeps v1 packs (declaring 1) working unchanged on the v2 CLI.

### 3. Execution model options

- **(a) External binary in the pack** (hooks-like but richer contract): arbitrary power, any language, no DSL ceiling. But: a Go module is platform-agnostic while binaries are platform-specific — `go mod download` materializes the SAME module dir on Linux and Windows, so a pack cannot cleanly ship `linux/amd64` + `windows/amd64` binaries without multi-module or build-from-source machinery; native code is the largest trust surface possible (unreviewable, no sandbox); Windows signing/AV friction. The CLI already has this escape hatch in weaker form: hooks run arbitrary commands.
- **(b) Go plugin (`.so`)**: in-process, type-safe — but Go's `plugin` package is **unsupported on Windows** (confirmed via `go doc plugin`; host binary must also match the plugin's exact Go version and build flags, which is impractical for a distributed CLI). **Reject — dead on arrival** for a Windows+Linux target.
- **(c) Declarative mini-DSL** (YAML recipe: ordered steps — create files from templates, copy binary assets, run command steps, ask prompts, conditionals): portable (data, not code), sandboxable (path + step allow-lists), introspectable (CLI completions and MCP schemas can be derived from the manifest — a generator can declare its own flags), deterministic and testable, versionable (recipe schema versioned inside the pack manifest). Cons: expressiveness ceiling — complex branching gets awkward (Angular went beyond pure DSL to TS functions for a reason); DSL design + validation + executor is real work.
- **(d) Registry of well-known generator types** the CLI implements (e.g. `docker`, `auth`, `jwt`), pack supplies config + templates: full Go power, deterministic, zero new sandbox, trivial upgrade story, CLI-reviewed code. Cons: the logic does NOT live in the pack — it contradicts the roadmap's "executable generation logic in packs" framing; the ecosystem is limited to what CLI releases ship. Notably, today's `GenerateComponent` switch IS already a small registry of well-known generator types.
- **Recommendation — layered (c) core + (d) builtins + (a)-style escape hatch**: v2 generators are **YAML recipes** executed by a new generator runner in the CLI. A recipe's steps include: `template:` (render a pack template file → target), `binary:` (copy pack binary asset), `run:` (delegate to the EXISTING `hooks.Entry` machinery — command/args/cwd/env/timeout/silent/ignore_failure — giving external-binary power without a new process protocol), and `prompt:` (declared inputs). Builtin well-known generators (`docker`, `auth`) are Go functions registered in a registry that recipes reference via `use: builtin/<name>`. This gives portability (recipes are data), power (run steps + builtins), introspectability (MCP/CLI), and reuse of the entire v1 trust + runner stack.

### 4. Interaction with hooks

- Fire sites live at the scaffold layer, not cmd: `GenerateComponent` pre-generate (`scaffold.go:421-432`) and post-generate (`:549-561`); `GenerateCRUD` same (`:568-580`, `:638-650`); `hooks.md:15-22` documents them. The runner is config-agnostic: `NewRunner(cfg, cmd, out)` + `Fire(t, EnvContext, defaultCwd)` (`internal/pkg/hooks/runner.go:71-73, 83-167`); `EnvContext` already carries `PackName`/`PackVersion` (`internal/pkg/hooks/types.go:49-50`).
- v1 constraint: pack hooks fire ONLY on `new --template`, NEVER on project-level `generate` (`openspec/specs/plugins/spec.md:487-495` — "generate in pack project fires no pack hooks"). Trust is a one-time opt-in stored in the sidecar `pack.json` (`packs/sidecar.go:14-21`, `HooksEnabled`).
- **How pack generators declare hooks**: two options — (i) per-generator `hooks:` (`pre`/`post` lists) inside the generator recipe, fired by the same `Runner` with `HookType` extended; or (ii) reuse pack-level `hooks:` with new hook types. Recommendation: **per-generator `pre`/`post` lists inside the recipe**, fired through the existing runner, **gated by the same sidecar `HooksEnabled` flag** (if the user declined pack hooks at install, `run:` steps and generator hooks do not fire — one trust decision, not two). `EnvContext` grows a `GeneratorName` field for `GENERATOR_NAME` env. This does NOT violate the v1 "no pack hooks on generate" rule: generator hooks are a new fire context scoped to pack-generator runs.

### 5. MCP surface

- Six tools (`internal/pkg/mcp/server.go:117-257`); `generate_component`'s `type` param is a hardcoded enum (`:179-183`); dispatch at `:395-457`.
- **Mapping `generate docker --compose` → MCP**: `generate_component` gains an optional `generatorArgs` object (JSON map) and its `type` enum is relaxed — either keep the six builtins and append "or a pack-declared generator name", or drop the enum and validate in the handler against the pack's manifest. The recipe DSL makes this **introspectable**: a cheap `list_generators` tool (or extending `tools/list`) returns each generator's name, description, and declared flag schema straight from the pack manifest. `generate docker --compose` → `{type: "docker", generatorArgs: {compose: true}}`.
- v1 deliberately deferred the `install_template` MCP tool (proposal.md:27); v2 MAY add it, but it is orthogonal to generators — keep scope tight unless the proposal wants it.

### 6. Security / trust

- Today's trust surface: `docs/packs.md:15-20` trust warning — pack hooks run shell commands; opt-in at install, recorded in `pack.json`; review `go-arch.yaml` before enabling. Integrity is go.sum/ziphash via `go mod download -json` (spec.md:216-231; proposal.md:63 — no extra sha256 layer). `docs/hooks.md:7-10`: ".go-arch.yaml is an executable surface."
- **What generators add**: (c) recipes write files and MAY run commands → two new threat vectors: (1) **target-path traversal** — a recipe step could write outside the project root (e.g. `../../etc/...`); (2) **run-step execution** — same surface as hooks, but reachable from `generate` (a context where v1 explicitly refused pack execution). Builtin registry (d) adds zero surface (CLI-reviewed Go).
- **Mitigations for v2**: validate every recipe target path resolves inside the project root (reject `..` / absolute escapes); `run:` steps and generator hooks ride the existing `HooksEnabled` sidecar flag; extend the install trust prompt to warn when a pack declares generators that run commands (or writes non-project files, if we allow that later — v2: disallow); document in `docs/packs.md` that generators are a bigger trust surface than templates. `template list` could surface "declares generators: N" for visibility.

### 7. Upgrade interplay

- `ManifestEntry` (`internal/pkg/scaffold/manifest.go:24-31`): `path`, `sha256`, `origin`, `template`, `source`, `metadata`; origins `scaffold|component|crud|binary` (`:16-21`). Pack-template upgrade: `source: pack:<name>@<ver>` entries re-render from the recorded pack dir, bypassing the chain (`upgrade.go:169-207`, `renderPackEntry` `:324-342`); missing pack → PROTECTED + warning (`:177-183`); version bump → still PROTECTED, no auto-substitution (spec.md:428-438).
- **Generator-produced files**: they are logic output, not template renders — the v1 guarantee "what generated the file is what re-renders it" (`docs/packs.md:141`) cannot hold. Proposal:
  - Record generator files with a new `origin: generator` + `source: pack:<name>@<ver>` + `metadata: {generator: <name>, args: <json>}`.
  - Upgrade classifies `origin: generator` entries as **PROTECTED by default** (never overwrite logic output, no silent re-run).
  - Exception: pure `template:` steps inside a recipe can also record `template` + `source` and stay **upgradable** via the existing `renderPackEntry` path — the file is byte-identical to a pack template render.
  - Pack bump semantics: identical to v1 — entries reference the recorded version; `template update` + re-generate is the migration path. Re-running generators on upgrade (idempotent compare-then-write) is a v2.1+ candidate, not v2.

## Execution Model Options

| Option | Pros | Cons | Complexity |
|---|---|---|---|
| (a) External binary in pack | Arbitrary power; any language; no DSL ceiling | Platform binaries can't ride a Go module cleanly (same module dir on all OSes); largest trust surface (native code, unreviewable); Windows AV/signing friction | High |
| (b) Go plugin (`.so`) | In-process; type-safe | **Unsupported on Windows** (`go doc plugin`); host must match plugin's exact Go version + build flags — impractical for a distributed CLI | High (broken) |
| (c) Declarative mini-DSL (YAML recipe) | Portable data; sandboxable (paths + steps); introspectable (CLI/MCP schemas derived from manifest); deterministic/testable; versionable | Expressiveness ceiling (complex branching awkward); DSL design + validation + executor is real work | Medium |
| (d) Well-known builtin registry | Full Go power; deterministic; zero new sandbox; CLI-reviewed; trivial upgrade | Logic lives in the CLI, not the pack (contradicts roadmap framing); ecosystem limited to CLI releases | Low–Medium |

**Recommendation**: (c) as the v2 core, with (d) builtins referenced by recipes (`use: builtin/<name>`), and (a)-style power available via `run:` steps that reuse the existing `hooks.Entry` runner — no new process protocol. Reject (b) outright.

## Contract Evolution

- **Option 1 — bump to `contract_version: 2` (recommended)**: add `generators:` to `knownManifestKeys`; CLI accepts {1, 2} and reports "requires contract vN; this CLI supports v1–v2" on anything else. Old CLIs reject v2 packs with the existing clean `contract_version_mismatch` error (`manifest.go:134-139`). v1 packs untouched. Matches the promise in `docs/packs.md:154-165`.
- **Option 2 — additive key, keep `contract_version: 1`**: shipped v1 CLIs still reject the pack, but with a confusing `unknown key "generators"` error; muddies "v1 = templates-only"; lenient parsing would silently ignore generators (the exact failure the contract prevents). No real compatibility win.
- **Tradeoff**: the bump makes v2 packs unusable on old CLIs by design — acceptable and honest; pack authors choose v1 (universal) or v2 (needs current CLI). A pack cannot serve both. Also flag: the `contract_version` check today is `!= SupportedContractVersion` (`manifest.go:134-139`); v2 must become a supported-set check — a small but real semantic change to spec'd behavior.

## Open Questions (for sdd-propose)

1. **Recipe schema**: exact step vocabulary for v2 (files/templates/binaries/run/prompt/conditionals?) and whether the recipe itself carries a `schema_version` for independent evolution.
2. **Invocation surface**: `go-arch generate <generator>` resolving the pack from `.go-arch.yaml` `template:` field — vs `go-arch generate <pack> <generator>` when generator names collide across packs, or when a project has a generator that isn't its scaffold pack.
3. **Input/flag model**: do generators declare their own flags/options (declared in the recipe), and how do they surface in CLI completions and MCP schemas (static vs `list_generators`-derived)?
4. **Trust granularity**: reuse the single `HooksEnabled` sidecar flag for generator `run:` steps + hooks (recommended) vs a separate per-generator opt-in?
5. **Upgrade semantics**: confirm PROTECTED-by-default for `origin: generator`, with only pure template steps upgradable — is re-running a generator ever in v2 scope (recommended: no)?
6. **Builtin registry**: which well-known generators ship in v2 (`docker`, `auth`?), and the exact `use: builtin/<name>` reference contract?
7. **MCP**: `generate_component` schema change + optional `list_generators` in v2, or CLI-only?
8. **Path sandbox**: hard rule "recipe targets MUST resolve inside project root" (recommended) — and the error code for violations.
9. **Missing pack on generate**: when `.go-arch.yaml` records `template: X` but X is uninstalled and the user runs a generator — clear error, and should `generate <builtin>` still work (yes)?
10. **Docs**: extend `docs/packs.md` with a v2 section vs a new `docs/generators.md` reference.

## Risks

- **Contract v2 schema instability**: the recipe DSL is a new public API — mistakes become permanent debt (Angular-schematics-v1 lesson; same risk flagged for v1 in the plugins proposal).
- **Trust expansion**: recipes write files and run commands from `generate`, a context v1 deliberately excluded pack execution from. Mitigated by path sandboxing + sidecar gate + install-time warning, but this is the single biggest new surface.
- **Upgrade divergence**: generator output is not re-renderable; defaulting to PROTECTED avoids silent divergence but means generator files never auto-update — users must re-run generators.
- **Name collisions**: pack generator names vs built-in types (`docker`) and across packs — precedence and error rules must be explicit.
- **MCP schema churn**: enum relaxation + free-form `generatorArgs` weakens static validation; introspection via the DSL is the mitigation.
- **Effort / review budget**: DSL parser + validator + executor + manifest v2 + CLI/MCP wiring is the largest SDD change since plugins; high 400-line-budget risk, likely chained PRs.
- **Engine/MCP stdout discipline**: any new generator output must route through `ui.Out` (the v1 `engine.go` fix is the precedent — MCP JSON-RPC must stay clean).

## Affected Areas (when implemented)

- `internal/pkg/packs/manifest.go` — `contract_version` supported-set {1,2}; `generators:` key; generator manifest types + validation.
- `internal/pkg/scaffold/scaffold.go` — new `GeneratePackGenerator` path; `GenerateComponent`/`GenerateCRUD` untouched for builtins.
- `internal/pkg/scaffold/manifest.go` — new `OriginGenerator`; `metadata` reuse for generator name/args.
- `internal/pkg/scaffold/upgrade.go` — classify `origin: generator` → PROTECTED; keep template-step entries upgradable.
- `internal/pkg/hooks/` — `EnvContext.GeneratorName`; reuse runner for recipe hooks/run steps.
- `cmd/generate.go` — resolve pack from `.go-arch.yaml` `template:`; dispatch to pack generator before built-in switch.
- `internal/pkg/mcp/server.go` — `generate_component` schema (pack generator + `generatorArgs`); optional `list_generators`.
- `internal/pkg/template/` — unchanged (recipes reuse `engine.Render` / pack binary copy).
- `docs/packs.md` (+ optional `docs/generators.md`), `README.md`, `docs/COMMANDS.md` — v2 contract + trust docs.

## Recommendation

Proceed to sdd-propose. Feasible on the current base. The proposal must lock: (1) contract v2 bump + `generators:` key shape, (2) the recipe DSL as the execution model (with builtin registry + `run:` escape hatch, plugins rejected), (3) per-generator hooks gated by the v1 sidecar flag, (4) PROTECTED-by-default upgrade semantics for generator output, (5) path sandboxing + trust-warning extension, (6) MCP `generate_component` + `list_generators` scope.

## Ready for Proposal

Yes — with the open questions above resolved in the proposal phase.
