# Tasks: Plugins — Installable Template Packs

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~3,200 (S1 ~600, S2 ~700, S3 ~750, S4 ~760, S5 ~550) |
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
| 1 | packs core: manifest + paths + errors + info | PR 1 | `go test ./internal/pkg/packs/ -run 'TestManifest\|TestParseRef\|TestLatestInstalled\|TestValidateSlug'` | N/A — pure parse/paths, no wiring yet | delete `internal/pkg/packs/{manifest,paths,errors,info}*.go` |
| 2 | install machinery: download/copy/sidecar/install | PR 2 | `go test ./internal/pkg/packs/ -run TestInstall` | `go test ./internal/pkg/packs/ -run TestInstall_RealDownloader -count=1` (network-gated, skipped `-short`) | delete `{download,copy,sidecar,install}.go` + `install_test.go` |
| 3 | template CLI group + engine pack step | PR 3 | `go test ./internal/pkg/template/ ./cmd/` | `go run . template list` with `HOME=temp` → `no packs installed`; `go run . template install <mod>@<ver>` (network) | revert `engine.go`, `cmd/template*.go`, lookup docs |
| 4 | dispatch: `new --template`, scaffoldPack, MCP | PR 4 | `go test ./internal/pkg/scaffold/ ./cmd/ ./internal/pkg/mcp/` | `go run . new demo --template express` with fixture pack under `HOME=temp` | revert `scaffold.go`, `cmd/new.go`, `mcp/server.go`, hooks env diffs; packs pkg stays |
| 5 | upgrade pack-source re-render + docs | PR 5 | `go test ./... && go vet ./...` | `go-arch upgrade` on pack-scaffolded project; removed pack → PROTECTED warning | revert `upgrade.go`, `upgrade_opts.go`, `docs/packs.md` |

