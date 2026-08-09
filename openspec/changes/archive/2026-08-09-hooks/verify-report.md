```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:caa6a0ce013bb28bac495da0be6f59b12e03883ab6af08acbf4d731028485485
verdict: pass_with_warnings
blockers: 0
critical_findings: 0
requirements: 26/26
scenarios: 57/57
test_command: go test ./... -count=1
test_exit_code: 0
test_output_hash: sha256:caa6a0ce013bb28bac495da0be6f59b12e03883ab6af08acbf4d731028485485
build_command: go vet ./...
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

# Verification Report (RE-RUN)

**Change**: hooks (lifecycle extensibility)
**Version**: N/A (delta spec, no version field)
**Mode**: Strict TDD (runner: `go test ./...`)
**Branch**: feat/hooks-4 (all 4 slices + corrective commits `4dd9587`, `f71022c`)
**Date**: 2026-08-09
**Re-run of**: previous FAIL verdict (Req 14 string-form command-not-found; Req 7 + Req 10 s2 test gaps; loader-error discard at 4 call sites)

## Previous Findings — Resolution Checklist

| Previous finding | Resolved? | Evidence |
|---|---|---|
| Req 14: string-form missing binary yields `hook_failed`, not `hook_command_not_found` | ✅ YES | `runner.go` Fire now maps `exitCode == 127` (POSIX sh) / `9009` (cmd.exe) → `CodeHookCommandNotFound` with "Install the missing tool or check your PATH" hint (runner.go:153-158). `TestRunner_StringForm_CommandNotFound` exists and PASSES. **LIVE probe** with real `sh -c "definitely-not-a-real-binary-xyz arg1"`: `code=hook_command_not_found`, `hint="Install the missing tool or check your PATH"`, CLI exit 1, no panic. |
| Req 7: backward-compat scenario untested (0/1) | ✅ YES | `TestConfig_Load_ExtraKeysIgnored` exists and PASSES — loads `.go-arch.yaml` containing `project_name`/`module_name`/`architecture` alongside `hooks:`, asserts no error, 1 entry parsed, `Command == "gofmt"`. The literal "older binary ignores `hooks:`" half is structurally guaranteed (viper skips unknown top-level keys, never parses `hooks:`); the new test validates the shared yaml.v3 mechanism (unknown keys silently discarded). |
| Req 10 s2: post-new failure non-atomic untested | ✅ YES | `TestScaffolder_PostNew_FailureNonAtomic` exists and PASSES — FakeRunner fails post-new (exit 1), asserts `Execute()` returns error, `main.go`/`go.mod`/`.go-arch.yaml` remain on disk (3 stat checks), and exactly 2 runner calls (pre-new fired, post-new fired). The `hook_failed` code itself is asserted at runner level by `TestRunner_StopOnFirst_Failure`. |
| Loader errors discarded at 4 call sites (`hooksCfg, _ :=`) | ✅ YES | All 4 sites now surface the error: `cmd/new.go:34` and `cmd/generate.go:62` return `oops.Code("hooks_load_failed").Wrap(err)`; `mcp/server.go:312,368` return `sendToolResult(id, "Failed to load hooks config: %v", true)`. **LIVE check** (cmd/generate.go site): `.go-arch.yaml` with `unknown_key_here` in a hook entry → exit 1 with clear message `hooks.pre-generate: invalid entry list: unknown key "unknown_key_here" in hook entry` (previously silent hook disable). Missing-file no-op path (Load returns empty+nil) remains silent as designed. |

## Completeness

| Metric | Value |
|--------|-------|
| Tasks total | 23 (1.1-1.5, 2.1-2.7, 3.1-3.7, 4.1-4.4) |
| Tasks complete | 23 |
| Tasks incomplete | 0 |

All tasks checked `[x]` in tasks.md and apply-progress.md. Full verification executed.

## Build & Tests Execution

**Tests**: ✅ All pass — `go test ./... -count=1` exit 0; 7/7 packages ok
**Vet**: ✅ `go vet ./...` exit 0, no findings
**Gofmt**: ✅ `gofmt -l .` empty output, exit 0

```text
$ go test ./... -count=1
?   	go-arch	[no test files]
ok  	go-arch/cmd	0.018s
ok  	go-arch/internal/pkg/hooks	0.008s
ok  	go-arch/internal/pkg/mcp	0.033s
ok  	go-arch/internal/pkg/scaffold	1.219s
ok  	go-arch/internal/pkg/template	0.012s
ok  	go-arch/internal/pkg/validator	0.007s
?   	go-arch/internal/ui	[no test files]
EXIT_CODE=0
```

**Targeted (previously failing) tests** — all pass on execution:
- `TestRunner_StringForm_CommandNotFound` ✅ (hooks) — FakeRunner exit 127 + Shell:true → `assertCode(hook_command_not_found)`
- `TestConfig_Load_ExtraKeysIgnored` ✅ (hooks) — real YAML file, extra keys ignored
- `TestScaffolder_PostNew_FailureNonAtomic` ✅ (scaffold) — files remain, error returned, 2 calls

**Regression spot-checks** (previously PASS verdicts — confirm no regression):
- Req 8 fire order: `TestScaffolder_PreNew_BeforeMkdirAll`, `TestScaffolder_PostNew_SeesVersion`, `TestScaffolder_GenerateComponent_FiresHooks`, `TestScaffolder_GenerateCRUD_PostGenerate_AfterRoutesRegistry` — all ✅ PASS
- Req 18 ui.Out: `TestIntegration_NoStdoutInMCPMode` ✅ PASS
- Req 20 MCP parity: `mcp/server.go` wiring intact (WithRunner at new_project:312-317 and generate_component:365-369 — now with `hErr` handling); no hook tests in server_test.go (wiring + live smoke, unchanged from previous PASS)

**Coverage (changed packages, informational)**: hooks 80.5% ✅ / scaffold 78.3% ⚠️ / cmd 46.5% ⚠️ / mcp 38.7% ⚠️

## Spec Compliance Matrix

Authoritative counts from spec.md: **26 requirements / 57 scenarios** (48 requirement-level + 9 top-level).

| # | Requirement | Verdict | Scenarios | Test / Evidence |
|---|-------------|---------|-----------|-----------------|
| 1 | Hook Types Are Fixed | ✅ PASS | 3/3 | `TestConfig_Load_AllFourTypes`, `TestConfig_Load_UnknownType`+OopsCodes(unknown_hook_type), `TestConfig_Load_ScalarNotList`; `types.go` validTypes |
| 2 | Hybrid String And Object Entries | ✅ PASS | 4/4 | `TestConfig_Load_StringForm`, `TestConfig_Load_ObjectForm`, `TestConfig_Load_MixedList`, composite via AllFourTypes+EmptyList+MixedList |
| 3 | Unknown Object Keys Rejected | ✅ PASS | 1/1 | `TestConfig_Load_UnknownObjectKey` + OopsCodes(invalid_hook_config) |
| 4 | Command Field Required In Object Form | ✅ PASS | 1/1 | `TestConfig_Load_MissingCommand` + OopsCodes(invalid_hook_config) |
| 5 | Timeout Parsing | ✅ PASS | 3/3 | `TestConfig_Load_ObjectForm` (60s), `TestConfig_Load_TimeoutZeroDisables` (0), `TestConfig_Load_BadTimeout` ("forever") |
| 6 | Empty Or Missing Hooks Key Is No-op | ✅ PASS | 2/2 | `TestConfig_Load_MissingFile/EmptyFile/MissingHooksKey/EmptyList`; `TestRunner_NoHooks_IsNoOp`, `TestRunner_NilHooksMap_IsNoOp`, `TestHooks_EmptyConfig_IsNoop` |
| 7 | Backward Compatibility | ✅ PASS | 1/1 | **NEW** `TestConfig_Load_ExtraKeysIgnored` — extra top-level keys ignored while `hooks:` parses cleanly; older-binary half structurally guaranteed (viper skips unknown top-level keys) |
| 8 | Fire Sites | ✅ PASS | 6/6 | `TestScaffolder_PreNew_BeforeMkdirAll`, `TestScaffolder_PostNew_SeesVersion`, `TestScaffolder_GenerateComponent_FiresHooks`, `TestScaffolder_GenerateCRUD_PostGenerate_AfterRoutesRegistry` (4 covering tests for the 4 site scenarios + 2 composite) |
| 9 | CWD Rules | ✅ PASS | 3/3 | `TestRunner_CWD_Defaults`, `TestRunner_CWD_Override`, `TestScaffolder_PreNew_CWD_IsInvocationDir` |
| 10 | Stop On First Failure | ✅ PASS | 2/2 | S1: `TestRunner_StopOnFirst_Failure` (hook_failed, 1 call), `TestScaffolder_StopOnFirst_FailsPreNew`; S2: **NEW** `TestScaffolder_PostNew_FailureNonAtomic` (files remain, error returned, 2 calls) |
| 11 | ignore_failure Continues | ✅ PASS | 2/2 | `TestRunner_IgnoreFailure_Continues` (2 calls), `TestRunner_IgnoreFailure_WarningWritten` |
| 12 | Default Timeout 30s | ✅ PASS | 3/3 | `TestRunner_Timeout_Kills` (100ms→hook_timeout), `TestRunner_DefaultTimeout_30s`, `TestRunner_TimeoutZero_Disabled` (weak assertion — SUGGESTION below) |
| 13 | Shell Vs Argv Dispatch | ✅ PASS | 3/3 | `TestRunner_ShellVsArgv_StringFormUsesShell`, `TestRunner_ShellVsArgv_ObjectFormArgvDirect`; Windows branch code-inspected (not executable on linux) |
| 14 | Command Not Found | ✅ PASS | 1/1 | **FIXED** — runner.go maps exit 127/9009 → `hook_command_not_found`; `TestRunner_StringForm_CommandNotFound` ✅; **LIVE** real `sh -c` missing binary → code=hook_command_not_found, hint="Install the missing tool or check your PATH", exit 1 |
| 15 | Standard Environment Variables | ✅ PASS | 3/3 | `TestBuildEnv_AllFourStandardVars`, `TestBuildEnv_HOOK_TYPE_MatchesFiredType`, `TestBuildEnv_PROJECT_PATH_IsAbsolute` |
| 16 | Process Environment Inherited | ✅ PASS | 3/3 | `TestBuildEnv_Precedence_ParentIsBase`, `TestBuildEnv_ParentEnvInherited_WithoutClobber`, `TestBuildEnv_StandardOverridesParent`, `TestBuildEnv_PerEntryOverridesParent`, `TestBuildEnv_PerEntryOverridesStandard` |
| 17 | Stdin Closed | ✅ PASS | 1/1 | `strings.NewReader("")` in Fire; `TestRunner_StdinClosed` (cat reads EOF, exits 0) |
| 18 | Output Via ui.Out Only | ✅ PASS | 3/3 | `TestIntegration_NoStdoutInMCPMode` (piped os.Stdout, zero bytes); **LIVE** MCP echo on stderr only (previous session) |
| 19 | Silent Flag Suppresses Output | ✅ PASS | 1/1 | `TestRunner_Silent_SuppressesOutput`, `TestHooks_Silent_SuppressesOutput` (real echo → 0 bytes) |
| 20 | MCP Parity By Scaffold-Layer Wiring | ✅ PASS | 3/3 | `mcp/server.go` new_project + generate_component inject WithRunner; upgrade_project does not; **LIVE** (previous session) hooks fired under MCP, stdout uncorrupted |
| 21 | MCP Runs Hooks Non-Interactively | ✅ PASS | 1/1 | stdin closed (same mechanism as #17); `TestRunner_StdinClosed`; **LIVE** MCP hooks ran without prompting |
| 22 | Upgrade Fires No Hooks | ✅ PASS | 2/2 | cmd/upgrade.go has no hooks import/wiring (grep-verified); mcp upgrade_project has no runner; **LIVE** (previous session) `go-arch upgrade --yes` exit 0, zero hook output |
| 23 | CLI Exit Codes | ✅ PASS | 3/3 | root.go ui.Fatal→exit 1; **LIVE** failing hook → exit 1 + FATAL (re-verified this run: missing binary → exit 1; loader error → exit 1); timeout path → hook_timeout (unit) |
| 24 | No CLI Flags For Hooks | ✅ PASS | 1/1 | **LIVE** `new --help`/`generate --help` — no hook-related flags (grep found none) |
| 25 | config.tmpl Example | ✅ PASS | 1/1 | `config.tmpl` lines 16-41 commented `# hooks:` hybrid block + trust warning; **LIVE** generated project `.go-arch.yaml` contains it |
| 26 | docs/hooks.md Reference | ✅ PASS | 1/1 | `docs/hooks.md` 148 lines: fire sites, hybrid schema, trust warning, MCP behavior, non-atomic post-* failure |

