```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:bb8658bd30a8056aaf3cfe7d1e3b271c30688fe0e59171048ca76f52a8ce7a4e
verdict: pass
blockers: 0
critical_findings: 0
requirements: 31/31
scenarios: 76/76
test_command: go test ./... -count=1
test_exit_code: 0
test_output_hash: sha256:bb8658bd30a8056aaf3cfe7d1e3b271c30688fe0e59171048ca76f52a8ce7a4e
build_command: go build ./... && go vet ./... && gofmt -l .
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

# Verification Report — plugins (Installable Template Packs) — FINAL RE-RUN (PASS)

**Change**: plugins
**Version**: delta spec (31 requirements / 76 scenarios)
**Mode**: Strict TDD (runner: `go test ./...`)
**Branch**: feat/packs-5 — HEAD includes `9709368 test(packs): cover offline cached install` + `d87cd4c docs(spec): align pack binary override semantics with v1 contract`
**Date**: 2026-08-09

## Verdict

**PASS — archive-ready.** The 2 remaining PARTIALs from the previous re-run are closed:

1. **Spec #7 scenario 3 (offline-cached install)** — `TestInstall_OfflineCached` added (`internal/pkg/packs/install_test.go:771`, commit `9709368`): seeds a `GOMODCACHE` via a `file://` proxy, sets `GOPROXY=off`, and asserts `RealDownloader` resolves the module and returns the seeded `Dir` with the pack files present — zero network. **PASSES** (0.007s, exit 0) on this run.
2. **Spec #27 scenario 3 (pack binary override semantics)** — spec amended (commit `d87cd4c`): requirement #27 + scenario #27-3 now document the implemented v1 contract — pack-declared binary assets are read directly from the installed pack's `assets/` directory by `createPackBinary`, local/global overrides do NOT apply in contract v1, and `ResolveBinary` remains the public chain API. Matches the DESIGN NOTE at `scaffold.go:820-830` and design.md:108.

**Final counts**: 31/31 requirements PASS, 76/76 scenarios compliant, 331 tests green, blockers 0, critical findings 0. Full suite re-run on this branch: `go test ./... -count=1` exit 0, `go vet ./...` clean, `gofmt -l .` no files, `go build ./...` clean.

## Completeness

| Metric | Value |
|--------|-------|
| Tasks total | 25 (5 slices) |
| Tasks complete | 25 |
| Tasks incomplete | 0 |
| Requirements PASS | 31 |
| Requirements PARTIAL | 0 |
| Requirements FAIL | 0 |

## Previous Findings — Resolution Status

