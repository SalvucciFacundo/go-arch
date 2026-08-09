# Exploration: Hooks (roadmap item 5) — pre-new / pre-generate / post-generate

## Status

**success** — Feasible. The codebase has clean, single-path command → scaffold entry points, an existing structured-YAML parsing precedent (manifest), and existing external-command execution patterns (serve, doctor). There are **no** hardcoded post-generate behaviors to migrate (gofmt / git init / go mod tidy are never executed by the CLI today — only hints are printed), so hooks are greenfield extension points, not a refactor.

## Executive Summary

Hooks (pre-new, pre-generate, post-generate) are highly feasible and low-risk: both `new` and `generate` funnel through exactly two scaffold entry points (`Scaffolder.Execute()` and `GenerateComponent()`/`GenerateCRUD()`) reached from thin Cobra `RunE` bodies, giving four precise injection sites. The natural config home is a root-level `hooks:` key in `.go-arch.yaml`, which is already upgrade-safe (ADR-8: upgrade never touches `.go-arch.yaml`). The two real design decisions are (1) config shape — string commands vs explicit `command`/`args` objects, and (2) wiring layer — cmd-layer hooks (explicit, but MCP must be wired separately) vs scaffold-layer hooks (MCP parity for free, but output routing must respect `ui.Out`). A `post-new` hook should be added alongside the roadmap's `pre-new` so `git init` / `go mod tidy` have a home; the recommended runner is `exec.Command` with `Dir` set (never `os.Chdir`) and output routed through `ui.Out` to keep MCP stdio clean. Roadmap order is validated: hooks define the extension points the plugin system (item 4) will later formalize.

## Findings

### 1. Execution flow (where pre/post hooks would fire)

**`new`** — entry `cmd/new.go:17` (`newCmd`, `Args: cobra.MinimumNArgs(1)`), `RunE` at `cmd/new.go:22`:
1. Wizard: `ui.RunWizard()` `cmd/new.go:25` → `internal/ui/prompts.go:22-107` (interactive survey; `ProjectConfig` built).
2. Scaffolder: `scaffold.NewScaffolder(config)` `cmd/new.go:34` → `scaffold.go:22-27`.
3. **`scaffolder.Execute()`** `cmd/new.go:35` → `scaffold.go:80-99`:
   - `os.MkdirAll(manifestDir())` `scaffold.go:84`
   - `switch Architecture` → `scaffoldMinimalist()` `scaffold.go:89,200-208` / `scaffoldStandard()` `210-229` / `scaffoldHexagonal()` `231-250`
   - each → `createCommonFiles()` `252-305` → `scaffoldWeb()` when `UseTemplHTMX` `150-198`
   - every file → `createFile()` `101-125` / `createBinaryFile()` `131-145` → `recordManifest()` `56-78`
4. `WriteVersionField(<project>/.go-arch.yaml)` `cmd/new.go:43-45` → `upgrade.go:359-377` (surgical, non-fatal).
5. Success + next-steps hints `cmd/new.go:47-51`.

**`generate`** — entry `cmd/generate.go:18` (`generateCmd`, `Args: cobra.ExactArgs(2)`, alias `g`), `RunE` at `cmd/generate.go:34`:
1. Config guard: `viper.GetString("project_name") == ""` → `oops Code("missing_config")` `cmd/generate.go:39-45`; `ProjectConfig` built from viper `cmd/generate.go:48-55`.
2. `--route` flag read `cmd/generate.go:59`; dispatch:
   - `crud` → **`scaffolder.GenerateCRUD(name)`** `cmd/generate.go:64` → `scaffold.go:443-500` (4-5 files via `createFile`, then route upsert + `renderRoutesRegistry()` `484-495`)
   - else → **`scaffolder.GenerateComponent(type, name, opts...)`** `cmd/generate.go:70` → `scaffold.go:323-440` (validation switch `340-413`, `createFile` `415`, origin re-record `422`, route upsert + `renderRoutesRegistry()` `425-437`)
3. Success + `templHint()` `cmd/generate.go:81-86,91-93`.