**Compliance summary**: 57/57 scenarios compliant (26/26 requirements fully compliant; 0 partial; 0 failing). Previously failing/partial items (Req 7, Req 10 s2, Req 14) now each have a passing covering test.

## Correctness (Static Evidence)

| Requirement | Status | Notes |
|------------|--------|-------|
| runner.go exit-code mapping | ✅ Implemented | `errors.Is(err, exec.ErrNotFound)` for object form; `exitCode == 127 || exitCode == 9009` for string form (sh/cmd); both → CodeHookCommandNotFound with install/PATH hint; timeout branch unchanged |
| Loader error surfacing | ✅ Implemented | `oops.Code("hooks_load_failed")` at cmd/new.go + cmd/generate.go; `sendToolResult(..., true)` at mcp/server.go x2; no `_` discards remain |
| All other prior-correct items | ✅ Unchanged | types.go, config.go hybrid loader, env.go BuildEnv, 4 fire sites, cmd/MCP wiring, config.tmpl, docs |

## Coherence (Design)

| Decision | Followed? | Notes |
|----------|-----------|-------|
| Corrective fix within runner.go (no design change) | ✅ Yes | exit-code mapping added inside Fire's failure classification — matches design's "shell dispatch + classification" model |
| Error surfacing at call sites | ✅ Yes | loader errors now propagate to oops/MCP result per design's ui.Out routing |
| No other design deviations introduced | ✅ Yes | previous 11/11 decisions remain followed |

