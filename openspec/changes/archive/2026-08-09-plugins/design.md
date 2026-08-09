# Design: Plugins — Installable Template Packs

**status**: success
**next_recommended**: tasks

## Technical Approach

Packs are shipped as Go modules (a `go.mod` + `go-arch.yaml` at module root), fetched via `go mod download -json`, and materialized under `~/.go-arch/packs/<name>@<version>/`. A new `internal/pkg/packs` package owns manifest parsing, install/remove/list/update, and path resolution. The engine chain extends to `local > global > pack > embedded` via `Engine` options.

Pack resolution happens **in `cmd/new.go` (and MCP handler) before `NewScaffolder` is constructed**: the resolved `PackInfo{Dir, Manifest}` is injected via a new `ScaffoldOption` `WithPackInfo(packs.PackInfo)`. This also enforces the empty-templates check (G4/P5) — `pack "X" has no templates` — **before** any directory is created, satisfying spec `spec.md:566-567`. The `Upgrade()` function accepts a `PackResolver` via a new `UpgradeOption` `WithResolver(r)` for testability. Pack-declared hooks are parsed into a `*hooks.Config` at install time, opt-in with trust warning, and fired pack-scoped at `new` (never at `generate`).

## Architecture Decisions

| Decision | Options considered | Choice | Rationale |
|---|---|---|---|
| Pack on-disk layout | `<name>/` flat vs `<name>@<version>/` versioned | **`<name>@<version>/`** | Spec: multiple versions coexist; upgrade records exact version; versioned dir = no clobber |
| `name` source | derived from module path vs from pack manifest | **from manifest** | Module path (`github.com/org/repo`) is ugly as a UX slug; manifest `name: express` is the user-facing handle |
| Engine extension | new `PackEngine` wrapper vs options on existing `Engine` | **options on `Engine`** (`WithPacksDir`, `WithPack(name, version)`) | Keeps one `Engine` type; zero call-site changes for non-pack callers |
| Pack dispatch in `Execute` | extend the arch switch vs add a pack branch before it | **pack branch before the switch** (early return via `s.scaffoldPack()`) | Packs ARE the architecture — they don't participate in Minimalist/Standard/Hexagonal dispatch |
| Upgrade re-render for pack entries | re-use chain with pack priority vs force pack path | **force pack path** (bypass local/global/embedded) | Spec: "MUST NOT fall back to any other source when the recorded pack is available" |
| `go mod download` execution | inline `exec.Command` vs injectable `Downloader` interface | **`Downloader` interface** with `RealDownloader` + `FakeDownloader` | Mirrors hooks' `CommandRunner` pattern; required for unit tests without network |
| Pack hooks merge | merge into `.go-arch.yaml` vs fire pack-scoped | **fire pack-scoped** | Spec: pack hooks never pollute project config. Install stores `hooks_enabled` in pack sidecar |
| Sidecar metadata | separate `pack.json` sidecar vs extend pack's `go-arch.yaml` | **`pack.json` sidecar** next to installed `go-arch.yaml` | Keeps installed pack byte-identical to upstream; sidecar holds CLI-owned state |
| `engine.go:48` fix | `fmt.Printf` → `ui.Out` | **`fmt.Fprintf(ui.Out, ...)`** | Single-line change; MCP sets `ui.Out = os.Stderr` already. Correct line is **48** (inside `RenderTo`), not 47 |
| Pack injection timing (G1) | `WithPack` EngineOption resolved post-hoc vs pre-resolved `WithPackInfo` ScaffoldOption | **Pre-resolved `WithPackInfo(PackInfo)` ScaffoldOption** | `template.NewEngine()` inside `NewScaffolder` (scaffold.go:41) runs before `scaffoldPack()` resolves the pack; wiring post-hoc is fragile. Resolving in `cmd/new.go` BEFORE `NewScaffolder` ensures the pack dir+manifest are available when the engine is constructed |
| Pack template→target mapping (G2) | arbitrary convention vs fixed strip-`.tmpl` convention | **Strip `.tmpl` convention**: pack `templates/<path>.tmpl` → target `<path>`, lookup key `<path>.tmpl` | Simple, deterministic, mirrors existing embedded tree convention |
| `.go-arch.yaml` creation for pack projects (G2) | CLI writes minimal config vs pack supplies `.go-arch.yaml.tmpl` | **Pack supplies `templates/.go-arch.yaml.tmpl`** as part of its templates tree. The CLI does NOT write a separate one — the pack's template produces it (rendered with `cfg` as data). This ensures `template: express` appears naturally in the output. The pack manifest's `name` is injected into `cfg.Template` so the rendered config contains `template: <name>` |
| Binary asset discovery (G3) | implicit walk vs manifest-declared list | **Manifest declares `binary_assets` list**: `binary_assets: [{source: "assets/htmx.min.js", target: "static/js/htmx.min.js"}]` | Explicit > implicit. `embed.FS` is unavailable at pack time; walking the pack dir for "looks binary" files is fragile |
| `ResolveBinary` return shape (G3) | `(src string, source string, err error)` vs structured result | **`(ResolvedSource, error)` where `ResolvedSource` carries `Kind` + a `Read() ([]byte, error)` func** | `embed.FS` has no filesystem path; a string return can't represent the embedded step. A struct with a read function unifies pack-dir reads and embedded FS reads |
| Empty-pack ordering (G4) | check inside `scaffoldPack()` after `MkdirAll` vs check in `cmd/new.go` pre-Execute | **Check in `cmd/new.go` (and MCP handler) immediately after pack resolution, BEFORE `NewScaffolder` / `Execute`** | `os.MkdirAll(s.manifestDir(), 0755)` at scaffold.go:124 runs before pack dispatch; only pre-Execute check guarantees no `myapp/` is created for an empty pack. This ALSO resolves G1 — the resolved pack flows in as `WithPackInfo` |
| Upgrade resolver injection (F3) | package-level `packs.Path()` call vs injectable `Resolver` | **`Upgrade(cfg, WithResolver(r))` via `UpgradeOption`** | `Upgrade()` is currently a package-level func (upgrade.go:70). Adding an option keeps the signature backward-compatible while enabling tests to inject a fake resolver that returns `not-installed` |
| Reinstall idempotency (same version) | `os.Rename(tmp, dst)` over existing dir vs remove-then-rename | **`os.RemoveAll(dst)` then `os.Rename(tmp, dst)`** | `os.Rename` on POSIX fails with `ENOTEMPTY` against an existing non-empty directory (spec.md:121-126 idempotent case). Remove-first is atomic from the consumer's perspective because the tmp is already a complete pack |
| Trust-prompt UX | `ui.Confirm()` | **`survey.AskOne(&survey.Confirm{...}, &answer)` directly** | `ui.Confirm` does NOT exist in `internal/ui` (output.go has Success/Warning/Error/Info; prompts.go has RunWizard). Use `survey.AskOne` inline, matching how `RunWizard` uses survey already |
| MCP `architecture` vs `template` (P3) | keep `architecture` required always vs make optional when `template` present | **`architecture` becomes optional when `template` is present** | When a pack is the architecture, requiring `architecture` is contradictory. CLI: `new --template express` accepts `--architecture` as a fallback default (if pack has no opinion) but does NOT require it. MCP schema: `required` becomes conditional — implement by removing `architecture` from the static `required` list and validating in the handler: "require `architecture` unless `template` is set" |
| `new --template` default config (P2) | unspecified pack defaults | **`cfg = packDefaults(projectName)` fills `ModuleName` from `projectName`, `Architecture` from `""` (pack IS the arch), `UseDocker/UseObservability/UseGRPC/UseTemplHTMX` from `false`**. User can override with explicit flags (`--module`, `--docker`). The pack manifest's `layout` drives directory creation, not the wizard's arch switch |
| `--template express` no `@version` (P2) | unspecified resolution | **No `@version` → `packs.LatestInstalled(name)`** | Mirrors spec `spec.md:122` edge-case: "without `@version`, use latest installed" |

