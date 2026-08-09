package packs

// PackInfo bundles a pack's on-disk location with its parsed manifest.
type PackInfo struct {
	Dir      string
	Manifest *Manifest
}
