# Design: Generate Templ Views

**Status**: draft | **next_recommended**: tasks

## Technical Approach

Extend the existing `GenerateComponent` dispatch in `internal/pkg/scaffold/scaffold.go` with two new cases (`page`, `component`) mapping to architecture-agnostic paths under `views/`. Add two new embedded templates (`web/page_generated.tmpl`, `web/component_generated.tmpl`) parametrized on the existing `{ui.ProjectConfig; EntityName}` data struct. Gate on `use_templ_htmx` inside the switch cases so both the CLI (`cmd/generate.go`) and MCP (`internal/pkg/mcp/server.go`) entry points inherit the check for free. Validate names with `go/token.IsIdentifier` (stdlib, already rejects keywords), and add a scoped `os.Stat` collision check before `createFile`.

## Architecture Decisions

| Decision | Option A | Option B | Decision |
|----------|----------|----------|----------|
| Filename convention | `strings.ToLower(name)` → `dashboard.templ` | snake_case → `user_card.templ` | `ToLower` — matches scaffold output (`home.templ`, `counter.templ`) and spec contract |
| Templ function name casing | `{{ title .EntityName }}` → `templ Dashboard()` | Raw `{{ .EntityName }}` | `title` — accepts lowercase input (`dashboard` → `Dashboard`) while still producing a valid exported templ func |
| Gate placement | Inside `GenerateComponent` page/component cases | In cmd + mcp before dispatch | Inside scaffold — single source of truth; both entry points inherit |
| Validation helper location | Helper in `scaffold` package | Helper in `cmd` package | `scaffold` — colocated with gate + collision; MCP path inherits |
| Collision check scope | In `page`/`component` cases only | In `createFile` (all types) | Cases only — preserves backend overwrite semantics |
| Success hint location | `cmd/generate.go` after `ui.Success`, conditional on type | `scaffold.go` inside `GenerateComponent` | `cmd` — hint is UI; MCP has its own success message |
| Page template body | `templ {Title}()` wrapping `@layouts.Base(0)` | Custom HTML | `@layouts.Base(0)` — matches scaffolded `home.templ` convention |
| Component template body | Self-contained div with `hx-get`, `hx-target`, `hx-swap` | Same as counter (hx-post) | `hx-get="/{lower}" hx-target="#{lower}" hx-swap="innerHTML"` — generic placeholder |

## Data Flow

```
generate page Dashboard
      │
      ▼
cmd/generate.go
  ├─ reads use_templ_htmx via viper ─► config.UseTemplHTMX
  └─ dispatches to GenerateComponent("page", "Dashboard")
            │
            ▼
   scaffold.GenerateComponent
      ├─ case "page":
      │    ├─ gate: if !UseTemplHTMX → oops("web_scaffold_required")
      │    ├─ validate: token.IsIdentifier("Dashboard") → ok
      │    ├─ collision: os.Stat("views/pages/dashboard.templ") → absent
      │    └─ createFile("views/pages/dashboard.templ", "web/page_generated.tmpl", data)
      └─ cmd prints ui.Success + templHint("page")
```

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/pkg/scaffold/scaffold.go` | Modify | Add `page`/`component` cases at line 243 with gate + validate + collision + createFile; add `isValidGoIdentifier` helper using `go/token` |
| `cmd/generate.go` | Modify | Add `UseTemplHTMX: viper.GetBool("use_templ_htmx")` to config mapping (lines 37-43); reword `Use:`/`Short:`/`Long:`; add `templHint(type)` helper returning the post-success hint; call it after `ui.Success` when type is `page` or `component` |
| `internal/pkg/mcp/server.go` | Modify | Extend enum at line 168 with `page`/`component`; update description at line 162; add `UseTemplHTMX` to config mapping at lines 292-298 |
| `internal/pkg/template/templates/web/page_generated.tmpl` | Create | Parametrized page template |
| `internal/pkg/template/templates/web/component_generated.tmpl` | Create | Parametrized component template |
| `internal/pkg/scaffold/scaffold_test.go` | Modify | Tests: generation + content, gate, collision (incl. scaffold-shipped `home.templ` protection), invalid name, backend unaffected, success hint |

## Interfaces / Contracts

### `web/page_generated.tmpl`

```
package pages