## Data Flow

### template install

```
cmd/template_install.go
  │ parse "<module>[@<version>]"  →  module, versionTag
  ▼
packs.Downloader.Download(module, versionTag)       ← go mod download -json
  │  returns source dir (GOMODCACHE/.../module@ver/)
  ▼
packs.LoadManifest(srcDir)                          ← parse + validate contract_version
  │  returns Manifest{name, version, hooks, layout, binary_assets}
  ▼
if !exists(srcDir + "/templates") or not a directory:
    return oops.Code("pack_no_templates").Errorf(...)   ← P1: explicit check
  ▼
if manifest.hooks non-empty:
    ui.Warning("⚠ Pack ... declares hooks that will run with your shell.")
    var answer bool
    survey.AskOne(&survey.Confirm{Message: "Enable hooks?"}, &answer)
    sidecar.HooksEnabled = answer
  else:
    sidecar.HooksEnabled = false
  ▼
dst = ~/.go-arch/packs/<name>@<version>/
if exists(dst): os.RemoveAll(dst)                     ← same-version reinstall fix
tmp = ~/.go-arch/packs/.tmp-<rand>/
copyDir(srcDir, tmp)
os.Rename(tmp, dst)                                    ← atomic per-posix
writeSidecar(dst, sidecar)                             ← pack.json
re-validate manifest from dst                          ← catches copy errors
on failure: os.RemoveAll(dst)                          ← no partial state
```

