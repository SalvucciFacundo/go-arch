package scaffold

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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
	OriginGenerator  Origin = "generator"
	OriginTemplate   Origin = "template"
)

// ManifestEntry records one generated file's fingerprint and provenance.
type ManifestEntry struct {
	Path         string            `yaml:"path"`
	SHA256       string            `yaml:"sha256"`
	Origin       Origin            `yaml:"origin"`
	TemplatePath string            `yaml:"template,omitempty"`
	Source       string            `yaml:"source,omitempty"`
	Metadata     map[string]string `yaml:"metadata,omitempty"`
}

// RouteEntry represents one route registration in the manifest.
type RouteEntry struct {
	Entity       string `yaml:"entity"`                  // e.g. "User"
	Handler      string `yaml:"handler"`                 // e.g. "User" (for NewUserHandler)
	Origin       string `yaml:"origin"`                  // "crud" or "handler"
	RoutePattern string `yaml:"route_pattern,omitempty"` // e.g. "GET /stats" (handler only)
}

// Manifest is the ownership source of truth for scaffold-generated files.
type Manifest struct {
	Version int                      `yaml:"version"`
	Files   map[string]ManifestEntry `yaml:"files"`
	Routes  []RouteEntry             `yaml:"routes,omitempty"` // ADDITIVE
	dir     string                   `yaml:"-"`                // project root (not serialized)
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

// ManifestExists reports whether a manifest file exists on disk.
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

// UpsertRoute upserts a route entry by entity (dedupe) and saves the manifest.
func (m *Manifest) UpsertRoute(entry RouteEntry) error {
	for i, r := range m.Routes {
		if r.Entity == entry.Entity {
			m.Routes[i] = entry
			return m.Save()
		}
	}
	m.Routes = append(m.Routes, entry)
	return m.Save()
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

// recordManifestWarning prints a non-fatal warning to stderr.
func recordManifestWarning(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "warning: "+format+"\n", args...)
}
