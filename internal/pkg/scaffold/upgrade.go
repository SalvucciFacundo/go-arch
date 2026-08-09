package scaffold

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"go-arch/internal/pkg/template"
	"go-arch/internal/ui"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ──────────────────────────────────────────────────────────
// Classification types
// ──────────────────────────────────────────────────────────

// Classification is the upgrade decision for a single file.
type Classification string

const (
	ClassUpgradable Classification = "upgradable" // template changed, disk untouched
	ClassProtected  Classification = "protected"  // user modified — NEVER overwrite
	ClassAbsent     Classification = "absent"     // file missing on disk — report only
	ClassUpToDate   Classification = "up_to_date" // no action needed (omitted from plan)
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

// ──────────────────────────────────────────────────────────
// hashBytes — in-memory sha256
// ──────────────────────────────────────────────────────────

// hashBytes computes sha256 hex digest of in-memory bytes.
func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// ──────────────────────────────────────────────────────────
// Core Upgrade entry point
// ──────────────────────────────────────────────────────────

// Upgrade loads the manifest (or falls back to legacy), re-renders each file,
// and classifies into upgradable / protected / absent.
//
// Root resolution: root is ALWAYS "." (CWD = project root). Upgrade runs from
// inside the project; --project-path does chdir before Upgrade is called.
// cfg.ProjectName is the project name metadata, NOT a directory path (ADR-7).
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
			rerender, rerenderErr := renderEntry(engine, entry, cfg)
			if rerenderErr == nil {
				rerenderHash := hashBytes(rerender)
				action.RerenderHash = rerenderHash
				action.RerenderBytes = rerender
				if diskHash != rerenderHash {
					action.Classification = ClassUpgradable // report-only in Apply
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
		rerender, err := renderEntry(engine, entry, cfg)
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

// ──────────────────────────────────────────────────────────
// renderEntry + buildRenderData
// ──────────────────────────────────────────────────────────

// renderEntry re-renders a manifest entry through the engine chain.
func renderEntry(engine *template.Engine, entry ManifestEntry, cfg *ui.ProjectConfig) ([]byte, error) {
	if entry.Origin == OriginBinary {
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

// ──────────────────────────────────────────────────────────
// Apply
// ──────────────────────────────────────────────────────────

// Apply writes only files that differ (compare-then-write), refreshes manifest.
// go.mod is report-only — never written.
// Non-upgradable classifications are skipped.
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

		// Ensure parent directory exists (e.g., cmd/api/ for main.go)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			return applied, err
		}

		// Compare-then-write
		existing, err := os.ReadFile(fullPath)
		if err != nil && !os.IsNotExist(err) {
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

// ──────────────────────────────────────────────────────────
// Surgical version write
// ──────────────────────────────────────────────────────────

var versionLineRe = regexp.MustCompile(`(?m)^go_arch_version:.*$`)

// WriteVersionField surgically updates (or appends) the go_arch_version field
// in .go-arch.yaml. All other bytes are preserved byte-for-byte (ADR-4).
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

// ──────────────────────────────────────────────────────────
// Legacy fallback
// ──────────────────────────────────────────────────────────

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
// Respects UseTemplHTMX for main.go / cmd/api/main.go mapping (Fix 5).
func legacyTemplateFor(path string, cfg *ui.ProjectConfig) string {
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
