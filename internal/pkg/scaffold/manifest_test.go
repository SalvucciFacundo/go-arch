package scaffold

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

// hashOf computes sha256 hex of a string for test assertions.
func hashOf(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func TestManifestPath(t *testing.T) {
	got := ManifestPath("/my/project")
	want := filepath.Join("/my/project", ".go-arch", "manifest.yaml")
	if got != want {
		t.Errorf("ManifestPath = %q, want %q", got, want)
	}
}

func TestManifestExists_Missing(t *testing.T) {
	dir := t.TempDir()
	if ManifestExists(dir) {
		t.Error("ManifestExists should return false when no manifest exists")
	}
}

func TestManifestExists_Found(t *testing.T) {
	dir := t.TempDir()
	manifestDir := filepath.Join(dir, ".go-arch")
	if err := os.MkdirAll(manifestDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(manifestDir, "manifest.yaml"), []byte("version: 1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if !ManifestExists(dir) {
		t.Error("ManifestExists should return true when manifest exists")
	}
}

func TestLoadManifest_Missing(t *testing.T) {
	dir := t.TempDir()
	m, err := LoadManifest(dir)
	if err != nil {
		t.Fatalf("LoadManifest should not error on missing file: %v", err)
	}
	if m.Version != 1 {
		t.Errorf("default Version = %d, want 1", m.Version)
	}
	if m.Files == nil {
		t.Error("default Files map should be non-nil")
	}
	if len(m.Files) != 0 {
		t.Errorf("default Files should be empty, got %d entries", len(m.Files))
	}
}

func TestLoadManifest_ValidYAML(t *testing.T) {
	dir := t.TempDir()
	manifestDir := filepath.Join(dir, ".go-arch")
	if err := os.MkdirAll(manifestDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := `version: 1
files:
  main.go:
    path: main.go
    sha256: abc123
    origin: scaffold
    template: minimalist/main.tmpl
  static/js/htmx.min.js:
    path: static/js/htmx.min.js
    sha256: def456
    origin: binary
    template: web/htmx.min.js
`
	if err := os.WriteFile(filepath.Join(manifestDir, "manifest.yaml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := LoadManifest(dir)
	if err != nil {
		t.Fatalf("LoadManifest failed: %v", err)
	}
	if m.Version != 1 {
		t.Errorf("Version = %d, want 1", m.Version)
	}
	if len(m.Files) != 2 {
		t.Fatalf("len(Files) = %d, want 2", len(m.Files))
	}

	mainEntry := m.Files["main.go"]
	if mainEntry.Path != "main.go" {
		t.Errorf("main.go path = %q, want main.go", mainEntry.Path)
	}
	if mainEntry.SHA256 != "abc123" {
		t.Errorf("main.go sha256 = %q, want abc123", mainEntry.SHA256)
	}
	if mainEntry.Origin != OriginScaffold {
		t.Errorf("main.go origin = %q, want scaffold", mainEntry.Origin)
	}
	if mainEntry.TemplatePath != "minimalist/main.tmpl" {
		t.Errorf("main.go template = %q, want minimalist/main.tmpl", mainEntry.TemplatePath)
	}

	binEntry := m.Files["static/js/htmx.min.js"]
	if binEntry.Origin != OriginBinary {
		t.Errorf("htmx origin = %q, want binary", binEntry.Origin)
	}
	if binEntry.TemplatePath != "web/htmx.min.js" {
		t.Errorf("htmx template = %q, want web/htmx.min.js", binEntry.TemplatePath)
	}
}

func TestManifest_Save_Load_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	m := &Manifest{
		Version: 1,
		Files: map[string]ManifestEntry{
			"main.go": {
				Path:         "main.go",
				SHA256:       "abc123",
				Origin:       OriginScaffold,
				TemplatePath: "minimalist/main.tmpl",
			},
		},
		dir: dir,
	}

	if err := m.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify file exists at correct path
	manifestPath := ManifestPath(dir)
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		t.Fatalf("manifest.yaml not created at %s", manifestPath)
	}

	// Load and verify
	loaded, err := LoadManifest(dir)
	if err != nil {
		t.Fatalf("LoadManifest after Save failed: %v", err)
	}
	if loaded.Version != 1 {
		t.Errorf("loaded Version = %d, want 1", loaded.Version)
	}
	entry := loaded.Files["main.go"]
	if entry.SHA256 != "abc123" {
		t.Errorf("round-trip sha256 = %q, want abc123", entry.SHA256)
	}
	if entry.Origin != OriginScaffold {
		t.Errorf("round-trip origin = %q", entry.Origin)
	}
}

func TestManifest_Save_CreatesDir(t *testing.T) {
	dir := t.TempDir()
	m := &Manifest{
		Version: 1,
		Files:   make(map[string]ManifestEntry),
		dir:     dir,
	}
	// .go-arch/ does not exist yet
	dotArch := filepath.Join(dir, ".go-arch")
	if _, err := os.Stat(dotArch); err == nil {
		t.Skip("pre-existing .go-arch dir — can't test auto-create")
	}

	if err := m.Save(); err != nil {
		t.Fatalf("Save should auto-create directory: %v", err)
	}
	if _, err := os.Stat(dotArch); os.IsNotExist(err) {
		t.Error(".go-arch dir was not created")
	}
}

func TestManifest_Upsert(t *testing.T) {
	m := &Manifest{Version: 1, Files: make(map[string]ManifestEntry)}

	// Insert
	m.Upsert(ManifestEntry{Path: "a.go", SHA256: "hash1", Origin: OriginScaffold})
	if len(m.Files) != 1 {
		t.Errorf("after insert: len = %d, want 1", len(m.Files))
	}

	// Replace (same path)
	m.Upsert(ManifestEntry{Path: "a.go", SHA256: "hash2", Origin: OriginComponent, Metadata: map[string]string{"entity_name": "Foo"}})
	if len(m.Files) != 1 {
		t.Errorf("after upsert: len = %d, want 1", len(m.Files))
	}
	entry := m.Files["a.go"]
	if entry.SHA256 != "hash2" {
		t.Errorf("upsert sha256 = %q, want hash2", entry.SHA256)
	}
	if entry.Origin != OriginComponent {
		t.Errorf("upsert origin = %q, want component", entry.Origin)
	}
	if entry.Metadata["entity_name"] != "Foo" {
		t.Errorf("upsert metadata entity_name = %q, want Foo", entry.Metadata["entity_name"])
	}

	// Insert different path (adds, not replaces)
	m.Upsert(ManifestEntry{Path: "b.go", SHA256: "hash3", Origin: OriginCrud})
	if len(m.Files) != 2 {
		t.Errorf("after second insert: len = %d, want 2", len(m.Files))
	}
}

func TestHashFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "test.txt")
	content := "hello world"
	if err := os.WriteFile(f, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	h, err := hashFile(f)
	if err != nil {
		t.Fatalf("hashFile failed: %v", err)
	}

	expectedHash := hashOf(content)
	if h != expectedHash {
		t.Errorf("hashFile = %q, want %q", h, expectedHash)
	}
}

func TestManifestEntry_Metadata(t *testing.T) {
	dir := t.TempDir()
	m := &Manifest{
		Version: 1,
		Files: map[string]ManifestEntry{
			"internal/domain/order_service.go": {
				Path:         "internal/domain/order_service.go",
				SHA256:       "abc123",
				Origin:       OriginCrud,
				TemplatePath: "common/crud_service.tmpl",
				Metadata: map[string]string{
					"entity_name": "Order",
				},
			},
		},
		dir: dir,
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	entry := loaded.Files["internal/domain/order_service.go"]
	if entry.Metadata["entity_name"] != "Order" {
		t.Errorf("metadata entity_name = %q, want Order", entry.Metadata["entity_name"])
	}
}

func TestOriginConsts(t *testing.T) {
	if OriginScaffold != "scaffold" {
		t.Errorf("OriginScaffold = %q, want scaffold", OriginScaffold)
	}
	if OriginComponent != "component" {
		t.Errorf("OriginComponent = %q, want component", OriginComponent)
	}
	if OriginCrud != "crud" {
		t.Errorf("OriginCrud = %q, want crud", OriginCrud)
	}
	if OriginBinary != "binary" {
		t.Errorf("OriginBinary = %q, want binary", OriginBinary)
	}
}

func TestUpsertRoute_Dedupe(t *testing.T) {
	dir := t.TempDir()
	m := &Manifest{
		Version: 1,
		Files:   make(map[string]ManifestEntry),
		dir:     dir,
	}

	// Insert first route
	if err := m.UpsertRoute(RouteEntry{
		Entity:  "User",
		Handler: "User",
		Origin:  "crud",
	}); err != nil {
		t.Fatalf("first UpsertRoute failed: %v", err)
	}
	if len(m.Routes) != 1 {
		t.Fatalf("after first insert: len(Routes) = %d, want 1", len(m.Routes))
	}
	if m.Routes[0].Entity != "User" {
		t.Errorf("Entity = %q, want User", m.Routes[0].Entity)
	}
	if m.Routes[0].Origin != "crud" {
		t.Errorf("Origin = %q, want crud", m.Routes[0].Origin)
	}

	// Upsert same entity — should replace, not append
	if err := m.UpsertRoute(RouteEntry{
		Entity:       "User",
		Handler:      "User",
		Origin:       "crud",
		RoutePattern: "GET /users",
	}); err != nil {
		t.Fatalf("second UpsertRoute failed: %v", err)
	}
	if len(m.Routes) != 1 {
		t.Fatalf("after dedupe: len(Routes) = %d, want 1", len(m.Routes))
	}
	if m.Routes[0].RoutePattern != "GET /users" {
		t.Errorf("after dedupe RoutePattern = %q, want GET /users", m.Routes[0].RoutePattern)
	}

	// Insert different entity — should append
	if err := m.UpsertRoute(RouteEntry{
		Entity:  "Order",
		Handler: "Order",
		Origin:  "handler",
	}); err != nil {
		t.Fatalf("third UpsertRoute failed: %v", err)
	}
	if len(m.Routes) != 2 {
		t.Fatalf("after third insert: len(Routes) = %d, want 2", len(m.Routes))
	}
	if m.Routes[1].Entity != "Order" {
		t.Errorf("second route Entity = %q, want Order", m.Routes[1].Entity)
	}
}

func TestManifest_Routes_YAML_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	m := &Manifest{
		Version: 1,
		Files:   make(map[string]ManifestEntry),
		Routes: []RouteEntry{
			{Entity: "User", Handler: "User", Origin: "crud"},
			{Entity: "Order", Handler: "Order", Origin: "handler", RoutePattern: "GET /orders"},
		},
		dir: dir,
	}

	if err := m.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := LoadManifest(dir)
	if err != nil {
		t.Fatalf("LoadManifest after Save failed: %v", err)
	}
	if len(loaded.Routes) != 2 {
		t.Fatalf("round-trip: len(Routes) = %d, want 2", len(loaded.Routes))
	}
	if loaded.Routes[0].Entity != "User" {
		t.Errorf("round-trip route 0 Entity = %q, want User", loaded.Routes[0].Entity)
	}
	if loaded.Routes[0].Origin != "crud" {
		t.Errorf("round-trip route 0 Origin = %q, want crud", loaded.Routes[0].Origin)
	}
	if loaded.Routes[1].Entity != "Order" {
		t.Errorf("round-trip route 1 Entity = %q, want Order", loaded.Routes[1].Entity)
	}
	if loaded.Routes[1].RoutePattern != "GET /orders" {
		t.Errorf("round-trip route 1 RoutePattern = %q, want GET /orders", loaded.Routes[1].RoutePattern)
	}
}

func TestManifest_Routes_Omitempty(t *testing.T) {
	dir := t.TempDir()
	// Manifest with nil Routes — should not emit "routes:" key in YAML
	m := &Manifest{
		Version: 1,
		Files:   make(map[string]ManifestEntry),
		Routes:  nil,
		dir:     dir,
	}

	if err := m.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	raw, err := os.ReadFile(ManifestPath(dir))
	if err != nil {
		t.Fatalf("read manifest failed: %v", err)
	}

	content := string(raw)
	if contains(content, "routes:") {
		t.Error("YAML output should NOT contain 'routes:' when Routes is nil (omitempty)")
	}

	// Load back — Routes should be empty/nil
	loaded, err := LoadManifest(dir)
	if err != nil {
		t.Fatalf("LoadManifest failed: %v", err)
	}
	if len(loaded.Routes) != 0 {
		t.Errorf("loaded Routes should be empty, got %d entries", len(loaded.Routes))
	}

	// Now save with routes populated — "routes:" should appear
	m2 := &Manifest{
		Version: 1,
		Files:   make(map[string]ManifestEntry),
		Routes:  []RouteEntry{{Entity: "X", Handler: "X", Origin: "crud"}},
		dir:     dir,
	}
	if err := m2.Save(); err != nil {
		t.Fatalf("Save with routes failed: %v", err)
	}
	raw2, err := os.ReadFile(ManifestPath(dir))
	if err != nil {
		t.Fatalf("read manifest failed: %v", err)
	}
	if !contains(string(raw2), "routes:") {
		t.Error("YAML output SHOULD contain 'routes:' when Routes is non-empty")
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
