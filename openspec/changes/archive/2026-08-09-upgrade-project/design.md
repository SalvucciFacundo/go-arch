# Design: upgrade-project

## Status

**Designed** — ready for tasks (sdd-tasks).

## Executive Summary

Add a generation-time fingerprint manifest (`.go-arch/manifest.yaml`), a `go-arch upgrade` cobra subcommand, and an `upgrade_project` MCP tool. The manifest records every scaffold write (sha256 + origin + template path + metadata). Upgrade re-renders each entry via the existing engine chain, classifies by three-way hash comparison (disk vs manifest vs re-render), and applies via compare-then-write. PROTECTED files (user-modified) are never overwritten. Legacy projects (no manifest) fall back to a static whitelist with per-file confirmation. `go_arch_version` is written surgically to `.go-arch.yaml` via line-level replacement to preserve byte-identity of all other keys.

## Context & Constraints

Verified from the codebase (file:line):

- `Scaffolder` struct holds `engine *template.Engine` + `config *ui.ProjectConfig` (scaffold.go:15-18). `config.ProjectName` is the directory name used by `new` ("TestApp"); `generate` passes `"."` via viper (generate.go:43). Upgrade runs from inside a project (CWD = project root), so **root is always `"."`** — never `cfg.ProjectName`.
- `createFile` does unconditional `os.Create` (truncate) + `engine.Render` (scaffold.go:48-67). `createBinaryFile` bypasses the engine (scaffold.go:73-83).
- `Execute()` early-returns per architecture (scaffold.go:36-45) — **no single exit point**. Manifest recording must happen inside `createFile`/`createBinaryFile`, not in a post-hoc `Execute` save.
- `engine.Render(wr, templatePath, data)` writes to any `io.Writer` (engine.go:31-42). `getTemplate` resolves local → global → embedded (engine.go:44-66). **Engine.go:38 prints `"Using custom template"` to STDOUT** — this corrupts JSON-RPC in MCP mode.
- `config.tmpl:13` has `generated_at: {{ now }}` — makes wholesale re-render non-idempotent. No `go_arch_version` field exists today.
- `gopkg.in/yaml.v3` (indirect dep) and `golang.org/x/term` (indirect) are available.
- MCP redirects UI to stderr (server.go:44); `generate_component` shows the chdir + viper.Reset pattern (server.go:293-344).
- Doctor/check show the cobra `RunE` + oops `missing_config` pattern (check.go:21-29, doctor.go:14-16).
- `mattn/go-isatty` is available as indirect dep; `golang.org/x/term` likewise. Either works for TTY detection.
- `cmd.Version` (version.go:6) is the canonical version string, set by GoReleaser.

## Architecture Decisions

### ADR-1: Manifest as ownership source of truth

| Option | Tradeoff | Decision |
|--------|----------|----------|
| Pure content diff (hardcoded whitelist) | Works today; cannot distinguish template change from user edit | ❌ Rejected |
| **Fingerprint manifest** (sha256 at write time) | Deterministic per-file ownership; requires scaffolder changes | ✅ **Chosen** |
| Version-gated diff only | Cheap gate; no user-edit detection | ❌ Insufficient alone |

**Rationale**: The manifest solves the central risk (clobbering user edits) at the root. Approach 2 from exploration.md. The manifest coexists with `.go-arch/templates/` and is additive — absence is tolerated (legacy fallback).

### ADR-2: Compare-then-write apply

| Option | Tradeoff | Decision |
|--------|----------|----------|
| Blind `os.Create` (current pattern) | Simple; clobbers unchanged files; breaks mtimes | ❌ Rejected |
| **Render → buffer → compare → conditional write** | Idempotent; preserves mtimes; slightly more code | ✅ **Chosen** |

**Rationale**: Spec mandates "write ONLY when bytes differ" and "idempotent on clean tree". Buffer comparison before write satisfies both.

### ADR-3: Engine chain reuse for re-render

**Choice**: Re-use `engine.Render` with the full local → global → embedded chain during upgrade. No new rendering path.

**Rationale**: Custom templates in `.go-arch/templates/` MUST be respected during upgrade (exploration risk #7). The engine already handles this. No new code needed — just call `Render` with a `bytes.Buffer`.

### ADR-4: Surgical `go_arch_version` write

| Option | Tradeoff | Decision |
|--------|----------|----------|
| YAML map round-trip (parse → set key → marshal) | Simple code; LOSES comments; reorders keys; changes formatting | ❌ **Rejected** — violates "other keys byte-identical" |
| **Line-level regex replace / append** | Preserves all bytes except the target line; trivial to test | ✅ **Chosen** |

**Rationale**: Spec scenario "Version field written surgically" requires "all other keys byte-identical". YAML map round-trip cannot guarantee this — `yaml.v3` sorts keys alphabetically and strips comments. Line-level replacement only touches the `go_arch_version:` line (or appends it).

**When written**: `cmd/new.go` calls `WriteVersionField` after `scaffolder.Execute()` succeeds (post-hoc). MCP `new_project` does the same. The template emits `go_arch_version:` (empty) as a placeholder; the post-hoc write fills it in. Absence is tolerated for existing projects.

### ADR-5: `go.mod` report-only

**Choice**: Never rewrite `go.mod`. Re-render the template, detect diff, print `go get` hints only.

**Rationale**: `go mod tidy` rewrites go.mod (indirect requires), so its fingerprint drifts immediately. Dep bumps are user decisions. Report-only avoids a class of false positives.

**go get hint mechanism**: For each dependency in the template's `go.mod` require block that differs from disk, print `go get <module>@latest`. If the diff is parseable (simple require-line comparison), list concrete hints. Otherwise, print a generic hint: `# go.mod has drifted — review and run 'go get <dep>@latest' as needed`. This is best-effort: the template go.mod is the ground truth for what the CLI expects.

### ADR-6: Non-TTY contract

| Mode | Behavior |
|------|----------|
| CLI, TTY, no `--yes` | Print plan. For legacy: per-file interactive prompts. For manifest: plan only (dry-run default). |
| CLI, TTY, `--yes` | Apply all upgradable without prompts. |
| CLI, non-TTY, no `--yes` | Print plan, exit 0, write nothing. |
| CLI, non-TTY, `--yes` | Apply all upgradable. |
| MCP (always non-TTY) | `apply: false` (default) → plan JSON. `apply: true` → commit changes. |

**Rationale**: MCP cannot prompt (UI on stderr, JSON on stdout). CI cannot prompt. `--yes` is the explicit opt-in for all non-interactive apply paths.

### ADR-7: Upgrade root resolution

**Choice**: `Upgrade()` resolves root as `"."` (CWD). Never uses `cfg.ProjectName` as root.

**Rationale**: Real projects store `project_name: <dir-name>` in `.go-arch.yaml` (config.tmpl:4). When running `upgrade` from inside the project, CWD is the project root. Using `cfg.ProjectName` as root would look for `<CWD>/<dir-name>/.go-arch/manifest.yaml` which does not exist → every real project falls back to legacy. The `ProjectName = "."` behavior only exists in test scaffolding (scaffold_test.go). The `--project-path` flag does `os.Chdir` BEFORE `Upgrade()` is called, so by the time `Upgrade()` runs, CWD is already the correct project root.

### ADR-8: `.go-arch.yaml` excluded from upgrade apply

**Choice**: `Upgrade()` skips `.go-arch.yaml` entirely — it is never proposed for upgrade, never applied, never re-rendered. The manifest may still record it for documentation, but the classifier excludes it from the plan.

**Rationale**: `config.tmpl:13` has `generated_at: {{ now }}` — re-rendering always produces different bytes → always classified as upgradable → `--yes` rewrites it → resets `generated_at` timestamp, churning the file and clobbering the surgical `go_arch_version` write. This breaks R3 (idempotent) and R4 (byte-identical keys). The `go_arch_version` field is managed surgically only (ADR-4).

### ADR-9: MCP engine stdout suppression

**Choice**: The MCP `upgrade_project` handler must redirect `engine.Render`'s stdout output to stderr (or discard it) during upgrade. Implement via a `RenderSilent` variant on the engine, OR temporarily swap the global `os.Stdout` to a `io.Discard` writer during the upgrade call in MCP context.

**Mechanism**: Add `Engine.RenderTo(wr io.Writer, templatePath string, data interface{}, quiet bool)` — when `quiet=true`, skip the `fmt.Printf` at engine.go:38. The upgrade path calls `RenderTo` with `quiet=true`. For CLI upgrade, use `RenderTo` with `quiet=false` (current behavior). For MCP upgrade, use `RenderTo` with `quiet=true`.

**Alternative (rejected)**: Temporarily swap `os.Stdout` — fragile, affects concurrent goroutines, and doesn't address the root cause (engine shouldn't print during programmatic use).

