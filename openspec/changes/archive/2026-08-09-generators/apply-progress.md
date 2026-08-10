# Apply Progress: generators — Slice 4

**Status**: success
**Branch**: feat/generators-4 (pushed)
**Target**: feat/generators-3 (parent in feature-branch-chain)

## TDD Cycle Evidence (Slice 4)

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| 4.2 | manifest.go | — | N/A (struct) | N/A | ✅ Constants added | ➖ Structural | ✅ Clean |
| 4.1 | scaffold_generator_test.go | Integration | ✅ scaffold 12/12 | ✅ Compile fail (GeneratePackGenerator undefined) | ✅ 7 tests pass | ✅ 7 cases | ✅ mapPromptResolver |
| 4.3 | scaffold.go | — | N/A | N/A | ✅ 7 tests green | N/A | ✅ Clean |
| 4.4 | cmd/generate_dispatch_test.go | Unit | ✅ cmd 8/8 | ✅ Fail (--list unknown flag) | ✅ 5 tests pass | ✅ 5 cases (args, --list, deterministic, unknown generator) | ✅ Clean |
| 4.5 | cmd/generate.go | — | N/A | N/A | ✅ 5 tests green | N/A | ✅ Clean |
| 4.6 | mcp/server_generator_test.go | Integration | ✅ mcp 10/10 | ✅ Fail (no list_generators tool) | ✅ 3 tests pass (tools, type relaxed, unknown type - pack generator test not written in RED) | ✅ 3 cases | ✅ structured JSON |
| 4.7 | mcp/server.go | — | N/A | N/A | ✅ 4 tests green | N/A | ✅ structured JSON |
| 4.8 | (verify) | — | N/A | N/A | ✅ go test ./... + vet + fmt | N/A | N/A |

## Test Summary (Slice 4)
- **Total tests written**: 15 (scaffold: 7, cmd: 5, mcp: 3) — note: tasks 4.4/4.6 RED evidence was overstated in original report; 4.6's fixture-pack generator test was not written as standalone RED
- **Total tests passing**: 15 + all pre-existing
- **Layers used**: Unit (5), Integration (10)

## Cumulative Test Summary (Slices 1+2+3+4)
- **Total tests written**: 89 (32 S1 + 23 S2 + 19 S3 + 15 S4)
- **Total tests passing**: 89
- **Layers used**: Unit (79), Integration (10)

## Work Done (Slice 4)

### Files Created
| File | Lines | Purpose |
|------|-------|---------|
| internal/pkg/scaffold/scaffold_generator_test.go | 690 | 7 E2E tests: recipe execution, manifest provenance, path escape, trust gate, data isolation |
| cmd/generate_dispatch_test.go | 222 | 5 tests: --list arg validation, dispatch, deterministic output, unknown generator |
| internal/pkg/mcp/server_generator_test.go | 199 | 4 tests: tools/list, type relax, unknown type error, list_generators structured output |

### Files Modified
| File | Change | Purpose |
|------|--------|---------|
| internal/pkg/scaffold/manifest.go | +6/-4 | OriginGenerator, OriginTemplate constants |
| internal/pkg/scaffold/scaffold.go | +205 | GeneratePackGenerator with prompt pre-flight, sandbox, manifest recording + mapPromptResolver |
| internal/pkg/hooks/runner.go | +6 | CommandRunner() accessor for generators integration |
| cmd/generate.go | +216/-28 | --list flag, custom args validator, 3-tier dispatch, unknown_generator error |
| internal/pkg/mcp/server.go | +185/-28 | Relax type enum, add generatorArgs + list_generators tool |

## Deviations from Design
- No interactive survey prompt for non-TTY — prompts resolve from args + defaults only. Interactive prompts deferred per design.
- The generator prompt resolution in scaffold does a pre-flight check, then also provides a PromptResolver to the executor to prevent the executor from overwriting resolved values with defaults.
- list_generators MCP response uses structured JSON (not text) for programmatic consumption.

## Issues Found
1. Cobra BoolVar flag state persists across test runs — switched to Flag().Bool() + GetBool().
2. manifestDir() falls back to ProjectName when no manifest exists — tests must create .go-arch/manifest.yaml.
3. Executor's internal prompt resolution would overwrite pre-resolved args — fixed with mapPromptResolver.

## Commits
| Hash | Message |
|------|---------|
| 69e89e5 | feat(scaffold): add GeneratePackGenerator with prompt pre-flight and provenance |
| 6a0d801 | feat(cli): add three-tier generate dispatch and --list |
| 815e8af | feat(mcp): relax generate_component, add generatorArgs and list_generators |

## Test Evidence (Task 4.8)

```
go test ./... -count=1 -short: 8 packages PASS
go vet ./...: clean
gofmt -w .: clean
```

