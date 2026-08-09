# Exploration: Quality Infrastructure (CI, golangci-lint, version command)

- **Change**: `quality-infra`
- **Status**: success — exploration complete, ready for proposal
- **Date**: 2026-08-08
- **Mode**: hybrid (OpenSpec + Engram, topic `sdd/quality-infra/explore`)

## Executive Summary

The repo is in good shape today: `go test ./...` passes for all 7 packages, `go vet ./...` is clean, and a real `golangci-lint v1.64.8` run found only **5 trivial `errcheck` findings** (unchecked `os.Chdir`, `os.MkdirAll`, `viper.BindPFlag` — all one-line fixes). The two real gaps: **no CI runs quality gates** (only `release.yml` on tags) and **3 tracked files fail `gofmt -l`** today (`cmd/root.go`, `internal/pkg/template/engine.go`, `internal/pkg/validator/validator.go`) — so a gofmt-gated CI would be red on day one unless those fixes ship with it. For the version command, GoReleaser's **default ldflags already inject `main.version`/`commit`/`date`** (`.goreleaser.yaml` has no `ldflags` override, so the documented default applies), and the linker tolerates `-X` targets for not-yet-existing vars — so adding `var version` to the `main` package requires **zero `.goreleaser.yaml` changes**; only a `cmd.Version` bridge in `main.go` plus a `cmd/version.go` cobra command are needed. Recommendation: fix the 5 lint + 3 gofmt issues **inside** this change rather than introducing a lint baseline.

## Verified Current State (file:line)

### CI / release

| Fact | Evidence |
|---|---|
| Only workflow is `release.yml`; runs **only on `v*` tag pushes** | `.github/workflows/release.yml:3-6` |
| No workflow runs `go test`, `go vet`, `gofmt`, or lint on push/PR | `.github/workflows/release.yml` (33 lines, single goreleaser job) |
| Go version `1.24`, `cache: true` via `actions/setup-go@v5` | `.github/workflows/release.yml:20-24` |
| GoReleaser via `goreleaser-action@v6`, `args: release --clean`, `version: latest` | `.github/workflows/release.yml:26-32` |
| `permissions: contents: write` (needed for GH release upload) | `.github/workflows/release.yml:8-9` |
| checkout uses `fetch-depth: 0` (required by goreleaser changelog) | `.github/workflows/release.yml:15-18` |

### GoReleaser config

| Fact | Evidence |
|---|---|
| `.goreleaser.yaml` is `version: 2`; builds from `./main.go`, binary `go-arch`, linux/windows/darwin × amd64/arm64, `CGO_ENABLED=0` | `.goreleaser.yaml:3, 11-22` |
| **No `ldflags` section** → GoReleaser default applies: `-s -w -X main.version={{.Version}} -X main.commit={{.Commit}} -X main.date={{.Date}} -X main.builtBy=goreleaser` | `.goreleaser.yaml:11-22`; confirmed in GoReleaser docs (go builder page, "Default: '-s -w -X main.version=…'" — verified 2026-08-08) |
| Linker silently ignores `-X` for non-existent symbols (verified locally: `go build -ldflags "-s -w -X main.version=9.9.9 …"` succeeds on current code) | local test, `/tmp/opencode/go-arch-test` |
| `go-arch version` currently fails: `unknown command "version" for "go-arch"` | local run of built binary |

### Commands & structure

| Fact | Evidence |
|---|---|
| `main.go` is a 7-line shim: `package main` → `cmd.Execute()` | `main.go:1-7` |
| `RootCmd` is an **exported global** (`RootCmd = &cobra.Command{Use: "go-arch", …}`, `SilenceUsage/SilenceErrors: true`) | `cmd/root.go:15-21` |
| `Execute()` wraps all errors via `ui.Fatal` with oops codes | `cmd/root.go:23-31` |
| Command registration pattern: each command is its own file, `func init() { RootCmd.AddCommand(xCmd) }` | `cmd/generate.go:13-15`, `cmd/new.go:12-14`, `cmd/check.go:13-15`, `cmd/setup.go:12-14` |
| `go.mod`: `go 1.24.0`, cobra v1.8.1, viper v1.18.2, samber/oops, survey, mgutz/ansi, jinzhu/inflection | `go.mod:1-12` |
| UI style: `ui.Info/Success/Warning/Error/Fatal` + `*Msg` variants, ANSI bold colors, `Out` writer redirectable for MCP mode | `internal/ui/output.go:12-63` |

