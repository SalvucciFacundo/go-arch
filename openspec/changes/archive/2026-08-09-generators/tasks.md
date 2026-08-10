# Tasks: Generators (plugins v2)

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~3,500–3,800 (S1 ~850, S2 ~900, S3 ~400, S4 ~1,050, S5 ~500) |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR 1 → PR 2 → PR 3 → PR 4 → PR 5 (feature-branch-chain) |
| Delivery strategy | auto-chain |
| Chain strategy | feature-branch-chain |

Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: feature-branch-chain
400-line budget risk: High

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|------|------|-----------|----------------------|-----------------|-------------------|
| 1 | contract v2 + recipe types + install trust | PR 1 | `go test ./internal/pkg/packs/ ./internal/pkg/generators/` | `go test ./internal/pkg/packs/ -run TestInstall -count=1` on a fixture v2 pack with run:-only generator | delete `internal/pkg/generators/{recipe,validate,errors,registry}.go`; revert `packs/{manifest,install}.go` |
| 2 | executor core + sandbox + RenderPackOnly | PR 2 | `go test ./internal/pkg/generators/ ./internal/pkg/template/` | `go run . generate <name>` with fixture pack under `GO_ARCH_PACKS_DIR` (escape recipe → zero writes) | delete `generators/{executor,sandbox}.go` + tests; revert `template/engine.go` |
| 3 | run steps + hooks exports + GENERATOR_NAME | PR 3 | `go test ./internal/pkg/hooks/ ./internal/pkg/generators/` | fixture pack with `run:` step + `HooksEnabled:false` → skip warning, template still writes | revert `hooks/{types,env,runner}.go`, `generators/runner.go` + tests |
| 4 | dispatch + MCP + provenance | PR 4 | `go test ./internal/pkg/scaffold/ ./cmd/ ./internal/pkg/mcp/` | `go-arch generate docker myservice` with fixture pack in `t.TempDir`; MCP `list_generators` via `go run . mcp` | revert `scaffold.go`, `cmd/generate.go`, `mcp/server.go` + tests; `generators` pkg stays |
| 5 | upgrade PROTECTED + docs | PR 5 | `go test ./... && go vet ./...` | `go-arch upgrade` on project with generator-origin manifest entry; removed pack → PROTECTED warning | revert `upgrade.go`, `docs/packs.md`, README/COMMANDS diffs |

