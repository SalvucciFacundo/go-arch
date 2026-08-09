# Tasks: Upgrade Project — Template Propagation via Fingerprint Manifest

> **Status: ARCHIVED** — 2026-08-09. All 19/19 tasks complete. Verify PASS 29/29 after the `--project-path` viper re-read fix (commit `576317c`). Change closed and archived per the SDD archive phase.

## Review Workload Forecast

Decision needed before apply: No
Chained PRs recommended: Yes
Chain strategy: feature-branch-chain
400-line budget risk: High

Estimated changed lines: ~1400-1800.
Delivery strategy: auto-chain.

### Suggested Work Units

| Unit | Goal | PR | Test command | Harness | Rollback |
|------|------|----|--------------|---------|----------|
| 1 | Manifest+RenderTo+seam | 1 (tracker) | `go test ./internal/pkg/scaffold/` | Temp scaffold | Revert manifest.go+seam |
| 2 | upgrade.go core+tests | 2 (←1) | `go test ./internal/pkg/scaffold/` | v2 template override | Revert upgrade.go core |
| 3 | Legacy+CLI+tests | 3 (←2) | `go test ./cmd/` | `go-arch upgrade` | Revert cmd/upgrade.go |
| 4 | MCP+version wiring | 4 (←3) | `go test ./internal/pkg/mcp/` | JSON-RPC to mcp | Revert server.go entry |

No threat matrix — no RED tests.

## Phase 1: Foundation (Manifest, Engine, Scaffold Seam)

- [x] 1.1 `manifest.go` (NEW): Origin consts; ManifestEntry; Manifest{Version,Files,dir}; ManifestPath; LoadManifest (missing→empty); ManifestExists; Save (atomic write); Upsert; hashFile
- [x] 1.2 `engine.go` (MOD): `RenderTo(wr, templatePath, data, quiet bool)` suppresses print; Render delegates quiet=false
- [x] 1.3 `scaffold.go` (MOD): `manifest *Manifest` field + `ensureManifest` (LoadManifest ProjectName)
- [x] 1.4 `scaffold.go` (MOD): recordManifest in createFile (OriginScaffold) + createBinaryFile (OriginBinary); hash `filepath.Join(s.config.ProjectName, path)`
- [x] 1.5 `scaffold.go` (MOD): recordManifest again in GenerateComponent/GenerateCRUD (OriginComponent/Crud) — upsert wins; metadata entity_name; Save non-fatal
- [x] 1.6 Test: manifest round-trip + `new` records scaffold/binary entries matching disk hashes

## Phase 2: Core Upgrade Engine

- [x] 2.1 `upgrade.go` (NEW): FileAction; UpgradePlan{TemplHint,ProjectRoot,AppliedCount}; Upgrade root ".", skip .go-arch.yaml, go.mod report-only, 4 classes (up_to_date omitted), renderEntry (binary→FS; else RenderTo quiet), buildRenderData (entity_name)
- [x] 2.2 `upgrade.go` (NEW): Apply() compare-then-write, skip non-upgradable + go.mod, refresh manifest hashes, Save, return applied count
- [x] 2.3 `upgrade.go` (NEW): WriteVersionField (line regex replace / append, other bytes identical)
- [x] 2.4 `upgrade.go` (NEW): legacy fallback — legacyWhitelist, ConfirmFunc, upgradeLegacy (whitelist + go.mod report-only + htmx binary copy), legacyTemplateFor (UseTemplHTMX→web/main.tmpl; arch map)
- [x] 2.5 `upgrade_test.go` (NEW): 4 classes via engine override; apply on-diff + idempotent + applied==N; PROTECTED never overwritten; .go-arch.yaml absent from plan; go.mod never written; surgical version; legacy web-main

## Phase 3: CLI Command + Version Field

- [x] 3.1 `cmd/upgrade.go` (NEW): flags --dry-run/--yes/--project-path; Changed() conflict; chdir; missing_config; TTY check; non-TTY no --yes → plan only; --yes → Apply + WriteVersionField + templ hint; displayPlan; go-get hints
- [x] 3.2 `cmd/new.go` (MOD): WriteVersionField after Execute succeeds (non-fatal warning)
- [x] 3.3 `prompts.go` (MOD): GoArchVersion field; `config.tmpl` (MOD): `go_arch_version: {{ .GoArchVersion }}`
- [x] 3.4 `cmd/upgrade_test.go` (NEW): conflict, missing_config, dry-run writes nothing, --yes applies, non-TTY plan-only (os.Pipe)

## Phase 4: MCP Tool

- [x] 4.1 `server.go` (MOD): upgrade_project in tools/list (projectPath, apply — NO dryRun per corrected design)
- [x] 4.2 `server.go` (MOD): handleToolCall — chdir+viper.Reset, Upgrade, Apply if apply, WriteVersionField real Version (wired, not "mcp"), JSON plan
- [x] 4.3 `server_test.go` (MOD): dry-run returns plan JSON + no writes; bare apply:true commits

## Phase 5: Verification

- [x] 5.1 `go test ./...`, `go vet ./...`, gofmt clean, `golangci-lint v1.64.8 run ./...` exit 0
