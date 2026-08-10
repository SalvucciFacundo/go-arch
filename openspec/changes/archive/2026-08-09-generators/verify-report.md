```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:ff5dc4855e57782dba070955db83bf0ed4b17ce5516bf2ea3e92d3a5f8da2712
verdict: pass
blockers: 0
critical_findings: 0
requirements: 31/31
scenarios: 75/75
test_command: go test ./... -count=1
test_exit_code: 0
test_output_hash: sha256:ff5dc4855e57782dba070955db83bf0ed4b17ce5516bf2ea3e92d3a5f8da2712
build_command: go vet ./...
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

# SDD Verify Report — generators (FINAL RE-RUN)

**Change**: generators (plugins v2)
**Version**: v2 contract
**Mode**: Strict TDD
**Branch**: feat/generators-5 (HEAD `725a9a9` — includes `41e3454` fix, `293acdd` spec/TDD amendment, `1d14ac3` second fix, `725a9a9` regression-guard tests)
**Date**: 2026-08-09 (final re-run after second corrective fix)
**Previous verdict**: FAIL (2 NEW CRITICAL regressions from the first corrective fix) — both resolved below.

## Status

**PASS** — 31/31 requirements, 75/75 scenarios compliant. The 2 CRITICAL regressions introduced by the first corrective fix are resolved with production-path regression-guard tests:

1. **REQ-10 S1 / REQ-22 S3 (pack_not_installed over-reach)**: `cmd/generate.go` and `mcp/server.go` now track a `packResolved` boolean — `pack_not_installed` fires only when the pack genuinely fails to resolve; an installed pack lacking the requested generator falls through to the `unknown_generator` grouped listing (pack / builtin / component) naming the pack's available generators. Live-confirmed.
2. **REQ-25 S1 (MCP prompt code regression)**: `scaffold.WithPromptErrorCode` option discriminates by caller — CLI non-interactive keeps `generator_prompt_unresolvable`, MCP keeps `missing_generator_argument`. Live-confirmed.

## Executive Summary

Full spec/design/tasks cross-check plus live smoke test executed on `feat/generators-5`. The suite is green (325 tests across 8 packages, vet + gofmt clean) and the happy path works end-to-end: recipe generation (template/binary/run/prompt/use), pre-flight path sandbox (zero writes on escape), HooksEnabled trust gate with skip warning, contract v2 manifest validation at load, three-tier dispatch with grouped unknown-generator listing, `pack_not_installed` only for genuinely missing packs, MCP `generate_component` relaxation + `generatorArgs` + `list_generators`, and `origin: generator` PROTECTED upgrade semantics. Report validated by `gentle-ai sdd-verify-validate` (valid: true, verdict pass).

## Previous Findings Resolved (this cycle)

| Finding | Resolved | Evidence |
|---------|----------|----------|
| CRITICAL-1: recipe validation not wired at load | ✅ | `packs/manifest.go:135-142` calls `generators.Validate` in UnmarshalYAML; `TestManifest_Load_V2InvalidRecipe_EmptySteps_Rejected` + `_UnknownStepType_Rejected` + positive control `TestManifest_Load_V2ValidRecipe_WithRunStep_ParsesOK` all PASS |
| CRITICAL-2: pack_not_installed never emitted | ✅ | `cmd/generate.go:146-152` + `mcp/server.go:505-511`; `TestGenerate_PackNotInstalled_Error`, `TestGenerate_PackNotInstalled_ComponentStillWorks`, `TestMCPServer_GenerateComponent_PackNotInstalled` PASS |
| WARNING: unknown_generator grouped listing | ✅ | `buildUnknownGeneratorError` groups by source; `TestGenerate_UnknownGenerator_GroupedListing` PASS |
| WARNING: hooks-skip warning | ✅ | `generators/executor.go` emits single warning; `TestRun_HooksEnabledFalse_SkipsPrePostHooks` PASS |
| WARNING: install trust text | ✅ | "declares hooks or generators that may run shell commands" |
| WARNING: MCP unknown-type code | ✅ | `unknown_generator` code in MCP response; with-template path covered by new test |
| WARNING: prompt error code (CLI) | ✅ | CLI non-interactive → `generator_prompt_unresolvable`; `TestRun_PreFlightPromptUnresolvable` (tightened to exact code) PASS |
| PARTIAL: dual-entry spec amendment (REQ-20) | ✅ | spec amended to single-entry-with-metadata; live manifest matches |
| PARTIAL: TDD evidence | ✅ | apply-progress honest (RED overclaims annotated, corrective-fix section) |
| NEW-CRITICAL-1: pack_not_installed over-reach | ✅ | `packResolved` discrimination; `TestGenerate_PackInstalled_UnknownGenerator_GroupedListing` + `TestMCPServer_GenerateComponent_WithTemplate_UnknownType` PASS — error contains `docker`/`service` group, NOT "not installed" |
| NEW-CRITICAL-2: MCP prompt code regression | ✅ | `WithPromptErrorCode` option; `TestMCPServer_GenerateComponent_MissingRequiredArg` PASS — error contains `missing_generator_argument`, NOT `generator_prompt_unresolvable` |

## Requirements Verdict (31 total)

**31 PASS / 0 PARTIAL / 0 FAIL** — REQ-01 through REQ-31 all compliant. Previously-failing items REQ-10 (S1), REQ-12 (S1), REQ-22 (S3), REQ-25 (S1), REQ-26 (S1) all upgraded to PASS this cycle.

## Test Evidence

- `go test ./... -count=1` → exit 0, 8/8 packages ok, 325 tests pass / 0 fail / 0 skip (hash `sha256:ff5dc485...`)
- `go vet ./...` → exit 0, clean
- `gofmt -l .` → empty
- Regression-guard tests (all PASS):
  - `TestGenerate_PackInstalled_UnknownGenerator_GroupedListing` — CLI, installed pack, unknown gen → grouped listing with pack generators
  - `TestMCPServer_GenerateComponent_WithTemplate_UnknownType` — MCP, same scenario
  - `TestMCPServer_GenerateComponent_MissingRequiredArg` — MCP missing required prompt → `missing_generator_argument`
  - `TestRun_PreFlightPromptUnresolvable` — tightened to exact `generator_prompt_unresolvable`
  - `TestManifest_Load_V2ValidRecipe_WithRunStep_ParsesOK` — positive control REQ-03 S3
  - `TestGenerate_PackNotInstalled_Error` / `_ComponentStillWorks` — missing pack still correct
  - `TestMCPServer_GenerateComponent_PackNotInstalled` — MCP missing pack path intact

## Live Spot-Check (executed 2026-08-09, after second fix)

| Scenario | Result |
|----------|--------|
| Invalid recipe pack (empty steps) → `packs.Load` | ✅ Rejected: "has no steps" |
| Unknown step type pack → `packs.Load` | ✅ Rejected: "step 0: unknown step type" |
| Missing pack → `generate` | ✅ `pack_not_installed` naming pack + install hint; component types still work |
| Installed pack, unknown generator → `generate bogus` | ✅ `unknown_generator` grouped listing (NOT "not installed") — pack generators named |
| Valid recipe → `generate docker` | ✅ Files + run marker + pre/post hooks; provenance exact (`origin: template/generator`, `metadata.generator`, args JSON); `template:` data isolation |
| Path escape attempt | ✅ `recipe_path_escape` — zero writes (pre-flight) |
| HooksEnabled=false + recipe with hooks | ✅ Warning `generator_run_skipped_trust`; steps skip safely |
| MCP `generate_component` + generatorArgs | ✅ args → `metadata.args {"db_driver":"mysql"}`; missing required arg → `missing_generator_argument` |
| MCP `list_generators` | ✅ returns pack generator with description |
| Upgrade | ✅ `origin: generator` → PROTECTED (exact spec warning); template-step entry → upgradable |
| v1 pack regression | ✅ v1 contract still accepted; v1+generators rejected |

## Verdict

**PASS** — 31/31 requirements, 75/75 scenarios compliant. 0 blockers, 0 critical findings. Validator admission: `gentle-ai sdd-verify-validate --requirements 31 --scenarios 75` → `{valid: true, verdict: pass}`.

**Blockers for archive**: none.

## Next Recommended

`archive` — the change is complete and archive-ready. Chain PRs #39→#43 + tracker → main, then archive.

## Risks (non-blocking, recorded)

- Latent (unchanged): builtin-before-pack dispatch order (REQ-09) becomes live only if a builtin is ever registered colliding with a pack generator — inert with the empty v2 registry; flip order in a follow-up before registering any builtin.
- MCP unknown-generator error formatting uses `formatMCGeneratorError` (prepends the oops code); non-generator MCP failures unaffected.
- Coverage of mcp/cmd for generator paths remains moderate (informational; guarded by production-path tests).