## Detailed Component Design

### 1. `internal/pkg/scaffold/manifest.go` (NEW)

**Responsibilities**: Manifest struct, Load/Save/Update, hashFile, atomic write.

```go
package scaffold

import (
    "crypto/sha256"
    "encoding/hex"
    "os"
    "path/filepath"

    "gopkg.in/yaml.v3"
)

// Origin classifies who wrote the file.
type Origin string

const (
    OriginScaffold   Origin = "scaffold"
    OriginComponent  Origin = "component"
    OriginCrud       Origin = "crud"
    OriginBinary     Origin = "binary"
)

// ManifestEntry records one generated file's fingerprint and provenance.
type ManifestEntry struct {
    Path         string            `yaml:"path"`
    SHA256       string            `yaml:"sha256"`
    Origin       Origin            `yaml:"origin"`
    TemplatePath string            `yaml:"template,omitempty"` // empty for binary
    Metadata     map[string]string `yaml:"metadata,omitempty"` // e.g. {"entity_name": "Order"}
}

// Manifest is the ownership source of truth for scaffold-generated files.
type Manifest struct {
    Version int                      `yaml:"version"`
    Files   map[string]ManifestEntry `yaml:"files"`
    dir     string                   // project root (not serialized)
}

// ManifestPath returns the canonical manifest path for a project root.
func ManifestPath(projectRoot string) string {
    return filepath.Join(projectRoot, ".go-arch", "manifest.yaml")
}

// LoadManifest reads the manifest from disk. Missing file → empty manifest (not error).
func LoadManifest(projectRoot string) (*Manifest, error) {
    p := ManifestPath(projectRoot)
    data, err := os.ReadFile(p)
    if os.IsNotExist(err) {
        return &Manifest{Version: 1, Files: make(map[string]ManifestEntry), dir: projectRoot}, nil
    }
    if err != nil {
        return nil, err
    }
    var m Manifest
    if err := yaml.Unmarshal(data, &m); err != nil {
        return nil, err
    }
    if m.Files == nil {
        m.Files = make(map[string]ManifestEntry)
    }
    m.dir = projectRoot
    return &m, nil
}

// Exists reports whether a manifest file exists on disk (distinct from empty).
func ManifestExists(projectRoot string) bool {
    _, err := os.Stat(ManifestPath(projectRoot))
    return err == nil
}

// Save writes the manifest atomically: temp file in .go-arch/ + rename.
func (m *Manifest) Save() error {
    dir := filepath.Join(m.dir, ".go-arch")
    if err := os.MkdirAll(dir, 0755); err != nil {
        return err
    }
    data, err := yaml.Marshal(m)
    if err != nil {
        return err
    }
    tmp, err := os.CreateTemp(dir, "manifest-*.tmp")
    if err != nil {
        return err
    }
    tmpName := tmp.Name()
    if _, err := tmp.Write(data); err != nil {
        _ = tmp.Close()
        _ = os.Remove(tmpName)
        return err
    }
    if err := tmp.Close(); err != nil {
        _ = os.Remove(tmpName)
        return err
    }
    return os.Rename(tmpName, ManifestPath(m.dir))
}

// Upsert inserts or replaces a manifest entry keyed by path.
func (m *Manifest) Upsert(entry ManifestEntry) {
    m.Files[entry.Path] = entry
}

// hashFile computes sha256 hex digest of a file's contents.
func hashFile(path string) (string, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return "", err
    }
    sum := sha256.Sum256(data)
    return hex.EncodeToString(sum[:]), nil
}
```

### 2. `internal/pkg/scaffold/scaffold.go` (MODIFY)

**Recording seam**: Add a `*Manifest` field, lazy-loaded. `createFile` and `createBinaryFile` record after successful write. **Manifest.Save() is NOT called in Execute** — instead, `recordManifest` saves after each successful write (see ADR-10 below).

```go
// Added to Scaffolder struct:
type Scaffolder struct {
    engine   *template.Engine
    config   *ui.ProjectConfig
    manifest *Manifest // lazy-loaded via ensureManifest
}

// ensureManifest opens the manifest once, cached for the Scaffolder's lifetime.
func (s *Scaffolder) ensureManifest() (*Manifest, error) {
    if s.manifest != nil {
        return s.manifest, nil
    }
    m, err := LoadManifest(".")
    if err != nil {
        return nil, err
    }
    s.manifest = m
    return m, nil
}

// recordManifest hashes the just-written file and upserts the entry.
// Called AFTER successful write in createFile / createBinaryFile.
// Manifest save failure is NON-FATAL: log to stderr and continue.
// The scaffold write already succeeded; the manifest is a secondary index.
func (s *Scaffolder) recordManifest(targetPath, templatePath string, origin Origin, metadata map[string]string) {
    m, err := s.ensureManifest()
    if err != nil {
        fmt.Fprintf(os.Stderr, "warning: manifest load failed: %v\n", err)
        return
    }
    fullPath := filepath.Join(".", targetPath)
    hash, err := hashFile(fullPath)
    if err != nil {
        fmt.Fprintf(os.Stderr, "warning: manifest hash failed for %s: %v\n", targetPath, err)
        return
    }
    m.Upsert(ManifestEntry{
        Path:         targetPath,
        SHA256:       hash,
        Origin:       origin,
        TemplatePath: templatePath,
        Metadata:     metadata,
    })
    if err := m.Save(); err != nil {
        fmt.Fprintf(os.Stderr, "warning: manifest save failed: %v\n", err)
    }
}
```

