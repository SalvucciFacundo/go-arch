# Tasks: Quality Infrastructure

> **Status: CLOSED** — Archived 2026-08-08. Verify verdict: PASS WITH WARNINGS (6/6 scenarios, 3/3 requirements). See `archive-report.md`.

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~150-175 |
| 400-line budget risk | Low |
| Chained PRs recommended | No |
| Suggested split | Single PR, 3 work-unit commits |
| Delivery strategy | auto-chain |
| Chain strategy | feature-branch-chain |

Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: feature-branch-chain
400-line budget risk: Low

### Work Units

| Unit | Goal | Focused test command | Runtime harness | Rollback boundary |
|------|------|----------------------|-----------------|-------------------|
| 1 | gofmt + errcheck fixes (5 files) | `go test ./...` | `golangci-lint@v1.64.8 run ./...` exits 0 | Revert 5 modified source/test files |
| 2 | version cmd + tests + docs | `go test ./cmd/ -run TestVersionCommand` | `go run . version` → `dev`; ldflags build → `1.2.3` | Remove 3 new files; revert `main.go`, `COMMANDS.md` |
| 3 | CI workflow + lint config | N/A (no Go code) | CI green; `gofmt -l .` empty | Delete 2 new YAML files |

## Phase 1: Mechanical Fixes (prereq: green CI)

- [x] 1.1 gofmt `cmd/root.go:19` — drop double space before trailing comment
- [x] 1.2 gofmt `internal/pkg/template/engine.go:73-74` — align `lower`/`upper` map values
- [x] 1.3 gofmt `internal/pkg/validator/validator.go:107` — strip trailing whitespace
- [x] 1.4 errcheck `cmd/root.go:38` — `_ = viper.BindPFlag(...)`
- [x] 1.5 errcheck `validator_test.go:20` — `os.Chdir(tmpDir)` with `t.Fatal(err)`
- [x] 1.6 errcheck `validator_test.go:21` — `defer func() { _ = os.Chdir(oldWd) }()`
- [x] 1.7 errcheck `validator_test.go:35` — `os.MkdirAll(...)` with `t.Fatal(err)`
- [x] 1.8 errcheck `scaffold_test.go:63` — `os.Chdir(tempDir)` with `t.Fatal(err)`
- [x] 1.9 Commit: `fix(quality): resolve gofmt and errcheck findings for CI gate`

## Phase 2: Version Command (RED → GREEN)

- [x] 2.1 RED — `cmd/version_test.go`: `TestVersionCommand` subtests `dev fallback` + `injected version` (global `RootCmd` per `generate_test.go:33-34`; defer-restore `Version`); fails: `Version` undefined
- [x] 2.2 GREEN — `version.go` (package main): `var version = "dev"` (ldflags target)
- [x] 2.3 GREEN — `main.go`: bridge `cmd.Version = version` before `cmd.Execute()`
- [x] 2.4 GREEN — `cmd/version.go`: `var Version = "dev"`, cobra `version` cmd (`cmd.Println(Version)`), `init()` → `RootCmd.AddCommand`; makes 2.1 pass

## Phase 3: Docs

- [x] 3.1 `docs/COMMANDS.md`: add `## 7. version 🏷️`, renumber current 7 → 8
- [x] 3.2 Commit: `feat(cli): add version subcommand with GoReleaser ldflags injection`

## Phase 4: CI Infrastructure

- [x] 4.1 Create `.golangci.yml`: `run.timeout: 5m`, explicit `linters.enable: []`
- [x] 4.2 Create `.github/workflows/ci.yml`: `test` job (gofmt gate → `go vet ./...` → `go test ./...`; setup-go@v5, 1.24, cache) + `lint` job (`golangci-lint-action@v9`, `version: v1.64.8`); push/PR to main
- [x] 4.3 Commit: `ci: add format, vet, test, and lint quality gates`

## Phase 5: Final Verification

- [x] 5.1 `gofmt -l` empty on tracked
- [x] 5.2 `go vet ./...` clean
- [x] 5.3 `go test ./...` green (spec: dev fallback, injected, registered)
- [x] 5.4 `golangci-lint@v1.64.8 run ./...` exits 0
- [x] 5.5 `go run . version` → `dev`; `go-arch --help` lists `version`

Threat matrix: N/A — no RED beyond 2.1.
