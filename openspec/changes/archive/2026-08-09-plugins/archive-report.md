# Archive Report: plugins

**Status**: ARCHIVED
**Archived**: 2026-08-09
**Change**: plugins (roadmap item 4 — installable template packs)
**Verify verdict**: PASS — 31/31 requirements, 76/76 scenarios
**Delivery mode**: Ordinary policy — receipt-driven development (review gate) disabled at clone scope; CI is authority. No review receipt exists and none fabricated.

## Executive Summary

Turned the CLI from a closed scaffolder into a host for a third-party template ecosystem. Introduced **packs** — versioned, contract-bound template directories (a `go-arch.yaml` manifest with `contract_version`, `name`, `version`, `hooks`, `layout`, `binary_assets` + a `templates/` tree) fetched via the Go module proxy (`go mod download -json`) and materialized under `~/.go-arch/packs/<name>@<version>/`. Added a `template install|list|remove|update` command group, extended the engine lookup chain to `local > global > pack > embedded`, added `new --template <pack>` (wizard bypass) and MCP `new_project.template`, recorded per-file provenance (`source: pack:<name>@<version>`) for upgrade, added opt-in pack-declared hooks fired pack-scoped with `PACK_NAME`/`PACK_VERSION` env vars, and a pack-aware binary-file copy path. Fully merged to main via PR #38.

## What Shipped

- **Contract v1 (Option C hybrid)**: `go-arch.yaml` manifest (`contract_version` enforced, slug `name`, semver `version`, `layout`, opt-in `hooks`, `binary_assets`) + `templates/` tree; strict `UnmarshalYAML` rejects unknown keys. Generators/full schematics explicitly deferred to v2. `internal/pkg/packs/` package owns manifest/paths/install/remove/list/update, downloader (`RealDownloader`/`FakeDownloader`), Windows-safe copy, and the `pack.json` sidecar (CLI-owned `HooksEnabled` state, upstream pack bytes stay pristine).
- **Fetch**: `go mod download -json` delegation — module integrity (go.sum/ziphash) enforced by the Go toolchain; offline `GOMODCACHE` reuse verified. No new external deps.
- **Engine chain**: `local > global > pack > embedded`, namespaced by pack name; `ResolveBinary(path) (ResolvedSource, error)` public chain API; the latent `fmt.Printf` at `engine.go` routed through `ui.Out` (MCP-safe).
- **Dispatch**: `new --template <pack>` bypasses the wizard; pack resolved pre-`NewScaffolder` and injected via `WithPackInfo`; empty-pack pre-check (`pack "X" has no templates`) before any directory is created. MCP `new_project.template` optional param mirrors the flag; `architecture` becomes optional when `template` is set.
- **Upgrade**: `ManifestEntry.source` (`omitempty`) records `pack:<name>@<version>`; upgrade re-renders from the recorded pack via `WithResolver`; missing/bumped pack → per-entry PROTECTED warning, never silent embedded fallback.
- **Pack hooks**: opt-in trust prompt at install; pack hooks fire pack-scoped at `new` only (never project-level generate, never merged into project config); `PACK_NAME`/`PACK_VERSION` injected into hook env.
- **Surface**: `docs/packs.md` (contract reference), README/ARCHITECTURE 4-step lookup updates, COMMANDS.md `template` group.

## Verification Summary

Final state at close (highest authority: orchestrator final-state facts + merged commits on main + final verify report):

- **Verify FINAL verdict**: PASS — **31/31 requirements, 76/76 scenarios**. Validator admission `valid: true, verdict: pass` (evidence revision sha256:bb8658bd…).
- **Tests**: `go test ./... -count=1` exit 0 — 331 tests green (7/7 packages ok); `go vet ./...` clean; `gofmt -l .` empty; `go build ./...` clean.
- **Merge chain**: slice PRs #32 → #33 → #34 → #35 → #36 merged into tracker `feat/packs`, then tracker merged to main via PR #38 (commit `87d5d46`).
- **Corrective cycle** (AFTER the first verify FAIL — 2 CRITICAL wiring gaps found by live smoke):
  1. CRITICAL — pack hooks never fired (runner built from global config; `HooksEnabled` never read): fixed so `cmd/new.go` + `mcp/server.go` read `packs.ReadSidecar` and merge pack manifest hooks when enabled. Production-path tests landed in `67e13da` + `67030fe`.
  2. CRITICAL — upgrade never re-rendered pack entries (`WithResolver` absent at both call sites): `cmd/upgrade.go` + `mcp/server.go` now pass `scaffold.WithResolver(scaffold.DefaultResolver{})`.
  3. Final partials closed: `9709368` (offline-cached install test, `TestInstall_OfflineCached`), `d87cd4c` (spec #27-3 amended to document the v1 pack-binary contract — pack dir authoritative, local/global overrides do not apply in v1; `ResolveBinary` remains the public chain API).
  - RED→GREEN confirmed against pre-fix `d83709e` in a temp worktree: `TestNewTemplatePackHooksFire`, `TestNewTemplateNotInstalledHint`, `TestUpgradePackSourceProductionPath` all FAIL pre-fix, PASS at HEAD.

### Follow-Ups (non-blocking, none spec-breaking)

1. **MCP pack-hook merge lacks a dedicated MCP-level test** (`server.go:372-379`): logic identical to the tested `cmd/new.go` path, but not covered by its own MCP test. Add for full parity.
2. **Low coverage on new template paths**: `mcp` 47.1%, `cmd` 50.0% (improved from 44.4% by corrective tests). A hardening pass could raise both.
3. **Pack binary assets bypass local/global overrides in v1** — documented contract decision (spec #27-3, DESIGN NOTE `scaffold.go:820-830`); not a defect.
4. **Generators/full schematics** deferred to contract v2 (documented in `docs/packs.md`).

## Artifacts

- exploration.md ✅ · proposal.md ✅ · specs/plugins/spec.md (delta — 31 requirements, 76 scenarios) ✅ · design.md ✅ · tasks.md ✅ (25/25 complete) · apply-progress.md ✅ · verify-report.md ✅ · archive-report.md ✅

## Spec Sync

- No prior main spec existed (`openspec/specs/plugins/` did not exist). The delta spec **is** the full spec. Copied verbatim (mechanical `cp` + empty `diff -r` readback) from `openspec/changes/plugins/specs/plugins/spec.md` to `openspec/specs/plugins/spec.md`.
- The archived copy retains the same 31 requirements / 76 scenarios; the synced main spec is byte-identical.

## Next Steps

- Change is closed. Follow-ups above are candidates for a future change. ROADMAP.md was already updated locally by the orchestrator (untracked; not touched by archive).

## Rollback Note

Independent reverts per slice (see `tasks.md` Commit Plan). Archived folder is an immutable audit trail. `ManifestEntry.source` is `omitempty` (old/new manifests parse both ways); `ProjectConfig.Template` and the `template` command group are additive — older binaries ignore the new fields.

## Delivery-Mode Note

Receipt-driven development (review gate) disabled at clone scope; CI is authority. No review receipt exists; nothing silently approved and none fabricated.
