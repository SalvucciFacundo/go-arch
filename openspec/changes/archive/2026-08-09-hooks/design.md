# Design: Hooks (lifecycle extensibility)

**status**: success
**next_recommended**: tasks

## Technical Approach

Add a `hooks:` root key to `.go-arch.yaml` with four lifecycle types (`pre-new`, `post-new`, `pre-generate`, `post-generate`). A new `internal/pkg/hooks/` package owns config parsing (hybrid string/object via `yaml.v3`, following `manifest.go:63`), a `Runner` that dispatches through a fakeable `CommandRunner` interface, and an env builder. The `Scaffolder` receives a `*hooks.Runner` via a `ScaffoldOption` and fires hooks inside `Execute`/`GenerateComponent`/`GenerateCRUD` — this gives MCP parity for free because `mcp/server.go:311` and `:358` already call those same methods. Output always routes through `ui.Out` (stderr under MCP); `os.Stdout` is never written to by the runner (verified by integration test).

## Architecture Decisions

| Decision | Options | Chosen | Rationale |
|---|---|---|---|
| Config parser | `viper.GetStringMap` vs separate `yaml.v3` read | **`yaml.v3` with `UnmarshalYAML` on `Entry`** | `viper.Get*` is flat (exploration §2); `manifest.go:63` already uses `yaml.Unmarshal`; hybrid string-vs-object needs `yaml.Node` type switching that mapstructure cannot express |
| Wiring layer | cmd-only vs scaffold-layer | **Scaffold-layer with cmd/MCP injection** | MCP parity without duplicating fire logic; cmd and MCP callers both call `hooks.Load(path)` and pass the runner via `WithRunner` |
| `pre-new` / `post-new` placement | cmd layer vs inside `Execute()` | **Inside `Execute()`** (first line / after `WriteVersionField`) | Preserves MCP parity; `pre-new` fires before `MkdirAll`; `post-new` fires after all files written AND after `go_arch_version` is written to `.go-arch.yaml` (spec execution.md:12) |
| `WriteVersionField` ownership | stays in `cmd/new.go` | **moved INTO `Execute()`** | Spec requires `post-new` to see `go_arch_version` in `.go-arch.yaml`; version injected via `WithVersion(v string)` so scaffold doesn't import cmd/mcp |
| Shell dispatch | `strings.Fields` vs `sh -c` / `cmd /c` | **`sh -c` (unix) / `cmd /c` (windows)** for strings; argv-direct for objects | Matches proposal decision 5; strings expand env vars (npm precedent); objects give cross-platform safety |
| Timeout implementation | manual `time.AfterFunc` vs `exec.CommandContext` | **`exec.CommandContext` with `context.WithTimeout`** | Existing codebase has no `CommandContext` (exploration §4); stdlib handles SIGKILL on cancel; `0` disables by passing `context.Background()` |
| `CommandRunner` interface | inline `exec.Command` vs interface | **Interface** | go-testing skill: small mock at command boundary; `FakeRunner` drives unit tests; real `ExecRunner` wraps `CommandContext` |
| Config path resolution | inline in each caller vs exported helper | **`hooks.ResolveConfigPath()`** exported from hooks package | Single source of truth for cmd/ and mcp/ callers; no naming drift |
| Error taxonomy | ad-hoc vs `oops` codes | **`oops` codes with `Hint`** | Matches repo convention (`cmd/generate.go:42`, `cmd/new.go:27`) |

## Data Flow

