package workspace

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/samber/oops"
	"gopkg.in/yaml.v3"
)

// slugRe matches a workspace service name: lowercase alphanumeric, internal
// single dashes, no leading/trailing dash (mirrors packs slug validation).
var slugRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// Load reads and validates a workspace file at path.
//
// The workspace file uses a strict YAML schema: unknown top-level keys and
// unknown service keys are rejected (yaml.v3 Decoder.KnownFields(true)).
// Service names must be valid slugs and unique; paths are required.
func Load(path string) (*Workspace, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, oops.
				Code("workspace_not_found").
				Hint("Run inside a monorepo with a go-arch.workspace.yaml, or pass --workspace <path>").
				Errorf("workspace file not found: %s", path)
		}
		return nil, oops.
			Code("workspace_invalid").
			Wrapf(err, "cannot read workspace file %s", path)
	}

	var raw struct {
		Services []Service `yaml:"services"`
	}
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&raw); err != nil {
		return nil, oops.
			Code("workspace_invalid").
			Hint("The workspace file has an unknown key or a structural YAML error").
			Wrapf(err, "invalid workspace file %s", path)
	}

	if len(raw.Services) == 0 {
		return nil, oops.
			Code("workspace_invalid").
			Hint("Declare at least one service in the workspace file").
			Errorf("workspace %s has no services", path)
	}

	ws := &Workspace{
		Dir:      filepath.Dir(path),
		Services: raw.Services,
	}

	seen := make(map[string]struct{}, len(ws.Services))
	for i := range ws.Services {
		svc := &ws.Services[i]
		if err := validateService(svc); err != nil {
			return nil, err
		}
		if _, dup := seen[svc.Name]; dup {
			return nil, oops.
				Code("service_duplicate").
				Hint("Service names must be unique in a workspace").
				Errorf("duplicate service name %q", svc.Name)
		}
		seen[svc.Name] = struct{}{}
	}

	return ws, nil
}

func validateService(svc *Service) error {
	if svc.Name == "" {
		return oops.
			Code("workspace_invalid").
			Hint("Each service needs a name").
			Errorf("service at index missing name")
	}
	if !slugRe.MatchString(svc.Name) {
		return oops.
			Code("workspace_invalid").
			Hint("Service names must be lowercase alphanumeric with internal dashes").
			Errorf("invalid service name %q", svc.Name)
	}
	if svc.Path == "" {
		return oops.
			Code("workspace_invalid").
			Hint("Each service needs a path relative to the workspace file").
			Errorf("service %q missing path", svc.Name)
	}
	return nil
}