### new --template <pack>

```
cmd/new.go
  │ if --template set:
  │   name, version := packs.ParseRef(flagTemplate)
  │   if version == "":
  │       version = packs.LatestInstalled(name)       ← P2: explicit resolution
  │   packInfo, err := packs.Resolver{}.Resolve(name, version)
  │   if err != nil: return err
  │   if len(packInfo.Manifest.Templates) == 0:       ← G4/P5: pre-Execute check
  │       return oops.Code("pack_no_templates").Errorf("pack %q has no templates", name)
  │       // NO directory created yet — myapp/ does not exist
  │   cfg = packDefaults(projectName)                  ← P2: explicit defaults
  │   cfg.Template = name
  │   // architecture is optional; pack IS the architecture
  ▼
scaffold.NewScaffolder(cfg,
    WithPackInfo(packInfo),                            ← G1: pre-resolved pack injected
    WithRunner(packHooksRunner))                       ← pack-scoped hooks
  ▼
Scaffolder.Execute()
  │ pre-new hook fire (pack-scoped, PACK_NAME/PACK_VERSION injected)
  │ s.scaffoldPack()                                   ← early return, does NOT hit arch switch
  │   ├─ os.MkdirAll(s.manifestDir(), 0755)            ← FIRST disk write
  │   ├─ for each layout dir: os.MkdirAll(...)
  │   ├─ walk packInfo.Dir/templates/ via fs.WalkDir:
  │   │     for each file:
  │   │       relPath := strings.TrimSuffix(trimmedPath, ".tmpl")
  │   │       targetPath := relPath                    ← G2 convention
  │   │       lookupKey := trimmedPath + ".tmpl"       ← e.g. ".go-arch.yaml.tmpl"
  │   │       createFile(targetPath, lookupKey, cfg)
  │   │         └─ engine.Render()  ← chain: local > global > pack > embedded
  │   │       recordManifest(..., Source="pack:<name>@<version>")
  │   ├─ for each entry in manifest.binary_assets:     ← G3: manifest-declared
  │   │       createPackBinary(entry.Target, entry.Source, packInfo.Dir)
  │   │         └─ readFile(packInfo.Dir + "/" + entry.Source) → write target
  │   └─ WriteVersionField(.go-arch.yaml, s.version)   ← .go-arch.yaml already exists from templates
  ▼
post-new hook fire (pack-scoped)
```