### Test patterns

- `cmd/generate_test.go` — table-driven test for pure funcs (`TestTemplHint`, lines 13-30) and CLI smoke via global `RootCmd` (`SetArgs` + `SetOut(buf)` + `Execute`, lines 35-88). Comment at lines 33-34 notes **cobra `RootCmd` has global state persisting across test functions** — a version command test must follow the same pattern.
- Existing test files: `internal/pkg/scaffold/scaffold_test.go`, `internal/pkg/template/engine_test.go`, `internal/pkg/validator/validator_test.go`, `internal/pkg/mcp/server_test.go`.

### Quality gates run locally (2026-08-08)

| Gate | Result |
|---|---|
| `go test ./...` | **PASS** — go-arch, cmd, internal/pkg/{mcp,scaffold,template,validator}; ui has no tests |
| `go vet ./...` | **CLEAN** (exit 0, no findings) |
| `gofmt -l` on tracked `.go` files | **3 files fail**: `cmd/root.go`, `internal/pkg/template/engine.go`, `internal/pkg/validator/validator.go` (all trivial: comment alignment, map-value alignment, trailing whitespace — see diffs below) |
| `golangci-lint run ./...` (v1.64.8, default linters) | **5 findings, all `errcheck`, all trivial** (see below) |

gofmt diffs (all mechanical):
- `cmd/root.go:19` — double space before `// Samber Standard:` comment (aligns after `SilenceUsage: true,`)
- `internal/pkg/template/engine.go:73-74` — `"lower"/"upper"` map keys need alignment
- `internal/pkg/validator/validator.go:107` — trailing whitespace line

golangci-lint findings (v1.64.8, default set = errcheck, govet, ineffassign, staticcheck, unused):
- `internal/pkg/validator/validator_test.go:20` — `os.Chdir(tmpDir)` return not checked
- `internal/pkg/validator/validator_test.go:21` — `defer os.Chdir(oldWd)` return not checked
- `internal/pkg/validator/validator_test.go:35` — `os.MkdirAll(..., 0755)` return not checked
- `internal/pkg/scaffold/scaffold_test.go:63` — `os.Chdir(tempDir)` return not checked
- `cmd/root.go:38` — `viper.BindPFlag(...)` return not checked

No `staticcheck`/`govet`/`ineffassign`/`unused` findings at all — the codebase is essentially clean.

### Tool versions (verified via GitHub API, 2026-08-08)

- golangci-lint latest: **v1.64.8** (what `go run …@latest` resolved to)
- golangci-lint-action latest: **v9.3.0**
- goreleaser-action latest: **v7.2.3** (repo currently on v6)
- actions/setup-go latest: **v7.0.0** (repo currently on v5)
- goreleaser latest: **v2.17.1** (release.yml uses `version: latest`)

### Repository hygiene notes

- `mcp-serve-demo/` and `briefing-go-arch-*.md` are **untracked** (`git status`); `mcp-serve-demo/cmd/api/main.go` fails gofmt but is not in CI scope until committed. A gofmt gate would catch it if/when committed — that is desired behavior.
- `docs/COMMANDS.md` documents 6 commands (`setup`, `new`, `generate`, `serve`, `check`, `mcp`) — a `version` command should be added there.
- `openspec/specs/cli/spec.md:29-34` lists commands (`check`, `generate`, `new`, `serve`, `setup`) — `version` is not spec'd; the delta spec for this change should add it.
- `openspec/config.yaml` has strict TDD (`tdd: true`, runner `go test ./...`), and `linter.available: false` under `testing.quality` — this change flips that to `true` on archive.

## Affected Areas

