package packs

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/samber/oops"
	"go-arch/internal/pkg/generators"
	"go-arch/internal/pkg/hooks"
	"gopkg.in/yaml.v3"
)

// SupportedContractVersions is the set of pack contract versions this CLI accepts.
var SupportedContractVersions = []int{1, 2}

// BinaryAsset describes a binary file to copy from the pack to the project.
type BinaryAsset struct {
	Source string `yaml:"source"`
	Target string `yaml:"target"`
}

// Manifest is the parsed go-arch.yaml from a pack root.
type Manifest struct {
	ContractVersion int                             `yaml:"contract_version"`
	Name            string                          `yaml:"name"`
	Version         string                          `yaml:"version"`
	Layout          []string                        `yaml:"layout,omitempty"`
	Hooks           map[hooks.Type][]hooks.Entry    `yaml:"hooks,omitempty"`
	BinaryAssets    []BinaryAsset                   `yaml:"binary_assets,omitempty"`
	Generators      map[string]generators.Generator `yaml:"generators,omitempty"`
}

// slugRE matches pack names: lowercase alphanumeric + dashes only.
var slugRE = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// semverRE matches a valid semver (no leading "v").
// Accepts: MAJOR.MINOR.PATCH, MAJOR.MINOR.PATCH-prerelease, MAJOR.MINOR.PATCH+build
var semverRE = regexp.MustCompile(`^\d+\.\d+\.\d+(-[\w.]+)?(\+[\w.]+)?$`)

// knownManifestKeys is the set of valid top-level YAML keys in a pack manifest.
var knownManifestKeys = map[string]bool{
	"contract_version": true,
	"name":             true,
	"version":          true,
	"layout":           true,
	"hooks":            true,
	"binary_assets":    true,
	"generators":       true,
}

// UnmarshalYAML implements strict deserialization with validation.
//
// Rejects unknown top-level keys, validates contract_version, name (slug),
// and version (semver). Hooks are decoded using the hooks.Entry unmarshaler.
func (m *Manifest) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.MappingNode {
		return oops.
			Code(CodeInvalidPackManifest).
			Errorf("pack manifest: expected a YAML mapping")
	}

	// Track which keys we've seen.
	seen := make(map[string]bool)

	for i := 0; i < len(value.Content); i += 2 {
		keyNode := value.Content[i]
		valNode := value.Content[i+1]
		key := keyNode.Value

		if !knownManifestKeys[key] {
			return oops.
				Code(CodeInvalidPackManifest).
				Errorf("pack manifest: unknown key %q", key)
		}
		seen[key] = true

		switch key {
		case "contract_version":
			if valNode.Kind != yaml.ScalarNode {
				return oops.
					Code(CodeInvalidPackManifest).
					Errorf("pack manifest: contract_version must be an integer")
			}
			if err := valNode.Decode(&m.ContractVersion); err != nil {
				return oops.
					Code(CodeInvalidPackManifest).
					Wrapf(err, "pack manifest: contract_version")
			}

		case "name":
			if valNode.Kind != yaml.ScalarNode {
				return oops.
					Code(CodeInvalidPackManifest).
					Errorf("pack manifest: name must be a string")
			}
			m.Name = valNode.Value

		case "version":
			if valNode.Kind != yaml.ScalarNode {
				return oops.
					Code(CodeInvalidPackManifest).
					Errorf("pack manifest: version must be a string")
			}
			m.Version = valNode.Value

		case "layout":
			if err := valNode.Decode(&m.Layout); err != nil {
				return oops.
					Code(CodeInvalidPackManifest).
					Wrapf(err, "pack manifest: layout")
			}

		case "hooks":
			if err := valNode.Decode(&m.Hooks); err != nil {
				return oops.
					Code(CodeInvalidPackManifest).
					Wrapf(err, "pack manifest: hooks")
			}

		case "binary_assets":
			if err := valNode.Decode(&m.BinaryAssets); err != nil {
				return oops.
					Code(CodeInvalidPackManifest).
					Wrapf(err, "pack manifest: binary_assets")
			}

		case "generators":
			if err := valNode.Decode(&m.Generators); err != nil {
				return oops.
					Code(CodeInvalidPackManifest).
					Wrapf(err, "pack manifest: generators")
			}
		}
	}

	// Required field validation.
	if !seen["contract_version"] {
		return oops.
			Code(CodeInvalidPackManifest).
			Errorf("pack manifest: missing required field \"contract_version\"")
	}
	if !supportsContractVersion(m.ContractVersion) {
		return oops.
			Code(CodeContractVersionMismatch).
			Errorf("pack %q requires contract v%d; this CLI supports contract v1–v2. Upgrade go-arch.",
				m.Name, m.ContractVersion)
	}

	if !seen["name"] {
		return oops.
			Code(CodeInvalidPackManifest).
			Errorf("pack manifest: missing required field \"name\"")
	}
	if m.Name == "" {
		return oops.
			Code(CodeInvalidPackManifest).
			Errorf("pack manifest: name must not be empty")
	}
	if !slugRE.MatchString(m.Name) {
		return oops.
			Code(CodeInvalidPackManifest).
			Errorf("pack manifest: name %q is not a valid slug (lowercase alphanumeric + dashes)", m.Name)
	}

	if !seen["version"] {
		return oops.
			Code(CodeInvalidPackManifest).
			Errorf("pack manifest: missing required field \"version\"")
	}
	if m.Version == "" {
		return oops.
			Code(CodeInvalidPackManifest).
			Errorf("pack manifest: version must not be empty")
	}
	if !semverRE.MatchString(m.Version) {
		return oops.
			Code(CodeInvalidPackManifest).
			Errorf("pack manifest: version %q is not a valid semver", m.Version)
	}

	// v1 packs must not declare generators (unknown to v1 contract).
	if m.ContractVersion == 1 && len(m.Generators) > 0 {
		return oops.
			Code(CodeInvalidPackManifest).
			Errorf("pack manifest: unknown key \"generators\"")
	}

	return nil
}

// supportsContractVersion returns true if v is in the supported set.
func supportsContractVersion(v int) bool {
	for _, sv := range SupportedContractVersions {
		if sv == v {
			return true
		}
	}
	return false
}

// Load reads and validates a go-arch.yaml at the given directory.
//
// Returns a parsed Manifest or an error with an oops code describing
// the validation failure.
func Load(dir string) (*Manifest, error) {
	path := filepath.Join(dir, "go-arch.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, oops.
				Code(CodeInvalidPackManifest).
				Errorf("pack manifest not found at %s", path)
		}
		return nil, fmt.Errorf("reading pack manifest: %w", err)
	}

	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}