### upgrade with pack source

```
scaffold.Upgrade(cfg, WithResolver(resolver))          ← F3: resolver injected
  for each entry:
    if entry.Source == "":                    → existing 3-step chain (local>global>embedded)
    if entry.Source == "pack:<name>@<ver>":
       packInfo, err := resolver.Resolve(name, ver)
       if err != nil (not-installed):
          classify PROTECTED
          warn("pack %q is not installed; entries protected", name+"@"+ver)
          continue
       render from packInfo.Dir/templates/<path>       ← bypass chain, direct read
       compare disk hash vs rerender hash
       → upgradable / protected / up_to_date
```

## File Changes

| File | Action | Description |
|---|---|---|
| `internal/pkg/packs/manifest.go` | New | `Manifest` struct (with `BinaryAssets`), `Load(path)`, `UnmarshalYAML` strict validation, `SupportedContractVersion = 1` |
| `internal/pkg/packs/paths.go` | New | `BaseDir()`, `Path(name, version)`, `ParseRef("express@1.0.0")`, `LatestInstalled(name)`, slug regex validation |
| `internal/pkg/packs/install.go` | New | `Install(module, version)`, `Remove`, `List`, `Update` — orchestrates download + `templates/` validation + trust prompt + remove-then-rename + sidecar + re-validate |
| `internal/pkg/packs/download.go` | New | `Downloader` interface + `RealDownloader` (exec `go mod download -json`) + `downloadJSON` struct {Dir, Error} |
| `internal/pkg/packs/sidecar.go` | New | `Sidecar{HooksEnabled, InstalledAt}` read/write of `pack.json` |
| `internal/pkg/packs/copy.go` | New | `copyDir(src, dst)` — Windows-safe, stdlib only |
| `internal/pkg/packs/errors.go` | New | oops codes: `invalid_pack_manifest`, `contract_version_mismatch`, `pack_not_found`, `pack_install_failed`, `pack_not_installed`, `pack_no_templates`, `pack_fetch_failed` |
| `internal/pkg/packs/manifest_test.go` | New | Table-driven: valid min, valid full, unknown key, missing name, bad slug, bad semver, contract mismatch |
| `internal/pkg/packs/install_test.go` | New | FakeDownloader: success, module-not-found, offline, idempotent re-install, hooks-trust sidecar, **no-templates rejection** |
| `internal/pkg/packs/paths_test.go` | New | ParseRef, LatestInstalled, slug validation table |
| `internal/pkg/template/engine.go` | Modify | Add `WithPacksDir`, `WithPack(name, ver)` options; add pack step in `getTemplate`; add `ResolveBinary(path) (ResolvedSource, error)`; replace `fmt.Printf` at **line 48** with `fmt.Fprintf(ui.Out, ...)` |
| `internal/pkg/template/engine_pack_test.go` | New | Chain precedence tests (4-step), pack miss fallback, no-pack-configured, binary resolution |
| `internal/pkg/scaffold/manifest.go` | Modify | `ManifestEntry.Source string \`yaml:"source,omitempty"\`` |
| `internal/pkg/scaffold/scaffold.go` | Modify | Add `WithPackInfo(PackInfo)` option (field on Scaffolder); `scaffoldPack()` method; `createPackBinary(target, relSource, packDir)`; early-return pack branch in `Execute`; fire pack-scoped hooks; pass `Source` into `recordManifest` |
| `internal/pkg/scaffold/upgrade.go` | Modify | `Upgrade(cfg, ...UpgradeOption)`; in re-render loop: when `entry.Source` starts with `pack:`, resolve via injected resolver; missing pack → PROTECTED + warning |
| `internal/pkg/scaffold/pack_resolver.go` | New | `Resolver` interface + default impl wrapping `packs.Path`; `Resolve(name, version)` returns `(PackInfo, error)` |
| `internal/pkg/scaffold/upgrade_opts.go` | New | `UpgradeOption` type + `WithResolver(Resolver) UpgradeOption` |
| `cmd/template.go` | New | `go-arch template` parent command |
| `cmd/template_install.go` | New | `template install <module>[@<version>]` — parses ref, `templates/` check, trust prompt via `survey.AskOne`, calls `packs.Install` |
| `cmd/template_list.go` | New | `template list` — sorted by name |
| `cmd/template_remove.go` | New | `template remove <name>[@<version>]` |
| `cmd/template_update.go` | New | `template update <name>` — re-fetches `@latest`, re-prompts for hooks |
| `cmd/new.go` | Modify | Add `--template` flag; when set: parse ref → resolve pack (LatestInstalled if bare) → validate non-empty templates → build `packDefaults` cfg → pass `WithPackInfo` to `NewScaffolder` |
| `internal/ui/prompts.go` | Modify | Add `Template string \`mapstructure:"template,omitempty"\`` to `ProjectConfig` |
| `internal/pkg/hooks/env.go` | Modify | `EnvContext` gains optional `PackName`/`PackVersion`; `BuildEnv` injects `PACK_NAME`/`PACK_VERSION` when non-empty |
| `internal/pkg/hooks/types.go` | Modify | Add `PackName`/`PackVersion` fields to `EnvContext` |
| `internal/pkg/mcp/server.go` | Modify | `new_project` schema: add optional `template: string`; **remove `architecture` from static `required`**; handler validates "architecture required unless template set"; sets `cfg.Template` |
| `internal/pkg/template/templates/common/config.tmpl` | Modify | Add commented `# template: <pack>` block (trust-free, just metadata) |
| `docs/packs.md` | New | Contract schema table, install location, `template` commands, trust warning, 4-step lookup, upgrade interaction |
| `docs/ARCHITECTURE.md` | Modify | Update lookup order to four steps |
| `README.md` | Modify | Same lookup update |
| `docs/COMMANDS.md` | Modify | Add `template install|list|remove|update` section |

