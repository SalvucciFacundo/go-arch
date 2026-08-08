# Delta for cli

## ADDED Requirements

### Requirement: UseTemplHTMX ProjectConfig Field

The `ProjectConfig` struct in `internal/ui/prompts.go` SHALL include a `UseTemplHTMX bool` field tagged with `mapstructure:"use_templ_htmx"`.

#### Scenario: Field present and tagged

- GIVEN the `ProjectConfig` struct definition
- WHEN inspected
- THEN it has a `UseTemplHTMX bool` field with tag ``mapstructure:"use_templ_htmx"``
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