**MCP** (must stay in parity): `new_project` → `scaffolder.Execute()` `internal/pkg/mcp/server.go:311-316`; `generate_component` → `GenerateCRUD`/`GenerateComponent` `server.go:358-374`. MCP does `os.Chdir(projectPath)` + `viper.Reset()` + re-read `server.go:330-347`. **If hooks are wired only in `cmd/`, MCP bypasses them; wiring in the scaffold layer gives parity for free.**

### 2. Existing config surface

- `.go-arch.yaml` is generated from `internal/pkg/template/templates/common/config.tmpl:1-14`: `project_name`, `module_name`, `architecture`, `db_driver`, `use_docker`, `use_observability`, `observability_backend`, `use_grpc`, `use_templ_htmx`, `go_arch_version`, `generated_at`. A root-level `hooks:` key slots in naturally.
- Loader: viper via `cobra.OnInitialize(initConfig)` `cmd/root.go:34`; `initConfig` `cmd/root.go:41-61` (`AddConfigPath(home)` + `AddConfigPath(".")`, `SetConfigName(".go-arch")`, `AutomaticEnv`, optional `--config`).
- Mapping: manual per-command `viper.Get*` into `ui.ProjectConfig` (`internal/ui/prompts.go:9-20`, mapstructure tags) — `cmd/generate.go:48-55`, `cmd/check.go:31-35`, `configFromViper` `cmd/upgrade.go:133-145`. **Gotcha: `viper.Get*` is flat; a nested `hooks:` map requires `viper.Unmarshal`/`viper.GetStringMap` or a separate `yaml.v3` decode — precedent exists in `manifest.go:63-71` (`yaml.Unmarshal`).**
- "Validation" is ad-hoc: `project_name == ""` → `oops Code("missing_config")` + `Hint("Run 'go-arch setup'…")` (`cmd/generate.go:40-45`, `cmd/check.go:24-29`, `cmd/upgrade.go:67-71`). There is **no config-schema validator**; `internal/pkg/validator` checks *project* architecture rules, not config (`validator.go:32-49`).
- ADR-8 (`upgrade.go:86-89`): upgrade skips `.go-arch.yaml` entirely (surgically managed) → **user-owned hooks in `.go-arch.yaml` are protected from upgrade clobbering.**

### 3. Existing extension points

- **Local template overrides**: `template.Engine.getTemplate` `engine.go:54-76` — resolution order local `.go-arch/templates/<path>` → global `~/.go-arch/templates/<path>` → embedded `TemplatesFS`; custom-source notice printed unless `quiet` (`engine.go:41-52`). This is the current "how a user extends generation" mechanism and the closest analog to hooks (per-repo extension, no plugin system yet).
- **Manifest fingerprints**: `.go-arch/manifest.yaml` (`ManifestPath` `manifest.go:49-51`); `ManifestEntry{Path, SHA256, Origin, TemplatePath, Metadata}` `manifest.go:24-30`; `LoadManifest` `54-72` (missing → empty manifest); atomic `Save` `81-105`; `Origin` consts `16-21`. `Upgrade()` `upgrade.go:70-214` classifies upgradable/protected/absent/up_to_date; `Apply()` compare-then-write `281-338`. Hooks do not need manifest integration — manifest is provenance, hooks are lifecycle.
- **Hardcoded "after generate" behaviors**: **none executed.** Only *hints* are printed: `templ generate` hint (`cmd/generate.go:92-93` `templHint`; `cmd/upgrade.go:122,273`), `go get …@latest` suggestions (`cmd/upgrade.go:199-225`), next-steps text (`cmd/new.go:48-51`). `gofmt`, `git init`, `go mod tidy` never run from the CLI (only inside tests: `scaffold_test.go:249-253,589-599`). These hint-only behaviors are the roadmap's motivation and can become sample `post-*` hooks — or stay as hints; no migration burden.

### 4. Execution environment