## Interfaces / Contracts

```go
// internal/pkg/packs/manifest.go
const SupportedContractVersion = 1

type BinaryAsset struct {
    Source string `yaml:"source"` // relative to pack root, e.g. "assets/htmx.min.js"
    Target string `yaml:"target"` // relative to project root, e.g. "static/js/htmx.min.js"
}

type Manifest struct {
    ContractVersion int                    `yaml:"contract_version"`
    Name            string                 `yaml:"name"`
    Version         string                 `yaml:"version"` // semver
    Layout          []string               `yaml:"layout,omitempty"`
    Hooks           map[hooks.Type][]hooks.Entry `yaml:"hooks,omitempty"`
    BinaryAssets    []BinaryAsset          `yaml:"binary_assets,omitempty"` // G3: explicit list
}

// internal/pkg/packs/info.go
type PackInfo struct {
    Dir      string
    Manifest *Manifest
}

// internal/pkg/scaffold/pack_resolver.go
type Resolver interface {
    Resolve(name, version string) (PackInfo, error)
}

// internal/pkg/template/engine.go — binary resolution (G3)
type SourceKind int
const (
    SourceLocal SourceKind = iota
    SourceGlobal
    SourcePack
    SourceEmbedded
)
type ResolvedSource struct {
    Kind SourceKind
    Read func() ([]byte, error) // closure over FS path, pack dir path, or embed.FS entry
}
func (e *Engine) ResolveBinary(path string) (ResolvedSource, error)

// internal/pkg/scaffold/upgrade_opts.go
type UpgradeOption func(*upgradeConfig)
func WithResolver(r Resolver) UpgradeOption
```

## Testing Strategy