```
┌─────────────────────────────────────────────────────────────────────────┐
│  cmd/new.go  ·  cmd/generate.go  ·  mcp/server.go (new_project)       │
│                                                                         │
│  path := hooks.ResolveConfigPath()                                      │
│  hooksCfg, err := hooks.Load(path)     // missing file → empty, nil     │
│  runner := hooks.NewRunner(hooksCfg, hooks.RealRunner{}, ui.Out)        │
│  scaffolder := scaffold.NewScaffolder(cfg,                              │
│      scaffold.WithRunner(runner),                                       │
│      scaffold.WithVersion(Version))     // mcp.Version for MCP          │
└──────────────────────────────┬──────────────────────────────────────────┘
                               │
                               ▼
┌─────────────────────────────────────────────────────────────────────────┐
│  Scaffolder.Execute()                                                   │
│                                                                         │
│  runner.Fire(PreNew, envCtx, cwd)      ← CWD: invocation dir            │
│  os.MkdirAll(projectName)                                               │
│  switch arch → scaffoldMinimalist/Standard/Hexagonal                    │
│  WriteVersionField(.go-arch.yaml, s.version)  // non-fatal if empty     │
│  runner.Fire(PostNew, envCtx, cwd)     ← CWD: new project dir (abs)     │
└──────────────────────────────┬──────────────────────────────────────────┘
                               │
                               ▼
┌─────────────────────────────────────────────────────────────────────────┐
│  runner.Fire(type Type, ctx EnvContext, defaultCwd string)              │
│                                                                         │
│  for _, entry := range cfg.Hooks[type]:                                 │
│    cmd := buildCommand(entry, ctx, type)                                │
│    cmd.Stdout, cmd.Stderr → ui.Out  (io.Writer)                         │
│    cmd.Stdin = strings.NewReader("")  // EOF immediately                │
│    err := realRunner.Run(ctx, cmd)                                      │
│    on error:                                                            │
│      ignore_failure → ui.Warning, continue                              │
│      else → return oops.Wrap(err, code)                                 │
└─────────────────────────────────────────────────────────────────────────┘
```

## File Changes

| File | Action | Description |
|---|---|---|
| `internal/pkg/hooks/types.go` | Create | `Type` enum, `Entry` struct, `Config` struct, `EnvContext` (includes `HookType`) |
| `internal/pkg/hooks/config.go` | Create | `Load(path)`, `ResolveConfigPath()`, `Entry.UnmarshalYAML(*yaml.Node)`, validation. **`Load` on missing file → empty `Config`, nil error** |
| `internal/pkg/hooks/config_test.go` | Create | Table-driven parse: string, object, mixed, unknown key, missing command, bad timeout, empty list, scalar-instead-of-list, missing file |
| `internal/pkg/hooks/runner.go` | Create | `Runner`, `CommandRunner` interface, `RealRunner`, `Fire(t, ctx, defaultCwd)` |
| `internal/pkg/hooks/runner_test.go` | Create | FakeRunner table tests: happy path, stop-on-first, ignore_failure, timeout kill, shell vs argv, CWD resolution, silent flag, stdin closed, HOOK_TYPE in env |
| `internal/pkg/hooks/env.go` | Create | `BuildEnv(parent []string, ctx EnvContext, entry Env map) []string` — merges parent + 4 standard vars (incl. `HOOK_TYPE` from `ctx.HookType`) + per-entry overrides |
| `internal/pkg/hooks/env_test.go` | Create | Precedence tests: parent < standard < per-entry; `HOOK_TYPE` matches fired type |
| `internal/pkg/hooks/errors.go` | Create | `oops` code constants: `unknown_hook_type`, `invalid_hook_config`, `hook_failed`, `hook_timeout`, `hook_command_not_found` |
| `internal/pkg/hooks/integration_test.go` | Create | Asserts zero bytes on `os.Stdout` during a hook run with `ui.Out` redirected (MCP guard) |
| `internal/pkg/scaffold/scaffold.go` | Modify | Add `runner *hooks.Runner` + `version string` fields, `ScaffoldOption` type, `WithRunner`, `WithVersion`, move `WriteVersionField` into `Execute()` before `Fire(PostNew)`, fire 4 sites |
| `cmd/new.go` | Modify | Load hooks via `hooks.ResolveConfigPath()`, construct runner, pass `WithRunner` + `WithVersion(Version)`; **remove direct `WriteVersionField` call** |
| `cmd/generate.go` | Modify | Load hooks via `hooks.ResolveConfigPath()`, construct runner, pass via `WithRunner` |
| `internal/pkg/mcp/server.go` | Modify | `new_project`: load hooks via `hooks.ResolveConfigPath()`, build runner, pass `WithRunner` + `WithVersion(Version)`; `generate_component`: load hooks, build runner, pass `WithRunner` |
| `internal/pkg/template/templates/common/config.tmpl` | Modify | Add commented `# hooks:` example block |
| `docs/hooks.md` | Create | Reference doc: schema, fire sites, trust warning, MCP behavior |
| `internal/pkg/scaffold/scaffold_test.go` | Modify | Extend: fire order, CWD for post-new, stop-on-first, post-new sees `go_arch_version` in `.go-arch.yaml` |

