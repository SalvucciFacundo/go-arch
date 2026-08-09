# Tasks: Hooks (lifecycle extensibility)

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~1,500 (S1 ~415, S2 ~470, S3 ~380, S4 ~230) |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR 1 → PR 2 → PR 3 → PR 4 (feature-branch-chain) |
| Delivery strategy | auto-chain |
| Chain strategy | feature-branch-chain |

Decision needed before apply: No
Chained PRs recommended: Yes
Chain strategy: feature-branch-chain
400-line budget risk: High

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|------|------|-----------|----------------------|-----------------|-------------------|
| 1 | Hooks config types + loader + errors | PR 1 | `go test ./internal/pkg/hooks/ -run TestConfig` | N/A — pure parse, no wiring yet | delete `internal/pkg/hooks/{types,config,errors,config_test}.go` |
| 2 | Runner + env + stdout guard | PR 2 | `go test ./internal/pkg/hooks/` | `go test ./internal/pkg/hooks/ -run TestIntegration -count=1` (real subprocess) | delete `runner.go`, `env.go` + their tests |
| 3 | Scaffold/cmd/MCP wiring | PR 3 | `go test ./internal/pkg/scaffold/ ./cmd/ ./internal/pkg/mcp/` | `go run . new demo` with hooks in `$HOME/.go-arch.yaml` | revert scaffold.go/cmd/mcp diffs; hooks pkg stays |
| 4 | config.tmpl + docs + real-tool tests | PR 4 | `go test ./... && go vet ./...` | `go-arch new demo`; grep `hooks` in generated `.go-arch.yaml` | revert config.tmpl/docs additions |

