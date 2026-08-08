# templ-view-generation Specification

## Purpose

Parametrized generation of templ pages and components for web-scaffold projects, with name validation, collision detection, and a web-scaffold gate.

## Requirements

### Requirement: Page File Generation

`go-arch generate page <Name>` MUST validate `<Name>` is a Go identifier, reject if `use_templ_htmx` is not `true`, reject if the target file exists, then write `views/pages/<lowercased_name>.templ`. The file MUST declare `package pages`, expose `templ <Title>()`, and render a call to the base layout.

#### Scenario: Generate page Dashboard

- GIVEN a project with `use_templ_htmx: true` and no existing `views/pages/dashboard.templ`
- WHEN `go-arch generate page Dashboard` runs
- THEN `views/pages/dashboard.templ` is created with `package pages`, `templ Dashboard()`, and a base-layout call

#### Scenario: Success hint printed

- GIVEN a successful page generation
- WHEN the command completes
- THEN a success message hints that `templ generate` must be re-run

### Requirement: Component File Generation

`go-arch generate component <Name>` MUST validate `<Name>` is a Go identifier, reject if `use_templ_htmx` is not `true`, reject if the target file exists, then write `views/components/<lowercased_name>.templ`. The file MUST declare `package components`, expose `templ <Title>()`, and include `hx-*` attributes.

#### Scenario: Generate component UserCard

- GIVEN a project with `use_templ_htmx: true` and no existing `views/components/usercard.templ`
- WHEN `go-arch generate component UserCard` runs
- THEN `views/components/usercard.templ` is created with `package components`, `templ UserCard()`, and `hx-*` attributes

#### Scenario: Filename lowercased

- GIVEN input name `MyWidget`
- WHEN generated
- THEN output path is `views/components/mywidget.templ`

### Requirement: Web Scaffold Gate

Both `generate page` and `generate component` MUST require `use_templ_htmx: true`. When `false` or missing, the command MUST fail with oops code `web_scaffold_required` and write nothing.

#### Scenario: Flag false or missing rejects

- GIVEN a project with `use_templ_htmx: false` (or the key absent)
- WHEN `go-arch generate page Dashboard` (or `component UserCard`) runs
- THEN the command fails with `web_scaffold_required`
- AND no file is written

### Requirement: CamelCase Name Validation

The name argument MUST be a valid Go identifier. Names with hyphens, spaces, or leading digits MUST fail with oops code `invalid_component_name` and write nothing.

#### Scenario: Invalid names rejected

- GIVEN a web-scaffold project
- WHEN `go-arch generate page user-card` or `go-arch generate component 123Name` runs
- THEN the command fails with `invalid_component_name`
- AND no file is written

### Requirement: Collision Detection

Before writing a page or component file, the system MUST check the target path via `os.Stat`. If the file exists, the command MUST fail with oops code `component_already_exists`. This check is scoped to `page` and `component` types ONLY.

#### Scenario: Existing target rejected

- GIVEN `views/pages/dashboard.templ` already exists
- WHEN `go-arch generate page Dashboard` runs
- THEN the command fails with `component_already_exists`
- AND the existing file is not modified

#### Scenario: Scaffold-shipped home.templ protected

- GIVEN a web-scaffold project with the original `views/pages/home.templ`
- WHEN `go-arch generate page Home` runs
- THEN the command fails with `component_already_exists`

### Requirement: MCP Enum Extension

The MCP `generate_component` tool's `type` enum MUST accept `page` and `component`. The tool description MUST reflect the expanded set.

#### Scenario: MCP accepts page and component

- GIVEN the MCP server is running
- WHEN `generate_component` is invoked with `type: "page"` or `type: "component"`
- THEN it dispatches to the matching generator with behavior identical to the CLI
