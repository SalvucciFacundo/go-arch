# Design: Generators (plugins v2)

**status**: success
**next_recommended**: tasks

## Technical Approach

A v2 pack contract introduces `generators:` as a map of **YAML recipe DSLs** executed by a new `internal/pkg/generators` package. Recipes are ordered step lists (`template`, `binary`, `run`, `prompt`, `use`) — declarative data, not code. `generate <name>` resolves through a three-tier lookup (pack → builtin → component) with pack winning on collision. `run:` steps ride a new `generators.Runner.FireEntries` reusing `hooks.resolveCommand`/`BuildEnv`/`resolveTimeout`, gated by the sidecar `HooksEnabled` flag. Path sandboxing is pre-flight: ALL target paths validated before ANY write using **separator-aware prefix checks**. Provenance: single-entry-with-metadata (see Refinements vs Spec). Prompt resolution is **pre-flight** (before any write) — resolved values accumulate into an `Args` map. Template steps render with **standard `ProjectConfig` data only** (prompt values are NOT template variables — this preserves the byte-identical upgrade claim). MCP relaxes `generate_component.type` enum + adds `generatorArgs` and `list_generators`. Contract v2 supported-set {1,2}.

## Architecture Decisions

| Decision | Choice | Alternative | Rationale |
|---|---|---|---|
| Package layout — no import cycle | `generators` defines `Record` type + returns `[]Record`; `scaffold` imports `generators` and does manifest upsert | `generators` imports `scaffold.Manifest` | Go forbids import cycles. `generators` MUST NOT import `scaffold`. The dependency arrow is one-way: `scaffold → generators`. |
| Step union | `Step` struct with custom `UnmarshalYAML` (mirrors `hooks.Entry`) | `interface{}` + discriminator | Struct is typed, testable, matches existing repo pattern. |
| Contract version | Supported set `{1, 2}` via slice check | Single constant | Backward-compat; v1 packs unaffected. |
| Sandbox API — separator-aware | `filepath.Clean` → reject absolute → reject `..` → `EvalSymlinks` → prefix check with `root + string(os.PathSeparator)` boundary | Naive `strings.HasPrefix` | `HasPrefix("/home/u/myapp", "/home/u/my")` matches sibling `/home/u/myapp-evil`. Separator boundary prevents false positives. Windows: `filepath.Clean` + volume-aware prefix (both paths cleaned, compare `filepath.VolumeName` first). |
| Executor model | Linear pass; fail-fast unless `ignore_failure`; partial state preserved | Two-pass commit | Recipes are data; rollback is fragile and not spec'd. |
| Run step mechanism | `generators.Runner.FireEntries(entries []hooks.Entry, ctx, cwd)` — new type in `generators` reusing `hooks.resolveCommand`/`BuildEnv`/`resolveTimeout` helpers (exported) | Inline CmdRunner in executor | Single mechanism. `FireEntries` is the ONLY path for `run:` step execution. `hooks.Runner` (config-registered) stays for pack-level hooks. Helpers (`ResolveCommand`, `BuildEnv`, `ResolveTimeout`) are exported from `hooks` for reuse. |
| Template rendering — no chain fallback | `engine.RenderPackOnly(wr, packDir, from, data)` — reads `<packDir>/templates/<from>` directly; returns `generator_template_not_found` on miss | `engine.RenderTo` with chain | Spec requires pack-only; no fallback to local/global/embedded. New method on `template.Engine`. |
| Prompt resolution | Pre-flight pass: ALL prompts resolved BEFORE any write → `Args` map → flows into `run:` step env + builtins | Inline per-step resolution | Spec: "no steps execute" when required prompt is missing (spec.md:504,522). Non-required/no-default → empty string (spec.md:508-511). |
| Template step data isolation | Template steps render with standard `ProjectConfig` data ONLY (via `buildRenderData`). Prompt values are NOT injected as template variables | Pass Args to template data | Preserves byte-identical upgrade claim — `renderPackEntry` in upgrade uses same `ProjectConfig` data and produces identical output. |
| Metadata args encoding | `json.Marshal(args)` → `Metadata["args"]` (JSON string in `map[string]string`) | Store as nested map | `ManifestEntry.Metadata` is `map[string]string`; JSON encoding preserves the structured args object. |
| `--list` arg validation | Custom `Args` validator: allow 0 args when `--list`, else `ExactArgs(2)` | `cobra.ExactArgs(2)` only | `generate --list` has 0 positional args; current validator rejects it. |
| Install trust — run:-only packs | Fire trust prompt when v2 pack declares generators with `run:` steps OR `pre:`/`post:` hooks, even without pack-level `hooks:` | Only check `len(m.Hooks) > 0` | A run:-only pack could silently execute commands without trust prompt. |
| Missing pack on generate | When `.go-arch.yaml` declares `template: X` but X not installed: (1) try builtin → (2) try component types → (3) if still unmatched, error `pack_not_installed` naming X + `go-arch template install` hint | Hard error immediately on pack load failure | Component types must still work when pack is missing (spec.md:268-272). |