Chain bases: PR 1 → tracker `feat/hooks`; PR 2 → `feat/hooks-1`; PR 3 → `feat/hooks-2`; PR 4 → `feat/hooks-3`. Only tracker merges to main (pattern from generate-routes chain #22-26).

## Slice 1 — hooks package core (PR 1)

Files: `internal/pkg/hooks/types.go`, `config.go`, `errors.go`, `config_test.go`. Est: ~415. Risk: Medium.

- [x] 1.1 RED: `config_test.go` — table tests: string/object/mixed entries, unknown type→`unknown_hook_type`, unknown key/missing command/bad timeout/scalar-not-list→`invalid_hook_config`, `timeout: 0` disables, empty/missing list no-op, missing file→empty config+nil
- [x] 1.2 GREEN: `types.go` — `Type` consts (PreNew/PostNew/PreGenerate/PostGenerate), `validTypes`, `Entry`, `Config`, `EnvContext`
- [x] 1.3 GREEN: `errors.go` — oops codes: `unknown_hook_type`, `invalid_hook_config`, `hook_failed`, `hook_timeout`, `hook_command_not_found`
- [x] 1.4 GREEN: `config.go` — `Entry.UnmarshalYAML` hybrid dispatch + validation; `Load(path)` (missing→empty,nil); `ResolveConfigPath()` (viper.ConfigFileUsed() else `$HOME/.go-arch.yaml`)
- [x] 1.5 Verify: `go test ./internal/pkg/hooks/` + `go vet ./internal/pkg/hooks/` green

## Slice 2 — runner + env (PR 2)

Files: `internal/pkg/hooks/runner.go`, `env.go`, `runner_test.go`, `env_test.go`, `integration_test.go`. Est: ~470. Risk: Medium.

- [x] 2.1 RED: `env_test.go` — precedence parent<standard<per-entry; 4 standard vars incl. `HOOK_TYPE`; `PROJECT_PATH` absolute
- [x] 2.2 GREEN: `env.go` — `BuildEnv(parent, ctx, perEntry)`
- [x] 2.3 RED: `runner_test.go` (FakeRunner) — happy path, stop-on-first→`hook_failed`, ignore_failure warns+continues, timeout kill→`hook_timeout`, silent, stdin closed (cat exits 0), HOOK_TYPE in env
- [x] 2.4 RED: threat-matrix — `TestRunner_ShellVsArgv` (`$HOME` literal argv / expanded shell), `TestRunner_CWD_Defaults`+`Override`, `TestRunner_CommandNotFound`→`hook_command_not_found`
- [x] 2.5 GREEN: `runner.go` — `CommandRunner`/`RunOpts`/`RealRunner` (exec.CommandContext), `NewRunner`, `Fire`: sh -c/cmd /c vs argv, `Dir` resolution, env merge, stdin `strings.NewReader("")`, timeout 30s/override/0
- [x] 2.6 RED: `integration_test.go` — `TestIntegration_NoStdoutInMCPMode`: os.Stdout piped, ui.Out=bytes.Buffer, assert zero bytes
- [x] 2.7 GREEN: output routed via ui.Out only; package green + vet

## Slice 3 — scaffold wiring (PR 3)

Files: `internal/pkg/scaffold/scaffold.go`, `cmd/new.go`, `cmd/generate.go`, `internal/pkg/mcp/server.go`, `internal/pkg/scaffold/scaffold_test.go`. Est: ~380. Risk: High.

- [x] 3.1 RED: `scaffold_test.go` (FakeRunner + WithRunner/WithVersion, t.TempDir) — pre-new before MkdirAll; post-new after WriteVersionField sees `go_arch_version`; post-generate after `renderRoutesRegistry`; stop-on-first; output via injected writer not os.Stdout
- [x] 3.2 GREEN: `scaffold.go` — `runner` + `version` fields, `ScaffoldOption`, `WithRunner`, `WithVersion`
- [x] 3.3 GREEN: `scaffold.go` — 4 fire sites (Execute pre/post-new; GenerateComponent+GenerateCRUD pre/post-generate); `WriteVersionField` moved into `Execute()` (non-fatal)
- [x] 3.4 GREEN: `cmd/new.go` — `ResolveConfigPath`+`Load`+`NewRunner(RealRunner{}, ui.Out)`, `WithRunner`+`WithVersion(Version)`; drop direct `WriteVersionField` call
- [x] 3.5 GREEN: `cmd/generate.go` — same runner construction + `WithRunner`
- [x] 3.6 GREEN: `mcp/server.go` — `new_project` + `generate_component` load hooks, build runner, inject (+`WithVersion(mcp.Version)` for new_project)
- [x] 3.7 Verify: `go test ./...` + `go vet ./...` green

## Slice 4 — polish + docs (PR 4)

Files: `internal/pkg/template/templates/common/config.tmpl`, `docs/hooks.md`, `internal/pkg/scaffold/scaffold_test.go`. Est: ~230. Risk: Low.

- [x] 4.1 `config.tmpl` — commented `# hooks:` hybrid example block
- [x] 4.2 `scaffold_test.go` — real-tool smoke (gofmt / go mod tidy on scaffolded project, skip `-short`); silent + empty-config no-op cases
- [x] 4.3 `docs/hooks.md` — fire sites, hybrid schema, trust warning (executable surface), MCP behavior (stderr, no prompts), non-atomic `post-*` failure
- [x] 4.4 Verify: `go test ./...` + `go vet ./...` green; tick completed tasks

## Commit Plan (work units)

- PR 1: `feat(hooks): add config types, hybrid loader and error codes` → `test(hooks): cover hybrid parse and validation`
- PR 2: `feat(hooks): add runner with shell/argv dispatch and timeouts` → `feat(hooks): build hook env with standard vars` → `test(hooks): FakeRunner, env precedence, stdout guard`
- PR 3: `feat(scaffold): fire lifecycle hooks and move version write into Execute` → `feat(cli): wire hooks runner into new and generate` → `feat(mcp): inject hooks runner into new_project and generate_component` → `test(scaffold): fire order, CWD, version visibility`
- PR 4: `feat(template): add commented hooks example` → `docs(hooks): add reference with trust warning` → `test(scaffold): real-tool hook smoke`
