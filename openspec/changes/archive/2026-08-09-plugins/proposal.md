# Proposal: Plugins — Installable Template Packs

**Status**: proposed
**Executive summary**: Introduce *packs* — versioned, installable template directories with a formal contract — and a `template` command group. This is the framework moment: the CLI stops being a closed scaffolder and becomes a host for a third-party template ecosystem.

## Intent

Third-party authors cannot today ship reusable template sets (e.g. an Express-style architecture) for `go-arch`. This change formalizes a *pack contract* (manifest + `templates/` + binary assets), adds a `template install|list|remove|update` command group, and wires `new --template <pack>` so packs participate in the existing engine lookup chain with per-file provenance for upgrade.

## Scope

**In scope**
- Pack contract v1: `go-arch.yaml` manifest (`contract_version`, `name`, `version`, `hooks`, `layout`) + `templates/` tree + binary assets.
- `template install|list|remove|update` commands, fetching via Go module proxy (`go mod download -json`). `@version` pinning, `@latest` default.
- Engine chain extended to `local > global > pack > embedded`, namespaced by pack name.
- `new --template <pack>` bypass wizard; `ProjectConfig.Template` field + `config.tmpl` documentation + MCP `new_project.template` param.
- Manifest gains `source: pack:<name>@<version>` on `ManifestEntry`; upgrade re-renders from the recorded pack; missing pack → entry marked PROTECTED with warning.
- Pack-aware `createBinaryFile` variant (v1 — copies from pack dir via the chain).
- Opt-in pack hooks merged at install with explicit trust warning (mirror `docs/hooks.md`); `PACK_NAME`/`PACK_VERSION` env vars injected.
- Fix the latent `fmt.Printf` at `engine.go:47` → route through `ui.Out` (MCP-safe).
- `docs/packs.md` + updates to `ARCHITECTURE.md`, `README.md`, `COMMANDS.md`.

