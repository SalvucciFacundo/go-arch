// Package workspace implements multi-project (workspace) support for go-arch.
//
// A workspace is defined by a go-arch.workspace.yaml file at the monorepo
// root that maps service names to relative paths. Commands can then operate
// across the set (workspace upgrade, workspace check) or target a single
// service (--service <name>).
package workspace

import "path/filepath"

// Service is a single generated project inside a workspace.
type Service struct {
	Name     string `yaml:"name"`
	Path     string `yaml:"path"`
	Template string `yaml:"template,omitempty"`
}

// Workspace is a loaded workspace file plus the directory it lives in.
type Workspace struct {
	// Dir is the absolute directory containing the workspace file.
	// Service paths are relative to this directory.
	Dir      string
	Services []Service
}

// Find returns the service with the given name, or ok=false.
func (w *Workspace) Find(name string) (*Service, bool) {
	for i := range w.Services {
		if w.Services[i].Name == name {
			return &w.Services[i], true
		}
	}
	return nil, false
}

// ResolvePath returns the absolute path of a service's directory.
func (w *Workspace) ResolvePath(s *Service) string {
	return filepath.Join(w.Dir, filepath.FromSlash(s.Path))
}