## Interfaces / Contracts

```go
// internal/pkg/hooks/types.go
package hooks

import "time"

type Type string
const (
    PreNew       Type = "pre-new"
    PostNew      Type = "post-new"
    PreGenerate  Type = "pre-generate"
    PostGenerate Type = "post-generate"
)

var validTypes = map[Type]bool{
    PreNew: true, PostNew: true, PreGenerate: true, PostGenerate: true,
}

type Entry struct {
    Command       string
    Args          []string
    Cwd           string
    Env           map[string]string
    Timeout       time.Duration
    Silent        bool
    IgnoreFailure bool
    Shell         bool            // true when parsed from string form
}

type Config struct {
    Hooks map[Type][]Entry
}

type EnvContext struct {
    ProjectName string
    ProjectPath string            // absolute
    Arch        string
    HookType    Type              // drives HOOK_TYPE env var
}

// internal/pkg/hooks/runner.go
type CommandRunner interface {
    Run(ctx context.Context, name string, args []string, opts RunOpts) (exitCode int, err error)
}

type RunOpts struct {
    Dir    string
    Env    []string
    Stdin  io.Reader
    Stdout io.Writer
    Stderr io.Writer
}

type Runner struct {
    cfg *Config
    cmd CommandRunner
    out io.Writer               // ui.Out — NEVER os.Stdout in MCP
}

func NewRunner(cfg *Config, cmd CommandRunner, out io.Writer) *Runner
func (r *Runner) Fire(t Type, ctx EnvContext, defaultCwd string) error

// internal/pkg/hooks/config.go
func Load(path string) (*Config, error)
// Load contract: nonexistent path → &Config{Hooks: map[Type][]Entry{}}, nil
func ResolveConfigPath() string
// viper.ConfigFileUsed() if non-empty, else $HOME/.go-arch.yaml
func (e *Entry) UnmarshalYAML(value *yaml.Node) error  // hybrid dispatch

// internal/pkg/hooks/env.go
func BuildEnv(parent []string, ctx EnvContext, perEntry map[string]string) []string
// Sets PROJECT_NAME, PROJECT_PATH, ARCHITECTURE, HOOK_TYPE from ctx
// Merges: parent env < ctx standard vars < perEntry overrides

// internal/pkg/scaffold/scaffold.go additions
type ScaffoldOption func(*Scaffolder)
func WithRunner(r *hooks.Runner) ScaffoldOption
func WithVersion(v string) ScaffoldOption
```

## Sequence Diagrams

