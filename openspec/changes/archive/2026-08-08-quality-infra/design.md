# Design: Quality Infrastructure

**Status**: designed | **Date**: 2026-08-08 | **Change**: `quality-infra`

## Executive Summary

Introduce CI quality gates (gofmt → vet → test → lint), a minimal `.golangci.yml`, and a `go-arch version` subcommand wired through GoReleaser's default ldflags. Co-ship 3 gofmt fixes and 5 errcheck fixes so CI is green on the first run under the pinned `golangci-lint v1.64.8`. No `.goreleaser.yaml` changes required.

## Context & Constraints

All facts below are verified from the exploration phase and confirmed against source.

| Constraint | Evidence |
|---|---|
| Only workflow is `release.yml` (tag-triggered, `v*`) | `.github/workflows/release.yml:3-6` |
| Go 1.24 pinned, `setup-go@v5`, `cache: true` | `.github/workflows/release.yml:20-24` |
| `.goreleaser.yaml` has no `ldflags` section → GoReleaser default applies (`-X main.version={{.Version}}`) | `.goreleaser.yaml:11-22` |
| Linker silently tolerates `-X` for absent symbols | verified locally |
| `main.go` is a 7-line shim calling `cmd.Execute()` | `main.go:1-7` |
| `RootCmd` is exported global; commands register via `init()` + `RootCmd.AddCommand()` | `cmd/root.go:15-21`, `cmd/generate.go:13-15` |
| 3 files fail `gofmt -l`; 5 errcheck findings under pinned `v1.64.8`; zero staticcheck/govet/unused | `gofmt -l`, `golangci-lint run ./...` (v1.64.8) |
| Test convention: global `RootCmd`, single test function for CLI smoke (cobra global state) | `cmd/generate_test.go:33-34` |

## Lint Finding Count Resolution

This design deliberately pins `golangci-lint v1.64.8` in **two places** so the quality gate is deterministic:

- `.github/workflows/ci.yml`: `golangci/golangci-lint-action@v9` with `version: v1.64.8` (see D1).
- Local verification command: `go run github.com/golangci/golangci-lint/cmd/golangci-lint@v1.64.8 run ./...` prints `golangci-lint has version v1.64.8`.

With that **exact** pinned version and the **default** linter set (`errcheck`, `govet`, `ineffassign`, `staticcheck`, `unused`), the authoritative finding count is **exactly 5 errcheck findings**, matching the exploration list:

| # | File:Line | Linter | Symptom |
|---|-----------|--------|---------|
| 1 | `cmd/root.go:38` | errcheck | `viper.BindPFlag(...)` return not checked |
| 2 | `internal/pkg/validator/validator_test.go:20` | errcheck | `os.Chdir(tmpDir)` return not checked |
| 3 | `internal/pkg/validator/validator_test.go:21` | errcheck | `defer os.Chdir(oldWd)` return not checked |
| 4 | `internal/pkg/validator/validator_test.go:35` | errcheck | `os.MkdirAll(..., 0755)` return not checked |
| 5 | `internal/pkg/scaffold/scaffold_test.go:63` | errcheck | `os.Chdir(tempDir)` return not checked |

A fresh-context validator run reported **~63 findings** and flagged this section as under-counted. Re-running the EXACT pinned command reproduces 5, not 63. The discrepancy is explained by the validator invoking golangci-lint **v2** (which ships a different default linter set, different config format, and additional opinionated linters). `v2` is explicitly **out of scope** per the proposal non-goals ("pin `v1.64.8`"; "no golangci-lint v2"). This design does not accommodate v2; it pins v1.64.8 end-to-end so the CI lint gate is reproducible across dev machines, CI runners, and this design document.

**Contract**: the CI lint job must pin the same version (`v1.64.8`) so the gate is deterministic and this design's "CI green on first run" success criterion holds. Any future upgrade to v2 is a separate change (different config format, different finding set, different scope).

## Architecture Decisions

### ADR-1: Version wiring via bridge variable