**Modified `createFile`**: After `engine.Render` succeeds, call `recordManifest(path, templatePath, OriginScaffold, nil)`. Binary origin when called for binary-like paths (not needed — binary has its own method).

**Modified `createBinaryFile`**: After `os.WriteFile` succeeds, call `recordManifest(targetPath, embeddedPath, OriginBinary, nil)`.

**Execute**: NO manifest save at the end. Each `createFile`/`createBinaryFile` call already saved the manifest atomically after its write.

**GenerateComponent**: After `createFile` returns nil, call `recordManifest(targetPath, templatePath, OriginComponent, {"entity_name": name})`. **Note**: `createFile` already recorded with `OriginScaffold` inside its own body. The subsequent `recordManifest` call with `OriginComponent` upserts the same path-keyed entry, overwriting the origin to the correct value. This is deliberate: `createFile` records first (scaffold origin), then the caller corrects the origin (component/crud). The manifest upsert (path-keyed) means the later write wins.

**GenerateCRUD**: Same pattern with `OriginCrud`. Metadata includes `entity_name`.

**go.mod special case**: `createFile("go.mod", ...)` records in manifest as usual (so we know it's scaffold-owned), but upgrade treats it as report-only (never overwrite).

### 3. `internal/pkg/scaffold/upgrade.go` (NEW)

**Responsibilities**: `Upgrade(cfg)` — load manifest, re-render each entry, classify, return plan. `Apply(plan)` — compare-then-write. Legacy fallback. Surgical version write.

```go
package scaffold

import (
    "bytes"
    "crypto/sha256"
    "encoding/hex"
    "fmt"
    "go-arch/internal/pkg/template"
    "go-arch/internal/ui"
    "os"
    "path/filepath"
    "regexp"
    "strings"
)

// Classification is the upgrade decision for a single file.
type Classification string

const (
    ClassUpgradable Classification = "upgradable"   // template changed, disk untouched
    ClassProtected  Classification = "protected"    // user modified — NEVER overwrite
    ClassAbsent     Classification = "absent"       // file missing on disk — report only
    ClassUpToDate   Classification = "up_to_date"   // no action needed (omitted from plan)
)

// FileAction is one entry in the upgrade plan.
type FileAction struct {
    Path           string         `json:"path"`
    Classification Classification `json:"classification"`
    Origin         Origin         `json:"origin"`
    ManifestHash   string         `json:"manifestHash,omitempty"`
    DiskHash       string         `json:"diskHash,omitempty"`
    RerenderHash   string         `json:"rerenderHash,omitempty"`
    RerenderBytes  []byte         `json:"-"` // held for apply; not serialized
}

// UpgradePlan is the full classified plan.
type UpgradePlan struct {
    Files         []FileAction `json:"files"`
    IsLegacy      bool         `json:"isLegacy"`
    GoArchVersion string       `json:"goArchVersion,omitempty"`
    TemplHint     bool         `json:"templHint"` // whether to print templ generate hint
    ProjectRoot   string       `json:"-"`
    AppliedCount  int          `json:"-"` // count of files actually written during Apply
}

// Upgrade loads the manifest (or falls back to legacy), re-renders each file,
// and classifies into upgradable / protected / absent.
//
// Root resolution: root is ALWAYS "." (CWD = project root). Upgrade runs from
// inside the project; --project-path does chdir before Upgrade is called.
// cfg.ProjectName is the project name metadata, NOT a directory path.
func Upgrade(cfg *ui.ProjectConfig) (*UpgradePlan, error) {
    root := "." // ADR-7: always CWD

    if !ManifestExists(root) {
        return upgradeLegacy(cfg, root)
    }

    m, err := LoadManifest(root)
    if err != nil {
        return nil, err
    }

    engine := template.NewEngine()
    plan := &UpgradePlan{ProjectRoot: root}

    for path, entry := range m.Files {
        // ADR-8: skip .go-arch.yaml entirely — it's managed surgically
        if path == ".go-arch.yaml" {
            continue
        }

        action := FileAction{
            Path:         path,
            Origin:       entry.Origin,
            ManifestHash: entry.SHA256,
        }

        fullPath := filepath.Join(root, path)
        diskBytes, err := os.ReadFile(fullPath)
        if err != nil {
            if os.IsNotExist(err) {
                action.Classification = ClassAbsent
                plan.Files = append(plan.Files, action)
                continue
            }
            return nil, err
        }
        diskHash := hashBytes(diskBytes)
        action.DiskHash = diskHash

        // go.mod is always report-only
        if path == "go.mod" {
            // Re-render to detect drift, but classify as special
            rerender, rerenderErr := renderEntry(engine, entry, cfg, root)
            if rerenderErr == nil {
                rerenderHash := hashBytes(rerender)
                action.RerenderHash = rerenderHash
                if diskHash != rerenderHash {
                    action.Classification = ClassUpgradable // but apply treats as report-only
                } else {
                    // up_to_date: OMIT from plan (ADR-4)
                    continue
                }
            } else {
                // up_to_date: OMIT from plan (ADR-4)
                continue
            }
            plan.Files = append(plan.Files, action)
            continue
        }

        // Disk vs manifest: user-modified?
        if diskHash != entry.SHA256 {
            action.Classification = ClassProtected
            plan.Files = append(plan.Files, action)
            continue
        }

        // Disk matches manifest — re-render to check for template changes
        rerender, err := renderEntry(engine, entry, cfg, root)
        if err != nil {
            // Template missing or render error: OMIT from plan (up_to_date, ADR-4)
            continue
        }
        rerenderHash := hashBytes(rerender)
        action.RerenderHash = rerenderHash

        if rerenderHash == diskHash {
            // up_to_date: OMIT from plan (ADR-4)
            continue
        }

        action.Classification = ClassUpgradable
        action.RerenderBytes = rerender
        // Track if templ hint needed
        if strings.HasPrefix(path, "views/") || path == "static/css/style.css" {
            plan.TemplHint = true
        }

        plan.Files = append(plan.Files, action)
    }

    return plan, nil
}

// renderEntry re-renders a manifest entry through the engine chain.
func renderEntry(engine *template.Engine, entry ManifestEntry, cfg *ui.ProjectConfig, root string) ([]byte, error) {
    if entry.Origin == OriginBinary {
        // Binary files are read from embedded FS directly (same as createBinaryFile)
        data, err := template.TemplatesFS.ReadFile("templates/" + entry.TemplatePath)
        return data, err
    }

    data := buildRenderData(cfg, entry)
    var buf bytes.Buffer
    if err := engine.RenderTo(&buf, entry.TemplatePath, data, true); err != nil {
        return nil, err
    }
    return buf.Bytes(), nil
}

// buildRenderData reconstructs the template data from config + manifest metadata.
func buildRenderData(cfg *ui.ProjectConfig, entry ManifestEntry) interface{} {
    if entityName, ok := entry.Metadata["entity_name"]; ok {
        return struct {
            ui.ProjectConfig
            EntityName string
        }{
            ProjectConfig: *cfg,
            EntityName:    entityName,
        }
    }
    return cfg
}

// Apply writes only files that differ (compare-then-write), refreshes manifest.
// go.mod is report-only — never written.
// Returns the number of files actually written.
func (p *UpgradePlan) Apply() (int, error) {
    m, err := LoadManifest(p.ProjectRoot)
    if err != nil {
        return 0, err
    }

    applied := 0
    for i := range p.Files {
        f := &p.Files[i]
        if f.Classification != ClassUpgradable {
            continue
        }
        if f.Path == "go.mod" {
            continue // report-only
        }

        fullPath := filepath.Join(p.ProjectRoot, f.Path)

        // Compare-then-write
        existing, err := os.ReadFile(fullPath)
        if err != nil {
            return applied, err
        }
        if bytes.Equal(existing, f.RerenderBytes) {
            continue // already up-to-date (race guard)
        }

        if err := os.WriteFile(fullPath, f.RerenderBytes, 0644); err != nil {
            return applied, err
        }

        // Refresh manifest entry
        newHash := hashBytes(f.RerenderBytes)
        entry := m.Files[f.Path]
        entry.SHA256 = newHash
        m.Files[f.Path] = entry
        applied++
    }

    p.AppliedCount = applied

    if err := m.Save(); err != nil {
        return applied, err
    }
    return applied, nil
}

// CountBy returns the number of files in a given classification.
func (p *UpgradePlan) CountBy(c Classification) int {
    n := 0
    for _, f := range p.Files {
        if f.Classification == c {
            n++
        }
    }
    return n
}

// --- Surgical version write ---

var versionLineRe = regexp.MustCompile(`(?m)^go_arch_version:.*$`)

// WriteVersionField surgically updates (or appends) the go_arch_version field
// in .go-arch.yaml. All other bytes are preserved byte-for-byte.
func WriteVersionField(configPath, version string) error {
    data, err := os.ReadFile(configPath)
    if err != nil {
        return err
    }
    content := string(data)
    newLine := "go_arch_version: " + version

    if versionLineRe.MatchString(content) {
        content = versionLineRe.ReplaceAllString(content, newLine)
    } else {
        if !strings.HasSuffix(content, "\n") {
            content += "\n"
        }
        content += newLine + "\n"
    }

    return os.WriteFile(configPath, []byte(content), 0644)
}

// --- Legacy fallback ---

// legacyWhitelist: scaffold-owned paths safe to propose for upgrade.
var legacyWhitelist = []string{
    "main.go",
    "cmd/api/main.go",
    ".env",
    "Dockerfile",
    "docker-compose.yaml",
    "Makefile",
    "api/proto/service.proto",
    "internal/telemetry/telemetry.go",
    "internal/telemetry/middleware.go",
    "internal/adapters/grpc/server.go",
    "static/css/style.css",
    "README.md",
    "views/layouts/base.templ",
    "views/pages/home.templ",
    "views/components/counter.templ",
    "static/js/htmx.min.js",
}

// ConfirmFunc is the callback for legacy per-file confirmation.
// Returns true if the user approves writing this file.
type ConfirmFunc func(path string) bool

// upgradeLegacy builds a plan for projects without a manifest, using the
// static whitelist. Each whitelisted file that exists and differs from a
// fresh render is proposed; caller decides how to confirm.
func upgradeLegacy(cfg *ui.ProjectConfig, root string) (*UpgradePlan, error) {
    engine := template.NewEngine()
    plan := &UpgradePlan{IsLegacy: true, ProjectRoot: root}

    for _, path := range legacyWhitelist {
        fullPath := filepath.Join(root, path)
        diskBytes, err := os.ReadFile(fullPath)
        if err != nil {
            if os.IsNotExist(err) {
                continue // skip absent files in legacy mode
            }
            return nil, err
        }

        // Determine template path from whitelist mapping
        tmpl := legacyTemplateFor(path, cfg)
        if tmpl == "" {
            continue
        }

        // Binary file: copy from embedded FS (no engine render)
        if path == "static/js/htmx.min.js" {
            embeddedData, err := template.TemplatesFS.ReadFile("templates/" + tmpl)
            if err != nil {
                continue
            }
            diskHash := hashBytes(diskBytes)
            embeddedHash := hashBytes(embeddedData)
            if diskHash == embeddedHash {
                continue // up-to-date
            }
            plan.Files = append(plan.Files, FileAction{
                Path:           path,
                Classification: ClassUpgradable,
                DiskHash:       diskHash,
                RerenderHash:   embeddedHash,
                RerenderBytes:  embeddedData,
            })
            continue
        }

        // Re-render via engine
        var buf bytes.Buffer
        if err := engine.RenderTo(&buf, tmpl, cfg, true); err != nil {
            continue // skip files whose templates fail
        }
        rerenderBytes := buf.Bytes()

        diskHash := hashBytes(diskBytes)
        rerenderHash := hashBytes(rerenderBytes)

        if diskHash == rerenderHash {
            continue // up-to-date, omit from plan
        }

        action := FileAction{
            Path:           path,
            Classification: ClassUpgradable, // in legacy, all diffs are proposals
            DiskHash:       diskHash,
            RerenderHash:   rerenderHash,
            RerenderBytes:  rerenderBytes,
        }
        plan.Files = append(plan.Files, action)

        if strings.HasPrefix(path, "views/") || path == "static/css/style.css" {
            plan.TemplHint = true
        }
    }

    // go.mod report-only
    goModPath := filepath.Join(root, "go.mod")
    if goModBytes, err := os.ReadFile(goModPath); err == nil {
        var buf bytes.Buffer
        if err := engine.RenderTo(&buf, "common/go.mod.tmpl", cfg, true); err == nil {
            if hashBytes(goModBytes) != hashBytes(buf.Bytes()) {
                plan.Files = append(plan.Files, FileAction{
                    Path:           "go.mod",
                    Classification: ClassUpgradable, // report-only in apply
                    RerenderBytes:  buf.Bytes(),
                })
            }
        }
    }

    return plan, nil
}

// legacyTemplateFor maps a whitelisted path to its embedded template.
// Respects UseTemplHTMX for main.go / cmd/api/main.go mapping.
func legacyTemplateFor(path string, cfg *ui.ProjectConfig) string {
    // Static mapping — covers the legacy whitelist only.
    switch path {
    case ".env":
        return "common/env.tmpl"
    case "Dockerfile":
        return "common/Dockerfile.tmpl"
    case "docker-compose.yaml":
        return "common/docker-compose.yaml.tmpl"
    case "Makefile":
        return "common/Makefile.tmpl"
    case "api/proto/service.proto":
        return "common/service.proto.tmpl"
    case "internal/telemetry/telemetry.go":
        return "common/telemetry.tmpl"
    case "internal/telemetry/middleware.go":
        return "common/telemetry_middleware.tmpl"
    case "internal/adapters/grpc/server.go":
        return "common/grpc_server.tmpl"
    case "README.md":
        return "web/readme.tmpl"
    case "static/css/style.css":
        return "web/style.css.tmpl"
    case "views/layouts/base.templ":
        return "web/base.templ.tmpl"
    case "views/pages/home.templ":
        return "web/page.templ.tmpl"
    case "views/components/counter.templ":
        return "web/component.templ.tmpl"
    case "static/js/htmx.min.js":
        return "web/htmx.min.js" // binary copy, not engine-rendered
    case "main.go":
        if cfg.UseTemplHTMX {
            return "web/main.tmpl"
        }
        if cfg.Architecture == "Minimalist" {
            return "minimalist/main.tmpl"
        }
        return "" // Standard/Hexagonal use cmd/api/main.go
    case "cmd/api/main.go":
        if cfg.UseTemplHTMX {
            return "web/main.tmpl"
        }
        switch cfg.Architecture {
        case "Standard":
            return "standard/main.tmpl"
        case "Hexagonal":
            return "hexagonal/main.tmpl"
        }
        return "" // Minimalist uses main.go
    default:
        return ""
    }
}

// hashBytes computes sha256 hex digest of in-memory bytes.
func hashBytes(data []byte) string {
    sum := sha256.Sum256(data)
    return hex.EncodeToString(sum[:])
}
```

### 4. `internal/pkg/template/engine.go` (MODIFY)

Add `RenderTo` variant that accepts a `quiet` flag to suppress the "Using custom template" print:

```go
// RenderTo is like Render but accepts a quiet flag to suppress the
// "Using custom template" print to stdout. Use quiet=true during upgrade
// to avoid corrupting JSON-RPC in MCP mode.
func (e *Engine) RenderTo(wr io.Writer, templatePath string, data interface{}, quiet bool) error {
    t, source, err := e.getTemplate(templatePath)
    if err != nil {
        return err
    }

    if !quiet && source != "embedded" {
        fmt.Printf("🎨 Using custom template (%s): %s\n", source, templatePath)
    }

    return t.Execute(wr, data)
}
```

**Backward compatibility**: The existing `Render` method remains unchanged. New callers use `RenderTo`. Existing scaffold paths continue to use `Render` (which prints as before).

### 5. `cmd/upgrade.go` (NEW)

**Responsibilities**: Cobra subcommand, flags, TTY detection, plan display, interactive confirm (legacy TTY), apply, surgical version write.

```go
package cmd

import (
    "fmt"
    "go-arch/internal/pkg/scaffold"
    "go-arch/internal/ui"
    "os"
    "path/filepath"

    "github.com/AlecAivazis/survey/v2"
    "github.com/samber/oops"
    "github.com/spf13/cobra"
    "github.com/spf13/viper"
    "golang.org/x/term"
)

func init() {
    RootCmd.AddCommand(upgradeCmd)
    upgradeCmd.Flags().Bool("dry-run", true, "Print plan only, do not apply changes (default)")
    upgradeCmd.Flags().Bool("yes", false, "Apply all upgradable files without prompting")
    upgradeCmd.Flags().String("project-path", "", "Override project root directory")
}

var upgradeCmd = &cobra.Command{
    Use:           "upgrade",
    Short:         "Propagate embedded template changes to this project via the fingerprint manifest",
    SilenceUsage:  true,
    SilenceErrors: true,
    RunE: func(cmd *cobra.Command, args []string) error {
        dryRunSet := cmd.Flags().Changed("dry-run")
        yesSet := cmd.Flags().Changed("yes")

        // Mutual exclusion: both explicitly supplied → usage error
        if dryRunSet && yesSet {
            return oops.Code("invalid_flags").
                Hint("Use --dry-run to preview, or --yes to apply. They are mutually exclusive.").
                Errorf("--dry-run and --yes are mutually exclusive")
        }

        projectPath, _ := cmd.Flags().GetString("project-path")
        if projectPath != "" {
            if err := os.Chdir(projectPath); err != nil {
                return oops.Code("invalid_project_path").
                    Hint("Check that the path exists and is a directory").
                    Errorf("cannot change to %s: %v", projectPath, err)
            }
        }

        // Validate config
        projectName := viper.GetString("project_name")
        if projectName == "" {
            return oops.Code("missing_config").
                Hint("Run 'go-arch setup' to initialize the project").
                Errorf("No valid configuration found. Are you in the root of a go-arch project?")
        }

        cfg := configFromViper(projectName)

        plan, err := scaffold.Upgrade(cfg)
        if err != nil {
            return oops.Code("upgrade_failed").Wrapf(err, "Upgrade classification failed")
        }

        // Display plan
        displayPlan(plan)

        yes, _ := cmd.Flags().GetBool("yes")
        isTTY := term.IsTerminal(int(os.Stdin.Fd()))

        // Decision: apply or plan-only
        if !yes {
            // Non-TTY without --yes: plan only (spec: exit 0)
            if !isTTY {
                return nil
            }
            // TTY without --yes: for legacy, interactive per-file; for manifest, plan only
            if plan.IsLegacy {
                return applyLegacyInteractive(plan, cfg)
            }
            return nil // manifest project, default dry-run
        }

        // --yes: apply all upgradable
        if plan.CountBy(scaffold.ClassUpgradable) == 0 {
            ui.Success("All files are up to date.")
            return nil
        }

        applied, err := plan.Apply()
        if err != nil {
            return oops.Code("upgrade_apply_failed").Wrapf(err, "Failed to apply upgrade")
        }

        // Surgical version write
        configPath := ".go-arch.yaml"
        if err := scaffold.WriteVersionField(configPath, Version); err != nil {
            // Non-fatal: warn but don't fail
            ui.Warning(fmt.Sprintf("Could not update go_arch_version: %v", err))
        }

        ui.Success(fmt.Sprintf("Applied %d update(s).", applied))

        if plan.TemplHint {
            fmt.Println("💡 Run `templ generate` to recompile updated views.")
        }

        return nil
    },
}

// configFromViper maps viper config to ProjectConfig (same pattern as generate.go).
func configFromViper(projectName string) *ui.ProjectConfig {
    return &ui.ProjectConfig{
        ProjectName:          projectName,
        ModuleName:           viper.GetString("module_name"),
        Architecture:         viper.GetString("architecture"),
        DBDriver:             viper.GetString("db_driver"),
        UseDocker:            viper.GetBool("use_docker"),
        UseObservability:     viper.GetBool("use_observability"),
        ObservabilityBackend: viper.GetString("observability_backend"),
        UseGRPC:              viper.GetBool("use_grpc"),
        UseTemplHTMX:         viper.GetBool("use_templ_htmx"),
    }
}

// displayPlan prints the upgrade plan grouped by classification.
func displayPlan(plan *scaffold.UpgradePlan) {
    if plan.IsLegacy {
        fmt.Println("⚠️  Legacy project (no manifest found). Using whitelist fallback.")
        fmt.Println()
    }

    upgradable := 0
    protected := 0
    absent := 0

    for _, f := range plan.Files {
        switch f.Classification {
        case scaffold.ClassUpgradable:
            upgradable++
            if f.Path == "go.mod" {
                fmt.Printf("📦 %s: go.mod has updates (report-only — run suggested `go get` commands)\n", f.Path)
                // Print go get hints from template go.mod
                printGoGetHints(f.RerenderBytes)
            } else {
                fmt.Printf("🔄 %s: update available\n", f.Path)
            }
        case scaffold.ClassProtected:
            protected++
            fmt.Printf("🔒 %s: user-modified (protected, skipping)\n", f.Path)
        case scaffold.ClassAbsent:
            absent++
            fmt.Printf("❌ %s: absent on disk (not recreating)\n", f.Path)
        }
    }

    if upgradable == 0 && protected == 0 && absent == 0 {
        ui.Success("All files are up to date.")
        return
    }

    fmt.Printf("\nSummary: %d upgradable, %d protected, %d absent\n", upgradable, protected, absent)
}

// printGoGetHints extracts require lines from the template go.mod and prints
// go get hints for each dependency. Best-effort: if parsing fails, print a
// generic hint.
func printGoGetHints(goModBytes []byte) {
    lines := strings.Split(string(goModBytes), "\n")
    inRequire := false
    for _, line := range lines {
        line = strings.TrimSpace(line)
        if strings.HasPrefix(line, "require (") {
            inRequire = true
            continue
        }
        if inRequire && line == ")" {
            inRequire = false
            continue
        }
        if inRequire && line != "" {
            // Extract module path from "module/path v1.2.3"
            parts := strings.Fields(line)
            if len(parts) >= 1 {
                fmt.Printf("   go get %s@latest\n", parts[0])
            }
        }
        if strings.HasPrefix(line, "require ") && !strings.Contains(line, "(") {
            parts := strings.Fields(line)
            if len(parts) >= 2 {
                fmt.Printf("   go get %s@latest\n", parts[1])
            }
        }
    }
}

// applyLegacyInteractive prompts per file for legacy projects on TTY.
func applyLegacyInteractive(plan *scaffold.UpgradePlan, cfg *ui.ProjectConfig) error {
    applied := 0
    for i := range plan.Files {
        f := &plan.Files[i]
        if f.Classification != scaffold.ClassUpgradable {
            continue
        }
        if f.Path == "go.mod" {
            continue // report-only
        }
        var confirm bool
        prompt := &survey.Confirm{
            Message: fmt.Sprintf("Update %s?", f.Path),
            Default: false,
        }
        if err := survey.AskOne(prompt, &confirm); err != nil {
            return err
        }
        if !confirm {
            f.Classification = scaffold.ClassProtected // mark as skipped
            continue
        }
        // Write confirmed file
        fullPath := f.Path // legacy plan uses relative paths from root
        if plan.ProjectRoot != "." {
            fullPath = filepath.Join(plan.ProjectRoot, f.Path)
        }
        if err := os.WriteFile(fullPath, f.RerenderBytes, 0644); err != nil {
            return err
        }
        applied++
    }

    // Surgical version write
    configPath := ".go-arch.yaml"
    if plan.ProjectRoot != "." {
        configPath = filepath.Join(plan.ProjectRoot, ".go-arch.yaml")
    }
    _ = scaffold.WriteVersionField(configPath, Version)

    ui.Success(fmt.Sprintf("Applied %d update(s).", applied))

    if plan.TemplHint {
        fmt.Println("💡 Run `templ generate` to recompile updated views.")
    }
    return nil
}
```

### 6. `internal/pkg/mcp/server.go` (MODIFY)

**Changes**: Add `upgrade_project` to `tools/list` and `handleToolCall`. Follow `generate_component` pattern for chdir + viper.Reset.

**tools/list entry** (append to the tools array):

```go
map[string]interface{}{
    "name":        "upgrade_project",
    "description": "Propagate embedded template changes to a previously generated project. Returns a classified plan (upgradable / protected / absent). Dry-run by default — mutates nothing. Set apply: true to commit changes.",
    "inputSchema": map[string]interface{}{
        "type": "object",
        "properties": map[string]interface{}{
            "projectPath": map[string]interface{}{
                "type":        "string",
                "description": "Optional: Path to the project root containing .go-arch.yaml",
            },
            "apply": map[string]interface{}{
                "type":        "boolean",
                "description": "When true, apply all upgradable changes and return the applied plan. Default: false.",
            },
        },
    },
},
```

**handleToolCall case** (add to switch):

```go
case "upgrade_project":
    var args struct {
        ProjectPath string `json:"projectPath"`
        Apply       bool   `json:"apply"`
    }
    if err := json.Unmarshal(arguments, &args); err != nil {
        sendError(id, -32602, "Invalid tool arguments", err.Error())
        return
    }

    if args.ProjectPath != "" {
        oldWd, err := os.Getwd()
        if err == nil {
            if chdirErr := os.Chdir(args.ProjectPath); chdirErr != nil {
                sendError(id, -32602, "Cannot change to project directory", chdirErr.Error())
                return
            }
            defer func() { _ = os.Chdir(oldWd) }()
        }
    }

    viper.Reset()
    viper.AddConfigPath(".")
    viper.SetConfigName(".go-arch")
    if err := viper.ReadInConfig(); err != nil {
        sendToolResult(id, fmt.Sprintf("Could not read .go-arch.yaml config. Error: %v", err), true)
        return
    }

    cfg := &ui.ProjectConfig{
        ProjectName:          viper.GetString("project_name"),
        ModuleName:           viper.GetString("module_name"),
        Architecture:         viper.GetString("architecture"),
        DBDriver:             viper.GetString("db_driver"),
        UseDocker:            viper.GetBool("use_docker"),
        UseObservability:     viper.GetBool("use_observability"),
        ObservabilityBackend: viper.GetString("observability_backend"),
        UseGRPC:              viper.GetBool("use_grpc"),
        UseTemplHTMX:         viper.GetBool("use_templ_htmx"),
    }

    plan, err := scaffold.Upgrade(cfg)
    if err != nil {
        sendToolResult(id, fmt.Sprintf("Upgrade failed: %v", err), true)
        return
    }

    // ADR-3 (MCP semantics): apply when args.Apply is true, regardless of dryRun.
    // dryRun is implicit (apply=false → plan only; apply=true → commit).
    if args.Apply {
        applied, applyErr := plan.Apply()
        if applyErr != nil {
            sendToolResult(id, fmt.Sprintf("Apply failed: %v", applyErr), true)
            return
        }
        // Surgical version write using the real Version, not literal "mcp"
        _ = scaffold.WriteVersionField(".go-arch.yaml", Version)
        plan.AppliedCount = applied
    }

    result, _ := json.MarshalIndent(plan, "", "  ")
    sendToolResult(id, string(result), false)
```

### 7. `internal/ui/prompts.go` (MODIFY)

Add `GoArchVersion` field:

```go
type ProjectConfig struct {
    ProjectName          string `mapstructure:"project_name"`
    ModuleName           string `mapstructure:"module_name"`
    Architecture         string `mapstructure:"architecture"`
    DBDriver             string `mapstructure:"db_driver"`
    UseDocker            bool   `mapstructure:"use_docker"`
    UseObservability     bool   `mapstructure:"use_observability"`
    ObservabilityBackend string `mapstructure:"observability_backend"`
    UseGRPC              bool   `mapstructure:"use_grpc"`
    UseTemplHTMX         bool   `mapstructure:"use_templ_htmx"`
    GoArchVersion        string `mapstructure:"go_arch_version"` // NEW
}
```

### 8. `templates/common/config.tmpl` (MODIFY)

Add `go_arch_version` line:

```
# Project Metadata
# Generated by Go-Arch CLI 🚀

project_name: {{ .ProjectName }}
module_name: {{ .ModuleName }}
architecture: {{ .Architecture }}
db_driver: {{ .DBDriver }}
use_docker: {{ .UseDocker }}
use_observability: {{ .UseObservability }}
observability_backend: {{ .ObservabilityBackend }}
use_grpc: {{ .UseGRPC }}
use_templ_htmx: {{ .UseTemplHTMX }}
go_arch_version: {{ .GoArchVersion }}
generated_at: {{ now }}
```

**How `go_arch_version` gets populated**:

1. **New project via `cmd/new.go`**: The wizard does NOT prompt for `GoArchVersion` (it's not in the survey questions). The field defaults to `""`. After `scaffolder.Execute()` succeeds, `cmd/new.go` calls `scaffold.WriteVersionField(".go-arch.yaml", cmd.Version)` to surgically fill in the version. The template's `{{ .GoArchVersion }}` renders as empty, then the post-hoc write fills it.

2. **New project via MCP `new_project`**: After scaffolding, the MCP handler calls `scaffold.WriteVersionField(".go-arch.yaml", cmd.Version)` to fill in the version.

3. **Upgrade apply**: Both CLI and MCP call `scaffold.WriteVersionField(".go-arch.yaml", cmd.Version)` after applying changes to update the version to the current CLI version.

4. **Existing projects without the field**: `WriteVersionField` appends the line if it doesn't exist (regex fallback). Absence is tolerated.

### 9. `cmd/new.go` (MODIFY)

After `scaffolder.Execute()` succeeds, call `WriteVersionField`:

```go
// After scaffolder.Execute() returns nil:
if err := scaffold.WriteVersionField(filepath.Join(config.ProjectName, ".go-arch.yaml"), Version); err != nil {
    // Non-fatal: warn but don't fail the new command
    ui.Warning(fmt.Sprintf("Could not set go_arch_version: %v", err))
}
```

## Data Flow

### New project (`go-arch new`)

```
RunWizard() → ProjectConfig (GoArchVersion = "")
    │
    ▼
Scaffolder.Execute()
    │
    ├── createFile(path, tmpl, data)
    │     ├── os.Create + engine.Render
    │     └── recordManifest(path, tmpl, OriginScaffold)
    │           ├── hashFile → upsert manifest entry
    │           └── manifest.Save()  ← atomic after EACH write
    │
    ├── createBinaryFile(path, embedded)
    │     ├── ReadFile + WriteFile
    │     └── recordManifest(path, embedded, OriginBinary)
    │           └── manifest.Save()  ← atomic after EACH write
    │
    └── (no final manifest.Save — each write already saved)

WriteVersionField(<project>/.go-arch.yaml, Version)  ← surgical post-hoc
```

### Upgrade (`go-arch upgrade`)

```
cmd.RunE
    │
    ├── validate .go-arch.yaml (missing → oops missing_config)
    │
    ├── --project-path? → os.Chdir(projectPath)
    │
    ├── scaffold.Upgrade(cfg)
    │     │
    │     ├── root := "." (always CWD, ADR-7)
    │     │
    │     ├── ManifestExists? → no → upgradeLegacy(whitelist)
    │     │
    │     └── LoadManifest → for each entry:
    │           ├── .go-arch.yaml → SKIP (ADR-8)
    │           ├── hashFile(disk) → diskHash
    │           ├── diskHash != manifestHash → PROTECTED
    │           ├── file missing → ABSENT
    │           ├── engine.RenderTo(buf, quiet=true) → rerenderHash
    │           ├── rerenderHash == diskHash → UP_TO_DATE (omit from plan, ADR-4)
    │           └── rerenderHash != diskHash → UPGRADABLE
    │
    ├── displayPlan(plan)
    │
    ├── --yes? → plan.Apply()
    │     ├── for each upgradable: compare bytes → write only on diff
    │     ├── refresh manifest entries
    │     ├── manifest.Save()
    │     └── return applied count
    │
    └── WriteVersionField(".go-arch.yaml", Version)  ← surgical
```

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/pkg/scaffold/manifest.go` | Create | Manifest struct, Load/Save/Upsert, hashFile, ManifestPath, ManifestExists |
| `internal/pkg/scaffold/upgrade.go` | Create | Upgrade, Apply, WriteVersionField, legacy fallback, classification, renderEntry |
| `internal/pkg/scaffold/scaffold.go` | Modify | Add `*Manifest` field, ensureManifest, recordManifest in createFile/createBinaryFile. NO save in Execute. |
| `internal/pkg/template/engine.go` | Modify | Add `RenderTo(wr, templatePath, data, quiet bool)` variant |
| `cmd/upgrade.go` | Create | Cobra subcommand, flags, TTY check, plan display, apply flow |
| `cmd/new.go` | Modify | Call `WriteVersionField` after `scaffolder.Execute()` |
| `internal/pkg/mcp/server.go` | Modify | Add upgrade_project to tools/list + handleToolCall |
| `internal/ui/prompts.go` | Modify | Add GoArchVersion field to ProjectConfig |
| `templates/common/config.tmpl` | Modify | Add `go_arch_version: {{ .GoArchVersion }}` line |
| `internal/pkg/scaffold/upgrade_test.go` | Create | Manifest recording, classification 4 classes, apply compare-then-write, PROTECTED never overwrite, legacy fallback, surgical version write |
| `cmd/upgrade_test.go` | Create | Flag validation, missing_config, dry-run default, --yes applies, non-TTY contract |
| `internal/pkg/mcp/server_test.go` | Create/Modify | upgrade_project dry-run + apply |

## Interfaces / Contracts

### Manifest YAML schema

```yaml
version: 1
files:
  main.go:
    path: main.go
    sha256: "abc123..."
    origin: scaffold
    template: minimalist/main.tmpl
  internal/domain/order_service.go:
    path: internal/domain/order_service.go
    sha256: "def456..."
    origin: crud
    template: common/crud_service.tmpl
    metadata:
      entity_name: Order
  static/js/htmx.min.js:
    path: static/js/htmx.min.js
    sha256: "ghi789..."
    origin: binary
    template: web/htmx.min.js
```

### Upgrade plan JSON (MCP output)

```json
{
  "files": [
    {
      "path": "cmd/api/main.go",
      "classification": "upgradable",
      "origin": "scaffold",
      "manifestHash": "abc...",
      "diskHash": "abc...",
      "rerenderHash": "xyz..."
    },
    {
      "path": "internal/handler/page.go",
      "classification": "protected",
      "origin": "scaffold",
      "manifestHash": "abc...",
      "diskHash": "different..."
    }
  ],
  "isLegacy": false,
  "templHint": true
}
```

**Note**: `up_to_date` entries are OMITTED from the plan (ADR-4). Only `upgradable`, `protected`, and `absent` appear.

### Classification table

| disk sha256 | manifest sha256 | rerender sha256 | Classification | Action |
|-------------|-----------------|-----------------|----------------|--------|
| == manifest | == disk | != disk | `upgradable` | Propose overwrite |
| != manifest | — | — | `protected` | Report, NEVER overwrite |
| file missing | entry exists | — | `absent` | Report, don't recreate |
| == manifest | == disk | == disk | `up_to_date` | **Omitted from plan** |

## Testing Strategy

| Layer | What to Test | Approach |
|-------|-------------|----------|
| Unit: manifest | Load (missing → empty), Save (atomic), Upsert, hashFile, round-trip | `os.MkdirTemp`, write YAML, load, assert fields |
| Unit: classification | All 4 classes (upgradable, protected, absent, up_to_date) | Engine override pattern (engine_test.go:114-151): scaffold v1, copy modified template into `.go-arch/templates/`, run Upgrade, assert classification |
| Unit: apply | Compare-then-write (write only on diff), manifest refresh, idempotent, **applied count correct** | Pre-populate manifest + disk, apply, assert bytes written + hash refreshed + `applied == N`. Re-apply → zero changes. |
| Unit: PROTECTED | Never overwrite user-modified file | Scaffold, hand-edit file, run Upgrade + Apply, assert file unchanged |
| Unit: surgical version | `WriteVersionField` — existing key replaced, missing key appended, other keys byte-identical | Write sample YAML, call WriteVersionField, compare byte-by-byte |
| Unit: legacy | Whitelist fallback, per-file confirm callback, go.mod report-only, **UseTemplHTMX respected** | Create project without manifest, populate whitelisted files, run Upgrade with `UseTemplHTMX=true`, assert main.go maps to web/main.tmpl |
| Unit: go.mod | Report-only — never written by Apply | Put go.mod in upgradable state, call Apply, assert go.mod unchanged |
| Unit: .go-arch.yaml | **Never appears in plan** | Scaffold project, run Upgrade, assert `.go-arch.yaml` not in plan.Files |
| Unit: MCP | Dry-run returns plan JSON, apply commits, default is dry-run, **apply=true writes files** | Call handleToolCall directly, assert result JSON + disk state |
| Cmd: flags | `--dry-run --yes` → usage error, default dry-run, `--project-path` chdir | `upgradeCmd.SetArgs`, `RootCmd.Execute`, assert error / file state |
| Cmd: missing_config | No `.go-arch.yaml` → oops `missing_config` | Run in empty tempdir |
| Cmd: non-TTY | Non-TTY without `--yes` → plan only | Pipe stdin from `os.Pipe()`, assert no writes |
| Integration: engine override | Re-render uses local → global → embedded chain | Drop modified template in `.go-arch/templates/`, upgrade, assert file uses custom template output |
| Integration: root resolution | Upgrade runs from CWD, not ProjectName | Scaffold project in `<tmpdir>/myapp`, chdir into `<tmpdir>/myapp`, run Upgrade, assert it finds the manifest (not looking for `<tmpdir>/myapp/myapp/.go-arch/manifest.yaml`) |

**Engine override test pattern** (from engine_test.go:114-151):
```go
// 1. Scaffold project (creates manifest with original fingerprints)
// 2. Copy "v2" template to .go-arch/templates/<path>
// 3. Run Upgrade(cfg)
// 4. Assert: affected file classified as ClassUpgradable
// 5. Call plan.Apply()
// 6. Assert: file contents match v2 template output
// 7. Assert: manifest entry refreshed
// 8. Assert: applied == N (correct count)
// 9. Run Upgrade again → zero upgradable (idempotent)
```

## Threat Matrix

N/A — no routing, shell, subprocess, VCS/PR automation, executable-file classification, or process-integration boundary. Upgrade operates purely within file I/O: read templates → render to buffer → compare → conditional write. No `os/exec` calls. No network.

## Migration / Rollout

**No migration required.** The manifest is additive — absence is tolerated and triggers the legacy fallback. Existing projects work without changes:

1. Legacy projects (no manifest): whitelist + per-file confirm (TTY) or plan-only (non-TTY).
2. New projects (after upgrade): manifest recorded automatically by `new`/`generate`/`crud`.
3. `go_arch_version` field: absence tolerated; written surgically on first upgrade-apply or post-hoc by `cmd/new.go`.
4. `config.tmpl` gains `go_arch_version` line: only affects new scaffolds; existing configs unchanged.

## Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| Manifest write corrupts on crash | Atomic write (temp + rename) |
| `go.mod` fingerprint drifts from `go mod tidy` | Report-only: never rewrite, only hint `go get` commands |
| User edits generated file → PROTECTED, never overwritten | Three-way hash: disk != manifest → protected |
| Template render fails during upgrade (template removed) | Skip that file, omit from plan (up_to_date) |
| Concurrent upgrade runs | File-level locking not needed — compare-then-write is idempotent |
| Legacy whitelist rots as templates evolve | Document as one-time migration path; manifest is the future |
| `generated_at: {{ now }}` breaks config re-render | `.go-arch.yaml` NEVER re-rendered wholesale; surgical line write only (ADR-8) |
| Binary files bypass engine (by design) | Manifest records them with `OriginBinary`; re-read from embedded FS |
| `engine.Render` prints to stdout in MCP mode | `RenderTo` with `quiet=true` suppresses the print (ADR-9) |

## Open Questions

- None. All spec scenarios are addressed by the design.

## next_recommended

`sdd-tasks` — break into implementation tasks following the file change order:
1. `manifest.go` (foundation, no deps)
2. `engine.go` — add `RenderTo` variant
3. Scaffold seam (modify `scaffold.go` to record manifest in createFile/createBinaryFile)
4. `upgrade.go` (classification + apply + surgical version)
5. Tests for manifest + classification + apply
6. `cmd/upgrade.go` (CLI layer)
7. `cmd/upgrade_test.go`
8. `cmd/new.go` — post-hoc `WriteVersionField`
9. MCP tool (`server.go` modification)
10. Config template + ProjectConfig field changes