```
go-arch new myproj (CLI)
──────────────────────
cmd/new.go RunE
  │
  ├─ ui.RunWizard() → config
  ├─ hooks.Load(hooks.ResolveConfigPath()) → hooksCfg
  ├─ hooks.NewRunner(hooksCfg, RealRunner{}, ui.Out) → runner
  ├─ scaffold.NewScaffolder(config, WithRunner(runner), WithVersion(Version))
  │
  ├─ scaffolder.Execute()
  │     ├─ runner.Fire(PreNew, EnvContext{…, HookType: PreNew}, cwd)
  │     │     └─ defaultCwd = cwd (project dir doesn't exist yet)
  │     ├─ os.MkdirAll(config.ProjectName)
  │     ├─ switch arch → scaffoldXxx → createCommonFiles
  │     ├─ WriteVersionField(.go-arch.yaml, s.version)  // non-fatal
  │     └─ runner.Fire(PostNew, EnvContext{…, HookType: PostNew}, projDir)
  │           └─ defaultCwd = abs(cwd/config.ProjectName)
  │           └─ hook sees go_arch_version in .go-arch.yaml ✓
  │
  └─ ui.Success(...)
  (no more WriteVersionField here — moved into Execute)


go-arch generate service Order (CLI)
────────────────────────────────────
cmd/generate.go RunE
  │
  ├─ viper.GetString("project_name") → guard
  ├─ config from viper
  ├─ hooks.Load(hooks.ResolveConfigPath()) → hooksCfg
  ├─ hooks.NewRunner(...) → runner
  ├─ scaffold.NewScaffolder(config, WithRunner(runner))
  │
  ├─ scaffolder.GenerateComponent("service", "Order")
  │     ├─ runner.Fire(PreGenerate, EnvContext{…, HookType: PreGenerate}, ".")
  │     ├─ createFile(...)
  │     ├─ recordManifest(...)
  │     └─ runner.Fire(PostGenerate, EnvContext{…, HookType: PostGenerate}, ".")
  │
  └─ ui.Success(...)


MCP new_project
───────────────
mcp/server.go handleToolCall("new_project")
  │
  ├─ args → cfg (ui.ProjectConfig)
  ├─ hooks.Load(hooks.ResolveConfigPath()) → hooksCfg    ← NEW
  ├─ hooks.NewRunner(hooksCfg, RealRunner{}, ui.Out)
  ├─ scaffold.NewScaffolder(cfg,
  │     WithRunner(runner), WithVersion(Version))         ← NEW
  │
  ├─ scaffolder.Execute()
  │     ├─ Fire(PreNew)
  │     ├─ scaffold files
  │     ├─ WriteVersionField
  │     └─ Fire(PostNew)
  │
  └─ sendToolResult(...)


MCP generate_component
──────────────────────
mcp/server.go handleToolCall("generate_component")
  │
  ├─ os.Chdir(projectPath)
  ├─ viper.Reset + ReadInConfig
  ├─ config from viper
  ├─ hooks.Load(hooks.ResolveConfigPath()) → hooksCfg   ← NEW
  ├─ hooks.NewRunner(...) → runner                       ← NEW
  ├─ scaffold.NewScaffolder(cfg, WithRunner(runner))
  │
  ├─ scaffolder.GenerateCRUD/Component(...)
  │     ├─ Fire(PreGenerate)
  │     ├─ generate files + routes registry
  │     └─ Fire(PostGenerate)
  │
  └─ sendToolResult(...)
```

## Testing Strategy

| Layer | What | Approach | File |
|---|---|---|---|
| Unit — config | string form, object form, mixed list, unknown type, unknown key, missing command, invalid timeout, `0` timeout, scalar-instead-of-list, empty list, missing key, **missing file → empty+nil** | Table-driven `yaml.Unmarshal` into `Config` | `hooks/config_test.go` |
| Unit — runner | happy path, stop-on-first, `ignore_failure` continues, timeout kills, shell vs argv, CWD default + override, silent flag, stdin closed, **HOOK_TYPE in env matches fired type** | `FakeRunner` captures calls; assert exit codes, args, `Dir`, env | `hooks/runner_test.go` |
| Unit — env | parent env inherited, 4 standard vars set (incl. `HOOK_TYPE`), per-entry overrides standard, `PROJECT_PATH` absolute | `BuildEnv` table tests | `hooks/env_test.go` |
| Integration | zero bytes on `os.Stdout` during hook run when `ui.Out` is a `bytes.Buffer` | Redirect `os.Stdout` to a pipe; assert empty | `hooks/integration_test.go` |
| Integration — scaffold | `pre-new` fires before MkdirAll; **`post-new` fires after WriteVersionField and sees `go_arch_version` in `.go-arch.yaml`**; `post-generate` fires after `renderRoutesRegistry`; stop-on-first in scaffold | `t.TempDir`, `FakeRunner` injected via `WithRunner` + `WithVersion` | `scaffold/scaffold_test.go` (extended) |
| Real-tool | `gofmt`, `go mod tidy` on scaffolded project | Skip under `testing.Short()` | `scaffold/scaffold_test.go` (extended) |

Skips: real-tool tests guarded by `if testing.Short() { t.Skip() }` per go-testing skill.

## Threat Matrix

The design spawns subprocesses from user-controlled config. The applicable boundary is shell/argv dispatch and output routing. The standard threat-matrix rows (git repo selection, commit state, push state, PR commands, documentation-like paths) are **N/A** — the runner does not invoke git, VCS, or PR automation.