| Option | Tradeoff | Decision |
|---|---|---|
| **3A**: `main.version` var + `cmd.Version` bridge in `main.go` | Zero `.goreleaser.yaml` changes; free injection of version/commit/date/builtBy; two vars to keep in sync (one-line bridge) | **Chosen** |
| 3B: Inject `cmd.Version` via explicit ldflags | Single var but loses default ldflags; must re-add `-s -w`; more config | Rejected |

### ADR-2: CI shape — two jobs, single Go version

| Option | Tradeoff | Decision |
|---|---|---|
| **Two jobs**: `test` (gofmt → vet → test) + `lint` (golangci-lint) | Lint runs in parallel with test; each job has clear responsibility; setup-go v5 parity with release.yml | **Chosen** |
| Single job, sequential | Simpler YAML but lint blocks test feedback | Rejected |
| Go version matrix | 2× CI cost, marginal value for pinned-1.24 CLI | Rejected (out of scope) |

### ADR-3: Lint pin — `v1.64.8`

Pin `version: v1.64.8` in `golangci-lint-action@v9`. `latest` can auto-upgrade to v2 (different config format) and break CI silently. Pinning eliminates drift. See **Lint Finding Count Resolution** above for the authoritative finding count under this pin and the rationale for excluding v2.

### ADR-4: Fix-not-baseline

Fix all 5 errcheck + 3 gofmt issues in this change rather than introducing `new-from-rev` baseline. Five one-line fixes cost less than baseline debt, and a baseline would mask known defects.

## Data Flow

```
  GoReleaser (tag push)
       │
       │  -ldflags "-s -w -X main.version=v1.2.3 -X main.commit=… -X main.date=…"
       ▼
  ┌─────────────┐     ┌──────────────┐     ┌───────────────┐     ┌────────┐
  │ version.go  │────▶│   main.go    │────▶│ cmd/version.go│────▶│ stdout │
  │ var version │     │ cmd.Version  │     │ versionCmd    │     │        │
  │  = "dev"    │     │  = version   │     │ Run: Println  │     │        │
  └─────────────┘     └──────────────┘     └───────────────┘     └────────┘
        ▲                                           │
        │              init()                       │
        │     RootCmd.AddCommand(versionCmd)────────┘
```

Build without ldflags: `version` stays `"dev"` → output: `dev`.

## Detailed Component Design

### D1 — CI workflow (`.github/workflows/ci.yml`)

```yaml
name: CI

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  test:
    name: Test
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.24'
          cache: true

      - name: Check formatting
        run: |
          unformatted=$(gofmt -l .)
          if [ -n "$unformatted" ]; then
            echo "Unformatted files:"
            echo "$unformatted"
            exit 1
          fi

      - name: Vet
        run: go vet ./...

      - name: Test
        run: go test ./...

  lint:
    name: Lint
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.24'
          cache: true

      - name: golangci-lint
        uses: golangci/golangci-lint-action@v9
        with:
          version: v1.64.8
```

**Responsibilities**: `test` job gates format + vet + tests; `lint` job gates lint. Both run on push/PR to `main`. `setup-go@v5` matches `release.yml` parity. The `lint` job's `version: v1.64.8` is the deterministic pin documented in **Lint Finding Count Resolution** — changing it to `latest` or any v2.x value invalidates the 5-finding contract.

### D2 — golangci config (`.golangci.yml`)

```yaml
run:
  timeout: 5m

linters:
  enable: []
```

Default linters (errcheck, govet, ineffassign, staticcheck, unused) are sufficient — no custom additions needed. The `enable: []` is explicit for clarity; omitting the block entirely also works. Under `v1.64.8` these defaults produce exactly the 5 findings listed in **Lint Finding Count Resolution**.

### D3 — Version command

#### `version.go` (repo root, `package main`)

```go
package main

// version is set at build time via GoReleaser's default ldflags:
// -X main.version={{.Version}}
var version = "dev"
```

#### `main.go` (modify — bridge)