**Out of scope**
- Generators / full schematics (manifest-driven Go code in packs) — deferred to contract v2.
- Pack registry / marketplace / discovery.
- Config-schema validator for packs.
- MCP `install_template` tool — deferred to v2 (slow, network-bound, blocks stdio JSON-RPC).
- Pack-authored new architectures (packs override templates, they don't add arch dispatch keys).
- `go-arch template search`, signatures, provenance beyond go.sum.

## Capabilities

### New Capabilities
- `pack-contract`: pack manifest schema, validation, `contract_version` enforcement, install location layout.
- `template-cli`: `template install|list|remove|update` command group with `@version` pinning.
- `pack-dispatch`: engine chain insertion, `new --template`, `ProjectConfig.Template`, MCP `new_project.template`.
- `pack-upgrade`: `ManifestEntry.source` field, upgrade re-render from recorded pack, PROTECTED-on-missing.
- `pack-hooks`: opt-in pack-declared hooks merged at install, trust warning, env var injection.

### Modified Capabilities
- `hooks`: add `PACK_NAME`/`PACK_VERSION` to `EnvContext`; pack-sourced `hooks.Config` path in loader.

## Approach

**Contract v1 (Option C hybrid)** — chosen over A (flat mirror, too weak: no layout/hooks, no upgrade story) and B (full schematics, too wide: unstable surface, Angular-schematics-v1 lesson). Captures 80% of the value (installable template+layout+hook packs) with a contract small enough to stabilize. Generators are explicitly v2.

**Pack manifest** — file `go-arch.yaml` at pack root (parallel with project config):
```yaml
contract_version: 1
name: express
version: 1.2.0
layout:
  - internal/handler
  - internal/service
  - internal/repository
hooks:                  # opt-in; trust-warned at install
  post-new:
    - command: go
      args: [mod, tidy]
```
`contract_version` mismatch → hard error: `pack "X" requires contract vN; this CLI supports vM. Upgrade go-arch.`.

**Install** — `go mod download -json <module>@<version>`; copy from `GOMODCACHE` into `~/.go-arch/packs/<name>@<version>/`. go.sum integrity is the trust mechanism (Go toolchain enforced); no additional sha256 layer. Trust warning printed only when pack declares hooks.

**Dispatch** — `new --template express` sets `ProjectConfig.Template`; `Execute()` switch at `scaffold.go:130` adds a `default`-equivalent pack branch that reads the pack manifest for `layout` and iterates `templates/` calling the existing `createFile` (engine resolves through the extended chain).

**Upgrade** — `source: pack:express@1.2.0` written into `ManifestEntry`. Re-render path: lookup the recorded pack@version; if missing → PROTECTED + warning; if bumped → re-render from new version (user opts in via `template update`).

**Trust model** — packs are an executable surface when they declare hooks. Install prints `⚠ Pack "X" declares hooks that will run with your shell. Review before enabling.` and requires explicit confirmation. Pack hooks fire pack-scoped (do not pollute project `.go-arch.yaml`) with `PACK_NAME`/`PACK_VERSION` added to env.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/pkg/template/engine.go` | Modified | New pack step in `getTemplate` between global and embedded; `NewEngine` gains `WithPacks(dir)` option. |
| `internal/pkg/pack/` | New | Manifest types, validation, `contract_version` enforcement, install/remove/list logic. |
| `internal/pkg/scaffold/manifest.go` | Modified | `Source string` field on `ManifestEntry` (omitempty, backward-compatible). |
| `internal/pkg/scaffold/scaffold.go` | Modified | Pack dispatch at architecture switch; pack-aware `createBinaryFile`. |
| `internal/pkg/hooks/{config,runner,types}.go` | Modified | `PackName`/`PackVersion` in `EnvContext`; `MergePackHooks` helper. |
| `cmd/template.go` + `cmd/template_{install,list,remove,update}.go` | New | Cobra command group. |
| `cmd/new.go` | Modified | `--template` flag. |
| `internal/ui/prompts.go` | Modified | `Template` field on `ProjectConfig`. |
| `internal/pkg/mcp/server.go` | Modified | `new_project.template` optional param. |
| `internal/pkg/template/templates/common/config.tmpl` | Modified | Document `template:` field. |
| `docs/packs.md` | New | Contract reference + trust warning (mirror `docs/hooks.md`). |
| `docs/ARCHITECTURE.md`, `README.md`, `docs/COMMANDS.md` | Modified | Lookup order updated to 4 steps; `template` command group. |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Contract v1 schema mistake becomes permanent API debt | Medium | `contract_version` field; small v1 surface; explicit v2 deferral documented in `docs/packs.md`. |
| Pack removed after files generated → upgrade divergence | Medium | `ManifestEntry.source` field; missing pack → PROTECTED + warning, never silent fallback to embedded. |
| Supply-chain: pack hooks run arbitrary commands | Medium | Trust warning at install, explicit opt-in confirmation, pack-scoped fire (no auto-merge into project config). |
| MCP stdio corruption from `fmt.Printf` at `engine.go:47` | High (known latent) | Route through `ui.Out` in this change. |
| Module proxy fetch slow on first install | Low | One-time fetch; `@latest` cached in `GOMODCACHE`; document in `docs/packs.md`. |

## Rollback Plan

- Contract v1 fields on `ManifestEntry` (`source`) are omitempty — old manifests parse cleanly; new manifests parse on old CLIs with the field dropped.
- `template` command group is additive — `git revert` of the change removes it cleanly.
- `ProjectConfig.Template` field is additive — old projects missing the field load as zero-value (no template).
- Engine chain: removing the pack step returns behavior to today's 3-step lookup.

## Dependencies

- Go toolchain already present (doctor checks it). No new external dependencies.

## Success Criteria

- [ ] `go-arch template install github.com/x/go-arch-express@1.0.0` materializes the pack under `~/.go-arch/packs/` and `list` shows it.
- [ ] `go-arch new myapp --template express` scaffolds using pack templates; `source: pack:express@1.0.0` recorded in manifest.
- [ ] `go-arch upgrade` on a pack-scaffolded project re-renders from the same pack@version; missing pack → PROTECTED + warning, no silent fallback.
- [ ] Pack with `hooks:` triggers a trust warning + confirmation at install; declined → hooks stripped, pack installs without hooks.
- [ ] `contract_version: 99` pack rejected with an error naming the required version.
- [ ] `go test ./...` passes; `engine.go:47` routes through `ui.Out`.

## Edge Cases

- **Pack not found at install** → clear error (`pack "X" not found in module proxy`), no partial materialization.
- **`contract_version` mismatch** → reject install; error names required vs supported version.
- **Version conflict**: two packs with same `name` but different modules → install path is `~/.go-arch/packs/<name>@<version>/`; `new --template express@1.0.0` pins exactly; without `@version`, use latest installed.
- **Pack removed after files generated** → upgrade marks affected entries PROTECTED, prints warning per entry, never silently falls back to embedded.
- **Offline install** → `go mod download` fails with network error; surface the error unchanged; `~/.go-arch/packs/` already cached ⇒ works offline for previously installed versions.
- **Corrupted download** (module cache truncated) → `go mod download` verifies module ziphash; failure aborts install before any copy.
- **Path collision** between packs → impossible by construction (namespaced `<name>@<version>/`).
- **Collision between pack template key and embedded key** → pack wins over embedded per precedence; documented in `docs/packs.md`.
- **Windows portability** → Go module proxy path uses only stdlib filepath; no shell-specific fetch; tests run on Windows CI.
- **Wizard + `--template` flag** → flag bypasses wizard entirely (no partial wizard then pack).
- **Pack declares hooks, user declines at install** → pack installs with `hooks:` stripped from the in-memory config; manifest records `hooks_enabled: false`; re-running `template update` re-prompts.
- **Empty pack** (manifest only, no `templates/`) → install succeeds, `new --template` errors with `pack "X" has no templates`.

## Open Questions

None — all exploration open questions are resolved in this proposal. Gatekeeper may surface new questions during spec/design.

## Next Step

- **next_recommended**: `spec` — author delta specs for `pack-contract`, `template-cli`, `pack-dispatch`, `pack-upgrade`, `pack-hooks`, and the `hooks` modification.
