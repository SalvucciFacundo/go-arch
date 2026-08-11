package scaffold

import "github.com/SalvucciFacundo/go-arch/v2/internal/pkg/packs"

// Resolver locates an installed pack by name and version.
// Tests inject a fake resolver that returns synthetic PackInfo
// without requiring the full install machinery on disk.
type Resolver interface {
	Resolve(name, version string) (packs.PackInfo, error)
}

// DefaultResolver is the production resolver backed by real installed packs.
type DefaultResolver struct{}

// Resolve locates a pack directory on disk via packs.Path.
// If version is empty, calls packs.LatestInstalled to discover the latest.
func (DefaultResolver) Resolve(name, version string) (packs.PackInfo, error) {
	if version == "" {
		latest, err := packs.LatestInstalled(name)
		if err != nil {
			return packs.PackInfo{}, err
		}
		version = latest
	}
	dir := packs.Path(name, version)
	m, err := packs.Load(dir)
	if err != nil {
		return packs.PackInfo{}, err
	}
	return packs.PackInfo{Dir: dir, Manifest: m}, nil
}