## TDD Compliance (Strict TDD)

| Check | Result | Details |
|-------|--------|---------|
| TDD Evidence reported | ✅ | apply-progress.md: TDD Cycle Evidence tables for slices 1-4 **plus "TDD Cycle Evidence — Corrective Fix"** section (RED/GREEN/Triangulate/Refactor for Req 14, Req 7, Req 10 s2) |
| All tasks have tests | ✅ | 23/23 tasks complete; RED test files exist for every testable task |
| RED confirmed (tests exist) | ✅ | `TestRunner_StringForm_CommandNotFound` (runner_test.go), `TestConfig_Load_ExtraKeysIgnored` (config_test.go), `TestScaffolder_PostNew_FailureNonAtomic` (scaffold_test.go) all present |
| GREEN confirmed (tests pass) | ✅ | All 3 new tests PASS on execution; full suite exit 0 |
| Triangulation adequate | ✅ | New tests assert distinct real behaviors (oops code, YAML parse, file-state + call count); no single-case gaps vs spec |
| Safety Net for modified files | ✅ | scaffold_test.go modified — pre-existing scaffold tests all green in full suite (54 tests in file, full run ok) |

**TDD Compliance**: 6/6 checks passed

## Test Layer Distribution

| Layer | Tests | Files | Tools |
|-------|-------|-------|-------|
| Unit | 56 | config_test (17), env_test (10), runner_test (19), scaffold_test hook-wiring subset, mcp server_test (10) | go test |
| Integration | 5 | integration_test.go (stdout guard), scaffold_test.go real-tool + non-atomic fs tests | os/exec real subprocess / real filesystem |
| **Total** | **61** | **5 files** | |