```go
package main

import "go-arch/cmd"

func main() {
	cmd.Version = version
	cmd.Execute()
}
```

#### `cmd/version.go` (new)

```go
package cmd

import "github.com/spf13/cobra"

// Version is set by main after GoReleaser injects main.version.
var Version = "dev"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Println(Version)
	},
}

func init() {
	RootCmd.AddCommand(versionCmd)
}
```

**Style notes**: Uses `Run` (not `RunE`) — the version command cannot fail. Uses `cmd.Println` for script-parsing-friendly plain output (matches cobra convention). No `ui.Info` wrapper — version output should be parseable.

#### `cmd/version_test.go` (new)

```go
package cmd

import (
	"bytes"
	"strings"
	"testing"
)

// TestVersionCommand verifies the version subcommand output.
// Runs inside one function because cobra RootCmd has global state
// that persists across test functions (same convention as generate_test.go:33-34).
func TestVersionCommand(t *testing.T) {
	t.Run("dev fallback", func(t *testing.T) {
		original := Version
		Version = "dev"
		defer func() { Version = original }()

		buf := new(bytes.Buffer)
		RootCmd.SetOut(buf)
		RootCmd.SetErr(buf)
		RootCmd.SetArgs([]string{"version"})

		if err := RootCmd.Execute(); err != nil {
			t.Fatalf("version command failed: %v", err)
		}

		if !strings.Contains(buf.String(), "dev") {
			t.Errorf("expected output to contain 'dev', got: %q", buf.String())
		}
	})

	t.Run("injected version", func(t *testing.T) {
		original := Version
		Version = "1.5.0"
		defer func() { Version = original }()

		buf := new(bytes.Buffer)
		RootCmd.SetOut(buf)
		RootCmd.SetErr(buf)
		RootCmd.SetArgs([]string{"version"})

		if err := RootCmd.Execute(); err != nil {
			t.Fatalf("version command failed: %v", err)
		}

		if !strings.Contains(buf.String(), "1.5.0") {
			t.Errorf("expected output to contain '1.5.0', got: %q", buf.String())
		}
	})
}
```

**Test approach**: Sets `cmd.Version` directly and invokes `RootCmd.Execute()` with `SetArgs`. Restores original value via `defer` to avoid polluting other tests. Mirrors `generate_test.go:35-88` pattern. Tests both spec scenarios (dev fallback + injected).

### D3-fixes — gofmt fixes (exact diffs from `gofmt -d`)

**`cmd/root.go:19`** — extra space before trailing comment:

```diff
-	SilenceUsage:  true,  // Samber Standard: We handle error display
+	SilenceUsage:  true, // Samber Standard: We handle error display
```

**`internal/pkg/template/engine.go:73-74`** — map value alignment:

```diff
-		"lower": strings.ToLower,
-		"upper": strings.ToUpper,
+		"lower":  strings.ToLower,
+		"upper":  strings.ToUpper,
```

**`internal/pkg/validator/validator.go:107`** — trailing whitespace on blank line:

```diff
 		importPath := strings.Trim(imp.Path.Value, "\"")
-		
+
 		// We are only interested in the project's own internal imports
```

### D3-fixes — errcheck fixes (5 findings)