- **CWD semantics**: `manifestDir()` `scaffold.go:32-37` — returns `"."` when a manifest exists in CWD (generate context), else `config.ProjectName` (new context). ADR-7 (`upgrade.go:69-71`): root is **always** CWD for generate/upgrade; `--project-path` does `os.Chdir` then `viper.Reset()` + re-read (`cmd/upgrade.go:46-63`). MCP mirrors this (`server.go:330-347`). **Hooks must not depend on `os.Chdir` being safe across the call stack — prefer `exec.Command` `Dir` field.**
- **Tests**: temp-dir pattern is `os.MkdirTemp` + `os.Chdir(tmpDir)` + `defer os.Chdir(oldWd)` (`cmd/generate_test.go:42-44`, `cmd/upgrade_test.go:99-101`, `mcp/server_test.go:24-26`, `scaffold/upgrade_test.go:82-86`); `.go-arch.yaml` fixtures are inline YAML strings (`generate_test.go:46-50`, `mcp/server_test.go:28-32`); no `testdata/` dirs seen. Integration tests that run real toolchains skip when the binary is absent (`scaffold_test.go:561,615`). go-testing skill: use `t.TempDir()`, keep external-command tests skippable under `-short`, mock the command boundary behind a small interface.
- **External-command execution today**: `cmd/serve.go:62-74` (`exec.Command("air")` / `exec.Command("go","run",path)` with `Stdout`/`Stderr` wired to `os.Stdout`/`os.Stderr`, `cmd.Run()`); `cmd/doctor.go:41` (`exec.Command("go","version").Output()`) and `exec.LookPath` for air/git (`doctor.go:48,55`). No `exec.CommandContext`, no `Dir`, no timeout — hooks will be the first to need those.

### 5. Conventions

- **Errors**: `samber/oops` everywhere — `Code("snake_case")` + `Hint` + `With` key-values, wrapped at the command boundary (`cmd/new.go:26-40`, `cmd/generate.go:73-79`); `ui.Fatal` in `Execute()` `cmd/root.go:23-31` (exit 1); `SilenceUsage`/`SilenceErrors` true `cmd/root.go:19-20`. Hook failures should follow the same shape (e.g. `Code("post_generate_hook_failed")`).
- **Output**: `internal/ui` helpers writing to the global `ui.Out` writer (`output.go:12`; MCP redirects it to `os.Stderr` `server.go:48`). **Gotcha: `scaffold.go` prints directly via `fmt.Printf` (`scaffold.go:81,452,494`), bypassing `ui.Out` — a latent MCP stdout-corruption risk on `new_project`/CRUD; hook output must go through `ui.Out` (or `cmd.OutOrStdout()`) and must not add more direct stdout prints.**
- **Flags**: no global verbose/quiet flags exist; "quiet" only exists as `RenderTo`'s parameter (`engine.go:41`). Hook output control (e.g. a `silent:` per-hook option) would be new surface — keep minimal.
- **Exit codes**: only `ui.Fatal → exit 1`; no sysexits usage.

## Recommended Hook Points

| Hook | Fire site | File:line | CWD | Notes |
|---|---|---|---|---|
| `pre-new` | Between wizard success and `NewScaffolder`/`Execute` | `cmd/new.go:30 → 34` (or `scaffold.go:80` if scaffold-layer) | invocation dir (project not created yet) | MCP `new_project` `server.go:311` |
| `post-new` | After `Execute()` + `WriteVersionField`, before `ui.Success` | `cmd/new.go:45 → 47` | **new project dir** (`config.ProjectName`) via `cmd.Dir` — never `os.Chdir` | natural home for `git init`, `go mod tidy`; roadmap lists only `pre-new`, symmetric `post-new` is recommended |
| `pre-generate` | After config validation, before scaffolder dispatch | `cmd/generate.go:55 → 61` | project root (already CWD) | |
| `post-generate` | After `GenerateCRUD`/`GenerateComponent` returns nil, before `ui.Success` | `cmd/generate.go:71 → 81` | project root | fires once per command, after routes registry render; MCP `generate_component` `server.go:358-374` |

**Layer decision (drives everything)**: cmd-layer wiring keeps the command surface explicit (only `new`/`generate` have hooks, per roadmap) but MCP must be separately wired; scaffold-layer wiring (`Execute`/`GenerateComponent`/`GenerateCRUD`) gives MCP parity automatically and guarantees the manifest/routes registry is final before `post-generate` runs, at the cost of hook logic living deeper and needing `ui.Out` routing care. Recommendation: scaffold-layer runner invoked from both cmd and MCP paths, or a small `hooks.Runner` type called at all four sites (cmd layer) plus two MCP sites.

## Config Design Options

