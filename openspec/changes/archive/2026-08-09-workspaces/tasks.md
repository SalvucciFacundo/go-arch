# Tasks: Workspaces — Multi-Project Support

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~1,800–2,000 (S1 ~350, S2 ~500, S3 ~450, S4 ~350, S5 ~250) |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR 1 → PR 2 → PR 3 → PR 4 → PR 5 (feature-branch-chain) |
| Delivery strategy | auto-chain |
| Chain strategy | feature-branch-chain |

Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: feature-branch-chain
400-line budget risk: High

### Slice 1 — workspace package core (PR 1 → tracker `feat/workspaces`, ~350 lines)

- [ ] 1.1 RED `loader_test.go`: table — valid workspace, duplicate name → `service_duplicate`, unknown key → `workspace_invalid`, missing name/path, bad slug
- [ ] 1.2 GREEN `workspace.go`: `Workspace{Dir, Services}`, `Service{Name, Path, Template}`, `Find(name)`
- [ ] 1.3 GREEN `loader.go`: `Load(path)` — yaml.v3 `Decoder.KnownFields(true)` strict, Validate
- [ ] 1.4 GREEN `discover.go`: `Discover(cwd)` upward walk → `workspace_not_found`
- [ ] 1.5 GREEN `errors.go`: 6 oops codes (`workspace_not_found`, `workspace_invalid`, `service_not_found`, `service_path_missing`, `service_duplicate`, `service_no_manifest`)
- [ ] 1.6 Verify: `go test ./internal/pkg/workspace/` + `go vet ./...` + gofmt green

### Slice 2 — WithRoot + workspace upgrade (PR 2 → `feat/workspaces-1`, ~500 lines)

- [ ] 2.1 RED `upgrade_test.go`: `WithRoot` uses root in filepath.Join; default "." preserved (ADR-7 regression)
- [ ] 2.2 GREEN `upgrade_opts.go`: add `Root` field + `WithRoot(root string)`
- [ ] 2.3 GREEN `upgrade.go`: use `uc.Root` in ManifestExists/LoadManifest/filepath.Join/plan.ProjectRoot
- [ ] 2.4 RED `workspace_upgrade_test.go`: t.TempDir monorepo 2 manifest services; dry-run → plans only; `--yes` → applied + version field; continue-on-error; legacy service → `service_no_manifest`
- [ ] 2.5 GREEN `cmd/workspace.go`: `workspace` parent + `upgrade` subcommand (chdir, loadServiceConfig, Upgrade with WithResolver+WithRoot, plan display, `--yes` → Apply + WriteVersionField, summary, non-zero exit on any failure)
- [ ] 2.6 GREEN `cmd/workspace_helpers.go`: `resolveWorkspace`, `withService`, `loadServiceConfig` (viper snapshot/restore best-effort)
- [ ] 2.7 GREEN `cmd/root.go`: persistent `--workspace` flag
- [ ] 2.8 Verify: `go test ./...` + `go vet ./...` + gofmt green

### Slice 3 — workspace check + --service flag (PR 3 → `feat/workspaces-2`, ~450 lines)

- [ ] 3.1 RED `workspace_check_test.go`: check over 2 services, per-service summary
- [ ] 3.2 GREEN `cmd/workspace.go`: `workspace check` subcommand (reuses withService, maps arch violations per service)
- [ ] 3.3 RED `service_flag_test.go`: `--service orders` on generate → files land in services/orders, CWD restored; unknown service → `service_not_found`; no workspace → error naming flag + hint `--workspace`
- [ ] 3.4 GREEN `cmd/generate.go`: `--service` flag + chdir + loadServiceConfig (reuse existing dispatch incl. --route)
- [ ] 3.5 GREEN `cmd/check.go`: `--service` flag + chdir + loadServiceConfig
- [ ] 3.6 GREEN `cmd/upgrade.go`: `--service` flag + chdir + loadServiceConfig (reuse existing Upgrade flow incl. WithResolver)
- [ ] 3.7 Verify: `go test ./...` + `go vet ./...` + gofmt green

### Slice 4 — hooks CWD + config isolation (PR 4 → `feat/workspaces-3`, ~350 lines, opt-in shared)

- [ ] 4.1 RED `hooks_cwd_test.go`: service post-generate hook writes marker → marker in service dir; PROJECT_PATH points at service
- [ ] 4.2 GREEN (if needed): hooks env under chdir — verify os.Getwd() correct; no refactor expected
- [ ] 4.3 RED `config_isolation_test.go`: monorepo-root config vs service config different settings → service used; config restored after (next command sees prior)
- [ ] 4.4 GREEN `loadServiceConfig`: viper snapshot/restore best-effort (reset + SetConfigFile + ReadInConfig, skip missing, restore)
- [ ] 4.5 Verify: single-project flow byte-identical (full existing suite green as regression)

### Slice 5 — docs (PR 5 → `feat/workspaces-4`, ~250 lines)

- [ ] 5.1 `docs/workspaces.md`: schema, discovery, workspace upgrade/check, --service, chdir semantics, continue-on-error, ADR-7 opt-in note, batch apply (dry-run/--yes, legacy non-interactive)
- [ ] 5.2 COMMANDS.md + README: workspace commands + --service flag
- [ ] 5.3 Verify: full suite green