import "{{ .ModuleName }}/views/layouts"

templ {{ title .EntityName }}() {
	@layouts.Base(0)
}
```

### `web/component_generated.tmpl`

```
package components

templ {{ title .EntityName }}() {
	<div id="{{ lower .EntityName }}" hx-get="/{{ lower .EntityName }}" hx-target="#{{ lower .EntityName }}" hx-swap="innerHTML">
		<p>{{ title .EntityName }} component</p>
	</div>
}
```

**Identifier casing contract**: the **function name** uses `{{ title .EntityName }}` so a lowercase input like `dashboard` produces `templ Dashboard()` (a valid exported templ symbol). The **filename** uses `strings.ToLower(name)` (`dashboard.templ`). Both inputs `Dashboard` and `dashboard` therefore yield the same templ function and file.

### New helper in `scaffold.go`

```go
import "go/token"

func isValidGoIdentifier(name string) bool {
    return token.IsIdentifier(name) // stdlib: rejects keywords, empty, hyphens, leading digits
}
```

### New helper in `cmd/generate.go`

```go
// templHint returns the post-success hint printed after page/component generation.
// Exported at package level (unexported function) so it can be unit-tested without
// executing the full cobra command.
func templHint(genType string) string {
    return fmt.Sprintf("💡 Run `templ generate` to compile the new %s.", genType)
}
```

### `switch` case shape in `GenerateComponent`

```go
case "page":
    if !s.config.UseTemplHTMX {
        return oops.Code("web_scaffold_required").
            Hint("Set `use_templ_htmx: true` in .go-arch.yaml or re-run `go-arch new` with the flag").
            Errorf("page generation requires the web scaffold")
    }
    if !isValidGoIdentifier(name) {
        return oops.Code("invalid_component_name").
            Hint("Name must be a valid Go identifier (e.g. UserCard, Dashboard)").
            Errorf("invalid component name: %s", name)
    }
    targetPath = filepath.Join("views/pages", strings.ToLower(name)+".templ")
    templatePath = "web/page_generated.tmpl"
    if _, err := os.Stat(filepath.Join(s.config.ProjectName, targetPath)); err == nil {
        return oops.Code("component_already_exists").
            Hint("Choose a different name or delete the existing file").
            Errorf("target file already exists: %s", targetPath)
    }
case "component":
    // Same three-guard pattern, targetPath = "views/components/" + strings.ToLower(name) + ".templ"
```

### Help-text reword (`cmd/generate.go`)

```go
Use:   "generate [type] [name]",
Short: "Generate a new component (service, repository, handler, crud, page, component)",
Long: `Generate components for the project.

Backend types: service, repository, handler, crud.
Web types (require use_templ_htmx: true in .go-arch.yaml):
  page      → views/pages/<lowercase_name>.templ
  component → views/components/<lowercase_name>.templ (a templ component)`,
