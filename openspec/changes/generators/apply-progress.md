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
| 4.4 | cmd/generate_dispatch_test.go | Unit | ✅ cmd 8/8 | ✅ Fail (--list unknown flag) | ✅ 5 tests pass | ✅ 5 cases | ✅ Clean |
| 4.5 | cmd/generate.go | — | N/A | N/A | ✅ 5 tests green | N/A | ✅ Clean |
| 4.6 | mcp/server_generator_test.go | Integration | ✅ mcp 10/10 | ✅ Fail (no list_generators tool) | ✅ 4 tests pass | ✅ 4 cases | ✅ Clean |
| 4.7 | mcp/server.go | — | N/A | N/A | ✅ 4 tests green | N/A | ✅ structured JSON |
| 4.8 | (verify) | — | N/A | N/A | ✅ go test ./... + vet + fmt | N/A | N/A |

## Test Summary (Slice 4)
- **Total tests written**: 16 (scaffold: 7, cmd: 5, mcp: 4)
- **Total tests passing**: 16 + all pre-existing
- **Layers used**: Unit (5), Integration (11)

## Cumulative Test Summary (Slices 1+2+3+4)
- **Total tests written**: 90 (32 S1 + 23 S2 + 19 S3 + 16 S4)
- **Total tests passing**: 90
- **Layers used**: Unit (79), Integration (11)

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
