package scaffold

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go-arch/internal/pkg/packs"
	"go-arch/internal/pkg/template"
	"go-arch/internal/ui"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/samber/oops"
)

// hashBytesForTest computes sha256 hex digest of in-memory bytes.
func hashBytesForTest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// ──────────────────────────────────────────────────────────
// Helper: create a minimal test project with manifest on disk
// ──────────────────────────────────────────────────────────

// setupTestProject scaffolds a minimal project in a temp dir (chdir'd),
// using the embedded engine, with a manifest. Returns the cfg used.
func setupTestProject(t *testing.T, tmplPath, diskPath string) (*ui.ProjectConfig, []byte) {
	t.Helper()

	cfg := &ui.ProjectConfig{
		ProjectName:  ".",
		ModuleName:   "github.com/test/app",
		Architecture: "Standard",
		DBDriver:     "None",
	}

	engine := template.NewEngine()
	var buf bytes.Buffer
	if err := engine.RenderTo(&buf, tmplPath, cfg, true); err != nil {
		t.Fatalf("render %s failed: %v", tmplPath, err)
	}
	diskBytes := buf.Bytes()
	diskHash := hashBytesForTest(diskBytes)

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(diskPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(diskPath, diskBytes, 0644); err != nil {
		t.Fatal(err)
	}

	// Create manifest with this entry
	m := &Manifest{
		Version: 1,
		Files: map[string]ManifestEntry{
			diskPath: {
				Path:         diskPath,
				SHA256:       diskHash,
				Origin:       OriginScaffold,
				TemplatePath: tmplPath,
			},
		},
		dir: ".",
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}

	return cfg, diskBytes
}

// ──────────────────────────────────────────────────────────
// 2.1 Classification tests
// ──────────────────────────────────────────────────────────

// TestUpgrade_ClassUpgradable verifies that when disk==manifest but
// re-render differs (via local template override), the file is classified
// as upgradable.
func TestUpgrade_ClassUpgradable(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	cfg, _ := setupTestProject(t, "common/env.tmpl", ".env")

	// Create local template override that differs from embedded
	localDir := filepath.Join(".go-arch", "templates", "common")
	if err := os.MkdirAll(localDir, 0755); err != nil {
		t.Fatal(err)
	}
	v2Content := "# V2 TEMPLATE OVERRIDE\nDB_HOST=testhost\n"
	if err := os.WriteFile(filepath.Join(localDir, "env.tmpl"), []byte(v2Content), 0644); err != nil {
		t.Fatal(err)
	}

	plan, err := Upgrade(cfg)
	if err != nil {
		t.Fatalf("Upgrade failed: %v", err)
	}

	upgraded := plan.CountBy(ClassUpgradable)
	if upgraded != 1 {
		t.Fatalf("upgradable count = %d, want 1. Plan: %+v", upgraded, plan.Files)
	}

	f := plan.Files[0]
	if f.Path != ".env" {
		t.Errorf("path = %q, want .env", f.Path)
	}
	if f.Classification != ClassUpgradable {
		t.Errorf("classification = %q, want upgradable", f.Classification)
	}
	if len(f.RerenderBytes) == 0 {
		t.Error("RerenderBytes should be non-empty for upgradable entries")
	}
}

// TestUpgrade_ClassProtected verifies that when disk hash != manifest hash,
// the file is classified as PROTECTED and never overwritten.
func TestUpgrade_ClassProtected(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	cfg, origBytes := setupTestProject(t, "common/env.tmpl", ".env")

	// Modify the file on disk (simulate user edit)
	modified := append([]byte("# USER EDIT\n"), origBytes...)
	if err := os.WriteFile(".env", modified, 0644); err != nil {
		t.Fatal(err)
	}

	plan, err := Upgrade(cfg)
	if err != nil {
		t.Fatalf("Upgrade failed: %v", err)
	}

	protected := plan.CountBy(ClassProtected)
	if protected != 1 {
		t.Fatalf("protected count = %d, want 1. Plan: %+v", protected, plan.Files)
	}

	f := plan.Files[0]
	if f.Classification != ClassProtected {
		t.Errorf("classification = %q, want protected", f.Classification)
	}

	// Apply should skip protected files
	applied, err := plan.Apply()
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if applied != 0 {
		t.Errorf("applied count = %d, want 0 (protected file should not be written)", applied)
	}

	// Disk content must be unchanged
	diskAfter, _ := os.ReadFile(".env")
	if !bytes.Equal(diskAfter, modified) {
		t.Error("protected file was modified by Apply()")
	}
}

// TestUpgrade_ClassAbsent verifies that a manifest entry whose file
// doesn't exist on disk is classified as absent.
func TestUpgrade_ClassAbsent(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	cfg, _ := setupTestProject(t, "common/env.tmpl", ".env")

	// Delete the file
	if err := os.Remove(".env"); err != nil {
		t.Fatal(err)
	}

	plan, err := Upgrade(cfg)
	if err != nil {
		t.Fatalf("Upgrade failed: %v", err)
	}

	absent := plan.CountBy(ClassAbsent)
	if absent != 1 {
		t.Fatalf("absent count = %d, want 1. Plan: %+v", absent, plan.Files)
	}

	f := plan.Files[0]
	if f.Classification != ClassAbsent {
		t.Errorf("classification = %q, want absent", f.Classification)
	}
}

// TestUpgrade_ClassUpToDate verifies that when disk==manifest==rerender,
// the file is OMITTED from the plan (up_to_date).
func TestUpgrade_ClassUpToDate(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	cfg, _ := setupTestProject(t, "common/env.tmpl", ".env")

	plan, err := Upgrade(cfg)
	if err != nil {
		t.Fatalf("Upgrade failed: %v", err)
	}

	// No files should be in the plan (all up_to_date)
	if len(plan.Files) != 0 {
		t.Errorf("expected empty plan for up-to-date project, got %d files", len(plan.Files))
	}

	protectedCount := plan.CountBy(ClassProtected)
	absentCount := plan.CountBy(ClassAbsent)
	upgradableCount := plan.CountBy(ClassUpgradable)
	if protectedCount+absentCount+upgradableCount != 0 {
		t.Errorf("all counts should be zero: upgradable=%d protected=%d absent=%d",
			upgradableCount, protectedCount, absentCount)
	}
}

// TestUpgrade_GoArchYAMLExcluded verifies ADR-8: .go-arch.yaml is never
// in the upgrade plan, regardless of whether it has drifted.
func TestUpgrade_GoArchYAMLExcluded(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	cfg, _ := setupTestProject(t, "common/env.tmpl", ".env")

	// Add .go-arch.yaml to manifest with a deliberately wrong hash
	fakeHash := hashBytesForTest([]byte("wrong content"))
	os.WriteFile(".go-arch.yaml", []byte("project_name: MyApp\n"), 0644)

	m, _ := LoadManifest(".")
	m.Upsert(ManifestEntry{
		Path:         ".go-arch.yaml",
		SHA256:       fakeHash,
		Origin:       OriginScaffold,
		TemplatePath: "common/config.tmpl",
	})
	m.Save()

	plan, err := Upgrade(cfg)
	if err != nil {
		t.Fatalf("Upgrade failed: %v", err)
	}

	for _, f := range plan.Files {
		if f.Path == ".go-arch.yaml" {
			t.Errorf(".go-arch.yaml should NOT appear in upgrade plan; classification=%s", f.Classification)
		}
	}
}

// TestUpgrade_GoModReportOnly verifies that go.mod is classified but
// Apply() never writes it (report-only per ADR-5).
func TestUpgrade_GoModReportOnly(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	cfg, _ := setupTestProject(t, "common/go.mod.tmpl", "go.mod")

	// Modify the template locally so re-render differs
	localDir := filepath.Join(".go-arch", "templates", "common")
	if err := os.MkdirAll(localDir, 0755); err != nil {
		t.Fatal(err)
	}
	v2Content := "module github.com/test/modified\n\ngo 1.23\n"
	if err := os.WriteFile(filepath.Join(localDir, "go.mod.tmpl"), []byte(v2Content), 0644); err != nil {
		t.Fatal(err)
	}

	plan, err := Upgrade(cfg)
	if err != nil {
		t.Fatalf("Upgrade failed: %v", err)
	}

	// go.mod should appear as upgradable in the plan
	hasGoMod := false
	for _, f := range plan.Files {
		if f.Path == "go.mod" {
			hasGoMod = true
			if f.Classification != ClassUpgradable {
				t.Errorf("go.mod classification = %q, want upgradable", f.Classification)
			}
		}
	}
	if !hasGoMod {
		t.Fatal("go.mod should appear in upgrade plan")
	}

	// Save current go.mod content
	before, _ := os.ReadFile("go.mod")

	// Apply should NOT write go.mod
	applied, err := plan.Apply()
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if applied != 0 {
		t.Errorf("applied count = %d, want 0 (go.mod is report-only)", applied)
	}

	after, _ := os.ReadFile("go.mod")
	if !bytes.Equal(before, after) {
		t.Error("go.mod was modified by Apply() — should be report-only")
	}
}

// ──────────────────────────────────────────────────────────
// 2.2 Apply tests
// ──────────────────────────────────────────────────────────

// TestUpgrade_Apply_CompareThenWrite verifies that Apply only writes
// when re-rendered bytes differ from disk bytes.
func TestUpgrade_Apply_CompareThenWrite(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	cfg, _ := setupTestProject(t, "common/env.tmpl", ".env")

	// Override template to get upgradable
	localDir := filepath.Join(".go-arch", "templates", "common")
	os.MkdirAll(localDir, 0755)
	v2Content := "# V2 TEMPLATE\nDB_HOST=v2host\n"
	os.WriteFile(filepath.Join(localDir, "env.tmpl"), []byte(v2Content), 0644)

	plan, _ := Upgrade(cfg)
	applied, err := plan.Apply()
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if applied != 1 {
		t.Errorf("applied count = %d, want 1", applied)
	}

	// Disk should now match v2
	diskAfter, _ := os.ReadFile(".env")
	if !bytes.Equal(diskAfter, []byte(v2Content)) {
		t.Errorf("disk content after apply:\n%s\n\nwant:\n%s", diskAfter, v2Content)
	}

	// Manifest entry should be refreshed
	m, _ := LoadManifest(".")
	entry := m.Files[".env"]
	expectedHash := hashBytesForTest([]byte(v2Content))
	if entry.SHA256 != expectedHash {
		t.Errorf("manifest hash after apply = %q, want %q", entry.SHA256, expectedHash)
	}
}

// TestUpgrade_Apply_Idempotent verifies that running Upgrade+Apply twice
// produces zero changes on the second run.
func TestUpgrade_Apply_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	cfg, _ := setupTestProject(t, "common/env.tmpl", ".env")

	// Override template → upgradable
	localDir := filepath.Join(".go-arch", "templates", "common")
	os.MkdirAll(localDir, 0755)
	os.WriteFile(filepath.Join(localDir, "env.tmpl"), []byte("# V2\n"), 0644)

	// First upgrade+apply
	plan1, _ := Upgrade(cfg)
	applied1, _ := plan1.Apply()
	if applied1 != 1 {
		t.Fatalf("first apply: applied = %d, want 1", applied1)
	}

	// Second upgrade → should be empty (all up-to-date)
	plan2, err := Upgrade(cfg)
	if err != nil {
		t.Fatalf("second Upgrade failed: %v", err)
	}
	if len(plan2.Files) != 0 {
		t.Errorf("second upgrade plan has %d files, want 0 (idempotent)", len(plan2.Files))
	}

	// Second apply → zero writes
	applied2, err := plan2.Apply()
	if err != nil {
		t.Fatalf("second Apply failed: %v", err)
	}
	if applied2 != 0 {
		t.Errorf("second apply: applied = %d, want 0", applied2)
	}
}