1. **String-list (npm-scripts style)** — `hooks: { post-generate: ["gofmt -w internal/"] }`. Pros: terse, familiar YAML, zero new types; `strings.Fields`-split argv or `sh -c`. Cons: shell parsing footgun, Windows `cmd.exe` vs `sh` divergence, no per-hook flags (silent/cwd/failure policy).
2. **Executable/object style** — `hooks: { post-generate: [{command: "gofmt", args: ["-w", "internal/"]}] }`. Pros: no shell interpolation, explicit args, cross-platform, trivially testable, room for `cwd`/`env`/`silent`/`ignore_failure` fields; matches the go-testing "small interface at the command boundary" guidance. Cons: verbose YAML for trivial one-liners.
3. **Hybrid (recommended)** — string shorthand (shell-executed, npm precedent) *and* object form (direct exec) accepted per entry; objects win on conflict/ambiguity. Pros: best UX for `git init`-style one-liners + full control when needed; the extension-point groundwork roadmap item 4 wants. Cons: most code; must define shell rules (recommend: strings run via `sh -c`/`cmd /c`, objects run argv-direct; document that config is trusted executable surface, like npm scripts).

Precedent in repo: `.goreleaser.yaml:8` already uses object-style `hooks:` for the release pipeline.

## Open Questions (proposal phase must decide)

1. **Wiring layer**: cmd-layer vs scaffold-layer (MCP parity implications, per Findings §1/§4).
2. **Config shape**: option 1, 2, or 3 above; and is `hooks` loaded via `viper.GetStringMap`/`viper.Unmarshal` or a separate `yaml.v3` decode (manifest precedent)?
3. **Scaffolded vs user-added**: does `config.tmpl` emit an empty `hooks:` section (discoverability) or is it user-added only?
4. **Failure semantics**: hook failure fatal (npm-style) vs per-hook `ignore_failure`; stop-on-first-error vs run-all-then-report; does a failed `post-new` fail the whole `new`?
5. **`post-new` cwd**: confirm the new project dir; also confirm `pre-new` runs in the invocation dir (or should the dir be created first?).
6. **Env surface**: pass `PROJECT_NAME`/`PROJECT_DIR`/`GO_ARCH_VERSION` to hook processes?
7. **Scope guard**: `upgrade` does **not** fire hooks (roadmap lists only pre/post new/generate) — confirm explicitly so future readers don't expect post-upgrade hooks.
8. **Naming**: roadmap says `pre-generate`/`post-generate`/`pre-new`; add `post-new` (symmetric, needed for `git init`)? Keep roadmap names otherwise.
9. **Shell rule**: strings via `sh -c` (unix) / `cmd /c` (windows) vs `strings.Fields` argv-only; and timeout/context (`exec.CommandContext`) policy.
10. **Interactive safety**: hooks run non-interactively in MCP — is that acceptable (MCP cannot prompt), or should hooks be skipped in MCP mode?

## Risks

- **MCP stdio corruption**: hook output to stdout breaks JSON-RPC. Must route through `ui.Out`/stderr; note the existing latent `fmt.Printf` in `scaffold.go:81,452,494` makes MCP `new_project`/CRUD already fragile — hooks must not compound it.
- **CWD bugs (the `manifestDir()` saga)**: `post-new` must run inside the new project; `os.Chdir` at scaffold depth is hazardous — use `cmd.Dir`.
- **Config-as-code surface**: `.go-arch.yaml` becomes executable surface (same trust model as npm scripts) — document it; a malicious config in a cloned repo can run arbitrary commands.
- **Cross-platform divergence**: `sh -c` vs `cmd /c`, and hooks targeting unix-only tools (`gofmt` fine, `templ` fine, `git init` fine — but custom hooks may assume POSIX).
- **Blocking/UX**: long-running or interactive hooks stall the CLI; needs `CommandContext` + timeout policy.
- **Partial state**: a failing `post-generate` after files were written leaves a half-processed project — define atomicity expectations in the spec.
- **Testability**: hooks execute real processes; per go-testing skill, abstract the runner behind an interface so unit tests swap in a fake, and keep true integration tests skippable with `-short`.

## Next Recommended

**propose** — the exploration supports a full SDD proposal. The proposal should fix: config shape (hybrid option 3 as default recommendation), wiring layer, `post-new` inclusion, failure semantics, and the MCP/stdout routing rule.