| Layer | What | Approach |
|---|---|---|
| Unit | Manifest parse/validation | Table-driven: 7+ cases (valid min/full, unknown key, missing name, bad slug, bad semver, contract mismatch v99, missing contract) |
| Unit | `ParseRef`/`LatestInstalled`/`ValidateSlug` | Table-driven on `t.TempDir()` with synthetic pack dirs |
| Unit | Install with `FakeDownloader` | Success, module-not-found, offline, idempotent re-install, hooks sidecar written, **`templates/` missing → `pack_no_templates`** |
| Unit | Engine 4-step chain precedence | 4 cases: pack wins over embedded, local wins over pack, global wins over pack, pack miss falls through |
| Unit | `ResolveBinary` chain | Same precedence via `ResolvedSource.Read` closure |
| Unit | `renderEntry` with `Source: pack:...` | Bypass chain, reads from synthetic pack dir |
| Unit | Missing-pack upgrade → PROTECTED | Synthetic manifest with source; **injected resolver returns not-installed**; assert classification + warning |
| Unit | Env vars `PACK_NAME`/`PACK_VERSION` | `BuildEnv` test: non-empty PackName → env contains them; empty → absent |
| Unit | Empty-templates pre-Execute check | `cmd/new.go`: resolve pack with empty `templates/` → error BEFORE `NewScaffolder`; assert `myapp/` NOT created |
| Integration | `new --template express` E2E | Fixture pack dir in `t.TempDir()` (pre-populated, no network); run `scaffoldPack`; assert generated files, manifest `source` field, `.go-arch.yaml` contains `template: express`, binary assets copied |
| Integration | `template install` with fake downloader | Asserts `templates/` validation + copy + remove-then-rename + sidecar + re-validation |
| Network-gated | Real `go mod download` | `if !testing.Short()` — skipped under `-short` |
| MCP | `new_project` with `template` param (no `architecture`) | Existing MCP test harness; verify `cfg.Template` propagates; verify `architecture` is NOT required when `template` set |
| RED / threat | `go mod download` stderr leak to stdout | Assert only `ui.Out` (stderr in MCP) receives output; stdout stays JSON-RPC clean |

## Threat Matrix

Shell-subprocess + executable-surface boundaries apply.

| Boundary | Applicability | Design response | RED test |
|---|---|---|---|
| Shell subprocess (`go mod download`) | Applicable | `Downloader` interface; stdlib `exec.CommandContext` with context cancellation; stderr captured separately | `FakeDownloader` returns error → install aborts with no partial dir |
| Executable file classification (hooks) | Applicable | Opt-in trust prompt via `survey.AskOne`; sidecar records `HooksEnabled`; pack-scoped fire; no merge into project config | Install with hooks declined → sidecar `HooksEnabled=false` → `new --template` fires no hooks |
| Documentation-like paths (`go-arch.yaml` at pack root) | N/A — no execution of doc files | Manifest parses as data; never executed | `go-arch.yaml` in fixture pack is parsed, not executed |
| Git/VCS/PR boundaries | N/A — no VCS interaction in this change | — | — |

## Migration / Rollout

No migration. `ManifestEntry.Source` is `omitempty` — old manifests parse on new CLI, new manifests parse on old CLI (field dropped). `ProjectConfig.Template` is additive. `template` command group is additive. Rollback = `git revert`.

## Sequence Diagrams