## Data Flow

```
CLI: go-arch generate <name> [args...]
  │
  ├─ arg validation: --list → 0 args allowed; else exact 2
  ├─ read .go-arch.yaml → template: <pack>[@<ver>]
  ├─ load installed pack:
  │    ├─ packs.Load(dir) → contract v2 check
  │    ├─ sidecar: HooksEnabled?
  │    └─ LOAD FAILURE → resolvePackMissing flow:
  │         ├─ try builtinRegistry[name]  ─► builtin
  │         ├─ try componentType[name]    ─► GenerateComponent (existing)
  │         └─ still unmatched → pack_not_installed error + install hint
  │
  ├─ three-tier resolve (pack loaded successfully):
  │    1. pack.Generators[name]  ─► pack generator
  │    2. builtinRegistry[name]  ─► builtin
  │    3. componentType[name]    ─► GenerateComponent (existing)
  │
  └─ scaffold.GeneratePackGenerator(name, args):
       ├─ PRE-FLIGHT: prompt resolution pass
       │    ├─ for each prompt step: generatorArgs[name] → default → "" (if !required) → error (if required+missing)
       │    ├─ resolved values → Args map[string]any
       │    └─ ANY required prompt unresolvable → ABORT (no writes)
       │
       ├─ PRE-FLIGHT: sandbox validation (ALL file-writing step paths)
       │    └─ ANY path escapes → ABORT (zero writes)
       │
       ├─ generators.Run(ctx, recipe, opts):
       │    ├─ pre hooks via Runner.FireEntries (if HooksEnabled)
       │    ├─ for each step:
       │    │     template → engine.RenderPackOnly (pack-scoped, NO chain fallback)
       │    │     binary   → copy from pack dir
       │    │     run      → Runner.FireEntries (if HooksEnabled, else warn+skip)
       │    │     use      → builtin registry lookup
       │    │     (prompt already resolved in pre-flight — skip at execution)
       │    └─ post hooks (if HooksEnabled + no step failure)
       │
       ├─ generators.Run returns []Record (path + origin + provenance metadata)
       │
       └─ scaffold records manifest from []Record:
            ├─ template-step files → origin: "template", source: "pack:<name>@<ver>",
            │    template: "<from>", metadata: {generator: "<name>", args: <json>}
            └─ other files → origin: "generator", source: "pack:<name>@<ver>",
                 metadata: {generator: "<name>", args: <json>}
```

## File Changes