// TestUpgrade_Apply_CountMatches verifies that plan.AppliedCount correctly
// reflects the number of files actually written.
func TestUpgrade_Apply_CountMatches(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	cfg, _ := setupTestProject(t, "common/env.tmpl", ".env")

	// Add a second entry to the manifest
	engine := template.NewEngine()
	var buf2 bytes.Buffer
	engine.RenderTo(&buf2, "common/env.tmpl", cfg, true)

	os.WriteFile(".env2", buf2.Bytes(), 0644)
	m, _ := LoadManifest(".")
	m.Upsert(ManifestEntry{
		Path:         ".env2",
		SHA256:       hashBytesForTest(buf2.Bytes()),
		Origin:       OriginScaffold,
		TemplatePath: "common/env.tmpl",
	})
	m.Save()

	// Override template → both entries become upgradable
	localDir := filepath.Join(".go-arch", "templates", "common")
	os.MkdirAll(localDir, 0755)
	os.WriteFile(filepath.Join(localDir, "env.tmpl"), []byte("# V2 OVERRIDE\n"), 0644)

	plan, _ := Upgrade(cfg)
	applied, _ := plan.Apply()

	if applied != 2 {
		t.Errorf("applied count = %d, want 2", applied)
	}
	if plan.AppliedCount != applied {
		t.Errorf("plan.AppliedCount = %d, want %d", plan.AppliedCount, applied)
	}
}

// ──────────────────────────────────────────────────────────
// 2.3 WriteVersionField tests
// ──────────────────────────────────────────────────────────

// TestWriteVersionField_Replace verifies that an existing go_arch_version
// line is replaced surgically with the new value.
func TestWriteVersionField_Replace(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".go-arch.yaml")

	content := `project_name: MyApp
architecture: Standard
go_arch_version: v1.0.0
use_docker: true
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	if err := WriteVersionField(configPath, "v2.0.0"); err != nil {
		t.Fatalf("WriteVersionField failed: %v", err)
	}

	result, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	resultStr := string(result)

	if !strings.Contains(resultStr, "go_arch_version: v2.0.0") {
		t.Errorf("expected 'go_arch_version: v2.0.0', got:\n%s", resultStr)
	}
	if strings.Contains(resultStr, "go_arch_version: v1.0.0") {
		t.Error("old version line still present")
	}

	// Other keys must be byte-identical
	if !strings.Contains(resultStr, "project_name: MyApp") {
		t.Error("project_name line was modified")
	}
	if !strings.Contains(resultStr, "architecture: Standard") {
		t.Error("architecture line was modified")
	}
	if !strings.Contains(resultStr, "use_docker: true") {
		t.Error("use_docker line was modified")
	}
}

// TestWriteVersionField_Append verifies that a missing go_arch_version
// field is appended.
func TestWriteVersionField_Append(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".go-arch.yaml")

	content := `project_name: MyApp
