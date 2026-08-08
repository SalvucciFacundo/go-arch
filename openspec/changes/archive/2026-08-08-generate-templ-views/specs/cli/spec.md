# Delta for CLI

## ADDED Requirements

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

## REMOVED Requirements

(None.)

## RENAMED Requirements

(None.)

## Editorial Changes

The Commands list entry for `generate` is updated from "Scaffolds components (services, repos, handlers)" to "Scaffolds components (services, repos, handlers, pages, templ components)" to reflect the expanded type set. This is a descriptive list, not a formal requirement block.
