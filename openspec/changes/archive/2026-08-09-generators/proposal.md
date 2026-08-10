# Proposal: Generators (plugins v2)

## Intent

Packs today cannot ship generator logic — `generate` is a hardcoded 6-type switch. v2 adds YAML recipe generators executed by the CLI, closing the "executable logic in packs" gap with portable data, trust-gated by the existing sidecar.

## Scope

### In Scope
- `contract_version: 2`; `generators:` in `knownManifestKeys`; supported-set {1,2}.
- Recipe DSL: `template`, `binary`, `run`, `prompt`, `use: builtin/<name>`.
- Per-generator hooks via `hooks.Runner`, gated by `HooksEnabled`.
- `generate <name>` (pack → builtin) + `generate --list`.
- Path sandbox: targets MUST resolve inside project root.
- `origin: generator` PROTECTED; template-step entries upgradable.
- MCP: `generate_component` relaxation + `list_generators`.
- `EnvContext.GeneratorName`.

### Out of Scope
- Native-code / Go-plugin generators; multi-arch packs.
- Generator marketplace; cross-pack dispatch.
- Re-running generators on `template update` (deferred v2.1).
- Conditional/branching DSL; generator-authored CLI flags.

## Capabilities

### New
- `generator-recipes`: v2 manifest `generators:` key, recipe schema, validation.
- `generator-dispatch`: `generate <name>` resolution, collision rules, `--list`.
- `generator-trust`: path sandbox, sidecar gate, install warning.
- `generator-provenance`: `origin: generator`, PROTECTED semantics.
- `generator-mcp`: `generate_component` schema + `list_generators`.

### Modified
- `plugins`: supported-set {1,2}; generator validation on strict parser.

## Approach

Declarative recipe DSL + builtin registry + `run:` escape reusing `hooks.Entry`. Reject Go plugins (Windows unsupported) and external binaries (breaks module-proxy). `generate <name>` resolves via `.go-arch.yaml` `template:` → installed pack → generator; miss → builtin; pack wins on collision; unknown → error with available names. `run:` + hooks require `HooksEnabled: true`; sandbox rejects absolute/`..`/escaping symlinks; install warning extended for command-running packs. `origin: generator` → PROTECTED; pure `template:` recipe steps ALSO record as `origin: template` and stay upgradable via `renderPackEntry`. v1 CLI rejects v2 packs with `contract_version_mismatch`.

## Affected Areas

- `internal/pkg/packs/manifest.go` — supported-set {1,2}, `generators:` key
- `internal/pkg/scaffold/{scaffold,manifest,upgrade}.go` — `GeneratePackGenerator`, `OriginGenerator`, PROTECTED classify
- `internal/pkg/hooks/` — `EnvContext.GeneratorName`, runner reuse
- `cmd/generate.go` — pack resolution, dispatch, `--list`
- `internal/pkg/mcp/server.go` — schema relaxation, `list_generators`
- `docs/packs.md`, `docs/COMMANDS.md` — v2 contract

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Recipe DSL schema debt | Med | Linear v2; defer conditionals |
| Trust expansion | Med | Sidecar + sandbox + warning |
| Effort budget (largest since plugins) | High | Chained PRs; empty builtin registry |

## Rollback Plan

Feature-gate v2 behind `packs.FeatureGate` — one-line disable.

## Success Criteria

- [ ] 3-step recipe runs via `generate <name>` with correct provenance.
- [ ] v1 CLI rejects v2 with `contract_version_mismatch`.
- [ ] Path escape → hard error, zero files written.
- [ ] `run:` with `HooksEnabled: false` skipped with warning.
- [ ] `list_generators` MCP returns manifest list.
- [ ] `generate --list` prints pack + builtins.
- [ ] `template update` re-renders only template-step entries.