Chain bases: PR 1 → tracker `feat/packs`; PR 2 → `feat/packs-1`; PR 3 → `feat/packs-2`; PR 4 → `feat/packs-3`; PR 5 → `feat/packs-4`. Only tracker merges to main (pattern from hooks chain #27-31).

## Slice 1 — packs package core (PR 1)

Files: `internal/pkg/packs/{manifest,paths,errors,info}.go` + `manifest_test.go` + `paths_test.go`. Est: ~600. Risk: Medium.

- [x] 1.1 RED: `manifest_test.go` — table: valid min, valid full (hooks+layout+binary_assets), unknown key→`invalid_pack_manifest`, missing name, bad slug `"Bad Name!"`, bad semver `"not-a-version"`, contract `99`→`contract_version_mismatch` (error contains `contract v99` and `v1`), missing contract→`invalid_pack_manifest`
- [x] 1.2 GREEN: `manifest.go` + `info.go` — `SupportedContractVersion=1`, `Manifest{ContractVersion,Name,Version,Layout,Hooks,binary_assets BinaryAssets}`, `BinaryAsset{Source,Target}`, `PackInfo{Dir,Manifest}`, strict `UnmarshalYAML` (reject unknown top-level keys), slug regex `^[a-z0-9]+(-[a-z0-9]+)*$`, semver validation (stdlib only, no new deps), `Load(path)`
- [x] 1.3 GREEN: `errors.go` — oops codes: `invalid_pack_manifest`, `contract_version_mismatch`, `pack_not_found`, `pack_install_failed`, `pack_not_installed`, `pack_no_templates`, `pack_fetch_failed`
- [x] 1.4 RED: `paths_test.go` — `ParseRef` table (`express@1.0.0`→name/ver, `express`→bare, empty→err), `LatestInstalled` on `t.TempDir()` synthetic dirs (latest wins, none→`pack_not_installed`), `ValidateSlug` table
- [x] 1.5 GREEN: `paths.go` — `BaseDir()`, `Path(name,ver)` → `<base>/<name>@<ver>`, `ParseRef(ref)`, `LatestInstalled(name)`, `ValidateSlug`
- [x] 1.6 Verify: `go test ./internal/pkg/packs/` + `go vet ./internal/pkg/packs/` green

## Slice 2 — install machinery (PR 2)

Files: `internal/pkg/packs/{download,copy,sidecar,install}.go` + `install_test.go`. Est: ~700. Risk: Medium.

- [x] 2.1 RED: `install_test.go` (FakeDownloader) — success (copy→sidecar→re-validate), module-not-found→`pack_not_found` + no partial dir, offline→`pack_fetch_failed` surfaced, corrupted ziphash abort→no dir, idempotent same-version reinstall, `templates/` missing→`pack_no_templates`, hooks accept→`HooksEnabled=true` / decline→`false`
- [x] 2.2 GREEN: `download.go` — `Downloader` interface, `RealDownloader` (`exec.CommandContext` `go mod download -json <mod>@<ver>`, parse `downloadJSON{Dir,Error}`, stderr captured separately), `FakeDownloader` (hooks `FakeRunner` precedent, in-package)
- [x] 2.3 GREEN: `copy.go` — `copyDir(src,dst)` via stdlib `WalkDir`, Windows-safe (filepath only, no shell)
- [x] 2.4 GREEN: `sidecar.go` — `Sidecar{HooksEnabled bool; InstalledAt time.Time}` read/write of `pack.json` in installed pack dir
- [x] 2.5 GREEN: `install.go` — `Install(module, version)` per design flow: Download→LoadManifest→templates/ exists check→hooks trust prompt (injectable confirm func; default `ui.Warning` + `survey.AskOne`)→`RemoveAll(dst)` if exists→copyDir to tmp→`Rename(tmp,dst)`→writeSidecar→re-validate→cleanup dst on failure; `Remove(name,ver)`, `List()` sorted, `Update(name)` (re-fetch `@latest`, re-prompt hooks)
- [x] 2.6 Verify: `go test ./internal/pkg/packs/` + vet green

## Slice 3 — template CLI group + engine chain (PR 3)

Files: `cmd/template.go` + `cmd/template_{install,list,remove,update}.go` + `cmd/template_test.go`, `internal/pkg/template/engine.go` + `engine_pack_test.go`, `docs/COMMANDS.md`, `README.md`, `docs/ARCHITECTURE.md`. Est: ~750. Risk: Medium.

- [x] 3.1 RED: `engine_pack_test.go` — 4-step precedence: pack overrides embedded (source `pack:express@1.0.0`), local overrides pack, global overrides pack, pack miss→embedded, no pack configured→3-step unchanged; `ResolveBinary` same precedence via `Read()` closure
- [x] 3.2 GREEN: `engine.go` — `Engine` gains packsDir/packName/packVersion + `NewEngine(opts...)`; `WithPacksDir(dir)`, `WithPack(name,ver)`; `getTemplate` pack step between global and embedded (`filepath.Join(packsDir, name+"@"+ver, "templates", path)`, source `"pack:<name>@<version>"`); `ResolveBinary(path)(ResolvedSource,error)` with `SourceKind` consts (Local/Global/Pack/Embedded) + `Read func() ([]byte, error)`; replace `fmt.Printf` @48 → `fmt.Fprintf(ui.Out, ...)`
- [x] 3.3 GREEN: `cmd/template.go` — `go-arch template` parent command registering install/list/remove/update
- [x] 3.4 GREEN: `cmd/template_install.go` — parse `"<module>[@<version>]"` (default `@latest`), call `packs.Install`, report via `ui.Out`
- [x] 3.5 GREEN: `cmd/template_list.go` — `packs.List`, sorted output, `no packs installed` when empty, exit 0
- [x] 3.6 GREEN: `cmd/template_remove.go` — `packs.Remove(name[,ver])`, latest when bare, `pack "X" is not installed` error
- [x] 3.7 GREEN: `cmd/template_update.go` — `packs.Update(name)`, re-prompt hooks
- [x] 3.8 RED: `cmd/template_test.go` — `t.Setenv("HOME", t.TempDir())`: list empty→`no packs installed`; remove non-installed→`pack "express" is not installed`; install without module arg→arg error
- [x] 3.9 GREEN: docs — `docs/COMMANDS.md` `template install|list|remove|update` section; `README.md:134-137` + `docs/ARCHITECTURE.md:57-80` lookup order → four steps
- [x] 3.10 Verify: `go test ./...` + vet green

## Slice 4 — dispatch: new --template + MCP (PR 4)

Files: `internal/pkg/scaffold/{scaffold,pack_resolver,manifest}.go` + `scaffold_pack_test.go`, `cmd/new.go` + `cmd/new_test.go`, `internal/ui/prompts.go`, `internal/pkg/hooks/{types,env}.go` + `env_test.go`, `internal/pkg/template/templates/common/config.tmpl`, `internal/pkg/mcp/server.go` + `server_test.go`. Est: ~760. Risk: High.

- [x] 4.1 RED: `scaffold_pack_test.go` E2E — fixture pack in `t.TempDir()` (pre-populated, no network): scaffoldPack generates files (strip `.tmpl`), layout dirs created, `.go-arch.yaml` contains `template: express`, manifest entries `source: pack:express@<ver>`, binary asset copied verbatim (origin `binary`), pack hooks fire (FakeRunner) with `PACK_NAME`/`PACK_VERSION`, project config has NO hooks block
- [x] 4.2 GREEN: `manifest.go` — `ManifestEntry.Source string yaml:"source,omitempty"`; `recordManifest` gains source param
- [x] 4.3 GREEN: `pack_resolver.go` — `Resolver` interface (`Resolve(name, ver) (packs.PackInfo, error)`) + default impl wrapping `packs.Path`/`LatestInstalled`; miss → `pack_not_installed`
- [x] 4.4 GREEN: `scaffold.go` — `packInfo` field + `WithPackInfo(packs.PackInfo)` option; `Execute` early-return pack branch → `s.scaffoldPack()` (before arch switch); `scaffoldPack()`: `MkdirAll(manifestDir)` + layout dirs, `fs.WalkDir(packDir/templates)` → strip `.tmpl` → `createFile(target, lookupKey, cfg)` → `recordManifest(Source="pack:<name>@<ver>")`; binary_assets loop → `createPackBinary(entry.Target, entry.Source, packDir)` via `ResolveBinary.Read()`; pre/post-new pack-scoped hooks; `WriteVersionField`
- [x] 4.5 GREEN: hooks `types.go`+`env.go` — `EnvContext` gains `PackName`/`PackVersion`; `BuildEnv` injects `PACK_NAME`/`PACK_VERSION` when non-empty; `env_test.go`: set→present, empty→absent
- [x] 4.6 GREEN: `cmd/new.go` — `--template` flag: bypass wizard, `ParseRef`→`LatestInstalled` if bare→`Resolver.Resolve`→empty-templates pre-check (`pack_no_templates`, NO dir created)→`packDefaults(projectName)` cfg→`cfg.Template=name`→`NewScaffolder(cfg, WithPackInfo, WithRunner, WithVersion)`
- [x] 4.7 GREEN: `prompts.go` — `ProjectConfig.Template string mapstructure:"template,omitempty"`; `config.tmpl` — conditional `{{ if .Template }}template: {{ .Template }}{{ end }}` block
- [x] 4.8 RED: `cmd/new_test.go` — empty pack → error BEFORE `NewScaffolder`, assert `myapp/` NOT created
- [x] 4.9 GREEN: `mcp/server.go` — `new_project` schema gains optional `template`; remove `architecture` from static `required`; handler validates "architecture required unless template set"; resolves pack → `cfg.Template` + `WithPackInfo`
- [x] 4.10 RED: `mcp/server_test.go` — `new_project` with `template` (no architecture) → `cfg.Template` propagates; missing pack → `pack "express" is not installed`; without template → unchanged behavior
- [x] 4.11 Verify: `go test ./...` + vet + gofmt green

## Slice 5 — upgrade pack-source + docs (PR 5)

Files: `internal/pkg/scaffold/{upgrade,upgrade_opts}.go` + `upgrade_test.go`, `docs/packs.md`. Est: ~550. Risk: Medium.

- [ ] 5.1 RED: `upgrade_test.go` — `renderEntry` with `Source: pack:...` reads from synthetic pack dir (bypasses chain); missing pack via injected resolver→PROTECTED + warning naming pack; version removed but newer installed→PROTECTED (no auto-substitute); non-pack entries unchanged
- [ ] 5.2 GREEN: `upgrade_opts.go` + `upgrade.go` — `UpgradeOption` + `WithResolver(Resolver)`; `Upgrade(cfg, ...UpgradeOption)`; `renderEntry` pack branch: `source` starts `pack:` → resolve via injected resolver → read `packInfo.Dir/templates/<TemplatePath>` directly (no chain fallback); resolve fail → `ClassProtected` + per-entry warning
- [ ] 5.3 GREEN: `docs/packs.md` — contract v1 schema table, install location, `template` command group, trust warning, four-step lookup, upgrade interaction, v2 deferral
- [ ] 5.4 Verify: `go test ./...` + `go vet ./...` + gofmt green; tick completed tasks

## Commit Plan (work units)

- PR 1: `feat(packs): add manifest types, strict validation and error codes` → `feat(packs): add pack paths, ref parsing and latest-installed resolution` → `test(packs): cover manifest validation and path resolution tables`
- PR 2: `feat(packs): add downloader with go mod download -json and fake for tests` → `feat(packs): add sidecar metadata and Windows-safe copy` → `feat(packs): implement install, remove, list, update with atomic replace` → `test(packs): cover install flows, trust sidecar and no-partial-state`
- PR 3: `feat(template): add pack step to engine chain and ResolveBinary` → `fix(template): route custom-template notice through ui.Out` → `feat(cli): add template install, list, remove, update commands` → `test(template): cover 4-step precedence and binary resolution` → `docs: document template command group and four-step lookup`
- PR 4: `feat(scaffold): add pack dispatch with WithPackInfo and scaffoldPack` → `feat(hooks): inject PACK_NAME and PACK_VERSION into hook env` → `feat(cli): add --template flag to new with wizard bypass` → `feat(mcp): accept optional template param in new_project` → `test(scaffold): E2E pack scaffold, source provenance and pack hooks`
- PR 5: `feat(scaffold): re-render pack-sourced entries and protect on missing pack` → `docs(packs): add pack contract reference` → `test(scaffold): cover pack upgrade re-render and PROTECTED`