architecture: Hexagonal
use_docker: false
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	if err := WriteVersionField(configPath, "v3.0.0"); err != nil {
		t.Fatalf("WriteVersionField failed: %v", err)
	}

	result, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	resultStr := string(result)

	if !strings.Contains(resultStr, "go_arch_version: v3.0.0") {
		t.Errorf("expected 'go_arch_version: v3.0.0', got:\n%s", resultStr)
	}

	// Original keys still present
	if !strings.Contains(resultStr, "project_name: MyApp") {
		t.Error("project_name line missing after append")
	}
	if !strings.Contains(resultStr, "architecture: Hexagonal") {
		t.Error("architecture line missing after append")
	}
	if !strings.Contains(resultStr, "use_docker: false") {
		t.Error("use_docker line missing after append")
	}
}

// TestWriteVersionField_OtherKeysIdentical verifies that all other bytes
// are preserved byte-for-byte (ADR-4: surgical write).
func TestWriteVersionField_OtherKeysIdentical(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".go-arch.yaml")

	content := `# A comment
project_name: MyApp
module_name: github.com/test/app
# Mid-file comment
architecture: Standard
go_arch_version: old-version
db_driver: PostgreSQL
# End comment
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	if err := WriteVersionField(configPath, "new-version"); err != nil {
		t.Fatalf("WriteVersionField failed: %v", err)
	}

	result, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	resultStr := string(result)

	// Verify surgical replacement
	if !strings.Contains(resultStr, "go_arch_version: new-version") {
		t.Error("version not updated")
	}

	// Comments and other keys intact
	for _, expected := range []string{
		"# A comment",
		"# Mid-file comment",
		"# End comment",
		"project_name: MyApp",
		"module_name: github.com/test/app",
		"architecture: Standard",
		"db_driver: PostgreSQL",
	} {
		if !strings.Contains(resultStr, expected) {
			t.Errorf("missing expected content: %q", expected)
		}
	}
}

// ──────────────────────────────────────────────────────────
// 2.4 Legacy fallback tests
// ──────────────────────────────────────────────────────────

// TestLegacyUpgrade_WebMainMapping verifies Fix 5: when UseTemplHTMX=true
// and Architecture=Standard, main.go maps to web/main.tmpl.
func TestLegacyUpgrade_WebMainMapping(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	cfg := &ui.ProjectConfig{
		ProjectName:  ".",
		ModuleName:   "github.com/test/legacy",
		Architecture: "Standard",
		UseTemplHTMX: true,
	}

	// No manifest → legacy code path
	// Create a .go-arch.yaml so config validation passes
	os.MkdirAll(".go-arch", 0755)
	os.WriteFile(filepath.Join(".go-arch", "manifest.yaml"), nil, 0644) // empty

	// Write a main.go on disk (v1 content from embedded template)
	engine := template.NewEngine()
	var v1Buf bytes.Buffer
	engine.RenderTo(&v1Buf, "web/main.tmpl", cfg, true)
	v1Content := v1Buf.Bytes()

	os.MkdirAll("cmd/api", 0755)
	os.WriteFile("cmd/api/main.go", v1Content, 0644)

	// Local override to trigger a diff
	localDir := filepath.Join(".go-arch", "templates", "web")
	os.MkdirAll(localDir, 0755)
	v2Content := []byte("// V2 WEB MAIN\npackage main\n")
	os.WriteFile(filepath.Join(localDir, "main.tmpl"), v2Content, 0644)

	// Remove manifest so we hit legacy code path
	os.Remove(filepath.Join(".go-arch", "manifest.yaml"))

	plan, err := Upgrade(cfg)
	if err != nil {
		t.Fatalf("legacy Upgrade failed: %v", err)
	}

	if !plan.IsLegacy {
		t.Fatal("expected IsLegacy=true for legacy project")
	}

	// cmd/api/main.go should be upgradable
	found := false
	for _, f := range plan.Files {
		if f.Path == "cmd/api/main.go" {
			found = true
			if f.Classification != ClassUpgradable {
				t.Errorf("cmd/api/main.go classification = %q, want upgradable", f.Classification)
			}
		}
	}
	if !found {
		t.Error("cmd/api/main.go not found in legacy plan")
	}
}

// TestLegacyUpgrade_GoModReportOnly verifies that go.mod in legacy mode
// is never written by Apply().
func TestLegacyUpgrade_GoModReportOnly(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	cfg := &ui.ProjectConfig{
		ProjectName:  ".",
		ModuleName:   "github.com/test/legacy2",
		Architecture: "Standard",
	}

	// Write go.mod with a known (mismatched) module path
	goModContent := []byte("module github.com/test/legacy2\n\ngo 1.23\n")
	os.WriteFile("go.mod", goModContent, 0644)

	// Also write a .env so the plan has at least one file
	engine := template.NewEngine()
	var envBuf bytes.Buffer
	engine.RenderTo(&envBuf, "common/env.tmpl", cfg, true)
	os.WriteFile(".env", envBuf.Bytes(), 0644)

	plan, err := Upgrade(cfg)
	if err != nil {
		t.Fatalf("legacy Upgrade failed: %v", err)
	}

	// Save current go.mod
	before, _ := os.ReadFile("go.mod")

	// Apply: go.mod should NOT be written
	applied, err := plan.Apply()
	if err != nil {
		t.Fatalf("legacy Apply failed: %v", err)
	}

	after, _ := os.ReadFile("go.mod")
	if !bytes.Equal(before, after) {
		t.Errorf("go.mod was modified by legacy Apply (report-only)")
	}

	// Print applied count for diagnostics
	t.Logf("Legacy applied count: %d", applied)
}

// ──────────────────────────────────────────────────────────
// Triangulation tests
// ──────────────────────────────────────────────────────────

// TestUpgrade_BinaryOrigin verifies that OriginBinary entries are
// re-rendered via embedded FS ReadFile, not the template engine.
func TestUpgrade_BinaryOrigin(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	// Read the real embedded htmx bytes
	embeddedData, err := template.TemplatesFS.ReadFile("templates/web/htmx.min.js")
	if err != nil {
		t.Fatal(err)
	}
	embeddedHash := hashBytesForTest(embeddedData)

	// Write a different version on disk (simulating outdated binary)
	modifiedData := []byte("// MODIFIED HTMX\n")
	if err := os.MkdirAll("static/js", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("static/js/htmx.min.js", modifiedData, 0644); err != nil {
		t.Fatal(err)
	}
	modifiedHash := hashBytesForTest(modifiedData)

	// Manifest records the MODIFIED hash (matching disk) so it passes the
	// protected check. The re-render will get the embedded version, which
	// differs → upgradable.
	m := &Manifest{
		Version: 1,
		Files: map[string]ManifestEntry{
			"static/js/htmx.min.js": {
				Path:         "static/js/htmx.min.js",
				SHA256:       modifiedHash,
				Origin:       OriginBinary,
				TemplatePath: "web/htmx.min.js",
			},
		},
		dir: ".",
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}

	cfg := &ui.ProjectConfig{
		ProjectName:  ".",
		ModuleName:   "github.com/test/binary",
		Architecture: "Standard",
	}

	plan, err := Upgrade(cfg)
	if err != nil {
		t.Fatalf("Upgrade failed: %v", err)
	}

	upgradable := plan.CountBy(ClassUpgradable)
	if upgradable != 1 {
		t.Fatalf("upgradable count = %d, want 1", upgradable)
	}

	f := plan.Files[0]
	if f.RerenderHash != embeddedHash {
		t.Errorf("rerender hash for binary = %q, want embedded hash %q", f.RerenderHash, embeddedHash)
	}
	if !bytes.Equal(f.RerenderBytes, embeddedData) {
		t.Errorf("binary RerenderBytes len=%d, want len=%d (should match embedded FS)", len(f.RerenderBytes), len(embeddedData))
	}

	// Apply should write the embedded version
	applied, err := plan.Apply()
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if applied != 1 {
		t.Errorf("applied count = %d, want 1", applied)
	}

	diskAfter, _ := os.ReadFile("static/js/htmx.min.js")
	if !bytes.Equal(diskAfter, embeddedData) {
		t.Error("binary file not updated to embedded version")
	}
}

// TestWriteVersionField_EmptyFile verifies that WriteVersionField appends
// to an empty file without error.
func TestWriteVersionField_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".go-arch.yaml")

	if err := os.WriteFile(configPath, []byte{}, 0644); err != nil {
		t.Fatal(err)
	}

	if err := WriteVersionField(configPath, "v1.0.0"); err != nil {
		t.Fatalf("WriteVersionField on empty file failed: %v", err)
	}

	result, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	resultStr := string(result)

	if !strings.Contains(resultStr, "go_arch_version: v1.0.0") {
		t.Errorf("expected 'go_arch_version: v1.0.0' in empty file, got:\n%s", resultStr)
	}
}

// ──────────────────────────────────────────────────────────
// Phase 3: Routes upgrade tests (3.5)
// ──────────────────────────────────────────────────────────

// TestUpgrade_CreatesRoutesGo verifies that upgrade creates an absent
// routes.go file in a web project and main.go gets the Register call.
// File on disk routes.go = missing → classified absent + rendered on apply.
// TestUpgrade_CreatesRoutesGo verifies that upgrade re-renders routes.go
// when it is present in the manifest (in-manifest case).
func TestUpgrade_CreatesRoutesGo(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	cfg := &ui.ProjectConfig{
		ProjectName:  ".",
		ModuleName:   "github.com/test/routes-upgrade",
		Architecture: "Standard",
		UseTemplHTMX: true,
	}

	// Create main.go on disk (matching embedded web/main.tmpl)
	engine := template.NewEngine()
	var mainBuf bytes.Buffer
	if err := engine.RenderTo(&mainBuf, "web/main.tmpl", cfg, true); err != nil {
		t.Fatal(err)
	}
	mainBytes := mainBuf.Bytes()
	mainHash := hashBytesForTest(mainBytes)
	if err := os.WriteFile("main.go", mainBytes, 0644); err != nil {
		t.Fatal(err)
	}

	// Create manifest with main.go but NO routes.go entry
	m := &Manifest{
		Version: 1,
		Files: map[string]ManifestEntry{
			"main.go": {
				Path:         "main.go",
				SHA256:       mainHash,
				Origin:       OriginScaffold,
				TemplatePath: "web/main.tmpl",
			},
			"internal/router/routes.go": {
				Path:         "internal/router/routes.go",
				SHA256:       "dummy",
				Origin:       OriginScaffold,
				TemplatePath: "common/routes.tmpl",
			},
		},
		dir: ".",
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}

	plan, err := Upgrade(cfg)
	if err != nil {
		t.Fatalf("Upgrade failed: %v", err)
	}

	// routes.go should be re-rendered from manifest (classified upgradable, not absent)
	foundRoutes := false
	for _, f := range plan.Files {
		if f.Path == "internal/router/routes.go" {
			foundRoutes = true
			// routes.go absent → re-rendered, classified as upgradable
			if f.Classification != ClassUpgradable {
				t.Errorf("absent routes.go classification = %q, want upgradable (re-rendered from manifest)", f.Classification)
			}
		}
	}
	if !foundRoutes {
		t.Error("expected routes.go in the plan")
	}

	// main.go should be absent too (no main.go entry visible — but we created it)
	// Actually main.go should show as up-to-date since hash matches and template unchanged
	// Let's verify main.go is NOT in plan (up to date)
	for _, f := range plan.Files {
		if f.Path == "main.go" {
			t.Errorf("main.go should be up-to-date (hash matches template), got classification=%s", f.Classification)
		}
	}
}

// TestUpgrade_RoutesGoNotProtected verifies that routes.go is NEVER classified
// as PROTECTED even when its disk hash differs from the manifest hash.
func TestUpgrade_RoutesGoNotProtected(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	cfg := &ui.ProjectConfig{
		ProjectName:  ".",
		ModuleName:   "github.com/test/routes-prot",
		Architecture: "Standard",
		UseTemplHTMX: true,
	}

	// Create routes.go on disk with content that DIFFERS from manifest hash
	routesContent := []byte("package router\n\nfunc Register(mux *http.ServeMux) {}\n")
	if err := os.MkdirAll("internal/router", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("internal/router/routes.go", routesContent, 0644); err != nil {
		t.Fatal(err)
	}

	// Manifest routes.go has a DIFFERENT hash (simulates user edit)
	// BUT it should still NOT be classified as PROTECTED — routes.go
	// is always re-rendered from manifest.Routes.
	m := &Manifest{
		Version: 1,
		Files: map[string]ManifestEntry{
			"internal/router/routes.go": {
				Path:         "internal/router/routes.go",
				SHA256:       "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", // different from disk
				Origin:       OriginScaffold,
				TemplatePath: "common/routes.tmpl",
			},
		},
		dir: ".",
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}

	plan, err := Upgrade(cfg)
	if err != nil {
		t.Fatalf("Upgrade failed: %v", err)
	}

	// routes.go must NOT be classified as PROTECTED
	for _, f := range plan.Files {
		if f.Path == "internal/router/routes.go" {
			if f.Classification == ClassProtected {
				t.Errorf("routes.go was classified PROTECTED — should never be protected")
			}
			// Should be upgradable (disk!=manifest but always re-rendered)
			if f.Classification != ClassUpgradable {
				t.Logf("routes.go classification: %s (expected upgradable)", f.Classification)
			}
		}
	}

	// There should be NO protected files from our test
	if plan.CountBy(ClassProtected) > 0 {
		// Only routes.go is in the manifest — all other protected counts are
		// from the temp dir. Check specifically that routes.go is not the
		// protected one.
		for _, f := range plan.Files {
			if f.Path == "internal/router/routes.go" && f.Classification == ClassProtected {
				t.Error("routes.go incorrectly classified as PROTECTED")
			}
		}
	}
}

// TestLegacyUpgrade_StandardArchitecture_NoTemplHTMX verifies that for
// Standard architecture without UseTemplHTMX, the legacy whitelist correctly
// maps cmd/api/main.go to standard/main.tmpl (not web/main.tmpl).
func TestLegacyUpgrade_StandardArchitecture_NoTemplHTMX(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	cfg := &ui.ProjectConfig{
		ProjectName:      ".",
		ModuleName:       "github.com/test/legacy-std",
		Architecture:     "Standard",
		UseTemplHTMX:     false,
		UseObservability: false,
		UseGRPC:          false,
	}

	// Write cmd/api/main.go using embedded standard/main.tmpl
	engine := template.NewEngine()
	var v1Buf bytes.Buffer
	if err := engine.RenderTo(&v1Buf, "standard/main.tmpl", cfg, true); err != nil {
		t.Fatal(err)
	}
	v1Content := v1Buf.Bytes()

	os.MkdirAll("cmd/api", 0755)
	os.WriteFile("cmd/api/main.go", v1Content, 0644)

	// Local override to trigger diff
	localDir := filepath.Join(".go-arch", "templates", "standard")
	os.MkdirAll(localDir, 0755)
	v2Content := []byte("package main\n// V2 STANDARD\nfunc main() {}\n")
	os.WriteFile(filepath.Join(localDir, "main.tmpl"), v2Content, 0644)

	plan, err := Upgrade(cfg)
	if err != nil {
		t.Fatalf("legacy Upgrade failed: %v", err)
	}

	if !plan.IsLegacy {
		t.Fatal("expected IsLegacy=true")
	}

	// cmd/api/main.go should be upgradable — mapped to standard/main.tmpl
	found := false
	for _, f := range plan.Files {
		if f.Path == "cmd/api/main.go" {
			found = true
			if f.Classification != ClassUpgradable {
				t.Errorf("cmd/api/main.go classification = %q, want upgradable", f.Classification)
			}
		}
	}
	if !found {
		t.Error("cmd/api/main.go not found in legacy plan for Standard architecture")
	}
}

// TestUpgrade_CreatesRoutesGoPreChange verifies the CRITICAL fix: for a
// genuinely pre-change web project (manifest WITHOUT a routes.go entry and no
// routes.go on disk), upgrade proposes creating routes.go so the upgraded
// main.go (which references router.Register) compiles. Also verifies Apply
// writes it and seeds the manifest entry.
func TestUpgrade_CreatesRoutesGoPreChange(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	cfg := &ui.ProjectConfig{
		ProjectName:  ".",
		ModuleName:   "github.com/test/pre-change",
		Architecture: "Standard",
		UseTemplHTMX: true,
	}

	// Pre-change project: main.go from the OLD web template (no Register call),
	// manifest with main.go but NO routes.go entry, no routes.go on disk.
	engine := template.NewEngine()
	var mainBuf bytes.Buffer
	if err := engine.RenderTo(&mainBuf, "web/main.tmpl", cfg, true); err != nil {
		t.Fatal(err)
	}
	mainBytes := mainBuf.Bytes()
	mainHash := hashBytes(mainBytes)
	if err := os.WriteFile("main.go", mainBytes, 0644); err != nil {
		t.Fatal(err)
	}

	m := &Manifest{
		Version: 1,
		Files: map[string]ManifestEntry{
			"main.go": {
				Path:         "main.go",
				SHA256:       mainHash,
				Origin:       OriginScaffold,
				TemplatePath: "web/main.tmpl",
			},
		},
		dir: ".",
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}

	// routes.go must NOT exist on disk.
	if _, err := os.Stat("internal/router/routes.go"); !os.IsNotExist(err) {
		t.Fatal("precondition: routes.go should not exist")
	}

	plan, err := Upgrade(cfg)
	if err != nil {
		t.Fatalf("Upgrade failed: %v", err)
	}

	// The plan must propose creating routes.go.
	foundRoutes := false
	for _, f := range plan.Files {
		if f.Path == "internal/router/routes.go" {
			foundRoutes = true
			if f.Classification != ClassUpgradable {
				t.Errorf("pre-change routes.go classification = %q, want upgradable (create it)", f.Classification)
			}
			if len(f.RerenderBytes) == 0 {
				t.Error("routes.go rerender bytes must be non-empty")
			}
		}
	}
	if !foundRoutes {
		t.Fatal("expected routes.go in the plan for a pre-change web project")
	}

	// Apply must write routes.go and seed the manifest entry.
	applied, err := plan.Apply()
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if applied < 1 {
		t.Errorf("expected at least 1 file applied, got %d", applied)
	}

	if _, err := os.Stat("internal/router/routes.go"); err != nil {
		t.Fatalf("routes.go should exist after apply: %v", err)
	}

	// Manifest must now have a routes.go entry with the correct template.
	m2, err := LoadManifest(".")
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := m2.Files["internal/router/routes.go"]
	if !ok {
		t.Fatal("manifest should now contain routes.go entry")
	}
	if entry.TemplatePath != "common/routes.tmpl" {
		t.Errorf("routes.go manifest TemplatePath = %q, want common/routes.tmpl", entry.TemplatePath)
	}

	// Second upgrade must be idempotent (routes.go now up-to-date, not re-proposed as creation).
	plan2, err := Upgrade(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range plan2.Files {
		if f.Path == "internal/router/routes.go" {
			t.Errorf("routes.go should be up-to-date on second upgrade, got classification=%s", f.Classification)
		}
	}
}

// ──────────────────────────────────────────────────────────
// Phase 5: Pack source upgrade tests (5.1)
// ──────────────────────────────────────────────────────────

// fakeResolver implements Resolver for testing pack-sourced upgrade.
type fakeResolver struct {
	resolve func(name, version string) (packs.PackInfo, error)
}

func (f *fakeResolver) Resolve(name, version string) (packs.PackInfo, error) {
	if f.resolve != nil {
		return f.resolve(name, version)
	}
	return packs.PackInfo{}, oops.
		Code(packs.CodePackNotInstalled).
		Errorf("pack %q is not installed", name+"@"+version)
}

// createPackTemplateDir creates a synthetic pack directory with a single
// template file at templates/<templatePath>. Returns the pack root dir.
func createPackTemplateDir(t *testing.T, packName, packVersion, templatePath, content string) string {
	t.Helper()
	packDir := filepath.Join(t.TempDir(), packName+"@"+packVersion)
	packTemplatesDir := filepath.Join(packDir, "templates")
	tmplFile := filepath.Join(packTemplatesDir, templatePath)
	if err := os.MkdirAll(filepath.Dir(tmplFile), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tmplFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return packDir
}

// TestUpgrade_PackSource_RerendersFromPack verifies that when a manifest
// entry has Source="pack:express@1.0.0", the re-render reads from the
// recorded pack directory directly, BYPASSING the local/global/embedded chain.
func TestUpgrade_PackSource_RerendersFromPack(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	cfg := &ui.ProjectConfig{
		ProjectName: ".",
		ModuleName:  "github.com/test/pack-upgrade",
	}

	// Embedded template renders V1 content.
	engine := template.NewEngine()
	var v1Buf bytes.Buffer
	if err := engine.RenderTo(&v1Buf, "common/env.tmpl", cfg, true); err != nil {
		t.Fatal(err)
	}
	v1Content := v1Buf.Bytes()
	v1Hash := hashBytesForTest(v1Content)

	// Write V1 to disk.
	if err := os.WriteFile(".env", v1Content, 0644); err != nil {
		t.Fatal(err)
	}

	// Synthetic pack dir with V2 template content (differs from V1).
	v2Template := "# PACK V2 ENV TEMPLATE\nDB_HOST=pack-host\n"
	packDir := createPackTemplateDir(t, "express", "1.0.0", "common/env.tmpl", v2Template)

	// Build a PackInfo from the synthetic dir.
	packInfo := packs.PackInfo{
		Dir: packDir,
		Manifest: &packs.Manifest{
			Name:    "express",
			Version: "1.0.0",
		},
	}

	// Manifest entry records pack source and V1 hash.
	m := &Manifest{
		Version: 1,
		Files: map[string]ManifestEntry{
			".env": {
				Path:         ".env",
				SHA256:       v1Hash,
				Origin:       OriginScaffold,
				TemplatePath: "common/env.tmpl",
				Source:       "pack:express@1.0.0",
			},
		},
		dir: ".",
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}

	resolver := &fakeResolver{
		resolve: func(name, version string) (packs.PackInfo, error) {
			if name == "express" && version == "1.0.0" {
				return packInfo, nil
			}
			return packs.PackInfo{}, fmt.Errorf("not installed")
		},
	}

	plan, err := Upgrade(cfg, WithResolver(resolver))
	if err != nil {
		t.Fatalf("Upgrade failed: %v", err)
	}

	// The pack V2 template should produce different output than disk V1.
	upgradable := plan.CountBy(ClassUpgradable)
	if upgradable != 1 {
		t.Fatalf("upgradable count = %d, want 1. Plan: %+v", upgradable, plan.Files)
	}

	f := plan.Files[0]
	if f.Path != ".env" {
		t.Errorf("path = %q, want .env", f.Path)
	}
	if f.Classification != ClassUpgradable {
		t.Errorf("classification = %q, want upgradable", f.Classification)
	}
	if len(f.RerenderBytes) == 0 {
		t.Error("RerenderBytes should be non-empty for upgradable entries")
	}

	// Apply should write the pack-rendered content.
	applied, err := plan.Apply()
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if applied != 1 {
		t.Errorf("applied count = %d, want 1", applied)
	}

	// Disk must now contain pack V2 content.
	diskAfter, _ := os.ReadFile(".env")
	rendered := string(diskAfter)
	if !strings.Contains(rendered, "PACK V2") {
		t.Errorf("disk content after apply does not come from pack:\n%s", rendered)
	}
}

// TestUpgrade_PackSource_MissingPackProtected verifies that when the pack
// recorded in Source is not installed, the entry is classified as PROTECTED
// and a warning is emitted naming the missing pack.
func TestUpgrade_PackSource_MissingPackProtected(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	cfg := &ui.ProjectConfig{
		ProjectName: ".",
		ModuleName:  "github.com/test/missing-pack",
	}

	// Write some content on disk.
	diskContent := []byte("# DISK CONTENT\nDB_HOST=localhost\n")
	if err := os.WriteFile(".env", diskContent, 0644); err != nil {
		t.Fatal(err)
	}
	diskHash := hashBytesForTest(diskContent)

	m := &Manifest{
		Version: 1,
		Files: map[string]ManifestEntry{
			".env": {
				Path:         ".env",
				SHA256:       diskHash,
				Origin:       OriginScaffold,
				TemplatePath: "common/env.tmpl",
				Source:       "pack:express@1.0.0",
			},
		},
		dir: ".",
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}

	// Resolver returns not-installed for any pack.
	resolver := &fakeResolver{
		resolve: func(name, version string) (packs.PackInfo, error) {
			return packs.PackInfo{}, oops.
				Code(packs.CodePackNotInstalled).
				Errorf("pack %q is not installed", name+"@"+version)
		},
	}

	// Capture stderr to verify warning message.
	oldErr := ui.Out
	defer func() { ui.Out = oldErr }()
	var stderr bytes.Buffer
	ui.Out = &stderr

	plan, err := Upgrade(cfg, WithResolver(resolver))
	if err != nil {
		t.Fatalf("Upgrade failed: %v", err)
	}

	protected := plan.CountBy(ClassProtected)
	if protected != 1 {
		t.Fatalf("protected count = %d, want 1. Plan: %+v", protected, plan.Files)
	}

	f := plan.Files[0]
	if f.Classification != ClassProtected {
		t.Errorf("classification = %q, want protected", f.Classification)
	}
	if f.Path != ".env" {
		t.Errorf("path = %q, want .env", f.Path)
	}

	// Warning must mention the pack name and version.
	warnOutput := stderr.String()
	if !strings.Contains(warnOutput, "express") {
		t.Errorf("warning should mention pack name 'express', got: %s", warnOutput)
	}
	if !strings.Contains(warnOutput, "1.0.0") {
		t.Errorf("warning should mention version '1.0.0', got: %s", warnOutput)
	}
}

// TestUpgrade_PackSource_VersionBumpProtected verifies that when the recorded
// pack version is no longer installed, the entries are protected — no
// auto-substitution to a newer installed version.
func TestUpgrade_PackSource_VersionBumpProtected(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	cfg := &ui.ProjectConfig{
		ProjectName: ".",
		ModuleName:  "github.com/test/version-bump",
	}

	diskContent := []byte("# DISK V1\n")
	if err := os.WriteFile(".env", diskContent, 0644); err != nil {
		t.Fatal(err)
	}
	diskHash := hashBytesForTest(diskContent)

	m := &Manifest{
		Version: 1,
		Files: map[string]ManifestEntry{
			".env": {
				Path:         ".env",
				SHA256:       diskHash,
				Origin:       OriginScaffold,
				TemplatePath: "common/env.tmpl",
				Source:       "pack:express@1.0.0",
			},
		},
		dir: ".",
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}

	// Only v1.1.0 is installed; v1.0.0 was removed (version bump scenario).
	resolver := &fakeResolver{
		resolve: func(name, version string) (packs.PackInfo, error) {
			// v1.0.0 is gone — only v1.1.0 exists
			if name == "express" && version == "1.1.0" {
				pd := createPackTemplateDir(t, "express", "1.1.0", "common/env.tmpl", "# V1.1\n")
				return packs.PackInfo{
					Dir: pd,
					Manifest: &packs.Manifest{
						Name:    "express",
						Version: "1.1.0",
					},
				}, nil
			}
			return packs.PackInfo{}, oops.
				Code(packs.CodePackNotInstalled).
				Errorf("pack %q is not installed", name+"@"+version)
		},
	}

	plan, err := Upgrade(cfg, WithResolver(resolver))
	if err != nil {
		t.Fatalf("Upgrade failed: %v", err)
	}

	// Must be PROTECTED — version 1.0.0 is not installed.
	protected := plan.CountBy(ClassProtected)
	if protected != 1 {
		t.Fatalf("protected count = %d, want 1. Plan: %+v", protected, plan.Files)
	}

	f := plan.Files[0]
	if f.Classification != ClassProtected {
		t.Errorf("classification = %q, want protected (no auto-substitute to v1.1.0)", f.Classification)
	}

	// Upgrade must NOT silently re-render from v1.1.0 — the plan file count
	// is exactly 1 (protected) and upgradable is 0.
	upgradable := plan.CountBy(ClassUpgradable)
	if upgradable != 0 {
		t.Errorf("upgradable count = %d, want 0 (no auto-substitute)", upgradable)
	}
}

// TestUpgrade_PackSource_NonPackEntriesUnchanged verifies that entries
// without a Source field are processed normally through the existing
// chain (local > global > embedded), with no pack-specific behavior.
func TestUpgrade_PackSource_NonPackEntriesUnchanged(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	cfg, _ := setupTestProject(t, "common/env.tmpl", ".env")

	// Create local template override to trigger upgradable
	localDir := filepath.Join(".go-arch", "templates", "common")
	if err := os.MkdirAll(localDir, 0755); err != nil {
		t.Fatal(err)
	}
	v2Content := "# V2 LOCAL OVERRIDE\nDB_HOST=override\n"
	if err := os.WriteFile(filepath.Join(localDir, "env.tmpl"), []byte(v2Content), 0644); err != nil {
		t.Fatal(err)
	}

	// Resolver always errors — should not be called for non-pack entries.
	resolver := &fakeResolver{
		resolve: func(name, version string) (packs.PackInfo, error) {
			t.Error("resolver should NOT be called for entries without Source")
			return packs.PackInfo{}, fmt.Errorf("unexpected call")
		},
	}

	plan, err := Upgrade(cfg, WithResolver(resolver))
	if err != nil {
		t.Fatalf("Upgrade failed: %v", err)
	}

	// Non-pack entry should still be upgradable via local override.
	upgradable := plan.CountBy(ClassUpgradable)
	if upgradable != 1 {
		t.Fatalf("upgradable count = %d, want 1 (non-pack entry should upgrade normally)", upgradable)
	}
}

// ──────────────────────────────────────────────────────────
// Phase 6: Generator-origin upgrade tests (Slice 5)
// ──────────────────────────────────────────────────────────

// TestUpgrade_GeneratorOrigin_Protected verifies that a manifest entry
// with origin: generator (no template field) is classified as PROTECTED
// during upgrade. Generator output is logic output, not template renders,
// and must never be silently overwritten.
func TestUpgrade_GeneratorOrigin_Protected(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	cfg := &ui.ProjectConfig{
		ProjectName: ".",
		ModuleName:  "github.com/test/gen-protected",
	}

	// Write a generator-produced file on disk.
	diskContent := []byte("# GENERATOR OUTPUT\ndocker-compose content\n")
	if err := os.WriteFile("docker-compose.yml", diskContent, 0644); err != nil {
		t.Fatal(err)
	}
	diskHash := hashBytesForTest(diskContent)

	// Manifest entry: origin=generator, NO template field.
	m := &Manifest{
		Version: 1,
		Files: map[string]ManifestEntry{
			"docker-compose.yml": {
				Path:   "docker-compose.yml",
				SHA256: diskHash,
				Origin: OriginGenerator,
				Source: "pack:express@1.0.0",
				Metadata: map[string]string{
					"generator": "docker",
					"args":      `{"compose": true}`,
				},
			},
		},
		dir: ".",
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}

	// Capture stderr for warning verification.
	oldOut := ui.Out
	defer func() { ui.Out = oldOut }()
	var stderr bytes.Buffer
	ui.Out = &stderr

	plan, err := Upgrade(cfg)
	if err != nil {
		t.Fatalf("Upgrade failed: %v", err)
	}

	protected := plan.CountBy(ClassProtected)
	if protected != 1 {
		t.Fatalf("protected count = %d, want 1", protected)
	}

	f := plan.Files[0]
	if f.Path != "docker-compose.yml" {
		t.Errorf("path = %q, want docker-compose.yml", f.Path)
	}
	if f.Classification != ClassProtected {
		t.Errorf("classification = %q, want protected", f.Classification)
	}
	if f.Origin != OriginGenerator {
		t.Errorf("origin = %q, want generator", f.Origin)
	}

	// Warning must mention the file path and PROTECTED.
	warnOutput := stderr.String()
	if !strings.Contains(warnOutput, "PROTECTED") {
		t.Errorf("warning should contain 'PROTECTED', got: %s", warnOutput)
	}
	if !strings.Contains(warnOutput, "docker-compose.yml") {
		t.Errorf("warning should name the file, got: %s", warnOutput)
	}

	// Apply should NOT write the file.
	applied, err := plan.Apply()
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if applied != 0 {
		t.Errorf("applied count = %d, want 0 (PROTECTED file should not be written)", applied)
	}

	diskAfter, _ := os.ReadFile("docker-compose.yml")
	if !bytes.Equal(diskAfter, diskContent) {
		t.Error("generator-origin file was modified by Apply()")
	}
}

// TestUpgrade_GeneratorOrigin_PackRemoved_StillProtected verifies that
// when the pack recorded in Source is removed, the generator-origin entry
// remains PROTECTED — no error, no attempt to re-render.
func TestUpgrade_GeneratorOrigin_PackRemoved_StillProtected(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	cfg := &ui.ProjectConfig{
		ProjectName: ".",
		ModuleName:  "github.com/test/gen-removed",
	}

	diskContent := []byte("# GENERATOR OUTPUT\nbinary output\n")
	if err := os.MkdirAll("bin", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("bin/output.bin", diskContent, 0644); err != nil {
		t.Fatal(err)
	}
	diskHash := hashBytesForTest(diskContent)

	m := &Manifest{
		Version: 1,
		Files: map[string]ManifestEntry{
			"bin/output.bin": {
				Path:   "bin/output.bin",
				SHA256: diskHash,
				Origin: OriginGenerator,
				Source: "pack:removed@0.1.0",
				Metadata: map[string]string{
					"generator": "binary-gen",
				},
			},
		},
		dir: ".",
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}

	// Resolver always returns not-installed.
	resolver := &fakeResolver{
		resolve: func(name, version string) (packs.PackInfo, error) {
			return packs.PackInfo{}, oops.
				Code(packs.CodePackNotInstalled).
				Errorf("pack %q is not installed", name+"@"+version)
		},
	}

	plan, err := Upgrade(cfg, WithResolver(resolver))
	if err != nil {
		t.Fatalf("Upgrade failed: %v", err)
	}

	protected := plan.CountBy(ClassProtected)
	if protected != 1 {
		t.Fatalf("protected count = %d, want 1", protected)
	}

	f := plan.Files[0]
	if f.Classification != ClassProtected {
		t.Errorf("classification = %q, want protected (generator entries stay PROTECTED even with pack removed)", f.Classification)
	}

	// No "pack not installed" error — PROTECTED is the safe default.
	if f.Origin != OriginGenerator {
		t.Errorf("origin = %q, want generator", f.Origin)
	}
}

// TestUpgrade_TemplateOriginWithGeneratorMetadata_Upgradable verifies that
// a template-origin entry (origin: template) with metadata.generator is
// still upgradable via renderPackEntry — byte-identical to a normal pack
// template re-render.
func TestUpgrade_TemplateOriginWithGeneratorMetadata_Upgradable(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	cfg := &ui.ProjectConfig{
		ProjectName: ".",
		ModuleName:  "github.com/test/gen-tmpl-upgradable",
	}

	// Embedded template renders V1 content.
	engine := template.NewEngine()
	var v1Buf bytes.Buffer
	if err := engine.RenderTo(&v1Buf, "common/env.tmpl", cfg, true); err != nil {
		t.Fatal(err)
	}
	v1Content := v1Buf.Bytes()
	v1Hash := hashBytesForTest(v1Content)

	if err := os.WriteFile(".env", v1Content, 0644); err != nil {
		t.Fatal(err)
	}

	// Synthetic pack dir with V2 template content (differs from V1).
	v2Template := "# PACK V2 ENV FROM GENERATOR\nDB_HOST=gen-host\n"
	packDir := createPackTemplateDir(t, "express", "1.0.0", "common/env.tmpl", v2Template)

	packInfo := packs.PackInfo{
		Dir: packDir,
		Manifest: &packs.Manifest{
			Name:    "express",
			Version: "1.0.0",
		},
	}

	// Manifest entry: origin=template WITH metadata.generator (single-entry-with-metadata).
	m := &Manifest{
		Version: 1,
		Files: map[string]ManifestEntry{
			".env": {
				Path:         ".env",
				SHA256:       v1Hash,
				Origin:       OriginTemplate,
				TemplatePath: "common/env.tmpl",
				Source:       "pack:express@1.0.0",
				Metadata: map[string]string{
					"generator": "docker",
					"args":      `{"db_driver": "postgres"}`,
				},
			},
		},
		dir: ".",
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}

	resolver := &fakeResolver{
		resolve: func(name, version string) (packs.PackInfo, error) {
			if name == "express" && version == "1.0.0" {
				return packInfo, nil
			}
			return packs.PackInfo{}, fmt.Errorf("not installed")
		},
	}

	plan, err := Upgrade(cfg, WithResolver(resolver))
	if err != nil {
		t.Fatalf("Upgrade failed: %v", err)
	}

	upgradable := plan.CountBy(ClassUpgradable)
	if upgradable != 1 {
		t.Fatalf("upgradable count = %d, want 1 (template-origin generator files are upgradable). Plan: %+v",
			upgradable, plan.Files)
	}

	f := plan.Files[0]
	if f.Path != ".env" {
		t.Errorf("path = %q, want .env", f.Path)
	}
	if f.Classification != ClassUpgradable {
		t.Errorf("classification = %q, want upgradable", f.Classification)
	}
	if f.Origin != OriginTemplate {
		t.Errorf("origin = %q, want template", f.Origin)
	}

	// Apply should write the pack-rendered V2 content.
	applied, err := plan.Apply()
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if applied != 1 {
		t.Errorf("applied count = %d, want 1", applied)
	}

	diskAfter, _ := os.ReadFile(".env")
	rendered := string(diskAfter)
	if !strings.Contains(rendered, "PACK V2 ENV FROM GENERATOR") {
		t.Errorf("disk content after apply does not come from pack:\n%s", rendered)
	}
}

// TestUpgrade_GeneratorOrigin_NoReRun verifies that upgrade never attempts
// to re-execute generator recipes. Generator entries remain PROTECTED
// even when the pack has newer template content. Re-running generators
// is deferred to v2.1.
func TestUpgrade_GeneratorOrigin_NoReRun(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	cfg := &ui.ProjectConfig{
		ProjectName: ".",
		ModuleName:  "github.com/test/gen-no-rerun",
	}

	diskContent := []byte("# GENERATOR V1 OUTPUT\n")
	if err := os.WriteFile("docker-compose.yml", diskContent, 0644); err != nil {
		t.Fatal(err)
	}
	diskHash := hashBytesForTest(diskContent)

	// Pack has NEWER content, but generator entries should NOT be re-rendered.
	v2Template := "# PACK V2 DOCKER CONTENT (should NOT be used)\n"
	packDir := createPackTemplateDir(t, "express", "1.1.0", "docker-compose.yml.tmpl", v2Template)

	packInfo := packs.PackInfo{
		Dir: packDir,
		Manifest: &packs.Manifest{
			Name:    "express",
			Version: "1.1.0",
		},
	}

	m := &Manifest{
		Version: 1,
		Files: map[string]ManifestEntry{
			"docker-compose.yml": {
				Path:   "docker-compose.yml",
				SHA256: diskHash,
				Origin: OriginGenerator,
				Source: "pack:express@1.1.0",
				Metadata: map[string]string{
					"generator": "docker",
				},
			},
		},
		dir: ".",
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}

	resolver := &fakeResolver{
		resolve: func(name, version string) (packs.PackInfo, error) {
			if name == "express" && version == "1.1.0" {
				return packInfo, nil
			}
			return packs.PackInfo{}, fmt.Errorf("not installed")
		},
	}

	plan, err := Upgrade(cfg, WithResolver(resolver))
	if err != nil {
		t.Fatalf("Upgrade failed: %v", err)
	}

	// Generator entries are PROTECTED — never re-rendered, even with pack available.
	protected := plan.CountBy(ClassProtected)
	if protected != 1 {
		t.Fatalf("protected count = %d, want 1 (generator entries never re-rendered)", protected)
	}

	upgradable := plan.CountBy(ClassUpgradable)
	if upgradable != 0 {
		t.Errorf("upgradable count = %d, want 0 (no generator recipe re-run)", upgradable)
	}

	// Disk content must be unchanged.
	diskAfter, _ := os.ReadFile("docker-compose.yml")
	if !bytes.Equal(diskAfter, diskContent) {
		t.Error("generator file was modified — should remain PROTECTED")
	}
}

// TestLegacyUpgrade_CreatesRoutesGoWeb verifies the legacy fallback proposes
// creating routes.go for a web project without a manifest (so the upgraded
// main.go referencing router.Register compiles).
func TestLegacyUpgrade_CreatesRoutesGoWeb(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	cfg := &ui.ProjectConfig{
		ProjectName:  ".",
		ModuleName:   "github.com/test/legacy-web",
		Architecture: "Standard",
		UseTemplHTMX: true,
	}

	// No manifest at all; a web project with old main.go on disk.
	engine := template.NewEngine()
	var mainBuf bytes.Buffer
	if err := engine.RenderTo(&mainBuf, "web/main.tmpl", cfg, true); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("main.go", mainBuf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}

	plan, err := Upgrade(cfg) // no manifest → upgradeLegacy
	if err != nil {
		t.Fatalf("Upgrade failed: %v", err)
	}
	if !plan.IsLegacy {
		t.Fatal("expected legacy plan")
	}

	foundRoutes := false
	for _, f := range plan.Files {
		if f.Path == "internal/router/routes.go" {
			foundRoutes = true
			if f.Classification != ClassUpgradable {
				t.Errorf("legacy routes.go classification = %q, want upgradable", f.Classification)
			}
		}
	}
	if !foundRoutes {
		t.Error("expected routes.go creation proposed in legacy web upgrade")
	}
}