## Branch State
- feat/generators-4 pushed to origin
- Targets feat/generators-3 (parent in feature-branch-chain)
- Next slice: create feat/generators-5 from feat/generators-4

## Remaining Tasks
- [ ] Slice 5 — upgrade PROTECTED + docs (~500 lines, PR 5)

---

# Apply Progress: generators — Slice 5 (FINAL)

**Status**: success
**Branch**: feat/generators-5 (pushed)
**Target**: feat/generators-4 (parent in feature-branch-chain)

## TDD Cycle Evidence (Slice 5)

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| 5.1 | upgrade_test.go | Unit | ✅ scaffold 21/21 | ✅ Warning missing (RED) | ✅ 4 tests pass | ✅ 4 cases (generator PROTECTED, pack removed, template metadata upgradable, no re-run) | ✅ Clean |
| 5.2 | upgrade.go | — | N/A | N/A | ✅ Early branch + warning | N/A | ✅ Clean |
| 5.3 | docs/packs.md | — | N/A (docs) | N/A | ✅ Contract v2 section added | N/A | ✅ Cognitive-doc-design |
| 5.4 | docs/COMMANDS.md, README.md | — | N/A (docs) | N/A | ✅ Generator usage + --list + MCP | N/A | ✅ Clean |
| 5.5 | (verify) | — | N/A | N/A | ✅ go test ./... + vet + fmt | N/A | N/A |

## Test Summary (Slice 5)
- **Total tests written**: 4
- **Total tests passing**: 4 + all pre-existing 90
- **Layers used**: Unit (4)

## Cumulative Test Summary (All Slices)
- **Total tests written**: 94 (32 S1 + 23 S2 + 19 S3 + 16 S4 + 4 S5)
- **Total tests passing**: 94
- **Layers used**: Unit (83), Integration (11)

## Work Done (Slice 5)

### Files Modified
| File | Change | Purpose |
|------|--------|---------|
| internal/pkg/scaffold/upgrade.go | +15 | Early branch: OriginGenerator → ClassProtected + per-entry warning |
| internal/pkg/scaffold/upgrade_test.go | +371 | 4 tests: PROTECTED classification, pack-removed still protected, template-origin upgradable via renderPackEntry, no generator re-run |
| docs/packs.md | +116/-10 | Contract v2 — Generators section: recipe DSL, trust model, path sandbox, provenance/upgrade semantics, lookup order |
| docs/COMMANDS.md | +26/-2 | Generator resolution order, --list output, MCP list_generators tool |
| README.md | +14/-2 | Generator usage examples, --list, MCP tools |

## Commits
| Hash | Message |
|------|---------|
| 19c405c | feat(scaffold): protect generator-origin entries during upgrade |
| 219209f | docs(packs): document contract v2 and generators reference |
| afa7252 | chore(sdd): mark slice 5 tasks complete in tasks.md |

## Test Evidence (Task 5.5)

```
go test ./...: 8 packages PASS (all green)
go vet ./...: clean
gofmt -w .: clean
```

## Branch State
- feat/generators-5 pushed to origin
- Targets feat/generators-4 (parent in feature-branch-chain)
- PR pending: user to create from feat/generators-5 → feat/generators-4

## Remaining Tasks
- None — all 5 slices complete. Ready for sdd-verify and sdd-archive.

---

# Apply Progress: generators — Corrective Fix (Post-Verify)

**Status**: success
**Branch**: feat/generators-5 (pushed)
**Verbatim**: fix(generators) per verify-report CRITICALs + WARNINGs

## Findings Fixed

| Finding | Severity | What Changed | Files |
|---------|----------|-------------|-------|
| CRITICAL-1 | Recipe validation not wired | Added `generators.Validate` call in manifest Load after generator decode | `packs/manifest.go`, `packs/manifest_v2_test.go` |
| CRITICAL-2 | pack_not_installed never emitted | Added pack-not-installed check in cmd/generate.go and MCP generate_component handler | `cmd/generate.go`, `cmd/generate_dispatch_test.go`, `mcp/server.go`, `mcp/server_generator_test.go` |
| WARNING-3 | unknown_generator flat listing | Grouped error listing by source (pack/builtin/component types) | `cmd/generate.go`, `cmd/generate_dispatch_test.go` |
| WARNING-4 | hooks-skip warning missing | Added single warning when pre/post hooks skipped due to HooksEnabled=false | `generators/executor.go`, `generators/executor_test.go` |
| WARNING-5 | install trust text | Updated warning to mention "hooks or generators" | `cmd/template_install.go` |
| WARNING-6 | MCP unknown-type code | Aligned MCP unknown-type error to include `unknown_generator` code | `mcp/server.go`, `mcp/server_generator_test.go` |
| WARNING-7 | prompt error code | Changed `missing_generator_argument` → `generator_prompt_unresolvable` in scaffold prompt resolution | `scaffold/scaffold.go` |
| PARTIAL-8 | dual-entry spec | Amended REQ-20 spec to reflect single-entry-with-metadata refinement | `specs/generators/spec.md` |
| PARTIAL-9 | TDD overclaim | Corrected S4 test counts and RED claims | `apply-progress.md` |