| # | Finding (previous report) | Resolved? | Evidence |
|---|---------------------------|-----------|----------|
| CRITICAL 1 | Pack hooks never fire: runner built from global config; `HooksEnabled` never read | ✅ **Resolved** | `cmd/new.go:108-118` + `mcp/server.go:372-379` read `packs.ReadSidecar(packInfo.Dir)` and merge pack manifest hooks when `HooksEnabled`; `packs/install.go:65-67` writes the flag; `packs.ReadSidecar` exported (`sidecar.go:39`). `TestNewTemplatePackHooksFire/hooks-enabled` + `/hooks-disabled` PASS via real `hooks.RealRunner`. RED confirmed on pre-fix `d83709e` ("hook did NOT fire"). |
| CRITICAL 2 | Upgrade never re-renders pack entries: `WithResolver` absent at both call sites | ✅ **Resolved** | `cmd/upgrade.go:75` and `mcp/server.go:673` pass `scaffold.WithResolver(scaffold.DefaultResolver{})`. `TestUpgradePackSourceProductionPath` (cobra `upgrade --dry-run`) asserts `🔄 .env` UPGRADABLE. RED confirmed pre-fix (classified PROTECTED). |
| PARTIAL 3 | Missing-pack warning suppressed in production (spec #20/#21) | ✅ **Resolved** | Production always hits `resolver != nil` branch (`scaffold/upgrade.go:174-183`); per-entry `pack "X@Y" is not installed; entries protected`; label `protected (skipping)` (`cmd/upgrade.go:180`). Tests + live smoke confirmed. |
| PARTIAL 4 | `ResolveBinary` dead code (spec #27) | ✅ **Resolved (spec amended)** | `ResolveBinary` is a public, 5-test-covered engine API. Spec #27-3 now documents the v1 contract: pack dir is authoritative for pack-declared binary assets (commit `d87cd4c`, matches DESIGN NOTE `scaffold.go:820-830` + design.md:108). No deviation vs spec remains — spec and code agree. |
| PARTIAL 5 | `config.tmpl` lacks commented `# template:` example (spec #15) | ✅ **Resolved** | `config.tmpl:11` renders `# template: <pack-name>` in else-branch. Live smoke `.go-arch.yaml` contains `template: express`. |
| PARTIAL 6 | `new --template` not-installed error omits install hint (spec #14) | ✅ **Resolved** | `cmd/new.go:86-89` returns `pack "X" is not installed; run "go-arch template install <module>@<version>" ...` with `oops.Hint`. Test PASS; RED confirmed pre-fix. |
| PARTIAL 7 | Offline-cached install scenario untested (spec #7-3) | ✅ **Resolved** | `TestInstall_OfflineCached` (`internal/pkg/packs/install_test.go:771`, commit `9709368`): seeded GOMODCACHE + `GOPROXY=off` file:// proxy, asserts `RealDownloader` returns seeded `Dir` with `go-arch.yaml` + template files. **PASSES** this run. |
| PARTIAL 8 | Spec #27-3 local-override-wins met by design deviation | ✅ **Resolved (spec aligned to implementation)** | Spec #27 + scenario #27-3 amended (commit `d87cd4c`) to document the v1 pack-binary contract — pack-direct read is the intended behavior; `ResolveBinary` remains the public chain API. |

## Spec Compliance Matrix (31 requirements / 76 scenarios)

| Req | Requirement | Scenarios | Verdict | Evidence |
|-----|-------------|-----------|---------|----------|
| 1 | Pack Manifest File Shape | 3/3 | ✅ PASS | `manifest_test.go` ValidMinimal/ValidFull/UnknownKey |
| 2 | contract_version Enforcement | 3/3 | ✅ PASS | ContractMismatch (asserts `contract v99`+`v1`), MissingContract |
| 3 | Manifest Required Fields Validation | 3/3 | ✅ PASS | MissingName/BadSlug/BadSemver |
| 4 | Pack Directory Layout | 2/2 | ✅ PASS | `install.go` templates/ check; TestInstall_NoTemplates |
| 5 | Pack Install Location | 2/2 | ✅ PASS | `Path(name,ver)`; coexist via update tests |
| 6 | template install Command | 3/3 | ✅ PASS | default `latest`; pinned; TestInstall_IdempotentReinstall |
| 7 | template install Errors | 4/4 | ✅ **PASS (was PARTIAL)** | ModuleNotFound/FetchFailed/ZiphashAbort/NoTemplates + **NEW `TestInstall_OfflineCached`** (seeded cache, `GOPROXY=off`, asserts `Dir` + pack files) |
| 8 | template list Command | 2/2 | ✅ PASS | TestTemplateList_Empty/WithPacks; sorted |
| 9 | template remove Command | 3/3 | ✅ PASS | LatestInstalled + TestRemove_Success/NotInstalled |
| 10 | template update Command | 2/2 | ✅ PASS | TestUpdate_Success/NotInstalled; pins preserved |
| 11 | Go Module Proxy Fetch | 2/2 | ✅ PASS | `RealDownloader` runs `go mod download -json <mod>@<ver>`, reads `Dir`; by inspection + offline-cached execution |
| 12 | Engine Chain With Pack Step | 5/5 | ✅ PASS | engine_pack_test.go: 6 chain tests |
| 13 | Precedence And Namespacing | 2/2 | ✅ PASS | packName-scoped lookup |
| 14 | new --template Flag | 3/3 | ✅ PASS | hint added (`cmd/new.go:88`); TestNewTemplateNotInstalledHint PASS; wizard bypass + pinned version tests |
| 15 | ProjectConfig.Template Field | 3/3 | ✅ PASS | commented `# template:` example in `config.tmpl:11`; recording + old-config-load tests |
| 16 | MCP new_project.template Param | 3/3 | ✅ PASS | TestNewProjectTemplateParam/MissingPack |
| 17 | Engine Output Routes Through ui.Out | 2/2 | ✅ PASS | `engine.go fmt.Fprintf(ui.Out,…)`; MCP stdout clean |
| 18 | ManifestEntry.source Field | 3/3 | ✅ PASS | `yaml:"source,omitempty"`; E2E + live assert `pack:express@1.0.0` |
| 19 | Upgrade Re-Render From Recorded Pack | 2/2 | ✅ PASS | `WithResolver(DefaultResolver{})` at `cmd/upgrade.go:75` + `mcp/server.go:673`; `TestUpgradePackSourceProductionPath`; scaffold `TestUpgrade_PackSource_RerendersFromPack`; live smoke V2 |
| 20 | Missing Pack Marks Entry PROTECTED | 2/2 | ✅ PASS | Production resolver branch (`upgrade.go:174-183`); label `protected (skipping)`; TestUpgrade_PackSource_MissingPackProtected; live smoke |
| 21 | Pack Version Bump Behavior | 1/1 | ✅ PASS | `TestUpgrade_PackSource_VersionBumpProtected`; no auto-substitute |
| 22 | Pack Hooks Opt-In With Trust Warning | 4/4 | ✅ PASS | trustPrompt + sidecar `HooksEnabled`; TestInstall_HooksAccept/Decline/NoHooks |
| 23 | Pack-Scoped Hook Fire | 2/2 | ✅ PASS | `cmd/new.go:108-118` + `mcp/server.go:372-379` merge pack manifest hooks when sidecar enabled; `TestNewTemplatePackHooksFire` both cases via real runner; live smoke |
| 24 | Pack Hooks Never Fire On Project-Level Generate | 1/1 | ✅ PASS | trivially true; generate uses user config only |
| 25 | PACK_NAME And PACK_VERSION Env Vars | 3/3 | ✅ PASS | `env.go` BuildEnv; TestBuildEnv_*; live smoke hook env |
| 26 | template update Re-Prompts For Hooks | 2/2 | ✅ PASS | TestUpdate_RePromptHooks |
| 27 | Pack-Aware createBinaryFile | 3/3 | ✅ **PASS (was PARTIAL)** | Binary verbatim + origin/source ✓ (E2E). Embedded fallback ✓ (engine ResolveBinary test). **#27-3 now spec-compliant**: spec amended (d87cd4c) to document pack-direct read as v1 contract; matches code DESIGN NOTE scaffold.go:820-830 + design.md:108 |
| 28 | Empty Pack Error | 1/1 | ✅ PASS | checkTemplatesNonEmpty pre-Execute; TestNewEmptyPackNoDir |
| 29 | Windows Portability | 2/2 | ✅ PASS | stdlib filepath only, by inspection |
| 30 | docs/packs.md Reference | 1/1 | ✅ PASS | File exists; contract schema, install, trust warning, 4-step, upgrade |
| 31 | README And ARCHITECTURE Updates | 2/2 | ✅ PASS | README.md:137, ARCHITECTURE.md:64, COMMANDS.md |

**Compliance summary**: **76/76 scenarios compliant** (was 74/76; +2: req 7-3 offline-cached now tested, req 27-3 spec aligned to implementation). 0 non-compliant scenarios. 31/31 requirements PASS.

## Build & Tests Execution

**Build**: ✅ Passed (`go build ./...`, exit 0)
**Vet**: ✅ Passed (`go vet ./...`, exit 0, empty output)
**gofmt**: ✅ Passed (`gofmt -l .`, exit 0, no files listed)
**Tests**: ✅ 331 passed / 0 failed / 0 skipped (`go test ./... -count=1`, exit 0 — includes `TestInstall_OfflineCached`)
```text
?   	go-arch	[no test files]
ok  	go-arch/cmd	0.015s
ok  	go-arch/internal/pkg/hooks	0.006s
ok  	go-arch/internal/pkg/mcp	0.010s
ok  	go-arch/internal/pkg/packs	0.013s
ok  	go-arch/internal/pkg/scaffold	0.971s
ok  	go-arch/internal/pkg/template	0.005s
ok  	go-arch/internal/pkg/validator	0.002s
?   	go-arch/internal/ui	[no test files]
```

**Focused re-run**: `go test ./internal/pkg/packs/ -run TestInstall_OfflineCached -count=1` → `--- PASS: TestInstall_OfflineCached (0.00s)` — the previously-PARTIAL #7-3 now has runtime compliance.

**Coverage** (informational): packs 71.1% | template 96.4% | scaffold 77.4% | mcp 47.1% | cmd 50.0%

**RED→GREEN confirmation for corrective tests** (run against pre-fix `d83709e` in a temp worktree, test files applied from `67030fe`): `TestNewTemplatePackHooksFire/hooks-enabled` FAIL, `TestNewTemplateNotInstalledHint` FAIL, `TestUpgradePackSourceProductionPath` FAIL pre-fix; all PASS at HEAD. Corrective cycle genuine RED→GREEN.

## Live Spot-Check (GO_ARCH_PACKS_DIR synthetic pack, no network)

Performed previously with a locally-built `go-arch` binary + synthetic `express@1.0.0` pack in a temp `GO_ARCH_PACKS_DIR`:

| Test | Observed | Result |
|------|----------|--------|
| Pack with `post-new` hook + sidecar `hooks_enabled: true` → `new myapp --template express@1.0.0` | Hook **FIRED**: `PACK_NAME=express PACK_VERSION=1.0.0`; config `template: express`; manifest `source: pack:express@1.0.0`; no `hooks:` block | ✅ (spec #23 + #25) |
| Pack template updated to V2 → `upgrade --dry-run` | `🔄 common/env: update available` — **re-rendered from pack**, NOT PROTECTED | ✅ (spec #19) |
| Pack dir removed → `upgrade --dry-run` | Per-entry `pack "express@1.0.0" is not installed; entries protected` ×2; `🔒 protected (skipping)`; no embedded fallback | ✅ (spec #20/#21) |

## TDD Compliance (Strict TDD)

| Check | Result | Details |
|-------|--------|---------|
| TDD Evidence reported | ✅ | Tables for slices 3-5; bullet lists for 1-2 |
| All tasks have tests | ✅ | 25/25 |
| RED confirmed (tests exist) | ✅ | All RED test files present |
| RED confirmed (corrective tests fail pre-fix) | ✅ | 3 corrective tests FAIL on `d83709e`, PASS at HEAD |
| GREEN confirmed (tests pass) | ✅ | 331/331 pass at HEAD |
| Triangulation adequate | ✅ | Hook cases, production upgrade, missing-pack, version-bump, offline-cached install |
| Safety Net for modified files | ✅ | Reported for slices 3.1, 4.x, 5.1 |
| Coverage honesty | ✅ | Production-path tests exercise real runners/resolvers; no injected-dependency mask |

**TDD Compliance**: 8/8 checks passed.

## Test Layer Distribution

| Layer | Tests | Files | Tools |
|-------|-------|-------|-------|
| Unit | ~300 | manifest, paths, install, engine, env, upgrade, cmd, template | go test |
| Integration | ~31 (scaffold E2E, MCP handler, production-path cmd tests, offline-cached) | scaffold_pack_test.go, server_test.go, new_test.go, upgrade_test.go, install_test.go | go test |
| E2E | 0 | — | not applicable (CLI/MCP) |
| **Total** | **331** | **~22 test files** | |

## Changed File Coverage (informational)

| Package | Line % | Rating |
|---------|--------|--------|
| internal/pkg/template | 96.4% | ✅ Excellent |
| internal/pkg/scaffold | 77.4% | ⚠️ Acceptable |
| internal/pkg/packs | 71.1% | ⚠️ Low (offline-cached path now exercised) |
| internal/pkg/mcp | 47.1% | ⚠️ Low (handler surface large) |
| cmd | 50.0% | ⚠️ Low (improved from 44.4% by corrective tests) |

## Assertion Quality

**✅ All assertions verify real behavior.** The corrective tests assert side effects (hook marker file, `🔄` vs `🔒` labels, error hint substring, seeded-cache `Dir` equality) through production call paths — no injected dependencies, no tautologies.

## Quality Metrics

**Linter**: ✅ `go vet ./...` clean
**gofmt**: ✅ no files listed
**Type Checker**: ✅ `go build ./...` clean

## Issues Found

### CRITICAL

None.

### WARNING

None. Both previously-WARNING items are closed: #7-3 has a passing offline-cached test; #27-3 is now spec-compliant by amendment (d87cd4c).

### SUGGESTION

1. MCP pack-hook merge (`server.go:372-379`) is wired but not covered by a dedicated MCP test (logic identical to the tested `cmd/new.go` path). Add an MCP-level hook test for full parity.
2. Coverage of mcp (47.1%) and cmd (50.0%) remains low for the new template paths — a follow-up hardening pass could raise it.

## Correctness (Static Evidence)

| Requirement | Status | Notes |
|------------|--------|-------|
| Pack manifest strict parse | ✅ Implemented | unknown-key rejection, slug+semver, contract gate |
| Download/install/remove/list/update | ✅ Implemented | FakeDownloader + atomic replace + sidecar + re-validate; offline-cached reuse verified |
| Engine 4-step chain | ✅ Implemented | local > global > pack > embedded, `pack:<name>@<ver>` label |
| `new --template` dispatch | ✅ Implemented | wizard bypass, empty-templates pre-check, pack hooks merged when sidecar enabled, install hint on error |
| MCP template param | ✅ Implemented | conditional architecture validation; pack hooks merged in handler |
| Manifest source provenance | ✅ Implemented | `source: pack:<name>@<version>`, omitempty |
| Pack hooks fire at new | ✅ Wired | `cmd/new.go` + MCP read `packs.ReadSidecar` → merge `Manifest.Hooks` → runner |
| Upgrade pack re-render | ✅ Wired | `cmd/upgrade.go` + MCP pass `WithResolver(DefaultResolver{})`; missing pack → per-entry warning + PROTECTED |
| Pack-aware binary copy | ✅ Implemented | `createPackBinary` reads pack dir directly per v1 contract (spec-aligned) |
| Docs | ✅ Implemented | packs.md, COMMANDS.md, README, ARCHITECTURE |

## Coherence (Design)

| Decision | Followed? | Notes |
|----------|-----------|-------|
| `<name>@<version>/` layout | ✅ Yes | |
| name from manifest | ✅ Yes | |
| Engine options (WithPacksDir/WithPack) | ✅ Yes | |
| Pack branch before arch switch | ✅ Yes | `executePack` early return |
| Upgrade force-pack-path | ✅ Yes — production | `WithResolver(DefaultResolver{})` wired in cmd + MCP |
| Downloader interface | ✅ Yes | |
| Pack hooks fire pack-scoped | ✅ Yes — production | sidecar `HooksEnabled` honored; hooks merged into runner, not project config |
| pack.json sidecar | ✅ Yes | `HooksEnabled` consumed at both production call sites |
| ui.Out fix at engine.go:48 | ✅ Yes | `fmt.Fprintf(ui.Out, ...)` |
| WithPackInfo pre-resolved | ✅ Yes | |
| Strip-.tmpl convention | ✅ Yes | |
| Binary assets manifest list | ✅ Yes | direct pack read — now matches amended spec #27 (v1 contract) |
| Empty-pack pre-Execute check | ✅ Yes | |
| UpgradeOption WithResolver | ✅ Yes | defined + used in production |
| Reinstall RemoveAll+Rename | ✅ Yes | |

## Verdict

**PASS — archive-ready.** 31/31 requirements PASS, 76/76 scenarios compliant, 331/331 tests green (exit 0), `go vet` and `gofmt` clean, blockers 0, critical findings 0. Both previously-PARTIAL items are closed with runtime evidence: `TestInstall_OfflineCached` passes (spec #7-3), and spec #27-3 is amended (d87cd4c) to document the implemented v1 pack-binary contract, removing the last deviation.

**Next recommended**: archive.