- `.github/workflows/ci.yml` — **new**: quality gates on push/PR
- `.golangci.yml` — **new**: lint config
- `cmd/version.go` — **new**: `Version` var + cobra command
- `cmd/version_test.go` — **new**: test following `generate_test.go` pattern
- `version.go` (repo root, `package main`) — **new**: `var version = "dev"` (GoReleaser injection target)
- `main.go` — modify: bridge `cmd.Version = version` (lines 5-6)
- `cmd/root.go` — modify: gofmt fix (line 19) + errcheck fix (line 38 `_ = viper.BindPFlag(...)`)
- `internal/pkg/template/engine.go` — modify: gofmt fix (lines 73-74)
- `internal/pkg/validator/validator.go` — modify: gofmt fix (line 107)
- `internal/pkg/validator/validator_test.go` — modify: errcheck fixes (lines 20, 21, 35)
- `internal/pkg/scaffold/scaffold_test.go` — modify: errcheck fix (line 63)
- `docs/COMMANDS.md` — modify: document `version` command
- `openspec/specs/cli/spec.md` — modify (on archive): add `version` to commands list + requirement

NOT changed: `.goreleaser.yaml` (default ldflags already cover injection), `.github/workflows/release.yml` (optionally bump action versions).

## Approaches

### Deliverable 1 — CI pipeline

**Approach 1A: Single `ci.yml` job, single Go version (RECOMMENDED)**
One `test` job on `ubuntu-latest`: checkout → setup-go `'1.24'` + `cache: true` → `gofmt` gate → `go vet ./...` → `go test ./...`; lint runs in the same or a parallel job via golangci-lint-action.
- Pros: cheap, fast, matches go.mod's `go 1.24.0`; setup-go `'1.24'` resolves latest 1.24.x patch; gofmt gate makes CI fail on formatting
- Cons: no cross-version compatibility signal
- Effort: Low

**Approach 1B: Go version matrix**
Matrix `go-version: ['1.24', '1.25']` (or `[1.24.x, stable]`).
- Pros: catches breakage on newer toolchains
- Cons: 2× CI cost, marginal value for a pinned-1.24 CLI; `go vet`/lint don't need the matrix
- Effort: Low

gofmt gate shape (either approach):
```yaml
- name: Check formatting
  run: |
    unformatted=$(gofmt -l .)
    if [ -n "$unformatted" ]; then
      echo "Unformatted files:"; echo "$unformatted"; exit 1
    fi
```
**Must ship with the 3 gofmt fixes**, or the very first CI run fails red.

### Deliverable 2 — golangci-lint

**Approach 2A: Fix findings, default linters (RECOMMENDED)**
`.golangci.yml` with defaults (errcheck, govet, ineffassign, staticcheck, unused) — since the config file exists, the default set is explicit-enough; add `run.timeout: 5m`. Fix the 5 errcheck findings (all one-liners). CI: `golangci/golangci-lint-action@v9` pinned `version: v1.64.8`.
- Pros: honest green board, no baseline debt, 5-line fix; lint blocks regressions immediately
- Cons: the 3 test-file fixes must touch tests (Chdir/MkdirAll errors → `t.Fatal`)
- Effort: Low

**Approach 2B: Baseline via `new-from-rev`**
Configure `issues.new-from-rev: HEAD~1` so only new issues fail, deferring the 5 existing findings.
- Pros: zero code changes; green CI from day one
- Cons: leaves known issues unfixed; baseline is debt that blocks future lint-enabled checks; contradicts "professional project" goal
- Effort: Low

**Optional polish**: add `revive` linter — NOT recommended initially (adds noise, more config); revisit later.

### Deliverable 3 — `go-arch version`

