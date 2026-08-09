# Proposal: Quality Infrastructure

**Change**: `quality-infra` | **Status**: proposed | **Date**: 2026-08-08

## Executive Summary

Introduce CI quality gates (gofmt, vet, test, lint), golangci-lint config, and `go-arch version` with GoReleaser-injected versioning. Co-ship 3 gofmt + 5 errcheck fixes so CI is green day one.

## Intent / Problem

No CI quality gates exist — only a tag-triggered `release.yml`. Three tracked files fail `gofmt -l`; `golangci-lint v1.64.8` finds 5 trivial `errcheck` findings. The CLI lacks `version` despite GoReleaser's default ldflags targeting `main.version`.

**Goals**: gate push/PR with format/vet/test/lint; fix defects (no baseline); expose `go-arch version` with zero `.goreleaser.yaml` changes.

**Non-Goals**: golangci-lint v2 (pin `v1.64.8`); Go version matrix; `release.yml` action bumps; `new-from-rev` baseline; `mcp-serve-demo/` (untracked).

## Scope

**In Scope**:
- **D1 — CI** (`.github/workflows/ci.yml`): `test` job (gofmt → `go vet ./...` → `go test ./...`) + `lint` job (`golangci-lint-action@v9` pinned `v1.64.8`); Go 1.24, `cache: true`; push/PR to `main`.
- **D2 — lint** (`.golangci.yml`): default linters + `timeout: 5m`; fix 5 errcheck findings (`os.Chdir` ×2, `os.MkdirAll` in tests; `viper.BindPFlag` in `cmd/root.go:38`) via `_ =` or explicit handling.
- **D3 — `go-arch version`**: `version.go` (root) `var version = "dev"`; `main.go` bridges `cmd.Version = version`; `cmd/version.go` registers via `init()`; test mirrors `cmd/generate_test.go`.
- **Co-shipped**: gofmt for `cmd/root.go`, `internal/pkg/template/engine.go`, `internal/pkg/validator/validator.go`; update `docs/COMMANDS.md`.

## Capabilities

**New**: `cli-version` — cobra `version` subcommand with GoReleaser default-ldflags injection (`main.version` → `cmd.Version`); `"dev"` fallback.

**Modified**: None (CI/lint operational; `version` additive).

## Approach

- **D1**: single workflow, two jobs, setup-go v5 (parity with release.yml).
- **D2**: minimal config — defaults sufficient since only `errcheck` fires.
- **D3**: Approach 3A — GoReleaser's documented default ldflags; no `.goreleaser.yaml` change. Test follows global-`RootCmd` convention.

## Affected Areas

| Area | Impact |
|------|--------|
| `.github/workflows/ci.yml`, `.golangci.yml` | New |
| `version.go`, `cmd/version.go`, `cmd/version_test.go` | New |
| `main.go` | Modified — bridge |
| `cmd/root.go`, `internal/pkg/template/engine.go`, `internal/pkg/validator/validator{,_test}.go`, `internal/pkg/scaffold/scaffold_test.go` | Modified — gofmt + errcheck |
| `docs/COMMANDS.md` | Modified — document `version` |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| CI red on first run | Low | Fixes co-shipped |
| golangci-lint v2 auto-upgrade | Low | Pin `v1.64.8` |
| `RootCmd` global state in test | Med | Follow `generate_test.go:33-34` convention |

## Rollback Plan

Single `git revert`: removes `ci.yml`, `.golangci.yml`, `version.go`, `cmd/version{,_test}.go`; reverts affected fixes. Nothing runtime depends on `version` yet.

## Dependencies

None external. GoReleaser default ldflags documented; no config change.

## Success Criteria

- [ ] CI passes: gofmt clean, vet clean, tests green, lint green
- [ ] `go-arch version` prints version; `dev` fallback without ldflags
- [ ] `golangci-lint run ./...` exits 0
- [ ] `gofmt -l` on tracked files empty
- [ ] GoReleaser builds succeed unchanged

## next_recommended

`sdd-spec`
