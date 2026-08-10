# Design: Workspaces — Multi-Project Support

**status**: success
**next_recommended**: tasks

## Technical Approach

A new `internal/pkg/workspace` package owns the workspace file loader (strict yaml.v3 with `KnownFields(true)`, `go-arch.workspace.yaml`), validation, discovery (explicit `--workspace` flag + upward walk), and path resolution. Workspace commands use the **chdir-hybrid** model: resolve the service path, `os.Chdir(path)` + defer restore (the codebase's established pattern — `cmd/upgrade.go:47`, `mcp/server.go:429-436`), then run the existing command logic unchanged — generate/hooks see the correct CWD automatically (`manifestDir()` returns "." with a manifest present; hooks `ProjectPath` from `os.Getwd()` is correct under chdir; `ResolveConfigPath` follows `viper.ConfigFileUsed()`, so config reload lands on the service). `Upgrade` gains a `WithRoot(root string)` UpgradeOption (mirrors `WithResolver` in `upgrade_opts.go`). Config isolation is opt-in: workspace commands reload the service's `.go-arch.yaml` via viper snapshot/restore; single-project flows are untouched (byte-identical). **`workspace upgrade` runs the FULL standard upgrade logic per service — including `plan.Apply()` under `--yes`, `WriteVersionField`, and `WithResolver` — so pack-sourced files re-render.** Multi-service commands continue on error with a summary; single `--service` preserves fail-fast.

## Architecture Decisions

| Decision | Options | Chosen | Rationale |
|---|---|---|---|
| Package layout | extend cmd vs new package | **New `internal/pkg/workspace/`** | Loader/schema/paths is a distinct concern; mirrors packs/generators precedent |
| Path resolution | chdir vs root-injection vs hybrid | **chdir-hybrid** | chdir is the proven pattern; generate/hooks already CWD-correct; WithRoot adds an injection escape for upgrade |
| Workspace file discovery | flag-only vs upward walk vs both | **Both** (`--workspace` persistent flag wins, upward walk fallback) | Spec workspace-discovery |
| Upgrade root | hardcoded "." vs WithRoot option | **`WithRoot(root string)` UpgradeOption** | Mirrors `WithResolver`; variadic = backward-compatible; default "." preserves ADR-7 |
| Config isolation | viper reload vs config injection | **Opt-in viper snapshot/restore** (see contract below) | Only workspace/`--service` paths trigger it; single-project byte-identical |
| Multi-service failure | fail-fast vs continue-on-error | **Continue-on-error + summary** | Workspace batch semantics; single `--service` stays fail-fast |
| Service path check | lazy at command time | **Validate at workspace load + re-check at command time** | Missing dir is an operational error, not a load error |
| Duplicate name code | workspace_invalid vs service_duplicate | **`service_duplicate` at load**; `workspace_invalid` reserved for structural/schema errors | Spec self-contradiction (workspace-file vs workspace-errors) resolved: semantic duplicate is a service error |
| Workspace file | required for --service, optional otherwise | **Optional until a workspace command/flag uses it** | Single-project flow never reads it |

## Package Layout

```
internal/pkg/workspace/
  workspace.go    — Workspace{Dir, Services []Service}, Service{Name, Path, Template}; Find(name)
  loader.go       — Load(path) strict yaml.v3 with Decoder.KnownFields(true), Validate
  discover.go     — Discover(cwd) — upward walk; explicit --workspace flag handled in cmd
  errors.go       — oops codes: workspace_not_found, workspace_invalid, service_not_found,
                    service_path_missing, service_duplicate, service_no_manifest
  loader_test.go, discover_test.go
```

## Data Flow

### workspace upgrade (FULL standard logic per service)

```
cmd/workspace.go
  │ ws := workspace.Load(flag or Discover(cwd))    ← workspace_not_found if absent
  │ for svc := range ws.Services (sequential):
  │   path := filepath.Join(ws.Dir, svc.Path)
  │   if !dirExists(path): report service_path_missing; continue
  │   oldWd, _ := os.Getwd()
  │   os.Chdir(path)                                ← chdir into service
  │   loadServiceConfig()                           ← viper snapshot + reload (defer restore)
  │   if !ManifestExists("."):                      ← legacy service
  │     report service_no_manifest (naming service); run upgradeLegacy non-interactively; continue
  │   plan, err := scaffold.Upgrade(cfg,
  │       scaffold.WithResolver(scaffold.DefaultResolver{}),   ← pack-source re-render works
  │       scaffold.WithRoot("."))                   ← default; explicit for clarity
  │   if err: summary[svc.Name] = err; os.Chdir(oldWd); continue
  │   print plan summary
  │   if yes: applied, err := plan.Apply()          ← DRY-RUN by default, --yes applies
  │           if err: summary[svc.Name] = err
  │           else: _ = scaffold.WriteVersionField(".go-arch.yaml", Version)  ← surgical ADR-4
  │   os.Chdir(oldWd); restoreConfig()
  │ print per-service summary; exit non-zero if any failed
```

Batch apply mode: **dry-run by default, `--yes` applies** (mirrors standalone `cmd/upgrade.go:88-98`). Legacy per-file interactive prompting is **non-interactive under batch** — legacy services apply fully with `--yes`, report `service_no_manifest`, no TTY prompt.

### generate --service

```
cmd/generate.go (--service flag set)
  │ ws := workspace.Load(flag or Discover(cwd))    ← workspace_not_found if absent
  │ svc := ws.Find(name)                            ← service_not_found if absent
  │ chdir into svc.Path (defer restore)
  │ loadServiceConfig()                             ← viper snapshot + reload (defer restore)
  │ run existing generate dispatch (unchanged, incl. --route)
```

### Upgrade WithRoot

```
scaffold.Upgrade(cfg, WithRoot(root), WithResolver(resolver))
  │ upgradeConfig.Root = root (default ".")         ← all filepath.Join(root, ...) use it; ADR-7 preserved
```

## Interfaces / Contracts

```go
// workspace package
func Load(path string) (*Workspace, error)          // workspace_not_found / workspace_invalid
func Discover(cwd string) (string, error)           // upward walk; workspace_not_found
func (w *Workspace) Find(name string) (*Service, bool)

// scaffold upgrade (modified)
type UpgradeOption func(*upgradeConfig)             // existing
func WithRoot(root string) UpgradeOption            // NEW — default "."

// cmd helpers (filed in cmd/workspace_helpers.go)
func resolveWorkspace(flag string) (*workspace.Workspace, error)  // flag wins, else Discover(cwd)
func withService(w *workspace.Workspace, name string, fn func() error) error  // chdir + viper snapshot/restore + defer
func loadServiceConfig() func()                      // snapshot/restore closure
```

### viper snapshot/restore contract (MED-4 fix)

```
snapshot:
  prev := viper.ConfigFileUsed()
reload (per service):
  viper.Reset()
  if service config exists: viper.SetConfigFile(absServiceConfig); viper.ReadInConfig()  // best-effort — skip on missing
  else: viper.AddConfigPath("."); viper.SetConfigName(".go-arch"); viper.ReadInConfig() // best-effort
restore (defer after service op):
  viper.Reset()
  if prev != "": viper.SetConfigFile(prev); viper.ReadInConfig()
  else: viper.AddConfigPath("."); viper.SetConfigName(".go-arch"); viper.ReadInConfig() // best-effort
```

Best-effort semantics: any `ReadInConfig` error on missing files is ignored (unlike `cmd/upgrade.go:58-62` which treats it as fatal — workspace reloads must not).

## File Change Plan

| File | Action | What |
|------|--------|------|
| `internal/pkg/workspace/workspace.go` | Create | Types (Dir, Services, Find) |
| `internal/pkg/workspace/loader.go` | Create | Load/Validate — `yaml.v3` `Decoder.KnownFields(true)` (note: packs/manifest.go:228 is NOT strict; workspace loader is stricter by design) |
| `internal/pkg/workspace/discover.go` | Create | Discover upward walk |
| `internal/pkg/workspace/errors.go` | Create | 6 oops codes |
| `internal/pkg/workspace/loader_test.go` | Create | Valid/duplicate→service_duplicate/unknown-key table |
| `internal/pkg/workspace/discover_test.go` | Create | Upward walk / none found |
| `internal/pkg/scaffold/upgrade_opts.go` | Modify | Add WithRoot + Root field (default ".") |
| `internal/pkg/scaffold/upgrade.go` | Modify | Use upgradeConfig.Root in ManifestExists/LoadManifest/filepath.Join/plan.ProjectRoot |
| `cmd/workspace.go` | Create | Parent command + upgrade/check subcommands; self-registers in its own `init()` (repo convention) |
| `cmd/workspace_helpers.go` | Create | resolveWorkspace, withService, loadServiceConfig |
| `cmd/root.go` | Modify | Persistent `--workspace` flag |
| `cmd/generate.go` | Modify | `--service` flag + chdir + config reload |
| `cmd/check.go` | Modify | `--service` flag + chdir + config reload |
| `cmd/upgrade.go` | Modify | `--service` flag + chdir + config reload (reuse existing Upgrade flow) |
| `docs/workspaces.md` | Create | Reference |
| `docs/COMMANDS.md`, `README.md` | Modify | Workspace docs |

## Testing Strategy

- **Unit — workspace package**: loader table (valid, duplicate→service_duplicate, unknown key→workspace_invalid, missing name/path, bad slug); discovery (upward walk, none found).
- **Unit — upgrade**: `WithRoot` (root used; default "." preserved — ADR-7 regression test).
- **Integration — workspace upgrade**: t.TempDir monorepo with 2 manifest services + 1 pack-source service; `workspace upgrade` (dry-run) → plans printed, nothing written; `workspace upgrade --yes` → files actually applied, version field written, **pack-source entry re-rendered (WithResolver)**, summary printed, continue-on-error with a failing service. Legacy service → `service_no_manifest` reported, legacy apply works with `--yes`.
- **Integration — workspace check**: both services checked, per-service summary.
- **Integration — --service**: `generate crud User --service orders` → files land in services/orders, CWD restored; unknown service → service_not_found; no workspace → error naming the flag + hint `--workspace`.
- **Integration — hooks CWD**: service with post-generate hook writing a marker → marker lands in the service dir; PROJECT_PATH points at the service.
- **Integration — config isolation**: monorepo-root config vs service config with different settings → service settings used; config restored after (subsequent command sees prior config).
- **Live smoke (verify phase)**: real chdir across services, pack-source re-render, Windows path notes.

## Key Risks

- **viper global-state snapshot/restore** (HIGH slice): initConfig runs once at startup; the opt-in snapshot/restore bounds it; best-effort reload must NOT be fatal on missing files. Live smoke verifies single-project byte-identical behavior.
- **os.Chdir global state**: sequential-only + defer restore (MCP precedent); no concurrent service ops in v1.
- **Manifest path collisions**: service manifests are relative; integration test verifies upgrade never writes monorepo-root files when chdir'd.
- **Batch apply semantics**: legacy interactive prompting is disabled under batch (non-interactive) — documented; `--yes` required to write anything.
- **Windows**: filepath semantics in YAML paths, chdir behavior — CI coverage note.
- **Workspace file schema is a new public contract**: strict KnownFields(true) validation + small v1 scope mitigate.