Chain bases: PR 1 → tracker `feat/generators`; PR 2 → `feat/generators-1`; PR 3 → `feat/generators-2`; PR 4 → `feat/generators-3`; PR 5 → `feat/generators-4`. Only tracker merges to main (pattern from hooks chain #27-31, plugins chain #32-38).

## Slice 1 — contract v2 + recipe types (PR 1)

Files: `internal/pkg/generators/{recipe,validate,errors,registry}.go` + `recipe_test.go`; `internal/pkg/packs/{manifest,install}.go` + `manifest_test.go` + `install_test.go`. Est: ~850. Risk: Medium.

- [x] 1.1 RED: `packs/manifest_test.go` — table: v2 manifest with `generators:` accepted + exposed; v1 manifest with `generators:` → `invalid_pack_manifest` naming `generators`; empty `generators: {}` accepted; contract `99` → `contract_version_mismatch` containing `contract v99` and `v1–v2`; missing contract → `invalid_pack_manifest`; v1 pack still accepted
- [x] 1.2 GREEN: `packs/manifest.go` — replace single `SupportedContractVersion` const with supported-set (e.g. `SupportedContractVersions = []int{1, 2}` + contains check); error message `this CLI supports v1–v2`; add `generators` to `knownManifestKeys`; `Manifest.Generators map[string]generators.Generator` decoded via yaml node so `Step.UnmarshalYAML` runs
- [x] 1.3 GREEN: `generators/errors.go` — oops code constants as plain strings (package MUST NOT import `packs`/`scaffold` — import cycle): `unknown_step_type`, `recipe_path_escape`, `unknown_builtin`, `generator_step_failed`, `generator_template_not_found`, `generator_run_skipped_trust`, `generator_prompt_unresolvable`, `missing_generator_argument`, `unknown_generator`, `pack_not_installed`, `invalid_pack_manifest`
- [x] 1.4 GREEN: `generators/recipe.go` — `Generator{Description,Steps,Pre,Post}`; `Step` struct (type/from/to/mode/command/shell/args/cwd/env/timeout/silent/ignore_failure/name/message/default/required/value/index) with custom `UnmarshalYAML` handling object form + string form (`Shell: true`, mirrors `hooks.Entry`); `Record{Path,Origin,Source,TemplatePath,Metadata map[string]string}`; helper `(g Generator) RunsCommands() bool` (any `run:` step or non-empty Pre/Post)
- [x] 1.5 GREEN: `generators/validate.go` — `Validate(genName string, g Generator) error` → `invalid_pack_manifest` naming generator + step index: unknown step type; template/binary missing `from`/`to`; prompt step unknown fields; `use:` not matching `builtin/<name>`; duplicate prompt names; empty `steps: []`
- [x] 1.6 GREEN: `generators/registry.go` — `BuiltinRegistry` map + `Register(name, fn)` / `Lookup(name)` (v2 ships empty)
- [x] 1.7 GREEN: `packs/install.go` — trust prompt fires when `len(m.Hooks) > 0` OR any generator `RunsCommands()`; `install_test.go`: run:-only v2 pack triggers prompt (accept + decline), template-only pack does NOT, warning text mentions generator command execution
- [x] 1.8 Verify: `go test ./internal/pkg/packs/ ./internal/pkg/generators/` + `go vet` + gofmt green

## Slice 2 — executor core + sandbox + RenderPackOnly (PR 2)

Files: `internal/pkg/generators/{executor,sandbox}.go` + `{executor,sandbox}_test.go`; `internal/pkg/template/engine.go` + `engine_test.go`. Est: ~900. Risk: High.

- [x] 2.1 RED: `sandbox_test.go` (`t.TempDir`) — table: `internal/handler/handler.go` accepted; absolute `/etc/passwd` → `recipe_path_escape`; `../../etc/shadow` → escape; symlink escape (`link → /tmp`, step `to: link/evil`) → escape; sibling false-positive `/home/u/myapp-evil` with root `/home/u/myapp` → escape; root-adjacent child accepted
- [x] 2.2 GREEN: `sandbox.go` — `ValidateTarget(projectRoot, relPath string) error`: `filepath.Clean` both paths; reject absolute; reject `..`; join root+rel → `EvalSymlinks` → separator-aware prefix check (`cleanedRoot + string(os.PathSeparator)`); `filepath.VolumeName` compared first (Windows); returns `recipe_path_escape`
- [x] 2.3 RED: `executor_test.go` (fake firer + fake PromptResolver) — steps execute A→B→C in order; step 2 fails → A written, error names step 2 (run), C NOT executed; `ignore_failure: true` → warning + continue; partial state preserved (step 1 file on disk + record returned); `run:` with `HooksEnabled: false` → `generator_run_skipped_trust` warning + template still writes; unknown builtin → `unknown_builtin` listing registered names; template missing → `generator_template_not_found`; `use:` builtin receives resolved args; records: template step → Origin "template" + TemplatePath, binary → Origin "generator" + mode honored (default 0644); pre/post hooks fire via firer (pre before step 1, post after last, post skipped on failure)
- [x] 2.4 RED: pre-flight tests — required prompt unresolved → `generator_prompt_unresolvable` + zero writes; required prompt + default → resolves without error; non-required no default → empty string; escape in step 3 → step 1 target NOT on disk (zero writes on any escape)
- [x] 2.5 GREEN: `executor.go` — `PromptResolver{Resolve(name,message,def string,required bool) (string,error)}`; `RunOptions{ProjectRoot,PackDir,PackName,PackVersion,GeneratorName,HooksEnabled,CmdRunner,PromptResolver,ResolvedArgs,Out}` + `Firer` seam field (package interface `entriesFirer { FireEntries(entries []hooks.Entry, ctx hooks.EnvContext, cwd string) error }`; satisfied by `*Runner` in Slice 3; tests use fake); `Run(ctx, g, opts) ([]Record, error)`: pre-flight prompt resolution → `ResolvedArgs`; pre-flight sandbox over ALL template/binary targets; linear pass (template → RenderPackOnly, binary → copy + mode, run → firer gated by HooksEnabled, use → registry, prompt → skip); fail-fast/ignore_failure; returns `[]Record`
- [x] 2.6 RED: `template/engine_test.go` — `RenderPackOnly`: pack template exists → renders; missing → `generator_template_not_found`; embedded template NOT used as fallback
- [x] 2.7 GREEN: `template/engine.go` — `RenderPackOnly(wr, packDir, from string, data interface{}) error` reads `<packDir>/templates/<from>` directly, NO chain fallback; error code const defined in template package (generators → template import direction forbids the reverse)
- [x] 2.8 Verify: `go test ./internal/pkg/generators/ ./internal/pkg/template/` + `go vet` + gofmt green

## Slice 3 — run steps + hooks (PR 3)

Files: `internal/pkg/generators/{runner,runner_test}.go`; `internal/pkg/hooks/{types,env,runner}.go` + `env_test.go`. Est: ~400. Risk: Medium.

- [x] 3.1 GREEN: `hooks/runner.go` — export `ResolveCommand(entry Entry) (string, []string)` and `ResolveTimeout(entry Entry) (time.Duration, bool)` (rename from lowercase; update `Fire` call sites in same commit); `BuildEnv` already exported
- [x] 3.2 GREEN: `hooks/types.go` + `hooks/env.go` — `EnvContext.GeneratorName string`; `BuildEnv` injects `GENERATOR_NAME` only when non-empty (mirrors `PACK_NAME` pattern)
- [x] 3.3 RED: `hooks/env_test.go` — `GENERATOR_NAME` present when set (alongside PACK_NAME/PACK_VERSION); absent when empty; two sequential invocations (docker → auth) do NOT leak
- [x] 3.4 RED: `generators/runner_test.go` — `FireEntries`: object-form argv; string-form via `sh -c`; timeout honored; `silent` → `io.Discard`; `ignore_failure` continues; failure stops with hooks-style error; per-entry env overrides; `GENERATOR_NAME` passed through
- [x] 3.5 GREEN: `generators/runner.go` — `Runner{cmd hooks.CommandRunner; out io.Writer}`, `NewRunner(cmd, out)`, `FireEntries(entries []hooks.Entry, ctx hooks.EnvContext, cwd string) error` reusing exported `hooks.ResolveCommand`/`hooks.BuildEnv`/`hooks.ResolveTimeout` + cwd join; satisfies Slice 2 `entriesFirer` seam
- [x] 3.6 Verify: `go test ./internal/pkg/hooks/ ./internal/pkg/generators/` + `go vet` + gofmt green

## Slice 4 — dispatch + MCP (PR 4)

Files: `internal/pkg/scaffold/{scaffold,manifest}.go` + `scaffold_generator_test.go`; `cmd/generate.go` + `cmd/generate_test.go`; `internal/pkg/mcp/server.go` + `server_test.go`. Est: ~1,050. Risk: High.

- [x] 4.1 RED: `scaffold_generator_test.go` — E2E fixture pack in `t.TempDir` (`GO_ARCH_PACKS_DIR`): 3-step recipe (template+binary+run) via `GeneratePackGenerator` → files written; manifest: template file → `origin: template` + `template` field + `metadata.generator` + `metadata.args` (JSON string) + `source: pack:<name>@<ver>`; binary/run output → `origin: generator`; pre-flight prompt abort → zero writes; path escape → zero writes; `HooksEnabled: false` → run skipped + template still written
- [x] 4.2 GREEN: `scaffold/manifest.go` — `OriginGenerator Origin = "generator"` and `OriginTemplate Origin = "template"` constants
- [x] 4.3 GREEN: `scaffold/scaffold.go` — `GeneratePackGenerator(name string, args map[string]any) error`: prompt pre-flight (args → default → interactive survey via `ui`); sandbox pre-flight; build `generators.RunOptions` (Firer: `generators.NewRunner(cmd, out)`); `generators.Run`; upsert manifest per `Record`: `json.Marshal(args)` → `metadata.args`, template-origin → `OriginTemplate` + TemplatePath, else `OriginGenerator`; all entries `source: pack:<name>@<ver>` + `metadata.generator`
- [x] 4.4 RED: `cmd/generate_dispatch_test.go` — args: `--list` with 0 args OK; without `--list` exactly 2; dispatch: pack generator wins over builtin/component (fixture pack + `.go-arch.yaml` `template:`); fallback to builtin when pack lacks name; fallback to component; no `template:` → pack tier skipped; pack not installed → `pack_not_installed` naming pack + `go-arch template install` hint, component type still works; unknown name → `unknown_generator` listing names grouped (pack/builtin/component); `--list` grouped + sorted + deterministic (empty builtins → "no builtin generators registered")
- [x] 4.5 GREEN: `cmd/generate.go` — custom `Args` validator (0 when `--list`, else 2); `--list` flag; read `.go-arch.yaml` `template:`; resolve installed pack (`packs.ParseRef`/`LatestInstalled`/`Path`) + sidecar `HooksEnabled`; 3-tier dispatch (pack → builtin → component) + `resolvePackMissing` flow; `unknown_generator` grouped error; `GeneratePackGenerator` for pack/builtin, `GenerateComponent`/`GenerateCRUD` for components
- [x] 4.6 RED: `mcp/server_generator_test.go` — `generate_component` type `"docker"` + `generatorArgs {compose: true}` → pack generator runs with args (fixture pack); type `"service"` → builtin unchanged; type `"bogus"` → `unknown_generator` error; `list_generators` → structured response with pack generators (`source: pack:express` + description) and without pack (builtins + component types only)
- [x] 4.7 GREEN: `mcp/server.go` — relax `generate_component.type` enum → plain string; add `generatorArgs` (object) param → handler passes to `GeneratePackGenerator`; add `list_generators` tool (schema + handler returning structured list); route generator output through `ui.Out` (JSON-RPC stays clean)
- [x] 4.8 Verify: `go test ./...` + `go vet ./...` + gofmt green

## Slice 5 — upgrade + docs (PR 5)

Files: `internal/pkg/scaffold/upgrade.go` + `upgrade_test.go`; `docs/packs.md`, `docs/COMMANDS.md`, `README.md`. Est: ~500. Risk: Low.

- [x] 5.1 RED: `upgrade_test.go` — `origin: generator` entry (no `template` field) → `ClassProtected` + warning `entry "..." is PROTECTED (generator output); re-run generator to update`; pack removed → still PROTECTED, no error; template-origin entry (`source: pack:` + `template` + `metadata.generator`) → still upgradable via `renderPackEntry` (byte-identical); upgrade never re-runs generator recipes
- [x] 5.2 GREEN: `upgrade.go` — early branch: `entry.Origin == OriginGenerator` → `ClassProtected` + per-entry warning (before pack-source re-render branch); template-origin entries flow into existing pack re-render
- [x] 5.3 GREEN: `docs/packs.md` — v2 section: `contract_version: 2` + `generators:` recipe schema (step types, validation), trust section (`run:`/hooks sidecar gate + install warning text), upgrade semantics (PROTECTED + manual re-run, v2.1 deferral), lookup order (pack → builtin → component), `generate --list`
- [x] 5.4 GREEN: `docs/COMMANDS.md` + `README.md` — `generate` section: generators, `--list`, 3-tier resolution, MCP `generatorArgs`
- [x] 5.5 Verify: `go test ./...` + `go vet ./...` + gofmt green; tick completed tasks

## Commit Plan (work units)

- PR 1: `feat(packs): accept contract v2 and decode generators manifest key` → `feat(generators): add recipe DSL types, validation and error codes` → `feat(generators): add builtin generator registry scaffold` → `feat(packs): prompt trust for command-running generators` → `test(packs): cover v2 acceptance and run-only trust prompt`
- PR 2: `feat(generators): add separator-aware path sandbox validation` → `feat(generators): implement linear recipe executor with pre-flight checks` → `feat(template): add pack-only render without chain fallback` → `test(generators): cover executor, sandbox and prompt pre-flight paths`
- PR 3: `refactor(hooks): export ResolveCommand and ResolveTimeout helpers` → `feat(hooks): inject GENERATOR_NAME into hook environment` → `feat(generators): add Runner.FireEntries for run steps and generator hooks` → `test(hooks): cover GENERATOR_NAME env and FireEntries semantics`
- PR 4: `feat(scaffold): add GeneratePackGenerator with prompt pre-flight and provenance` → `feat(cli): add three-tier generate dispatch and --list` → `feat(mcp): relax generate_component, add generatorArgs and list_generators` → `test(scaffold): cover generator E2E, provenance and zero-write escapes` → `test(cli): cover dispatch order and --list output` → `test(mcp): cover generator dispatch and list_generators`
- PR 5: `feat(scaffold): protect generator-origin entries during upgrade` → `docs(packs): document contract v2 and generators reference` → `docs: update generate command and README for generators` → `test(scaffold): cover generator PROTECTED upgrade semantics`