```

### Error codes

| Code | Trigger | Hint |
|------|---------|------|
| `web_scaffold_required` | `UseTemplHTMX == false` for page/component | "Set `use_templ_htmx: true` in .go-arch.yaml" |
| `invalid_component_name` | `!isValidGoIdentifier(name)` | "Name must be a valid Go identifier (e.g. UserCard)" |
| `component_already_exists` | `os.Stat` finds target file | "Choose a different name or delete the existing file" |

## Testing Strategy

| Layer | What to Test | Approach |
|-------|-------------|----------|
| Unit | `isValidGoIdentifier` — true for `Dashboard`/`UserCard`/`dashboard`; false for `user-card`/`123Name`/empty/`if` | Table-driven in `scaffold_test.go` |
| Unit | Page generation — file at `views/pages/dashboard.templ`; content contains `package pages`, `templ Dashboard()`, `@layouts.Base(0)` | tempdir + chdir pattern from `TestScaffolder_CRUD`; exercise both `Dashboard` and `dashboard` inputs |
| Unit | Component generation — file at `views/components/usercard.templ`; contains `package components`, `templ Usercard()`, `hx-get=` | Same pattern |
| Unit | Gate rejection — `UseTemplHTMX: false` returns error containing `web_scaffold_required`, no file written | Assert error message substring |
| Unit | Collision rejection (generic) — pre-create target file, call GenerateComponent, assert `component_already_exists`, file unchanged | `os.WriteFile` before call |
| Unit | Collision rejection (scaffold-shipped `home.templ` protected) — scaffold a web project (use existing `TestScaffolder_Web`-style setup or run `Scaffolder.New(...)` with `UseTemplHTMX: true`), then call `GenerateComponent("page", "Home")`; assert `component_already_exists` and the original `views/pages/home.templ` byte-identical | Explicit test case covering the "Scaffold-shipped home.templ protected" spec scenario |
| Unit | Invalid name rejection — `user-card` returns `invalid_component_name`, no file written | Assert error message |
| Unit | Backend unchanged — `service`/`repository`/`handler` with `UseTemplHTMX: false` succeed (no gate, no collision check) | Regression |
| Unit | `templHint` returns expected string — `templHint("page")` contains `templ generate`; `templHint("component")` contains `templ generate`; unknown type returns empty or is not called | Direct call to the unexported helper from a `_test.go` in package `cmd` (same-package tests see unexported symbols) |
| Unit | Success hint is printed after page/component generation — use cobra's `ExecuteC` with `cmd.SetOut(buf)` / `cmd.SetErr(buf)` on a freshly-built `generateCmd` whose `ui.Success` writes to the same buffer; assert `buf.String()` contains `templ generate` after a successful `page`/`component` run. If `ui.Success` writes to stdout directly without a pluggable writer, refactor `templHint` into a pure helper (shown above) and test it directly, asserting the integration via the cmd-level test only at the smoke level | Cobra-buffer pattern in `cmd/generate_test.go` |
| Unit | Help lists all six types — build the root command, capture `--help` output via `rootCmd.SetOut(buf); rootCmd.SetArgs([]string{"generate", "--help"}); rootCmd.Execute()`, assert `buf.String()` contains each of `service`, `repository`, `handler`, `crud`, `page`, `component` | Cobra-buffer pattern in `cmd/generate_test.go` (same file as hint test) |
| Integration | MCP dispatch exercise — in `internal/pkg/mcp/server_test.go`, call the registered `generate_component` handler with args `{type: "page", name: "Dashboard"}` and `{type: "component", name: "UserCard"}` against a tempdir web project (UseTemplHTMX: true), assert the tool returns success and the target file exists with the expected content. Do NOT just assert enum membership — actually exercise the handler (use the same `handleToolCall` / tool-dispatch helper existing MCP tests use, or construct a `mcp.ToolCall` and invoke the registered handler) | Extends existing MCP tests; asserts real dispatch, not enum contents |
| Build | No `templ` binary needed for any of the above (file-existence + content checks only) | Existing pattern from scaffold_test.go:556-558 |

## Threat Matrix

N/A — no routing, shell, subprocess, VCS/PR automation, executable-file classification, or process-integration boundary. This change is pure in-process file generation.

## Migration / Rollout

No migration required. All changes are additive:
- Existing backend generation (service/repository/handler/crud) behavior is unchanged — the gate, validation, and collision check fire only for `page`/`component` cases.
- Pre-existing `.go-arch.yaml` files lacking `use_templ_htmx` default to `false` via viper → gate rejects page/component for them (correct: no `views/` tree exists).
- No config schema change, no database, no feature flag.

## Open Questions

None — all spec handoff questions resolved. The `view` alias for `page` is explicitly out of scope per the proposal.