| File | Action | Description |
|---|---|---|
| `internal/pkg/generators/recipe.go` | Create | `Generator`, `Step`, `Record` types + `UnmarshalYAML` on `Step`. `Record` carries `Path`, `Origin`, `Source`, `TemplatePath`, `Metadata map[string]string`. |
| `internal/pkg/generators/validate.go` | Create | Parse-time validation: required fields, unknown prompt fields, `use:` pattern, duplicate prompt names, empty recipes. Returns `invalid_pack_manifest`. |
| `internal/pkg/generators/executor.go` | Create | `Run(ctx, recipe, opts) ([]Record, error)` — linear step execution. Returns records for manifest recording. Does NOT import `scaffold`. |
| `internal/pkg/generators/runner.go` | Create | `Runner.FireEntries(entries []hooks.Entry, ctx hooks.EnvContext, cwd string) error` — reuses exported `hooks.ResolveCommand`, `hooks.BuildEnv`, `hooks.ResolveTimeout`. Single mechanism for `run:` step execution. |
| `internal/pkg/generators/sandbox.go` | Create | `ValidateTarget(projectRoot, relPath string) error` — separator-aware prefix check (see decisions). |
| `internal/pkg/generators/registry.go` | Create | Empty `BuiltinRegistry` map + `Register`/`Lookup`. |
| `internal/pkg/generators/errors.go` | Create | oops codes: `unknown_step_type`, `recipe_path_escape`, `unknown_builtin`, `generator_step_failed`, `generator_template_not_found`, `generator_run_skipped_trust`, `generator_prompt_unresolvable`, `missing_generator_argument`, `unknown_generator`, `pack_not_installed`. |
| `internal/pkg/generators/recipe_test.go` | Create | Table-driven parse/validation tests. |
| `internal/pkg/generators/sandbox_test.go` | Create | Table-driven: absolute, `..`, symlink, sibling false-positive (`/home/u/myapp-evil`), valid. Uses `t.TempDir`. |
| `internal/pkg/generators/executor_test.go` | Create | Fake `CommandRunner` + `PromptResolver`; step failure, `ignore_failure`, partial state, trust gate, record output. |
| `internal/pkg/generators/runner_test.go` | Create | `FireEntries` tests reusing `hooks` test patterns. |
| `internal/pkg/packs/manifest.go` | Modify | Add `generators` to `knownManifestKeys`; change supported version to set `{1,2}`; decode `Generators map[string]generators.Generator`; v1+generators → `invalid_pack_manifest`. |
| `internal/pkg/packs/manifest_test.go` | Modify | Tests for v2 acceptance, v1 rejection, empty map, unknown step. |
| `internal/pkg/packs/install.go` | Modify | Trust prompt fires when `len(m.Hooks) > 0` OR any v2 generator declares `run:` steps / `pre:`/`post:` hooks. |
| `internal/pkg/packs/install_test.go` | Modify | Test: v2 pack with run:-only generator triggers trust prompt. |
| `internal/pkg/hooks/types.go` | Modify | Add `GeneratorName string` to `EnvContext`. |
| `internal/pkg/hooks/env.go` | Modify | Emit `GENERATOR_NAME` when non-empty. |
| `internal/pkg/hooks/env_test.go` | Modify | Leak test + presence test. |
| `internal/pkg/hooks/runner.go` | Modify | Export `ResolveCommand`, `ResolveTimeout` (rename from lowercase). `BuildEnv` already exported. |
| `internal/pkg/scaffold/manifest.go` | Modify | Add `OriginGenerator Origin = "generator"` constant. |
| `internal/pkg/scaffold/upgrade.go` | Modify | `origin: generator` → `ClassProtected` with explicit warning; pure `template` entries still upgradable via `renderPackEntry`. |
| `internal/pkg/scaffold/scaffold.go` | Modify | Add `GeneratePackGenerator(name string, args map[string]any) error` — does pre-flight prompt resolution, pre-flight sandbox, calls `generators.Run`, receives `[]Record`, records manifest. |
| `internal/pkg/template/engine.go` | Modify | Add `RenderPackOnly(wr io.Writer, packDir, from string, data interface{}) error` — reads `<packDir>/templates/<from>` directly; returns `generator_template_not_found` on miss. NO chain fallback. |
| `internal/pkg/template/engine_test.go` | Modify | Test `RenderPackOnly`: existing template succeeds; missing template returns correct error code. |
| `cmd/generate.go` | Modify | Replace `cobra.ExactArgs(2)` with custom validator (allow 0 args when `--list`, else 2). Three-tier resolve before `GenerateComponent`; `--list` output grouped by source; pack resolution via `.go-arch.yaml` `template:`. |
| `cmd/generate_test.go` | Create | Dispatch tests with fake resolver. Arg validation tests (--list vs exact 2). |
| `internal/pkg/mcp/server.go` | Modify | Relax `generate_component.type` enum → string; add `generatorArgs` param; add `list_generators` tool. |
| `docs/packs.md` | Modify | v2 contract + trust section. |

## Interfaces / Contracts

