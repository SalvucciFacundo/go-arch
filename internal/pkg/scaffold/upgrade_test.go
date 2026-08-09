package scaffold

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"go-arch/internal/pkg/template"
	"go-arch/internal/ui"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