```
┌──────────────────────────────────────────────────────────────────────┐
│ template install github.com/org/go-arch-express@1.0.0                │
└──────────────────────────────────────────────────────────────────────┘

user                 cmd/template_install    packs         Downloader       fs
 │  "install <mod>"         │                  │               │             │
 │─────────────────────────>│                  │               │             │
 │                          │ ParseRef("...@1.0.0")            │             │
 │                          │─────────────────>│               │             │
 │                          │<──(name,ver)─────│               │             │
 │                          │ Download(mod,ver)│               │             │
 │                          │─────────────────────────────────>│             │
 │                          │                 {Dir:/cache/...@1.0.0}         │
 │                          │<──────────────────────────────────             │
 │                          │ LoadManifest(srcDir)            │             │
 │                          │─────────────────>│               │             │
 │                          │<── Manifest{...}─│               │             │
 │                          │ validate templates/ exists (P1)  │             │
 │                          │─────┐             │               │             │
 │                          │<────┘ OK or pack_no_templates     │             │
 │                          │                  │               │             │
 │   ⚠ Pack declares hooks  │                  │               │             │
 │<─────────────────────────│                  │               │             │
 │   Enable? [y/N]          │                  │               │             │
 │   (survey.AskOne)        │                  │               │             │
 │─────────────────────────>│                  │               │             │
 │                          │ if exists(dst): RemoveAll(dst)   │             │
 │                          │ tmp=.tmp-<rand>; copyDir(src,tmp)│             │
 │                          │ Rename(tmp, dst=<base>/express@1.0.0/)         │
 │                          │──────────────────────────────────────────────>│
 │                          │ writeSidecar(dst, {HooksEnabled})             │
 │                          │──────────────────────────────────────────────>│
 │                          │ re-validate manifest from dst   │             │
 │                          │─────────────────>│               │             │
 │                          │ OK               │               │             │
 │<── "installed" ──────────│                  │               │             │


┌──────────────────────────────────────────────────────────────────────┐
│ new myapp --template express                                         │
└──────────────────────────────────────────────────────────────────────┘

user            cmd/new           packs.Resolver     scaffold.Scaffolder    engine
 │  new myapp --template express    │                      │                   │
 │─────────────────────────────────>│                      │                   │
 │          [wizard BYPASSED]        │                      │                   │
 │                                   │ ParseRef("express")  │                   │
 │                                   │ version="" → LatestInstalled            │
 │                                   │ Resolve("express", version)             │
 │                                   │─────────────────────>│                   │
 │                                   │<── PackInfo{Dir,Manifest}               │
 │                                   │                      │                   │
 │                          [G4: validate Manifest.Templates non-empty]        │
 │                          [if empty: return pack_no_templates; NO myapp/]    │
 │                                   │                      │                   │
 │                                   │ cfg = packDefaults("myapp")             │
 │                                   │ cfg.Template = "express"                │
 │                                   │ NewScaffolder(cfg,                      │
 │                                   │   WithPackInfo(packInfo),               │
 │                                   │   WithRunner(...))                      │
 │                                   │──────────────────────────>│              │
 │                                   │                      Execute()           │
 │                                   │                      scaffoldPack()      │
 │                                   │                      ┌─ MkdirAll(manifestDir)
 │                                   │                      ├─ layout MkdirAll │
 │                                   │                      ├─ walk pack/templates/
 │                                   │                      │   strip ".tmpl"  │
 │                                   │                      │   createFile(target, key, cfg) ──> Render(chain)
 │                                   │                      │   recordManifest(Source="pack:express@<ver>")
 │                                   │                      ├─ binary_assets loop:
 │                                   │                      │   createPackBinary(entry.Target, entry.Source, packDir)
 │                                   │                      └─ WriteVersionField (config already rendered)
 │  [pre-new hook pack-scoped,       │                      │                   │
 │   PACK_NAME=express] ────────────<│──────────────────────│<─ Runner.Fire    │
 │  [post-new hook pack-scoped] ─────<│──────────────────────│<─ Runner.Fire    │
 │<── done ──────────────────────────│                      │                   │


┌──────────────────────────────────────────────────────────────────────┐
│ upgrade (pack-sourced project)                                       │
└──────────────────────────────────────────────────────────────────────┘

user         scaffold.Upgrade     manifest       resolver        engine
 │  upgrade        │                  │               │               │
 │────────────────>│                  │               │               │
 │                 │ LoadManifest     │               │               │
 │                 │─────────────────>│               │               │
 │                 │<── entries ──────│               │               │
 │                 │ for each entry:  │               │               │
 │                 │   if Source="pack:express@1.0.0":                 │
 │                 │     packInfo := resolver.Resolve("express","1.0.0")│
 │                 │     ─────────────────────────────>│               │
 │                 │<────────────────────────── PackInfo or not-installed
 │                 │     not-installed? classify PROTECTED             │
 │                 │              warn("pack ... not installed")       │
 │                 │     otherwise: read templates/<path> directly     │
 │                 │              hash, classify upgradable/protected  │
 │<── plan ────────│                  │               │               │
```

