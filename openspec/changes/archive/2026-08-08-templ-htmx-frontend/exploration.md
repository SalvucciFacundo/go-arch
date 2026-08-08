# Exploration: templ + HTMX frontend flag (`UseTemplHTMX`)

Status: success

## Executive Summary

The `UseTemplHTMX` flag is feasible and additive. All three architectures (Minimalist / Standard / Hexagonal) keep working unchanged; the frontend layer is fully conditional. The critical design problem is **confirmed**: Standard and Hexagonal write `cmd/api/main.go` *before* `createCommonFiles()` runs, so a web main written from `scaffoldWeb()` would silently overwrite the architecture main and lose telemetry/gRPC init. Resolution: guard the architecture-main writes with `if !s.config.UseTemplHTMX` and make a single architecture-agnostic `web/main.tmpl` a *superset* (HTTP mux + static serving + all existing telemetry/gRPC blocks), writing it to `cmd/api/main.go` for Standard/Hexagonal and to root `main.go` for Minimalist (avoids dual mains). Empirically confirmed pre-existing bug: fresh Hexagonal projects do NOT build today (`hexagonal/main.tmpl` imports empty `internal/adapters`/`internal/domain` packages) — the new web main must not inherit those imports.

## Verified Current-State Facts (file:line)

1. **cmd/api/main.go collision — CONFIRMED**.
   - `scaffoldStandard()` writes `cmd/api/main.go` from `standard/main.tmpl` at `scaffold.go:86` BEFORE calling `createCommonFiles()` at line 89.
   - `scaffoldHexagonal()` writes `cmd/api/main.go` from `hexagonal/main.tmpl` at `scaffold.go:105` BEFORE `createCommonFiles()` at line 108.
   - `scaffoldMinimalist()` writes root `main.go` from `minimalist/main.tmpl` at `scaffold.go:67` — it has NO `cmd/api/main.go`.
   - If `scaffoldWeb()` (inside `createCommonFiles`) also writes `cmd/api/main.go`, it overwrites the architecture main → telemetry init, gRPC goroutine, and `select{}` keep-alive are lost. Real risk, matches briefing constraint #1.