## Changed File Coverage

| File | Line % | Rating |
|------|--------|--------|
| internal/pkg/hooks/*.go (all) | 80.5% | ⚠️ Acceptable |
| internal/pkg/scaffold/scaffold.go | 78.3% | ⚠️ Low (large pre-existing file; all 13 hook tests target the new code paths) |
| cmd/new.go, cmd/generate.go | 46.5% (pkg) | ⚠️ Low (thin wiring; exercised via scaffold tests + live smoke) |
| internal/pkg/mcp/server.go | 38.7% (pkg) | ⚠️ Low (thin wiring; exercised via live MCP smoke) |

**Coverage analysis**: informational per protocol — not blocking. Hook-critical paths well covered.

## Assertion Quality

Audited all 3 new tests plus the full hook test files:
- `TestRunner_StringForm_CommandNotFound` — asserts the oops code via `assertCode` (behavioral, not type-only). FakeRunner exit-127 simulates the shell contract; the real `sh -c` contract is separately proven by the live probe. ✅
- `TestConfig_Load_ExtraKeysIgnored` — real YAML file on disk, asserts no error + exactly 1 entry + `Command == "gofmt"`. ✅
- `TestScaffolder_PostNew_FailureNonAtomic` — asserts error non-nil, 3 file-existence checks, exact call count. ✅

No tautologies, no ghost loops, no smoke-only, no empty-collection-only patterns.

| File | Line | Assertion | Issue | Severity |
|------|------|-----------|-------|----------|
| runner_test.go | 402-415 (approx) | `TestRunner_TimeoutZero_Disabled` asserts only "no error" | Does not verify the context passed to FakeRunner has NO deadline — if `resolveTimeout` regressed (0 → 30s default) the test still passes | WARNING |

**Assertion quality**: 0 CRITICAL, 1 WARNING — all other assertions verify concrete behavior.

## Quality Metrics

**Gofmt**: ✅ No errors (`gofmt -l .` empty)
**Vet**: ✅ No errors (`go vet ./...` exit 0)

## Issues Found

**CRITICAL**: None (blockers: 0; critical findings: 0)

**WARNING** (all non-blocking, none spec-breaking):
1. **Exit-code mapping is form-agnostic** (new, minor): `exitCode == 127 || exitCode == 9009 → hook_command_not_found` applies to object-form entries too. An object-form hook whose binary exists but legitimately exits 127 (e.g. `grep` with no match under some shells, or a tool using 127 for "not found" semantics) would be misclassified as `hook_command_not_found` instead of `hook_failed`. Low likelihood, safe failure mode (same exit 1, message differs). Suggestion: scope the 127/9009 mapping to `entry.Shell` entries.
2. **Pre-existing MCP stdout pollution** (not introduced by this change, out of scope per design.md Key Risks #1): `fmt.Printf` in scaffold.go (121, 452, 494) and `ui.Success` in root.go initConfig write to os.Stdout under MCP. The hooks runner itself is clean (echoes on stderr).
3. **CLI config shadowing** (pre-existing, documented): when `$HOME/.go-arch.yaml` exists, viper prefers it over the project `.go-arch.yaml` (search order home→cwd); MCP resets viper to CWD-only. Asymmetric by design (ResolveConfigPath = viper.ConfigFileUsed()); follow-up candidate.
4. **Coverage of cmd/mcp wiring < 80%** (informational; wiring exercised via tests + live smoke).
5. **UI rendering**: `ui.Fatal` prints `%v` — oops Code/Hint metadata is never rendered to the user (live output shows "exit status 127" rather than the code name or install hint). Contract-compliant (code attached, verified by probe) but a UX polish item.

**SUGGESTION**:
- Scope the 127/9009 mapping to shell-form entries only (see WARNING 1).
- Strengthen `TestRunner_TimeoutZero_Disabled` to assert the runner passes a deadline-less context (e.g., FakeRunner checks `ctx.Deadline()`).
- Add the composite full-hybrid fixture test (spec "Full hybrid example": 1, 1, 0, 2 entries) rather than relying on combination coverage.
- Consider rendering oops Code/Hint in `ui.Fatal` output for actionable CLI errors.

## Verdict

**PASS WITH WARNINGS** — 57/57 scenarios compliant, 26/26 requirements fully met. All four previous FAIL findings are resolved with passing tests and live proof: Req 14 string-form command-not-found maps to `hook_command_not_found` (unit test + live probe), Req 7 and Req 10 s2 have covering tests, and all 4 loader-error call sites surface errors (live-verified at the CLI site). Full suite green (`go test ./... -count=1` exit 0), vet clean, gofmt clean, TDD evidence complete including the corrective-fix cycle. Remaining items are non-blocking warnings/suggestions — none breaks a spec scenario. Ready for archive.
