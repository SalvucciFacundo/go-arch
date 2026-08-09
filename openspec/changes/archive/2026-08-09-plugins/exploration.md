# Exploration: Plugin System / External Generators (roadmap item 4)

Status: **success** — feasible. The template engine's override chain, the fingerprint manifest, and the just-merged hooks runner give us the three load-bearing walls. The design work is: (a) formalize the pack directory layout + manifest schema as the public contract, (b) pick a fetch mechanism for `template install`, (c) decide how packs interact with `new` dispatch, upgrade, and trust.

## Executive Summary

The framework moment is structurally ready. `engine.go` already has a 3-level template lookup chain whose lookup key is a plain relative path string — a pack is just a new source inserted before the embedded fallback, namespaced by pack name. The fingerprint manifest already records per-file provenance and can grow a `source`/`pack` field so pack-owned files upgrade correctly. The hooks runner (PR #31) is config-agnostic (`hooks.Config` → `Runner.Fire`), so a pack manifest can feed hooks with zero runner changes. Two decisions dominate: the public template contract shape (directory layout + manifest schema — must be versioned, it becomes a stable API) and the install fetch mechanism (no HTTP/git fetch exists in Go code today; `install.sh` is the only download precedent, with sha256 checksums). `new` is currently wizard-only with a hardcoded 3-architecture switch — `new --template <pack>` needs a dispatch path that bypasses or extends that switch.

## Findings

### 1. Template resolution chain today

- Engine embeds the whole template tree: `//go:embed all:templates/*` → `var TemplatesFS embed.FS` (`internal/pkg/template/engine.go:18-19`).
- Lookup key is a **relative path string**, e.g. `"common/go.mod.tmpl"`, `"minimalist/main.tmpl"` — relative to the templates root, always with `.tmpl` suffix for rendered files. No name-based registry; it is purely path-based.
- `getTemplate` (`engine.go:54-76`) resolves in fixed order:
  1. **Local project override**: `filepath.Join(".go-arch", "templates", templatePath)` — `os.Stat` hit wins, source `"local"` (`engine.go:56-60`).
  2. **Global user override**: `filepath.Join(home, ".go-arch", "templates", templatePath)` — source `"global"` (`engine.go:63-70`).
  3. **Embedded fallback**: `filepath.Join("templates", templatePath)` parsed from `embed.FS` — source `"embedded"` (`engine.go:73-75`).
- Engine struct holds only `fs embed.FS` (`engine.go:21-23`); `NewEngine()` hardcodes `TemplatesFS` (`engine.go:25-28`). There is no pluggable source list — the chain is a hardcoded if-else.
- Custom-template notice: `RenderTo` prints `"🎨 Using custom template (%s)"` to `os.Stdout` via `fmt.Printf` when source != embedded (`engine.go:47-49`) — note the latent MCP-corruption risk already flagged in the hooks exploration (bypasses `ui.Out`).
- **Where a pack plugs in**: a new step between global and embedded, e.g. `filepath.Join(home, ".go-arch", "packs", packName, "templates", templatePath)`, namespaced so packs cannot collide. All callers are already path-based: `createFile` → `engine.Render` (`scaffold.go:184`), generate dispatch (`scaffold.go:494`), routes registry re-render with `quiet=true` (`scaffold.go:654`), upgrade re-render (`upgrade.go:245`).
- Doc precedent: `docs/ARCHITECTURE.md:57-80` "Deep Customization" documents the 3-step lookup; `README.md:134-137` documents it too.

### 2. Manifest & fingerprints

- `ManifestEntry` shape (`internal/pkg/scaffold/manifest.go:24-30`): `{path, sha256, origin, template (omitempty), metadata map[string]string (omitempty)}`. Free-form `metadata` already exists.
- `Manifest` (`manifest.go:41-46`): `{version int, files map[string]ManifestEntry, routes []RouteEntry}` + unexported `dir`. Saved atomically via temp-file + rename (`manifest.go:81-105`). `Upsert` keyed by path (`manifest.go:108-110`).
- Origins: `scaffold | component | crud | binary` (`manifest.go:16-21`). New origin values are additive and free.
- Upgrade classification (`upgrade.go:161-182`): disk sha256 != manifest sha256 → **PROTECTED**; else re-render through the full engine chain and compare to disk → **upgradable** (template changed) or omitted. `go.mod` is report-only (`upgrade.go:122-141`); `.go-arch.yaml` is skipped entirely — ADR-8 (`upgrade.go:86-89`).
- **Gap for packs**: the manifest records `TemplatePath` (the lookup key) but NOT the source (local/global/embedded). `renderEntry` re-renders through the same chain (`upgrade.go:237-249`), so today a file whose template came from a pack would, after pack removal, re-render from embedded and silently diverge. A pack-aware manifest needs a new field (e.g. `source: "pack:express@1.2.0"`) or upgrade must scope pack files out of v1.
- Precedent for manifest growth: the additive `routes:` list was added by the generate-routes change with byte-identity guarantees (`openspec/specs/generate-routes/spec.md:28-44`).

### 3. Hooks integration (just merged, PR #31)

- Declared in `.go-arch.yaml` under the optional `hooks:` map (`internal/pkg/hooks/config.go:18-20`), loaded via `hooks.Load(hooks.ResolveConfigPath())` (`config.go:77-96`); config path = `viper.ConfigFileUsed()` else `$HOME/.go-arch.yaml` (`config.go:102-111`). Strict validation: known types only, list values, object keys whitelist (`config.go:27-70, 113-122`).
- Four fire sites at the **scaffold layer** (not cmd layer — this is why MCP parity was free): `Execute` fires PreNew/PostNew (`scaffold.go:109-119, 151-161`); `GenerateComponent` fires Pre/PostGenerate (`scaffold.go:390-401, 519-530`); `GenerateCRUD` same (`scaffold.go:538-549, 608-619`). `upgrade` never fires hooks.
- Runner is config-agnostic: `NewRunner(cfg, cmd, out)` + `Fire(t, EnvContext, defaultCwd)` (`runner.go:71-73, 83-167`). Env context built from `EnvContext{ProjectName, ProjectPath, Arch, HookType}` (`types.go:44-49`).
- **Could a pack declare hooks?** Yes — the runner only needs a `*hooks.Config`, and a pack manifest can be parsed into one. Where they live is the design question:
  - (a) **Pack manifest only** — pack ships `hooks:`; the CLI merges them into the project config at install time or fires them pack-scoped. Risk: supply-chain — the trust warning (`docs/hooks.md:7-10`: ".go-arch.yaml is an executable surface") extends to packs, so merge must be explicit/opt-in, not silent.
  - (b) **Project config only** — packs never declare hooks; hooks stay user-owned (strictest, simplest, least powerful).
  - Recommendation for proposal: (a) with explicit opt-in — pack hooks fire only when the user installs/enables the pack, with a printed trust warning (install.sh precedent prints warnings; hooks.md trust-warning precedent). Pack hooks could also receive new env vars (PACK_NAME/PACK_VERSION) via the existing `BuildEnv` path (`hooks/env.go`).
- Note: hooks config schema lives in `.go-arch.yaml`; there is no config-schema validator package — the pack manifest will need its own `UnmarshalYAML` validation (precedent: `config.go:27-70`).

### 4. Scaffolder flow (`new` picks architecture + templates)

- `new` is **wizard-only**: `cmd/new.go:22-30` — arg is the project name (`cobra.MinimumNArgs(1)`), `ui.RunWizard()` collects everything else; there are no flags on `newCmd`.
- Dispatch: `Execute` switches on `s.config.Architecture` (`scaffold.go:130-139`) → `scaffoldMinimalist/Standard/Hexagonal` (`scaffold.go:265-315`) → `createCommonFiles` (`scaffold.go:317-370`) → `scaffoldWeb` when `UseTemplHTMX` (`scaffold.go:215-263`). Unknown arch → `"unsupported architecture"` error.
- Config shape: `ui.ProjectConfig` flat struct with mapstructure tags (`internal/ui/prompts.go:9-20`): `project_name, module_name, architecture, db_driver, use_docker, use_observability, observability_backend, use_grpc, use_templ_htmx, go_arch_version`. Wizard options hardcode `["Minimalist", "Standard", "Hexagonal"]` (`prompts.go:47`); MCP `new_project` schema hardcodes the same enum (`server.go:134`).
- **How `new --template <pack>` would select a pack**: add a `--template <pack>` flag to `newCmd`; when set, skip/branch the wizard and dispatch to a pack-aware scaffold path. `ProjectConfig` needs a new field (e.g. `template: express`) — which is a config-shape change with upgrade implications (ADR-8 skips `.go-arch.yaml` in upgrade, so adding a field is safe for old projects; the `config.tmpl` must document it).
- The pack's generator set (which templates run, what dirs are created) must come from the pack manifest, not the architecture switch. The switch at `scaffold.go:130` is the exact insertion point.
- MCP `new_project` (`server.go:279-326`) mirrors `new`; a `template` param there would mirror the flag.

### 5. Install surface / external dep resolution

- **No HTTP client and no git clone anywhere in Go code.** Grep of `cmd/` and `internal/`: only `exec.Command` usages are `go version` (`cmd/doctor.go:41`), `air` LookPath (`cmd/doctor.go:48`), `serve` (`cmd/serve.go:62-74`), and MCP `go install github.com/air-verse/air@latest` (`internal/pkg/mcp/server.go:543`) — the **only** actual install execution in the codebase.
- `install.sh` is the real download precedent: GitHub release API → tarball + `checksums.txt` → sha256 verify → extract (`install.sh:58-90`). This is the trust model the CLI already ships.
- `go get` appears only as **hints** printed by upgrade for go.mod changes (`cmd/upgrade.go:199-225`), never executed.
- Viper config paths: `$HOME/.go-arch.yaml` and `.` (`cmd/root.go:41-61`); hooks config path resolution (`hooks/config.go:102-111`). Global template home `~/.go-arch/templates/` already exists as a convention (`engine.go:63-70`).
- So `template install` fetch is **greenfield**: no existing Go-side fetch infrastructure to reuse. The module proxy route can reuse the Go toolchain (guaranteed present — doctor checks it) with zero new deps (`go mod download -json`); tarball route mirrors install.sh.

### 6. Config & docs conventions

- Templates live at `internal/pkg/template/templates/{common,hexagonal,minimalist,standard,web}/` (embedded via `engine.go:18-19`). Convention: `.tmpl` suffix, subdir = scope (`common/` shared, per-arch dirs for `main.tmpl`).
- `config.tmpl` documents features as commented YAML sections with trust warnings — the hooks block (`templates/common/config.tmpl:16-41`) is the template for documenting a `template:`/pack key.
- `docs/`: `ARCHITECTURE.md` (Deep Customization section :57-80, lookup order), `COMMANDS.md` (9 sections + Metadata System :112), `hooks.md` just added as a dedicated reference with schema tables + trust warning — the model for a future `docs/packs.md` (or `docs/templates.md`).
- `README.md:134-137` documents the override lookup order.

### 7. MCP surface

- Six tools (`internal/pkg/mcp/server.go:115-251`): `new_project`, `generate_component`, `check_architecture`, `serve_project`, `setup_environment`, `upgrade_project`. Each maps 1:1 to a CLI command; `tools/call` dispatch at `server.go:277-628`.
- MCP parity precedent: hooks got parity for free by wiring at the scaffold layer (server.go:312-325, 368-374) — same trick applies to pack dispatch inside `Execute`.
- **Would template install need an MCP tool?** Open question. `setup_environment` is the precedent for a machine-level, mutating tool (installs air via `go install`, `server.go:542-556`), so an `install_template` tool is defensible. Counter-argument: install is a rare, machine-level op with network fetch (slow inside stdio JSON-RPC; blocks the MCP loop); the other five tools are project-scoped. Proposal should decide: CLI-only in v1 (recommended) vs `install_template` mirroring `setup_environment`.

## Contract Options (public template contract)

### Option A — Flat mirror pack (minimal)

Pack = a directory whose `templates/` subtree mirrors the embedded layout plus a small manifest.

```
go-arch-express/
├── go-arch.yaml            # {name, version, contract_version}
└── templates/
    └── common/
        └── handler.tmpl
```

Engine chain becomes: local → global → **installed packs** (namespaced `~/.go-arch/packs/<name>/templates/<path>`) → embedded. Lookup key unchanged; packs are per-file overrides/extensions.

- Pros: tiny engine change (one more os.Stat link); zero new dispatch machinery; existing templates work untouched; binary assets copyable.
- Cons: no generators/logic (pure template override — closer to today's overrides than to Angular schematics); no way to define new project layouts or architecture dispatch; packs can only override known paths, not invent flows; collisions require namespacing anyway.

### Option B — Manifest-driven generator pack (full schematics)

Pack root declares a full manifest: generators (named entry points), layout dirs, templates, hooks, dependencies.

```
go-arch-express/
├── go-arch.yaml
└── generators/
    └── express/
        ├── manifest.yaml   # layout, templates map, hooks, deps
        └── templates/…
```

`new --template express` reads the generator manifest and dispatches. Upgrade re-renders via `source: pack:express@version`.

- Pros: the actual "framework moment"; packs can add layouts, declare hooks/deps, expose multiple generators; versioned upgrade story.
- Cons: largest design surface (manifest schema spec + validation + versioning + dispatch engine); most spec/task work; contract risk highest if schema is unstable.

### Option C — Hybrid v1 (recommended)

Pack = `go-arch.yaml` manifest + `templates/` (rendered files) + optional binary assets. Manifest v1 fields: `name`, `version` (semver), `contract_version`, `hooks` (opt-in, trust-warned), `layout` (dirs to create). Generators deferred to contract v2.

```
go-arch-express/
├── go-arch.yaml            # contract_version: 1, name, version, hooks, layout
└── templates/
    └── common/…
```

- Pros: captures 80% of the value (installable template+layout+hook packs) with a small, versionable contract; upgrade story solvable with a manifest `source` field; `new --template <pack>` maps to the architecture switch cleanly; generators remain a v2 additive (contract_version guards).
- Cons: not yet "full schematics" — custom Go generators wait for v2; still needs the manifest validation + engine source insertion.

**Recommendation: Option C** — define contract v1 as templates + layout + opt-in hooks, versioned via `contract_version`, with generators explicitly deferred to v2. It matches the roadmap's own sequencing discipline ("plugins only when the base is stable") and keeps the contract small enough to stabilize.

## Install Approaches (fetch mechanism)

### Approach 1 — Go module proxy

`go-arch template install github.com/alguien/go-arch-express@latest` → `go mod download -json` (or `go install`-style resolution) → read the module cache dir → copy `templates/` + manifest into `~/.go-arch/packs/<name>/`.

- Pros: semver + integrity (go.sum/proxy) for free; the roadmap's example IS a module path; no git/curl dependency; portable (Windows-safe); offline module cache.
- Cons: pack must be a valid Go module (adds a go.mod to pack repos — acceptable and normalizing); requires Go toolchain (already a hard dependency, doctor checks it); fetch is slower than raw git clone.

### Approach 2 — git clone

`git clone --depth 1 <url> ~/.go-arch/packs/<name>`; version pin via `git checkout <tag>`.

- Pros: works for any git repo; no go.mod requirement; fast.
- Cons: needs `git` binary (doctor already checks it — but it's optional today); no built-in integrity verification (custom hashing needed, or tag+commit pinning); Windows git quirks; submodules/monorepos add complexity.

### Approach 3 — GitHub release tarball + checksums (mirror install.sh)

Resolve `releases/latest`, download tarball + `checksums.txt`, verify sha256, extract (install.sh:58-90 is the exact precedent).

- Pros: mirrors the CLI's own install trust model; checksum-verified; no go/git dependency; non-Go pack authors supported.
- Cons: needs a net/http client in Go (greenfield) + tarball extraction code; GitHub-only (roadmap example is github.com so acceptable); tag resolution + API dependency.

**Recommendation: Approach 1 (module proxy) primary, with the pack path resolvable to a plain HTTPS tarball in a later iteration.** It gives versioning + integrity with zero new infrastructure and matches the roadmap's `github.com/alguien/go-arch-express` syntax naturally. Approach 3 is the pragmatic fallback if the Go-module requirement is considered too heavy for template-only pack authors.

## Open Questions (for sdd-propose)

1. Contract v1 scope: templates + layout + opt-in hooks only (generators deferred to v2)? Confirm.
2. Pack manifest identity: file name (`go-arch.yaml` at pack root vs `pack.yaml`), fields, and `contract_version` enforcement (reject newer contract versions with a clear error).
3. Pack-declared hooks: merge into project config (trust warning at install), fire pack-scoped, or reject in v1? Where does the trust boundary live?
4. `new --template <pack>`: bypass the wizard entirely, or wizard gains pack-aware options? Does `ProjectConfig` gain a `template`/`pack` field, and is it documented in `config.tmpl`?
5. Upgrade interaction: add `source: pack:<name>@<version>` to `ManifestEntry`, or scope pack files out of `go-arch upgrade` in v1? What happens on pack version bump (re-render from new version vs keep old)?
6. Command surface: `go-arch template install|list|remove|update` group, or single `install` command? `@version` pinning syntax and default (`@latest`)?
7. MCP: `install_template` tool in v1 (mirroring `setup_environment`) or CLI-only?
8. Override precedence: confirm local > global > pack > embedded (and per-pack namespace to avoid collisions).
9. Binary assets in packs (e.g. custom `htmx.min.js`-like files): allowed via a non-rendered copy path — pack-aware `createBinaryFile` variant needed?
10. Trust: install-time sha256 verification (install.sh model) required for v1, or is module-proxy go.sum integrity sufficient?

## Risks

- **Supply-chain / trust**: packs render templates and can (optionally) run hooks — executable surface. Mitigation: trust warnings at install (docs/hooks.md precedent), opt-in hooks, integrity verification.
- **Upgrade divergence**: manifest entries do not record template source; pack files upgraded through the embedded chain silently diverge if a pack is uninstalled or bumped. Mitigation: `source` field or v1 scope exclusion.
- **Contract stability**: this IS the public API. Any v1 schema mistake becomes a permanent compatibility burden for every published pack. Mitigation: `contract_version` field, small v1 scope, explicit v2 deferral.
- **MCP stdio corruption**: pack render/hook output must route through `ui.Out` (stderr in MCP); the existing `fmt.Printf` at `engine.go:48` bypasses it (known latent issue).
- **Wizard coupling**: `new` is wizard-only; adding `--template` must not regress the interactive flow or the MCP `new_project` schema.
- **Path collisions** between packs and between packs and built-in templates — namespace by pack name.
- **Windows portability** for install fetch: module proxy approach is the most portable; git/tar/curl paths are not.
- **Config-shape change**: `ProjectConfig` gains a field → `config.tmpl`, upgrade ADR-8 exemption, and MCP schema must stay in sync.

## Affected Areas (when implemented)

- `internal/pkg/template/engine.go` — pack source insertion in `getTemplate` chain (+ `NewEngine` options).
- `internal/pkg/template/` — new pack manifest types/validation (or new `internal/pkg/pack/` package).
- `internal/pkg/scaffold/manifest.go` — optional `source`/pack field on `ManifestEntry`.
- `internal/pkg/scaffold/scaffold.go` — `new --template` dispatch at the architecture switch (:130), pack-aware `createBinaryFile`.
- `internal/pkg/hooks/` — reuse; optionally pack-sourced `hooks.Config`.
- `cmd/` — new `template` command group; `new.go` flag; `generate.go` untouched.
- `internal/ui/prompts.go` + `internal/pkg/mcp/server.go` — `ProjectConfig` field, wizard/MCP schema sync.
- `internal/pkg/template/templates/common/config.tmpl` + `docs/packs.md` (or `docs/templates.md`) + `README.md` — contract documentation.
- `install.sh` — unchanged; fetch logic lives in Go.

## Recommendation

Proceed to sdd-propose. Feasible with the current base; the proposal must lock the contract v1 scope (Option C), the install mechanism (module proxy primary), the trust model for pack hooks, and the upgrade `source` story before any code.

## Ready for Proposal

Yes — with the open questions above resolved in the proposal phase.
