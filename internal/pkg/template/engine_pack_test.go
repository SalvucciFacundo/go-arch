package template

import (
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// Engine chain precedence tests (4-step: local > global > pack > embedded)
// ---------------------------------------------------------------------------

func TestEngine_PackPrecedence_OverridesEmbedded(t *testing.T) {
	tmpDir := t.TempDir()
	packsDir := filepath.Join(tmpDir, "packs")
	packDir := filepath.Join(packsDir, "express@1.0.0", "templates", "common")
	if err := os.MkdirAll(packDir, 0755); err != nil {
		t.Fatal(err)
	}

	tmplContent := "module {{ .ModuleName }}\n// FROM PACK"
	if err := os.WriteFile(filepath.Join(packDir, "go.mod.tmpl"), []byte(tmplContent), 0644); err != nil {
		t.Fatal(err)
	}

	engine := NewEngine(WithPacksDir(packsDir), WithPack("express", "1.0.0"))

	_, source, err := engine.getTemplate("common/go.mod.tmpl")
	if err != nil {
		t.Fatalf("getTemplate failed: %v", err)
	}

	want := "pack:express@1.0.0"
	if source != want {
		t.Errorf("source = %q; want %q", source, want)
	}
}

func TestEngine_PackPrecedence_LocalOverridesPack(t *testing.T) {
	tmpDir := t.TempDir()
	packsDir := filepath.Join(tmpDir, "packs")

	// Create pack template.
	packTmp := filepath.Join(packsDir, "express@1.0.0", "templates", "common")
	if err := os.MkdirAll(packTmp, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packTmp, "handler.tmpl"), []byte("pack content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create local override (.go-arch/templates/...).
	localTmplDir := filepath.Join(".go-arch", "templates", "common")
	if err := os.MkdirAll(localTmplDir, 0755); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(".go-arch")

	if err := os.WriteFile(filepath.Join(localTmplDir, "handler.tmpl"), []byte("local content"), 0644); err != nil {
		t.Fatal(err)
	}

	engine := NewEngine(WithPacksDir(packsDir), WithPack("express", "1.0.0"))

	_, source, err := engine.getTemplate("common/handler.tmpl")
	if err != nil {
		t.Fatalf("getTemplate failed: %v", err)
	}

	if source != "local" {
		t.Errorf("source = %q; want %q (local should override pack)", source, "local")
	}
}

func TestEngine_PackPrecedence_GlobalOverridesPack(t *testing.T) {
	tmpDir := t.TempDir()
	packsDir := filepath.Join(tmpDir, "packs")

	// Create pack template.
	packTmp := filepath.Join(packsDir, "express@1.0.0", "templates", "common")
	if err := os.MkdirAll(packTmp, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packTmp, "handler.tmpl"), []byte("pack content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create global override via $HOME/.go-arch/templates/...
	globalTmplDir := filepath.Join(tmpDir, "global", ".go-arch", "templates", "common")
	if err := os.MkdirAll(globalTmplDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(globalTmplDir, "handler.tmpl"), []byte("global content"), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", filepath.Join(tmpDir, "global"))

	engine := NewEngine(WithPacksDir(packsDir), WithPack("express", "1.0.0"))

	_, source, err := engine.getTemplate("common/handler.tmpl")
	if err != nil {
		t.Fatalf("getTemplate failed: %v", err)
	}

	if source != "global" {
		t.Errorf("source = %q; want %q (global should override pack)", source, "global")
	}
}

func TestEngine_PackPrecedence_PackMissFallsBackToEmbedded(t *testing.T) {
	tmpDir := t.TempDir()
	packsDir := filepath.Join(tmpDir, "packs")

	// Create pack WITHOUT the template being requested.
	packTmp := filepath.Join(packsDir, "express@1.0.0", "templates", "common")
	if err := os.MkdirAll(packTmp, 0755); err != nil {
		t.Fatal(err)
	}
	// No go.mod.tmpl in the pack — only handler.tmpl.
	if err := os.WriteFile(filepath.Join(packTmp, "handler.tmpl"), []byte("pack handler"), 0644); err != nil {
		t.Fatal(err)
	}

	engine := NewEngine(WithPacksDir(packsDir), WithPack("express", "1.0.0"))

	// go.mod.tmpl is NOT in the pack, so it should fall through to embedded.
	_, source, err := engine.getTemplate("common/go.mod.tmpl")
	if err != nil {
		t.Fatalf("getTemplate failed: %v", err)
	}

	if source != "embedded" {
		t.Errorf("source = %q; want %q (pack miss should fall through to embedded)", source, "embedded")
	}

	// But handler.tmpl IS in the pack — should come from pack.
	_, source2, err := engine.getTemplate("common/handler.tmpl")
	if err != nil {
		t.Fatalf("getTemplate for handler.tmpl failed: %v", err)
	}

	want := "pack:express@1.0.0"
	if source2 != want {
		t.Errorf("source = %q; want %q (handler.tmpl should come from pack)", source2, want)
	}
}

func TestEngine_PackPrecedence_NoPackConfigured(t *testing.T) {
	// Without WithPack, behavior should be identical to 3-step chain.
	engine := NewEngine()

	_, source, err := engine.getTemplate("common/go.mod.tmpl")
	if err != nil {
		t.Fatalf("getTemplate failed: %v", err)
	}

	// Without local/global overrides, embedded wins.
	if source != "embedded" {
		t.Errorf("source = %q; want %q (no pack configured, embedded should win)", source, "embedded")
	}
}

func TestEngine_PackPrecedence_PackAsOnlySource(t *testing.T) {
	// Dynamic (no embedded) template: pack is the only source available.
	tmpDir := t.TempDir()
	packsDir := filepath.Join(tmpDir, "packs")

	packTmp := filepath.Join(packsDir, "express@1.0.0", "templates", "dynamic")
	if err := os.MkdirAll(packTmp, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packTmp, "page.tmpl"), []byte("pack dynamic page"), 0644); err != nil {
		t.Fatal(err)
	}

	engine := NewEngine(WithPacksDir(packsDir), WithPack("express", "1.0.0"))

	// "dynamic/page.tmpl" does NOT exist in embedded FS — only in pack.
	_, source, err := engine.getTemplate("dynamic/page.tmpl")
	if err != nil {
		t.Fatalf("getTemplate for dynamic/page.tmpl failed: %v", err)
	}

	want := "pack:express@1.0.0"
	if source != want {
		t.Errorf("source = %q; want %q", source, want)
	}
}

// ---------------------------------------------------------------------------
// ResolveBinary chain precedence (same as template: local > global > pack > embedded)
// ---------------------------------------------------------------------------

func TestEngine_ResolveBinary_LocalWins(t *testing.T) {
	tmpDir := t.TempDir()
	packsDir := filepath.Join(tmpDir, "packs")

	// Create local binary override.
	localAssetDir := filepath.Join(".go-arch", "assets")
	if err := os.MkdirAll(localAssetDir, 0755); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(".go-arch")
	if err := os.WriteFile(filepath.Join(localAssetDir, "lib.js"), []byte("local binary"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create pack binary (should NOT win).
	packAssetDir := filepath.Join(packsDir, "express@1.0.0", "assets")
	if err := os.MkdirAll(packAssetDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packAssetDir, "lib.js"), []byte("pack binary"), 0644); err != nil {
		t.Fatal(err)
	}

	engine := NewEngine(WithPacksDir(packsDir), WithPack("express", "1.0.0"))
	rs, err := engine.ResolveBinary("assets/lib.js")
	if err != nil {
		t.Fatalf("ResolveBinary failed: %v", err)
	}

	if rs.Kind != SourceLocal {
		t.Errorf("Kind = %v; want %v (local should win)", rs.Kind, SourceLocal)
	}

	data, err := rs.Read()
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if string(data) != "local binary" {
		t.Errorf("data = %q; want %q", string(data), "local binary")
	}
}

func TestEngine_ResolveBinary_GlobalWins(t *testing.T) {
	tmpDir := t.TempDir()
	packsDir := filepath.Join(tmpDir, "packs")

	// Create global binary override.
	globalAssetDir := filepath.Join(tmpDir, "global", ".go-arch", "assets")
	if err := os.MkdirAll(globalAssetDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(globalAssetDir, "lib.js"), []byte("global binary"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", filepath.Join(tmpDir, "global"))

	// Create pack binary (should NOT win).
	packAssetDir := filepath.Join(packsDir, "express@1.0.0", "assets")
	if err := os.MkdirAll(packAssetDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packAssetDir, "lib.js"), []byte("pack binary"), 0644); err != nil {
		t.Fatal(err)
	}

	engine := NewEngine(WithPacksDir(packsDir), WithPack("express", "1.0.0"))
	rs, err := engine.ResolveBinary("assets/lib.js")
	if err != nil {
		t.Fatalf("ResolveBinary failed: %v", err)
	}

	if rs.Kind != SourceGlobal {
		t.Errorf("Kind = %v; want %v (global should win)", rs.Kind, SourceGlobal)
	}

	data, err := rs.Read()
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if string(data) != "global binary" {
		t.Errorf("data = %q; want %q", string(data), "global binary")
	}
}

func TestEngine_ResolveBinary_PackWins(t *testing.T) {
	tmpDir := t.TempDir()
	packsDir := filepath.Join(tmpDir, "packs")

	// Create pack binary.
	packAssetDir := filepath.Join(packsDir, "express@1.0.0", "assets")
	if err := os.MkdirAll(packAssetDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packAssetDir, "lib.js"), []byte("pack binary"), 0644); err != nil {
		t.Fatal(err)
	}

	engine := NewEngine(WithPacksDir(packsDir), WithPack("express", "1.0.0"))
	rs, err := engine.ResolveBinary("assets/lib.js")
	if err != nil {
		t.Fatalf("ResolveBinary failed: %v", err)
	}

	if rs.Kind != SourcePack {
		t.Errorf("Kind = %v; want %v (pack should win when no local/global)", rs.Kind, SourcePack)
	}

	data, err := rs.Read()
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if string(data) != "pack binary" {
		t.Errorf("data = %q; want %q", string(data), "pack binary")
	}
}

func TestEngine_ResolveBinary_EmbeddedFallback(t *testing.T) {
	tmpDir := t.TempDir()
	packsDir := filepath.Join(tmpDir, "packs")

	engine := NewEngine(WithPacksDir(packsDir), WithPack("express", "1.0.0"))

	// This path doesn't exist anywhere — but embedded has it.
	// Use an actual embedded asset path. Since the embedded FS contains
	// templates/, we use a known embedded template as a binary test.
	rs, err := engine.ResolveBinary("common/go.mod.tmpl")
	if err != nil {
		t.Fatalf("ResolveBinary failed: %v", err)
	}

	if rs.Kind != SourceEmbedded {
		t.Errorf("Kind = %v; want %v (embedded fallback)", rs.Kind, SourceEmbedded)
	}

	data, err := rs.Read()
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if len(data) == 0 {
		t.Error("embedded read returned empty data")
	}
}

func TestEngine_ResolveBinary_NoPackConfig(t *testing.T) {
	engine := NewEngine()

	rs, err := engine.ResolveBinary("common/go.mod.tmpl")
	if err != nil {
		t.Fatalf("ResolveBinary failed: %v", err)
	}

	if rs.Kind != SourceEmbedded {
		t.Errorf("Kind = %v; want %v (no pack, embedded fallback)", rs.Kind, SourceEmbedded)
	}
}