2. **`createCommonFiles()` hook point** (`scaffold.go:111-159`): writes `go.mod`, `.go-arch.yaml`, `.env`, then optional blocks `if s.config.UseDocker` (126), `if s.config.UseObservability` (136), `if s.config.UseGRPC` (146). `scaffoldWeb()` belongs here as a fourth optional block, same pattern.
3. **`createFile(path, templatePath, data)`** (`scaffold.go:44-63`): `filepath.Join(ProjectName, path)` → `MkdirAll` → `os.Create` → `engine.Render`. Defaults `data` to `s.config` when nil (line 58-60). New web templates can reuse it unchanged; only htmx.min.js needs a new binary-copy helper.
4. **Template engine embedding** (`engine.go:17-19`): `//go:embed all:templates/*` on exported `TemplatesFS` — any new `templates/web/*.tmpl` AND a binary `templates/web/htmx.min.js` are embedded automatically, no directive change needed. Resolution chain local `.go-arch/templates/` → global `~/.go-arch/templates/` → embedded (`engine.go:44-66`). Embedded paths inside `TemplatesFS` use the `templates/...` prefix (`engine.go:63`).
5. **htmx.min.js MUST be copied binary**. `engine.Render` uses `text/template` (`engine.go:31-41`) which chokes on `{{`/`}}` in htmx source. Correct path: `TemplatesFS.ReadFile("templates/web/htmx.min.js")` + `os.WriteFile(target, data, 0644)` — bypasses engine entirely and deliberately bypasses the local/global override chain (asset, not user template). Confirmed, matches briefing.
6. **go.mod.tmpl shape** (`common/go.mod.tmpl`, 21 lines): flat `require` blocks, one per optional feature (`{{ if .UseObservability }}` / `{{ if .UseGRPC }}`), pinned exact versions. Templ gets its own `{{ if .UseTemplHTMX }} require ( github.com/a-h/templ v0.3.906 ) {{ end }}` block — templ latest is v0.3.906 (Context7, high source reputation; re-verify at design time).
7. **Wizard flow** (`internal/ui/prompts.go`): `ProjectConfig` struct at lines 9-18 (flags with `mapstructure` tags). `RunWizard()` at 20-98: `survey.Ask(mainQs, config)` then conditional ObservabilityBackend question. Add `UseTemplHTMX bool \`mapstructure:"use_templ_htmx"\`` to the struct + a `survey.Confirm` ("Include templ + HTMX frontend?", default false) after UseGRPC (line 71-77). Flow: `cmd/new.go:24` → `RunWizard()` → `scaffold.NewScaffolder(config)` (line 33) → `Execute()` → architecture func → `createCommonFiles()`. No CLI flags bypass the wizard.
8. **`.go-arch.yaml` round-trip** (`common/config.tmpl:1-12`): serializes every flag; add `use_templ_htmx: {{ .UseTemplHTMX }}` for fidelity. Pre-existing gap (not a regression): `cmd/generate.go:37-43` maps only a subset of fields back into `ProjectConfig`.
9. **Test pattern** (`scaffold_test.go:10-82`): table-driven `TestScaffolder_Layouts` with `expectFiles []string`, `os.MkdirTemp` + chdir, `os.Stat` existence checks. New cases follow this exactly.
10. **Architecture mains today** (`standard/main.tmpl` 42 lines, `hexagonal/main.tmpl` 44 lines): near-identical — fmt.Println header, optional telemetry init, optional gRPC goroutine, `select{}`. Difference: imports (Standard: `internal/handler`; Hexagonal: `internal/adapters` + `internal/domain`).
11. **PRE-EXISTING BUG (empirically confirmed)**: fresh Hexagonal projects do not build. `hexagonal/main.tmpl:7-8` imports `internal/adapters` and `internal/domain`, but a fresh scaffold creates those as EMPTY dirs → `go build` fails with "no required module provides package .../internal/adapters". Reproduced in /tmp. The new `web/main.tmpl` must NOT copy unused imports. (Out of scope to fix here, but hexagonal+web acceptance "generated project builds" depends on the web main being clean.)

## Recommended Approach — cmd/api/main.go collision

**Option A (recommended): guard architecture mains + single web-main superset.**

1. In `scaffoldMinimalist()`: `if !s.config.UseTemplHTMX { createFile("main.go", "minimalist/main.tmpl", nil) }`.
2. In `scaffoldStandard()` / `scaffoldHexagonal()`: `if !s.config.UseTemplHTMX { createFile("cmd/api/main.go", "{arch}/main.tmpl", nil) }`.
3. In `createCommonFiles()`, append:
   ```go
   if s.config.UseTemplHTMX {
       if err := s.scaffoldWeb(); err != nil { return err }
   }
   ```
4. `scaffoldWeb()`:
   - Creates dirs: `views/layouts`, `views/pages`, `views/components`, `static/css`, `static/js`.
   - Renders `web/*.tmpl` → targets below (reuse `createFile`).
   - Writes web main via `createFile` to `cmd/api/main.go` (Standard/Hexagonal) or `main.go` (Minimalist) — chosen in Go code by `s.config.Architecture`; the template is architecture-agnostic.
   - Copies htmx.min.js via new helper `createBinaryFile(path, embeddedPath)`:
     ```go
     data, err := template.TemplatesFS.ReadFile("templates/web/htmx.min.js")
     ...
     os.WriteFile(filepath.Join(s.config.ProjectName, path), data, 0644)
     ```
5. `web/main.tmpl` = superset: import `internal/handler` + conditional `internal/telemetry` + conditional `internal/adapters/grpc`, plus `net/http`; body = telemetry init + gRPC goroutine + mux (`http.NewServeMux`) + page handler route + `http.StripPrefix("/static/", http.FileServer(http.Dir("static")))` + `http.ListenAndServe(":8080", mux)`. **Do NOT import `internal/adapters`/`internal/domain`** (avoid the hex bug; the web demo needs neither).

