# Archive Report — generators (plugins v2)

**Status**: ARCHIVED
**Change**: generators
**Contract**: v2 (recipe DSL generators in template packs)
**Merged**: PR #44 → main (commit 9c94669, + lint fix 422b163)
**Date**: 2026-08-09

## Executive Summary

Closed the generators change — contract v2 for template packs adds executable generation logic via YAML recipe DSLs. A pack can now declare `generators:` (steps: `template`, `binary`, `run`, `prompt`, `use: builtin/<name>`) executed by a new `internal/pkg/generators` runner with pre-flight path sandboxing and the existing `HooksEnabled` trust gate. `generate <name>` dispatches pack → builtin → component; MCP gains `generate_component` relaxation + `generatorArgs` + `list_generators`. Generator-produced files are `origin: generator` (PROTECTED on upgrade, never silently overwritten); pure template steps stay upgradable via single-entry-with-metadata provenance. Verify PASS 31/31 requirements, 75/75 scenarios (validator admission `valid: true, verdict: pass`).

## What Ships

- **Contract v2** (`contract_version: 2`, supported set {1,2}): `generators:` manifest key; recipe DSL with linear steps; recipes validated at manifest load (invalid/empty → hard error); v1 packs unaffected, v1+generators rejected.
- **Recipe executor** (`internal/pkg/generators/`): pre-flight prompt resolution + pre-flight separator-aware path sandbox (zero writes on escape → `recipe_path_escape`); fail-fast with `ignore_failure`; run steps via `FireEntries` reusing hooks helpers, gated by `HooksEnabled` sidecar (disabled → `generator_run_skipped_trust` warning + skip); `RenderPackOnly` (no chain fallback → `generator_template_not_found`).
- **Dispatch**: three-tier pack → builtin → component (pack wins); `generate --list` grouped availability; `pack_not_installed` ONLY for genuinely missing packs (installed-but-absent → `unknown_generator` grouped listing).
- **Provenance**: `origin: generator` PROTECTED on upgrade; template-step files record single-entry-with-metadata (`metadata.generator`, args JSON) and stay upgradable via `renderPackEntry`.
- **MCP**: `generate_component.type` relaxed + `generatorArgs` (missing required → `missing_generator_argument`); `list_generators` tool.
- **Env**: `GENERATOR_NAME` injected into generator hooks.
- **Docs**: `docs/packs.md` "Contract v2 — Generators"; COMMANDS/README updated.

## Verification Summary

- **Final verdict**: PASS — 31/31 requirements, 75/75 scenarios.
- Validator admission: `gentle-ai sdd-verify-validate --requirements 31 --scenarios 75` → `{valid: true, verdict: pass}` (revision sha256:ff5dc485...).
- Test suite: `go test ./...` 325 pass / 0 fail / 0 skip, 8 packages; `go vet ./...` clean; `gofmt -l .` empty.
- Live smoke: valid recipe generates (files + run marker + hooks + exact provenance); invalid/empty recipes rejected at load; path escape → zero writes; missing pack vs absent generator discriminated; MCP args + missing-arg code; `list_generators`; upgrade PROTECTED vs upgradable; v1 regression clean.

### Corrective Cycle (2 rounds)

1. **`41e3454` + `293acdd`** (after verify FAIL — 2 CRITICAL wiring gaps caught by live smoke, invisible to unit tests): wired `generators.Validate` into manifest load; emitted `pack_not_installed`; grouped unknown-generator listing; hooks-skip warning; install trust text; spec REQ-20 amendment (single-entry-with-metadata); TDD evidence correction.
2. **`1d14ac3` + `725a9a9`** (after verify RE-RUN FAIL — the first fix introduced 2 NEW CRITICAL regressions in shared paths): `packResolved` discrimination (pack-missing vs generator-absent); `WithPromptErrorCode` option (CLI `generator_prompt_unresolvable` vs MCP `missing_generator_argument`); tightened permissive executor assertion; production-path regression-guard tests.

Lesson reinforced: tests that exercise production call paths (cmd/MCP handlers) with exact-code assertions catch control-flow regressions that injected-dependency unit tests structurally cannot.

## Follow-Ups (non-blocking)

- Builtin-before-pack dispatch order latent (inert with empty v2 registry); flip before registering any builtin.
- Interactive TTY prompt survey for recipe prompts (spec only requires args/defaults/non-interactive-error — no scenario requires interactive prompt).
- MCP pack-hook merge lacks a dedicated MCP-level test (logic identical to tested cmd path).
- Conditionals/branching in recipes + generator re-run on upgrade → deferred to v2.1.
- mcp/cmd coverage for generator paths remains moderate (informational).

## Artifacts

| Artifact | Path |
|----------|------|
| Proposal | `openspec/changes/archive/2026-08-09-generators/proposal.md` |
| Exploration | `openspec/changes/archive/2026-08-09-generators/exploration.md` |
| Design | `openspec/changes/archive/2026-08-09-generators/design.md` |
| Tasks | `openspec/changes/archive/2026-08-09-generators/tasks.md` |
| Apply progress | `openspec/changes/archive/2026-08-09-generators/apply-progress.md` |
| Verify report | `openspec/changes/archive/2026-08-09-generators/verify-report.md` |
| Spec (delta) | `openspec/changes/archive/2026-08-09-generators/specs/generators/spec.md` |
| Spec (synced) | `openspec/specs/generators/spec.md` (31 requirements, byte-identical) |

## Delivery Note

Receipt-driven review disabled at clone scope (user decision after escalating upstream #2743). Delivery under ordinary policy — CI gates (test/lint) are the authority. No review receipt exists; none fabricated.