```go
// generators/recipe.go
type Generator struct {
    Description string        `yaml:"description,omitempty"`
    Steps       []Step        `yaml:"steps"`
    Pre         []hooks.Entry `yaml:"pre,omitempty"`
    Post        []hooks.Entry `yaml:"post,omitempty"`
}

type Step struct {
    Type          string            // "template"|"binary"|"run"|"prompt"|"use"
    From, To      string
    Mode          os.FileMode       // binary only (default 0644)
    Command       string            // run (object form)
    Shell         string            // run (string form — shell command line)
    Args          []string          // run (object form)
    Cwd           string            // run
    Env           map[string]string // run
    Timeout       time.Duration     // run
    Silent        bool              // run
    IgnoreFailure bool              // run
    Name          string            // prompt
    Message       string            // prompt
    Default       string            // prompt
    Required      bool              // prompt
    Value         string            // use: "builtin/<name>"
    Index         int               // source position for errors
}

// Record is what generators.Run returns per file written.
// scaffold consumes these to update the manifest.
type Record struct {
    Path         string            // relative to project root
    Origin       string            // "generator" or "template"
    Source       string            // "pack:<name>@<version>"
    TemplatePath string            // for template origin: pack-relative path
    Metadata     map[string]string // includes "generator" and "args" (JSON)
}

// generators/executor.go
type PromptResolver interface {
    Resolve(name, message, def string, required bool) (string, error)
}

type RunOptions struct {
    ProjectRoot    string
    PackDir        string
    PackName       string
    PackVersion    string
    GeneratorName  string
    HooksEnabled   bool
    CmdRunner      hooks.CommandRunner
    PromptResolver PromptResolver
    ResolvedArgs   map[string]any // pre-flight resolved prompt values
    Out            io.Writer
}

func Run(ctx context.Context, g Generator, opts RunOptions) ([]Record, error)

// generators/runner.go
type Runner struct { /* cmd hooks.CommandRunner; out io.Writer */ }

func NewRunner(cmd hooks.CommandRunner, out io.Writer) *Runner
func (r *Runner) FireEntries(entries []hooks.Entry, ctx hooks.EnvContext, cwd string) error

// generators/sandbox.go
func ValidateTarget(projectRoot, relPath string) error
// Separator-aware: filepath.Clean(projectRoot) + string(os.PathSeparator) prefix boundary

// template/engine.go (new method)
func (e *Engine) RenderPackOnly(wr io.Writer, packDir, from string, data interface{}) error
// Reads <packDir>/templates/<from> directly. Returns generator_template_not_found on miss.
// NO chain fallback — this is the critical difference from RenderTo.
```

## Testing Strategy

| Layer | What | Approach |
|---|---|---|
| Unit | Recipe parse, validation, sandbox rules (incl. sibling false-positive), dispatch order, `Record` output, `FireEntries` | Table-driven tests; fake `CommandRunner`, fake `PromptResolver`, `t.TempDir` for sandbox. |
| Unit | `RenderPackOnly` — existing template, missing template error | `t.TempDir` with templates subdir. |
| Unit | `--list` arg validation (0 args with --list, 2 args without) | Cobra test with `Execute()`. |
| Integration | Recipe E2E with fixture pack in `t.TempDir` | Build a fake pack dir with `templates/` + `go-arch.yaml`; invoke `scaffold.GeneratePackGenerator`; assert files written + manifest entries. |
| Integration | Path escape → zero writes | 3-step recipe with step 3 escaping; verify step 1's target NOT on disk. |
| Integration | Pre-flight prompt abort | Required prompt + no args + piped stdin → `generator_prompt_unresolvable`; zero files written. |
| Integration | Trust gate | `HooksEnabled: false` → `run:` skipped + warning; `template:` still writes. |
| Integration | `RenderPackOnly` no fallback | Pack template missing → `generator_template_not_found`; verify embedded template NOT used. |
| Integration | Install trust for run:-only pack | Pack with generators declaring `run:` but no pack-level hooks → trust prompt fires. |
| E2E | `go-arch generate docker myservice` via cobra test | Real pack installed to `t.TempDir` via `GO_ARCH_PACKS_DIR`; assert final manifest. |

## Threat Matrix