**`cmd/root.go:38`** — `_ =` for `viper.BindPFlag` (binding can't fail in practice):

```go
_ = viper.BindPFlag("config", RootCmd.PersistentFlags().Lookup("config"))
```

**`internal/pkg/validator/validator_test.go:20`** — `os.Chdir` with error check:

```go
if err := os.Chdir(tmpDir); err != nil {
    t.Fatal(err)
}
```

**`internal/pkg/validator/validator_test.go:21`** — `defer os.Chdir` wrapped:

```go
defer func() { _ = os.Chdir(oldWd) }()
```

**`internal/pkg/validator/validator_test.go:35`** — `os.MkdirAll` with error check:

```go
if err := os.MkdirAll("internal/domain", 0755); err != nil {
    t.Fatal(err)
}
```

**`internal/pkg/scaffold/scaffold_test.go:63`** — `os.Chdir` with error check:

```go
if err := os.Chdir(tempDir); err != nil {
    t.Fatal(err)
}
```

### Co-shipped — `docs/COMMANDS.md`

Add section 7 (renumber current 7 → 8):

```markdown
## 7. `version` 🏷️
**Usage**: `go-arch version`

Prints the build version. When built via GoReleaser (tagged release), the version is injected automatically. Local development builds print `dev`.
```

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `.github/workflows/ci.yml` | Create | CI workflow: test + lint jobs, lint pinned `v1.64.8` |
| `.golangci.yml` | Create | Minimal lint config, 5m timeout |
| `version.go` | Create | `var version = "dev"` (ldflags target) |
| `cmd/version.go` | Create | Cobra version subcommand |
| `cmd/version_test.go` | Create | Tests: dev fallback + injected |
| `main.go` | Modify | Bridge `cmd.Version = version` |
| `cmd/root.go` | Modify | gofmt fix (line 19) + errcheck fix (line 38) |
| `internal/pkg/template/engine.go` | Modify | gofmt fix (lines 73-74) |
| `internal/pkg/validator/validator.go` | Modify | gofmt fix (line 107) |
| `internal/pkg/validator/validator_test.go` | Modify | errcheck fixes (lines 20, 21, 35) |
| `internal/pkg/scaffold/scaffold_test.go` | Modify | errcheck fix (line 63) |
| `docs/COMMANDS.md` | Modify | Document `version` command |

## Testing Strategy

| Layer | What | Approach |
|-------|------|----------|
| Unit | `version` command output (dev + injected) | `cmd/version_test.go` — set `cmd.Version`, invoke `RootCmd.Execute()`, assert stdout contains expected string |
| Unit | Existing tests still pass | `go test ./...` unchanged — errcheck fixes don't alter behavior |
| Integration | CI green on first run under pinned `v1.64.8` | `ci.yml` gates: gofmt + vet + test + lint (v1.64.8) all pass |
| E2E | GoReleaser build → version output | Verified via `go build -ldflags "-X main.version=…" -o go-arch .` locally |

**Spec scenario mapping**:
- `cli-version/spec.md` "Default dev fallback" → `TestVersionCommand/dev fallback`
- `cli-version/spec.md` "Injected version printed" → `TestVersionCommand/injected version`
- `cli-version/spec.md` "Command registered with root" → implicitly tested by `RootCmd.Execute()` with `"version"` arg
- `cli-version/spec.md` "Zero-config release build" → verified by GoReleaser default ldflags (no `.goreleaser.yaml` change)
- `cli/spec.md` delta "Version command executes" → `TestVersionCommand` (both subtests)
- `cli/spec.md` delta "Root help lists version" → covered by cobra's command registration; optional explicit test

## Threat Matrix

N/A — no routing, shell, subprocess, VCS/PR automation, executable-file classification, or process-integration boundary.

## Risks & Mitigations

| Risk | Likelihood | Mitigation |
|------|-----------|------------|
| CI red on first run | Low | 3 gofmt + 5 errcheck fixes co-shipped; lint gate pinned to `v1.64.8` |
| golangci-lint v2 auto-upgrade | Low | Pin `v1.64.8` in `ci.yml`; v2 is out of scope for this change |
| Version-pin drift between local/CI | Low | CI action pin and local verification command both use `v1.64.8`; documented in **Lint Finding Count Resolution** |
| `RootCmd` global state in test | Med | Follow `generate_test.go:33-34` convention; restore `Version` via defer |
| Untracked `mcp-serve-demo/` fails gofmt if committed | Low | Desired behavior — gate catches it when committed |

## Migration / Rollout

No migration required. All changes are additive (new command, new CI) or mechanical fixes (whitespace, error handling). Single `git revert` removes everything.

## Open Questions

None.

## next_recommended

`sdd-tasks`