| Boundary | Applicability | Design response | RED test |
|---|---|---|---|
| Subprocess: shell vs argv | Applicable | Strings → `sh -c`/`cmd /c`; objects → argv-direct. No shell interpolation for objects | `TestRunner_ShellVsArgv` — asserts `$HOME` literal in argv form, expanded in string form |
| Subprocess: output routing | Applicable | `cmd.Stdout`/`cmd.Stderr` → `ui.Out`; integration test redirects `os.Stdout` to pipe, asserts zero bytes | `TestIntegration_NoStdoutInMCPMode` |
| Subprocess: timeout/cancellation | Applicable | `exec.CommandContext` + `context.WithTimeout`; `0` → `context.Background()` | `TestRunner_Timeout_Kills` — `sleep 60` with `100ms` timeout |
| Subprocess: stdin closure | Applicable | `cmd.Stdin = strings.NewReader("")` (EOF immediately) | `TestRunner_StdinClosed` — `cat` exits 0 |
| Subprocess: CWD authority | Applicable | `cmd.Dir` set per hook type; never `os.Chdir`; per-hook `cwd:` resolved relative to default | `TestRunner_CWD_Defaults` + `TestRunner_CWD_Override` |
| Config trust model | Applicable | `docs/hooks.md` carries npm-script-equivalent trust warning; `config.tmpl` has commented example with caution | docs review |

## Error Taxonomy

| Code | When | Hint |
|---|---|---|
| `unknown_hook_type` | Config has key outside the four valid types | "Valid hook types: pre-new, post-new, pre-generate, post-generate" |
| `invalid_hook_config` | Malformed entry (unknown object key, missing command, bad duration, scalar-instead-of-list) | "See docs/hooks.md for the hooks schema" |
| `hook_failed` | Hook exits non-zero and `ignore_failure` is not set | "Fix the hook command or set ignore_failure: true" |
| `hook_timeout` | Hook exceeds its timeout | "Increase the timeout: field or optimize the hook command" |
| `hook_command_not_found` | Binary not in `PATH` (`exec.LookPath` fails) | "Install the missing tool or check your PATH" |

CLI boundary (`cmd/root.go:23-31`): `ui.Fatal(oops.Wrap(...))` → exit 1, no change needed.

## Migration / Rollout

No migration required. Older CLI versions skip `hooks:` via viper's unknown-key tolerance (verified by spec scenario). The feature is additive and non-breaking. Rollout: land the package + wiring + docs in one PR; no feature flag needed because missing/empty `hooks:` is a silent no-op.

## Open Questions / Refinements from Proposal

All 10 proposal questions are resolved. This design makes three explicit refinements beyond the proposal:

1. **Config loading via `yaml.v3` direct read** — not `viper.GetStringMap` — because viper's flat map cannot express the hybrid string/object form without a custom decoder that mapstructure cannot drive.
2. **`pre-new` / `post-new` fire inside `Execute()`** — not cmd-layer — to preserve MCP parity while satisfying spec ordering (before MkdirAll / after all files written).
3. **`mcp/server.go` modified** (proposal listed it as "Unchanged") — `new_project` and `generate_component` tool handlers now call `hooks.ResolveConfigPath()`, `hooks.Load()`, `hooks.NewRunner()`, and pass `WithRunner` + `WithVersion`. This is a legitimate refinement: spec `mcp-and-upgrade.md:9-11` requires hooks to fire under MCP, which is impossible without runner injection at the `server.go:311` and `:358` call sites. The proposal's "Unchanged" annotation assumed scaffold-layer wiring would be sufficient, but the runner still needs to be *constructed and injected* somewhere — for MCP, that place is `server.go`.

## Key Risks

1. **`fmt.Printf` in `scaffold.go`** (lines 81, 452, 494) still writes to `os.Stdout` directly. The hooks runner will NOT compound this, but MCP users remain exposed to those existing prints. Out of scope for this change but worth a follow-up.
2. **Non-atomic `post-*`**: a failing `post-new` leaves a half-scaffolded project on disk. Documented, not fixed. `ignore_failure` is the escape hatch.
3. **Cross-platform shell divergence**: strings run via `sh -c` (unix) or `cmd /c` (windows). Documented in `docs/hooks.md`; users wanting portability should use object form.
4. **`WriteVersionField` failure is now silent inside `Execute()`**: if it fails, `post-new` still fires (non-fatal). A `post-new` hook that reads `.go-arch.yaml` may or may not see `go_arch_version` depending on whether `WriteVersionField` succeeded. Tests assert the happy path; the failure mode is documented as acceptable (version field is informational).