The subprocess boundary is **not new** — `hooks.Runner` already executes user-supplied commands. Generators REUSE that boundary via `generators.Runner.FireEntries` (same helpers, same `HooksEnabled` gate). No new routing, git, PR, or VCS integration is added.

| Boundary | Applicability | Design response |
|---|---|---|
| Shell subprocess | Applicable | `generators.Runner.FireEntries` reuses `hooks.ResolveCommand`/`BuildEnv`/`ResolveTimeout`. Gated by sidecar. |
| Path traversal | Applicable | Pre-flight `ValidateTarget` with separator-aware prefix check. |
| Git/VCS/PR/commit | N/A | No git integration added. |
| Executable-file classification | N/A | `binary:` step copies files, does not `chmod +x` unless pack opts in (mode field). |

RED tests: absolute path, `..` escape, symlink escape, sibling false-positive, `run:` with `HooksEnabled: false`, unknown builtin, `RenderPackOnly` missing template, pre-flight prompt abort.

## Migration / Rollout

No migration. Feature-gate: `packs.SupportedContractVersions = []int{1, 2}` — a one-line disable if needed. v1 packs continue to work unchanged. v2 packs require current CLI. Document in `docs/packs.md`.

## Key Decisions (refinements flagged)

- **🚩 Single-entry-with-metadata for template steps**: `Manifest.Files` is a `map[string]ManifestEntry` keyed by path — cannot store two entries for the same path. **Resolution**: store the `origin: template` entry (upgradable) under the path key, and record generator provenance in `metadata.generator` and `metadata.args` on that same entry. The entry is upgradable via `renderPackEntry` AND carries generator audit info. See "Refinements vs Spec" below.
- **🚩 `use:` builtin args passing**: pass the full resolved `Args` map as the builtin's context; builtins are Go functions receiving `(ctx, args)`.
- **🚩 `binary:` step chmod**: default `0644`; packs opt into executable via explicit `mode: 0755`. No auto-detection.

## Refinements vs Spec

The single-entry-with-metadata refinement affects the following spec scenarios:

| Spec scenario | Literal reading | Actual behavior | How requirement is satisfied |
|---|---|---|---|
| :410-416 "Template step produces dual entries" | Two manifest entries for same path | Single entry: `origin: template`, `metadata.generator: "docker"`, `metadata.args: <json>` | The underlying requirement is **dual provenance** (template for upgrade + generator for audit). Single entry with metadata satisfies both: upgrade uses `origin: template` + `template` field; audit reads `metadata.generator` + `metadata.args`. |
| :375-379 "Generator file records origin and source" | `origin: "generator"` on all files | Template-step files carry `origin: template`; non-template files carry `origin: generator` | Audit provenance is preserved via `metadata.generator` + `metadata.args` on ALL entries regardless of origin. The `source: "pack:<name>@<ver>"` is present on all entries. |
| :418-423 "Template step re-renderable on upgrade" | Two entries — one upgradable, one PROTECTED | Single `origin: template` entry is upgradable; no separate PROTECTED entry exists | Re-render still works via `renderPackEntry`. PROTECTED classification is not needed for template-origin files because they ARE re-renderable. Generator-only files (binary, run output) don't have `template` field → classified PROTECTED by absence of template path. |

The underlying requirements (upgrade re-render, audit trail, protected generator output) are all satisfied. The literal "dual entry" and "origin: generator on template files" scenarios do not hold as written, but the behaviors they describe are correctly implemented.

## Open Questions

- None blocking. All proposal open questions resolved.

## Key Risks

| Risk | Mitigation |
|---|---|
| YAML union parsing edge cases | Table-driven tests with malformed input; strict unknown-field rejection. |
| Sandbox race (TOCTOU between validate and write) | Acceptable for v2: recipe is pack-authored, trust is opt-in via sidecar. Document. |
| Effort budget (largest change since plugins) | Chained PRs: (1) contract v2 + types, (2) sandbox + executor, (3) CLI dispatch + MCP, (4) provenance + upgrade, (5) docs. |
| Engine stdout in MCP | Route all output through `ui.Out`; executor takes `io.Writer`. |
| Windows separator prefix check | `filepath.Clean` both paths; compare `filepath.VolumeName` first; then check `cleaned + string(os.PathSeparator)` prefix. Test on CI with Windows runner. |
