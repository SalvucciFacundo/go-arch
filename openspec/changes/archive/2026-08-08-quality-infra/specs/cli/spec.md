# Delta for CLI

## ADDED Requirements

### Requirement: Version Subcommand in CLI

The CLI MUST provide a `version` subcommand that prints the current build version. The command MUST be registered with `RootCmd` and appear in `go-arch --help` output. Behavioral details (ldflags injection via `main.version`, `dev` fallback, GoReleaser zero-config compatibility) are specified in the `cli-version` capability spec (`openspec/specs/cli-version/spec.md`).

#### Scenario: Version command executes

- GIVEN the CLI binary
- WHEN `go-arch version` runs
- THEN the command exits 0 and prints a version string to stdout

#### Scenario: Root help lists version

- GIVEN the CLI binary
- WHEN `go-arch --help` runs
- THEN the output lists `version` among available subcommands
