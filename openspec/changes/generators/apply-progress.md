# Apply Progress: generators — Slice 2

**Status**: success
**Branch**: feat/generators-2 (pushed)
**Target**: feat/generators-1 (parent in feature-branch-chain)

## TDD Cycle Evidence (Slice 2)

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| 2.1 | `generators/sandbox_test.go` | Unit | ✅ 15/15 generators | ✅ Compile fail (ValidateTarget undefined) | ✅ 8 table cases + 1 xplat pass | ✅ 9 cases | ✅ Clean |
| 2.2 | `generators/sandbox.go` | — | N/A | N/A | ✅ Implemented | N/A | ✅ Clean |
| 2.3 | `generators/executor_test.go` | Unit | ✅ 24/24 generators | ✅ Compile fail (Run undefined) | ✅ 12 executor tests pass | ✅ 12 cases | ✅ Clean |
| 2.4 | (pre-flight, same file) | Unit | ✅ 24/24 generators | ✅ Compile fail | ✅ Among 12 executor tests | ✅ prompt+escape cases | ✅ Clean |
| 2.5 | `generators/executor.go` | — | N/A | N/A | ✅ Implemented | N/A | ✅ Clean |
| 2.6 | `template/engine_test.go` | Unit | ✅ 12/12 template | ✅ Compile fail (RenderPackOnly undefined) | ✅ 3 test cases pass | ✅ 3 cases | ✅ Clean |
| 2.7 | `template/engine.go` | — | N/A | N/A | ✅ Implemented | N/A | ✅ Clean |
| 2.8 | `go test ./...` | All | — | — | ✅ All 8 packages PASS | ✅ go vet + gofmt clean | ✅ Clean |

## Test Summary (Slice 2)
- **Total tests written**: 23 (sandbox: 9, executor: 12, engine: 3)
- **Total tests passing**: 23 + all pre-existing (8 packages total)
- **Layers used**: Unit (23)
- **Approval tests**: None — no refactoring tasks
- **Pure functions created**: 3 (ValidateTarget, resolveExistingSymlinkPath, executeTemplateStep, executeBinaryStep)

## Cumulative Test Summary (Slices 1+2)
- **Total tests written**: 55 (32 Slice 1 + 23 Slice 2)
- **Total tests passing**: 55
- **Layers used**: Unit (55)

## Work Done (Slice 2)

### Files Created
| File | Lines | Purpose |
|------|-------|---------|
| `internal/pkg/generators/sandbox.go` | 85 | ValidateTarget — separator-aware path sandbox validation |
| `internal/pkg/generators/sandbox_test.go` | 163 | Table-driven tests: absolute, .., symlink, sibling false-positive |
| `internal/pkg/generators/executor.go` | 319 | Run() — linear executor with pre-flight prompt+sandbox, entriesFirer seam |
| `internal/pkg/generators/executor_test.go` | 632 | 12 executor tests: order, fail-fast, ignore_failure, trust gate, pre-flight |

### Files Modified
| File | Change | Purpose |
|------|--------|---------|
| `internal/pkg/template/engine.go` | +62 / -0 | RenderPackOnly — pack-only render, NO chain fallback |
| `internal/pkg/template/engine_test.go` | +80 / -0 | 3 RenderPackOnly tests: exists, missing, no embedded fallback |

## Deviations from Design
None — implementation matches design.md and tasks.md exactly.

Key implementation details:
- entriesFirer seam: `type entriesFirer interface { FireEntries(entries []hooks.Entry, ctx hooks.EnvContext, cwd string) error }` — defined in executor.go; fake in tests; concrete *Runner in Slice 3.
- Sandbox separator-aware: `filepath.Clean(root) + string(os.PathSeparator)` boundary; symlink resolution handles non-existing files (walks up to find existing prefix).
- PromptResolver interface: `Resolve(name, message, def string, required bool) (string, error)` — called during pre-flight; accumulator in `ResolvedArgs map[string]any`.
- Run steps do NOT produce Records (only template/binary steps produce file Records).
- TemplateData field accepts `interface{}` for template rendering data (scaffold passes ProjectConfig in Slice 4).

## Test Evidence (Task 2.8)
```
$ go test ./... -count=1 -short
?       go-arch [no test files]
ok      go-arch/cmd     0.037s
ok      go-arch/internal/pkg/generators    0.024s
ok      go-arch/internal/pkg/hooks    0.025s
ok      go-arch/internal/pkg/mcp  0.030s
ok      go-arch/internal/pkg/packs    0.034s
ok      go-arch/internal/pkg/scaffold 0.987s
ok      go-arch/internal/pkg/template 0.023s
ok      go-arch/internal/pkg/validator    0.008s

$ go vet ./...
(clean)

$ gofmt -d .
(clean)
```

## Commits
| Hash | Message |
|------|---------|
| `7e644d9` | feat(generators): add separator-aware path sandbox validation |
| `6e60130` | feat(template): add pack-only render without chain fallback |
| `6aed070` | feat(generators): implement linear recipe executor with pre-flight checks |

## Remaining Tasks
- [ ] Slice 3 — run steps + hooks exports + GENERATOR_NAME (~400 lines, PR 3)
- [ ] Slice 4 — dispatch + MCP + provenance (~1,050 lines, PR 4)
- [ ] Slice 5 — upgrade PROTECTED + docs (~500 lines, PR 5)

## Branch State
- `feat/generators-2` pushed to origin
- Targets `feat/generators-1` (parent in feature-branch-chain)
- Next slice: create `feat/generators-3` from `feat/generators-2`

## Discoveries
1. Symlink resolution for non-existing files requires walking up the path to find the longest existing prefix — `filepath.EvalSymlinks` fails on non-existing leaves.
2. `RunOptions` is passed by value to `Run()` — the test must pre-initialize `ResolvedArgs` map before calling Run, otherwise the internal `make()` only affects the local copy.
3. The `entriesFirer` seam pattern mirrors `hooks.CommandRunner` — the interface is defined in generators, a fake is used in tests, and the concrete `*Runner` type satisfies it in Slice 3.
4. Run steps do NOT produce `Record` entries — only file-producing steps (template, binary, use) create records. This keeps the record set focused on manifest-tracking outputs.
5. The `String()` format used by `fmt.Errorf` in tests returns Go-syntax nil values as `<nil>`, not `nil` — relevant when debugging map access with `any` values.