## Key Risks

Carried forward from the proposal:

| Risk | Likelihood | Mitigation |
|---|---|---|
| Contract v1 schema mistake becomes permanent API debt | Medium | `contract_version` field; small v1 surface; explicit v2 deferral in `docs/packs.md` |
| Pack removed after files generated → upgrade divergence | Medium | `ManifestEntry.source` field; missing pack → PROTECTED + warning, never silent fallback |
| Supply-chain: pack hooks run arbitrary commands | Medium | Trust warning at install, explicit opt-in confirmation, pack-scoped fire (no auto-merge) |
| MCP stdio corruption from `fmt.Printf` at `engine.go:48` | High (known latent) | Route through `ui.Out` in this change |
| Module proxy fetch slow on first install | Low | One-time fetch; `@latest` cached in `GOMODCACHE`; documented in `docs/packs.md` |
| **NEW — `WithPackInfo` option couples cmd and scaffold** | Low | Scaffold already has `WithRunner`/`WithVersion`; `WithPackInfo` is one more option, zero impact when absent |
| **NEW — `ResolvedSource.Read` closure hides errors until read-time** | Low | Callers MUST invoke `Read()` and surface errors; unit test asserts each `Kind` produces readable bytes |
| **NEW — Remove-then-rename window between `RemoveAll(dst)` and `Rename(tmp, dst)` leaves pack absent** | Low | `Rename` is atomic on POSIX; on Windows the window is negligible for local packs. Tests assert install completes successfully even on re-install |

## Open Questions

None — all proposal decisions are resolved. This design refines (not reopens) them.

## Key Refinements from Proposal

- **Refinement 1 — pack sidecar**: Proposal left `hooks_enabled` storage implicit. Added `pack.json` sidecar so install state is CLI-owned and upstream pack bytes stay pristine.
- **Refinement 2 — upgrade bypass**: Spec says pack entries MUST NOT fall back to embedded. Chain priority would let local overrides win on upgrade; design bypasses the chain entirely for pack-sourced entries.
- **Refinement 3 — `WithPackResolver` vs hard-coded packs dir**: Scaffold depends on a `Resolver` interface (not `packs` package directly) to keep `scaffold` testable without the full install machinery.
- **Refinement 4 — binary assets**: Manifest declares an explicit `binary_assets` list with `source`/`target` pairs; `ResolveBinary` returns a `ResolvedSource` struct with a `Read` closure to unify pack-dir and embedded FS reads.
- **Refinement 5 — pack injection timing (G1)**: Resolving the pack in `cmd/new.go` (pre-`NewScaffolder`) and injecting via `WithPackInfo(PackInfo)` ensures the engine sees the pack when `template.NewEngine()` runs inside `NewScaffolder`.
- **Refinement 6 — empty-pack pre-check (G4)**: Validation happens in `cmd/new.go` before any directory is created, preventing the scaffold.go:124 `MkdirAll` side-effect on empty packs.
- **Refinement 7 — pack template convention (G2)**: Pack `templates/<path>.tmpl` → target `<path>`, lookup key `<path>.tmpl`. Pack supplies `templates/.go-arch.yaml.tmpl` so the project's `.go-arch.yaml` contains `template: <name>`.
- **Refinement 8 — upgrade resolver (F3)**: `Upgrade()` gains `UpgradeOption` with `WithResolver(Resolver)`, keeping backward compatibility while enabling test injection.
- **Refinement 9 — MCP architecture conditional (P3)**: `architecture` is optional when `template` is set, in both MCP schema and CLI flag path.
- **Refinement 10 — reinstall atomicity**: `RemoveAll(dst)` + `Rename(tmp, dst)` replaces naive rename to handle the POSIX ENOTEMPTY case.
