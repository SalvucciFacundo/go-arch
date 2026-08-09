# Hooks Configuration Schema

## Purpose

Defines the `hooks:` section of `.go-arch.yaml` — a root-level key whose value is a map from hook type to an ordered list of hook entries. The schema accepts a hybrid of string shorthand and object form per entry.

## Requirements

### Requirement: Hook Types Are Fixed

The `hooks:` map SHALL accept exactly four keys: `pre-new`, `post-new`, `pre-generate`, `post-generate`. Each value is a YAML list of hook entries.

#### Scenario: All four types accepted

- GIVEN a `.go-arch.yaml` with `hooks:` containing all four keys, each mapping to a list
- WHEN the config is loaded
- THEN the runner parses each key without error

#### Scenario: Unknown hook type rejected

- GIVEN a `.go-arch.yaml` with `hooks: { bogus-hook: ["echo hi"] }`
- WHEN the config is loaded
- THEN the loader returns an error with oops code `unknown_hook_type`

### Requirement: Hybrid String And Object Entries

Each hook list entry SHALL be either:
- a **string** (shell shorthand), OR
- an **object** with `command` (required string) plus optional `args` (list of strings), `cwd` (string), `env` (map of string→string), `timeout` (duration string, e.g. `60s`), `silent` (bool), `ignore_failure` (bool).

#### Scenario: String shorthand accepted

- GIVEN `hooks: { post-generate: ["gofmt -w ."] }`
- WHEN parsed
- THEN one entry is produced with command `gofmt`, args `[-w, .]`, shell mode true

#### Scenario: Object form accepted

- GIVEN `hooks: { post-generate: [{command: "go", args: ["mod", "tidy"], timeout: "60s"}] }`
- WHEN parsed
- THEN one entry is produced with command `go`, args `[mod, tidy]`, timeout 60s, shell mode false

#### Scenario: Mixed list accepted

- GIVEN a list containing both a string and an object
- WHEN parsed
- THEN both entries are produced in order

### Requirement: Unknown Object Keys Rejected

Object-form entries MUST NOT contain keys beyond the allowed set listed above. Unknown keys MUST produce a parse error.

#### Scenario: Unknown key in object rejected

- GIVEN `hooks: { post-generate: [{command: "gofmt", unknown_field: true}] }`
- WHEN parsed
- THEN the loader returns an error with oops code `invalid_hook_config`

### Requirement: Command Field Required In Object Form

Object entries without a `command` key MUST fail validation with oops code `invalid_hook_config`.

#### Scenario: Missing command rejected

- GIVEN `hooks: { post-generate: [{args: ["-w", "."]}] }`
- WHEN parsed
- THEN the loader returns `invalid_hook_config`

### Requirement: Timeout Parsing

`timeout` values MUST parse as Go duration strings (`30s`, `2m`, `500ms`). The literal `0` MUST be accepted as "disable timeout". Invalid strings MUST fail with `invalid_hook_config`.

#### Scenario: Valid duration accepted

- GIVEN `timeout: "90s"`
- WHEN parsed
- THEN the timeout is 90 seconds

#### Scenario: Zero disables timeout

- GIVEN `timeout: 0`
- WHEN parsed
- THEN the timeout is disabled (infinite)

#### Scenario: Invalid string rejected

- GIVEN `timeout: "forever"`
- WHEN parsed
- THEN the loader returns `invalid_hook_config`

### Requirement: Empty Or Missing Hooks Key Is No-op

When the `hooks:` key is absent, or a specific hook type has an empty list, the runner MUST NOT execute anything and MUST NOT error.

#### Scenario: Missing hooks key

- GIVEN a `.go-arch.yaml` without a `hooks:` key
- WHEN `new` or `generate` runs
- THEN no hooks run and the command succeeds

#### Scenario: Empty list

- GIVEN `hooks: { pre-new: [] }`
- WHEN `new` runs
- THEN no pre-new hooks run and the command succeeds

### Requirement: Backward Compatibility

Older CLI versions (without hooks support) MUST silently ignore the `hooks:` key when loading `.go-arch.yaml`, because Viper skips unknown top-level keys by default.

#### Scenario: Older CLI ignores hooks key

- GIVEN a `.go-arch.yaml` with a `hooks:` block, loaded by a CLI binary that predates hooks support
- WHEN any command reads the config
- THEN the `hooks:` key is silently ignored and no error is raised

## Scenarios

### Scenario: Full hybrid example

- GIVEN `.go-arch.yaml` containing:
  ```yaml
  hooks:
    pre-new:
      - echo "about to scaffold"
    post-new:
      - command: git
        args: [init]
        cwd: "."
        timeout: 30s
    pre-generate: []
    post-generate:
      - gofmt -w .
      - command: go
        args: [mod, tidy]
        ignore_failure: true
  ```
- WHEN parsed
- THEN 4 hook types are registered with 1, 1, 0, and 2 entries respectively

### Scenario: Non-list value rejected

- GIVEN `hooks: { pre-new: "echo hi" }` (scalar instead of list)
- WHEN parsed
- THEN the loader returns `invalid_hook_config`