## Test Evidence

```
go test ./... -count=1: 8 packages PASS (320 tests, up from 313)
go vet ./...: clean
gofmt -l .: clean
```

New tests (7):
- TestManifest_Load_V2InvalidRecipe_EmptySteps_Rejected (packs)
- TestManifest_Load_V2InvalidRecipe_UnknownStepType_Rejected (packs)
- TestGenerate_PackNotInstalled_Error (cmd)
- TestGenerate_PackNotInstalled_ComponentStillWorks (cmd)
- TestGenerate_UnknownGenerator_GroupedListing (cmd)
- TestMCPServer_GenerateComponent_PackNotInstalled (mcp)
- TestRun_HooksEnabledFalse_SkipsPrePostHooks (generators)

## Commits
| Hash | Message |
|------|---------|
| 41e3454 | fix(generators): wire recipe validation at load and emit pack_not_installed |
| 293acdd | docs(sdd): amend spec for dual-entry refinement and fix TDD evidence |
| 1d14ac3 | fix(generators): distinguish missing pack from absent generator and restore MCP arg code |
| 725a9a9 | test(generators): cover dispatch discrimination and MCP/CLI prompt codes |

## Second Corrective Fix (Post-ReVerify) — 2026-08-09

### Regressions Fixed

| # | Finding | Fix | Evidence |
|---|---------|-----|----------|
| NEW-CRITICAL-1 | pack_not_installed over-reach (REQ-10 S1 / REQ-22 S3) | Track `packResolved` in cmd/generate.go and mcp/server.go; only emit pack_not_installed when pack genuinely fails to resolve; when pack IS installed but lacks generator, fall through to unknown_generator grouped listing | `TestGenerate_PackInstalled_UnknownGenerator_GroupedListing` (CLI) + `TestMCPServer_GenerateComponent_WithTemplate_UnknownType` (MCP) |
| NEW-CRITICAL-2 | MCP prompt code regression (REQ-25 S1) | Add `GeneratePackOption` / `WithPromptErrorCode` to scaffold; CLI uses default `CodeGeneratorPromptUnresolvable`, MCP passes `CodeMissingGeneratorArgument` | `TestMCPServer_GenerateComponent_MissingRequiredArg`; tightened `executor_test.go` L416 |

### Files Changed

| File | Action | What Was Done |
|------|--------|---------------|
| `internal/pkg/scaffold/scaffold.go` | Modified | Added `GeneratePackConfig`, `GeneratePackOption`, `WithPromptErrorCode`; `GeneratePackGenerator` accepts variadic opts; `mapPromptResolver` uses configurable `errorCode` |
| `cmd/generate.go` | Modified | Track `packResolved` bool; only emit `pack_not_installed` when pack NOT resolved |
| `internal/pkg/mcp/server.go` | Modified | Track `packResolved` bool; pass `WithPromptErrorCode(CodeMissingGeneratorArgument)` to scaffold; enhanced MCP unknown type error to list pack generators; added `formatMCGeneratorError` helper |
| `cmd/generate_dispatch_test.go` | Modified | `TestGenerate_PackInstalled_UnknownGenerator_GroupedListing` — CLI installed-pack + unknown gen → grouped listing |
| `internal/pkg/mcp/server_generator_test.go` | Modified | `TestMCPServer_GenerateComponent_WithTemplate_UnknownType` — MCP installed-pack + unknown gen; `TestMCPServer_GenerateComponent_MissingRequiredArg` — MCP missing required prompt → missing_generator_argument |
| `internal/pkg/generators/executor_test.go` | Modified | Tightened dual-code assertion to exact `CodeGeneratorPromptUnresolvable` |
| `internal/pkg/packs/manifest_v2_test.go` | Modified | `TestManifest_Load_V2ValidRecipe_WithRunStep_ParsesOK` — positive control for run step validation (REQ-03 S3) |

### New Tests (5)

- TestGenerate_PackInstalled_UnknownGenerator_GroupedListing (cmd) — REGRESSION GUARD
- TestMCPServer_GenerateComponent_WithTemplate_UnknownType (mcp) — REGRESSION GUARD
- TestMCPServer_GenerateComponent_MissingRequiredArg (mcp) — REGRESSION GUARD
- TestManifest_Load_V2ValidRecipe_WithRunStep_ParsesOK (packs) — REQ-03 S3
- TestMCPServer_GenerateComponent_PromptCode_Discrimination (mcp) — doc-test

### Test Evidence

```
go test ./... -count=1: 8 packages PASS (325 tests, +5 from prior 320)
go vet ./...: clean
gofmt -l .: clean
```
