# cli-version Specification

## Purpose

Provides the `go-arch version` subcommand, printing the build version. The version is injected at build time via GoReleaser's default ldflags (`main.version`), requiring zero `.goreleaser.yaml` changes. Unset builds (local dev) fall back to `dev`.

## Requirements

### Requirement: Version Subcommand Registration

The CLI MUST provide a `version` subcommand registered with `RootCmd` via the package `init()` pattern (consistent with `generate`, `new`, `check`, `setup`). When invoked, the command MUST print the current version string to stdout.

#### Scenario: Default dev fallback (no ldflags)

- GIVEN the binary is built without `-ldflags` injecting `main.version`
- WHEN `go-arch version` runs
- THEN the output contains `dev`

#### Scenario: Injected version printed

- GIVEN `cmd.Version` is set to `"1.5.0"` (simulating GoReleaser ldflags injection at build time)
- WHEN `go-arch version` runs
- THEN the output contains `1.5.0`

#### Scenario: Command registered with root

- GIVEN the CLI binary
- WHEN `go-arch --help` runs
- THEN the output lists `version` as an available subcommand

### Requirement: GoReleaser Default-Ldflags Compatibility

The version source variable MUST reside in `package main` as `var version` so GoReleaser's documented default ldflags (`-X main.version={{.Version}}`) inject the release version with zero `.goreleaser.yaml` configuration changes.

#### Scenario: Zero-config release build

- GIVEN the existing `.goreleaser.yaml` with no explicit `ldflags` section
- WHEN GoReleaser builds a tagged release (e.g., `v2.0.0`)
- THEN the `go-arch version` command prints the tag-derived version
