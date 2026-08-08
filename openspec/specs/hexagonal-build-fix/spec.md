# hexagonal-build-fix Specification

## Purpose

Ensure fresh Hexagonal projects compile with `go build ./...` immediately after scaffolding, both with and without the web flag. The pre-existing bug: `hexagonal/main.tmpl` imports `internal/adapters` and `internal/domain`, but the scaffold creates those as empty directories, so the build fails.

## Requirements

### Requirement: Hexagonal Build Success Without Web

A freshly scaffolded Hexagonal project with `UseTemplHTMX=false` MUST compile cleanly with `go build ./...`.

#### Scenario: Hexagonal + web OFF builds

- GIVEN a user runs `go-arch new` with Hexagonal architecture and `UseTemplHTMX=false`
- WHEN `go build ./...` is executed in the generated project
- THEN the build exits 0 with no errors about missing `internal/adapters` or `internal/domain` packages

#### Scenario: Hexagonal main has no empty-package imports

- GIVEN the scaffolder writes the Hexagonal main
- WHEN the main template is rendered
- THEN it does NOT import `internal/adapters` or `internal/domain` unless those packages contain Go source files

### Requirement: Web Main Clean Imports

The architecture-agnostic web main used under `UseTemplHTMX=true` MUST NOT import empty packages, preserving the hex build guarantee in the web path.

#### Scenario: Hexagonal + web ON builds after templ generate

- GIVEN a user runs `go-arch new` with Hexagonal architecture and `UseTemplHTMX=true`
- WHEN `templ generate` then `go build ./...` are executed
- THEN the build exits 0
- AND the web main imports only non-empty packages (`internal/handler`, `net/http`, and any conditional telemetry/gRPC packages)