**Approach 3A: `main.version` var + `cmd.Version` bridge (RECOMMENDED)**
- New `version.go` (repo root, `package main`): `var version = "dev"` — **exactly** the name GoReleaser's default ldflags target (`-X main.version=…`)
- `main.go`: `cmd.Version = version` before `cmd.Execute()`
- New `cmd/version.go`: `var Version = "dev"` + `versionCmd` (`Use: "version"`, `Short: "Print the version"`) printing via `ui.Info`/`cmd.Println`; `init()` adds it to `RootCmd`; optionally `RootCmd.Version = Version` to also get cobra's built-in `--version` flag (cobra 1.8.1 adds the flag only, no subcommand collision — verified in cobra source `command.go:108-109, 885-916`)
- **Zero `.goreleaser.yaml` changes** — default ldflags inject version/commit/date/builtBy
- Local build: `go build -ldflags "-X main.version=v1.2.3" -o go-arch .`
- Pros: free GoReleaser wiring, standard `main.version` convention, no config drift; commit/date also available if we want richer output
- Cons: two vars to keep in sync (`main.version` ↔ `cmd.Version`) — mitigated by the one-line bridge
- Effort: Low

**Approach 3B: Inject `cmd.Version` directly via explicit ldflags**
`.goreleaser.yaml` `ldflags: -s -w -X go-arch/cmd.Version={{.Version}}`.
- Pros: single var, no bridge
- Cons: must re-add `-s -w` manually; loses the default `main.version/commit/date/builtBy` injections unless duplicated; two places to maintain if commit/date added later
- Effort: Low

**Version output shape** (for consistency with `ui`): `go-arch version v1.2.3` via `ui.Info`, or plain `cmd.Println(Version)` for script-parsing friendliness. Recommend plain output `go-arch version v1.2.3` (or just the version) with `dev` default when unset.

**Test shape** (`cmd/version_test.go`): mirror `TestGenerateCLI` — `RootCmd.SetArgs([]string{"version"})` + `SetOut(buf)` + `Execute`, assert output contains `Version`; plus a default-`dev` assertion when built without ldflags.

## Recommendation

1. **CI**: `ci.yml` with a `test` job (gofmt gate → `go vet ./...` → `go test ./...`) and a `lint` job (golangci-lint-action v9, pinned v1.64.8), both on `go 1.24` single version, `cache: true`. Ship the 3 gofmt fixes in the same change so CI is green from the first run.
2. **golangci-lint**: `.golangci.yml` with default linters + `timeout: 5m`; fix all 5 errcheck findings (2 test files + 1 line in `cmd/root.go`). No baseline.
3. **Version**: Approach 3A — `main.version` var at repo root, `cmd.Version` bridge in `main.go`, `cmd/version.go` command + test, no `.goreleaser.yaml` change. Document in `docs/COMMANDS.md`.
4. Scope-control note: `release.yml` action bumps (setup-go v5→v7, goreleaser-action v6→v7) are optional drive-by improvements; keep them out of this change unless desired to avoid scope creep (they don't affect the 3 deliverables).

## Risks

- **Red CI on first run** if the 3 gofmt fixes (and 5 lint fixes) don't ship with `ci.yml`/`.golangci.yml` — mitigation: they are part of this change.
- **golangci-lint version drift**: `latest` in CI can break on new major versions (v2 config format differs). Pin `v1.64.8`.
- **Untracked `mcp-serve-demo/`** contains an unformatted file; committing it later will fail the gofmt gate — desired, but flag to contributors.
- **`RootCmd` global state** in tests: the version-command test must restore/avoid shared state (existing convention at `cmd/generate_test.go:33-34`).
- **Go version matrix cost**: unnecessary for a 1.24-pinned CLI; a matrix is easy to add later if compatibility signals are needed.
- **Cobra `--version` flag vs custom subcommand**: both can coexist (verified); if only the flag is wanted later, `RootCmd.Version` + `SetVersionTemplate` is the cobra-native path.
- **CI cost**: 2 jobs (test+lint) on every push/PR is minimal; no matrix keeps it negligible.

## Ready for Proposal

**Yes.** The orchestrator should tell the user: the codebase is nearly clean (tests pass, vet clean, only 5 trivial errcheck + 3 gofmt issues); the change is small and low-risk; recommend shipping CI + lint + version command in one change with the fixes included, proposal phase next.

## next_recommended

`sdd-propose` — scope is fully explored; no further exploration needed.