**Option B (rejected): merge `{{ if .UseTemplHTMX }}` HTTP blocks into each architecture main.tmpl.** Pros: no guard/overwrite logic. Cons: HTTP server code duplicated across 3 templates, web main no longer single-source, hexagonal template still carries its broken imports, and the collision logic moves into templates (harder to test). Rejected.

## Recommended Template File List — `templates/web/`

| Template (embedded) | Generated target | Notes |
|---|---|---|
| `web/base.templ.tmpl` | `views/layouts/base.templ` | HTML shell: `<link>` to `/static/css/style.css`, `<script src="/static/js/htmx.min.js">`, `@content templ.Component` param |
| `web/page.templ.tmpl` | `views/pages/home.templ` | Composes `@layouts.Base(...)` |
| `web/component.templ.tmpl` | `views/components/counter.templ` | Counter with `hx-post`/`hx-target`/`hx-swap` |
| `web/style.css.tmpl` | `static/css/style.css` | Base dark styles |
| `web/handler.tmpl` | `internal/handler/page.go` | Renders templ page; package `handler` (name-safe for all archs — additive dir for Hexagonal) |
| `web/main.tmpl` | `cmd/api/main.go` or `main.go` | Superset main (see above) |
| `web/htmx.min.js` (plain binary, NOT `.tmpl`) | `static/js/htmx.min.js` | v1.9.12 from unpkg, binary-copied |

Counter component decision: keep the demo FUNCTIONAL — the whole point of HTMX is showing `hx-post`/`hx-target`/`hx-swap`. Recommendation: fold the counter endpoint into `page.go` (GET `/` renders page, POST `/counter` re-renders the counter fragment with incremented value; in-memory int state in the handler, `sync.Mutex` for safety). A dead button in the flagship demo undercuts acceptance criterion 3. Design phase picks exact routing/state.

## Edge Cases and Risks

1. **Standard/Hexagonal main overwrite** — resolved by guard + superset main (Option A). If both writes run, telemetry/gRPC init silently lost. Must be covered by test asserting `cmd/api/main.go` content contains `http.ListenAndServe` when web on.
2. **Minimalist dual-main** — root `main.go` + `cmd/api/main.go` both `package main` DO compile (`go build ./...` allows multiple mains), but `go-arch serve` runs root `main.go` (`serve.go:31-34`), which would serve the non-HTTP main. Fix: web main REPLACES root main.go for Minimalist.
3. **`templ generate` post-step required** — generated `.templ` files don't compile until `templ generate` runs. Generated project's `go build ./...` fails without it. Must be documented: extend the success message in `cmd/new.go:42` (conditional `ui.Info("Next: templ generate && go run ./cmd/api")`) and/or add a `web/readme.tmpl`. No generated README exists today — design phase decides where the note lives.
4. **htmx license** — htmx v1.9.12 is BSD-2-Clause; vendored into the go-arch repo (MIT) and every generated project. MUST keep an attribution (generated README or THIRD_PARTY_NOTICES). Include in acceptance criteria.
5. **Hexagonal pre-existing build break** — fresh hexagonal projects don't build today (verified). Not caused by this change, but `web/main.tmpl` must not import empty packages; hexagonal+web acceptance depends on it.
6. **Engine override chain** — `web/*.tmpl` participate in local/global override (good, consistent with existing templates); htmx.min.js binary intentionally does not.
7. **go.mod require correctness** — pin `github.com/a-h/templ v0.3.906` (verify latest at design time). `go mod tidy` in the generated project is the safety net; acceptance runs `templ generate && go build ./...`.
8. **Review budget** — authored lines ≈ 250-280 (6 templates ~150, scaffold.go ~45, prompts.go ~8, go.mod.tmpl ~4, config.tmpl ~1, tests ~35). htmx.min.js (~48KB) is a vendored binary asset, excluded from authored count. Within the 400-line guard.

## Ready for Proposal

Yes. Next phase: `propose`. Orchestrator should tell the user: the collision is confirmed and the resolution (guarded arch mains + superset web main) is specific; the hex build bug is pre-existing and the web main design avoids it; templ version to pin is v0.3.906 (re-verify); htmx 1.9.12 binary copy path verified against `TemplatesFS` embedding.
