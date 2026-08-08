# CLI Specification: go-arch

## Purpose
A command-line tool to scaffold Go projects with clean architecture and validate project health.

## Core Standards (Samber Upgrade)

### 1. Error Handling
- **Library**: `github.com/samber/oops`.
- **Pattern**: Every error returned from internal logic MUST be wrapped with context and a machine-readable code.
- **Root Handling**: The `RootCmd` executor handles final error reporting and exiting.

### 2. User Interface
- **Library**: `internal/ui`.
- **Helpers**: 
  - `Success`, `Warning`, `Error`, `Info` for standard lines.
  - `Analyzing` for long-running checks.
  - `*Msg` versions (e.g., `InfoMsg`) for inline colored text.
- **Aesthetics**: Bold colors, icons, and structured output.

### 3. CLI Framework
- **Framework**: Cobra.
- **Configuration**: Viper (YAML based).
- **Interactive Prompts**: Survey.
- **UX Rules**: 
  - `SilenceUsage: true` and `SilenceErrors: true` in `RootCmd`.
  - Manual error reporting via `ui.Fatal`.

## Commands
- `check`: Validates architectural rules.
- `generate`: Scaffolds components (services, repos, handlers, pages, templ components).
- `new`: Interactive project initialization wizard.
- `serve`: Runs the project with hot-reload (Air support).
- `setup`: Environment preparation and tool installation.

## Requirements

### Requirement: UseTemplHTMX ProjectConfig Field

The `ProjectConfig` struct in `internal/ui/prompts.go` SHALL include a `UseTemplHTMX bool` field tagged with `mapstructure:"use_templ_htmx"`.

#### Scenario: Field present and tagged

- GIVEN the `ProjectConfig` struct definition
- WHEN inspected
- THEN it has a `UseTemplHTMX bool` field with tag `mapstructure:"use_templ_htmx"`
- AND the field defaults to false

### Requirement: Wizard Confirm Prompt for templ + HTMX

`RunWizard()` SHALL ask a `survey.Confirm` question "Include templ + HTMX frontend?" (default false) after the gRPC question, populating `UseTemplHTMX`.

#### Scenario: Wizard prompt presented

- GIVEN a user runs `go-arch new` interactively
- WHEN the wizard reaches the web-frontend question
- THEN a Confirm prompt is shown and the answer is stored in `config.UseTemplHTMX`

#### Scenario: Default is false

- GIVEN a user accepts the default at the templ + HTMX prompt
- WHEN the wizard completes
- THEN `config.UseTemplHTMX` is false

### Requirement: Config YAML Round-Trip

The `common/config.tmpl` SHALL emit `use_templ_htmx: {{ .UseTemplHTMX }}` so the flag survives `.go-arch.yaml` serialization.

#### Scenario: Config file contains the flag

- GIVEN scaffolding completes with any architecture
- WHEN `.go-arch.yaml` is read
- THEN it contains `use_templ_htmx: true` or `use_templ_htmx: false` matching the wizard input

### Requirement: Conditional Templ Require in go.mod.tmpl

`common/go.mod.tmpl` SHALL include a conditional `require` block for `github.com/a-h/templ` (pinned at v0.3.906 or later as verified at design time) gated on `{{ if .UseTemplHTMX }}`.

#### Scenario: Templ require present when flag ON

- GIVEN `UseTemplHTMX=true`
- WHEN `go.mod` is rendered
- THEN the output contains `github.com/a-h/templ` in a require block

#### Scenario: Templ require absent when flag OFF

- GIVEN `UseTemplHTMX=false`
- WHEN `go.mod` is rendered
- THEN no `github.com/a-h/templ` line appears

### Requirement: Generate Command Supports Page and Component Types

The `generate` command MUST accept `page` and `component` as valid component types in addition to the existing `service`, `repository`, `handler`, and `crud`. The command help text MUST list all six types and disambiguate `component` as a concrete type (templ component) from the umbrella term for generated artifacts.

(Previously: `generate` supported only `service`, `repository`, `handler`, `crud`; help text read "Scaffolds components (services, repos, handlers)".)

#### Scenario: Generate page succeeds in web project

- GIVEN a project with `use_templ_htmx: true` and a valid CamelCase name
- WHEN `go-arch generate page <Name>` runs
- THEN the templ page file is created at `views/pages/<lowercased>.templ`

#### Scenario: Generate component succeeds in web project

- GIVEN a project with `use_templ_htmx: true` and a valid CamelCase name
- WHEN `go-arch generate component <Name>` runs
- THEN the templ component file is created at `views/components/<lowercased>.templ`

#### Scenario: Help lists all six types

- WHEN `go-arch generate --help` runs
- THEN the help output lists `service`, `repository`, `handler`, `crud`, `page`, and `component` as valid types

### Requirement: Generate Oops Codes for Web Generation

The `generate` command MUST emit three new oops codes for web-scaffold generation failures:
- `web_scaffold_required` when `use_templ_htmx` is false/missing for `page`/`component` types.
- `invalid_component_name` when the name argument is not a valid Go identifier.
- `component_already_exists` when the target file already exists (scoped to `page`/`component` only).

(Previously: `generate` emitted only `missing_config` and `generation_failed`.)

#### Scenario: web_scaffold_required emitted when flag off

- GIVEN `use_templ_htmx: false`
- WHEN `go-arch generate page Dashboard` runs
- THEN the error returned carries oops code `web_scaffold_required`

#### Scenario: invalid_component_name emitted for bad name

- GIVEN a web-scaffold project
- WHEN `go-arch generate component user-card` runs
- THEN the error returned carries oops code `invalid_component_name`

#### Scenario: component_already_exists emitted on collision

- GIVEN the target file already exists
- WHEN `go-arch generate page <Name>` runs
- THEN the error returned carries oops code `component_already_exists`

### Requirement: Backend Generation Unchanged

The `generate` command MUST NOT alter behavior for existing backend types (`service`, `repository`, `handler`, `crud`). The web-scaffold gate, CamelCase validation, and collision check apply ONLY to `page` and `component`.

#### Scenario: Backend types unaffected

- GIVEN any project config (web or non-web)
- WHEN `go-arch generate service Order` runs
- THEN behavior is identical to pre-change behavior (no web-scaffold gate, no collision check, no CamelCase requirement beyond existing rules)
