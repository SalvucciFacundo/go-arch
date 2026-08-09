# Delta for cli

## ADDED Requirements

### Requirement: Upgrade Subcommand Registered Under RootCmd

The CLI MUST register an `upgrade` subcommand under `RootCmd` following the doctor/check command pattern (cobra `RunE`, `SilenceUsage`, oops-wrapped errors, `ui.*` output). The command name is `upgrade`, help text describes propagating embedded template changes via a fingerprint manifest.

#### Scenario: Upgrade command executes

- GIVEN the CLI binary
- WHEN `go-arch upgrade` runs in a valid project directory
- THEN the command exits 0 and prints a plan (dry-run default)

#### Scenario: Root help lists upgrade

- GIVEN the CLI binary
- WHEN `go-arch --help` runs
- THEN the output lists `upgrade` among available subcommands

#### Scenario: Upgrade help describes flags

- GIVEN the CLI binary
- WHEN `go-arch upgrade --help` runs
- THEN the output documents `--dry-run` (default), `--yes`, and `--project-path` flags

### Requirement: Upgrade Command Flags

`go-arch upgrade` SHALL support three flags: `--dry-run` (bool, default true) — print plan only; `--yes` (bool, default false) — apply all upgradable files without per-file prompts; `--project-path` (string, optional) — override the project root (default: current directory). `--dry-run` and `--yes` are mutually exclusive; when both are supplied the command MUST fail with a usage error.

#### Scenario: Default is dry-run

- GIVEN no flags are supplied
- WHEN `go-arch upgrade` runs
- THEN no files are written and the plan is printed

#### Scenario: --yes applies all upgradable

- GIVEN `--yes` is supplied
- WHEN `go-arch upgrade` runs
- THEN all upgradable files are applied without prompting

#### Scenario: --project-path overrides root

- GIVEN `--project-path /some/dir`
- WHEN `go-arch upgrade` runs
- THEN the project root is resolved from `/some/dir` instead of `.`

#### Scenario: --dry-run and --yes conflict

- GIVEN both `--dry-run` and `--yes` are supplied
- WHEN the command parses flags
- THEN it fails with a usage error and writes nothing

### Requirement: Upgrade Missing Config Error

When `.go-arch.yaml` is missing or unreadable, `go-arch upgrade` MUST fail with oops code `missing_config`, matching the existing check/doctor pattern.

#### Scenario: Missing config emits missing_config

- GIVEN a directory without `.go-arch.yaml`
- WHEN `go-arch upgrade` runs
- THEN the command fails with oops code `missing_config`
- AND no manifest work is attempted
