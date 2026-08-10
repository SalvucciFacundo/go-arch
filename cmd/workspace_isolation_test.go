package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

// TestServiceHooksCWD verifies that hooks run inside the service directory when
// a command targets a service via --service (spec hooks-cwd).
func TestServiceHooksCWD(t *testing.T) {
	root, wsPath, svcDir := setupServiceGenerateEnv(t)

	// Add a post-generate hook to the service config that writes a marker.
	hookCfg := `project_name: orders
module_name: example.com/orders
architecture: Hexagonal
use_templ_htmx: true
hooks:
  post-generate:
    - command: "sh"
      args: ["-c", "echo hook-ran > hook-marker.txt"]
`
	if err := os.WriteFile(filepath.Join(svcDir, ".go-arch.yaml"), []byte(hookCfg), 0o644); err != nil {
		t.Fatal(err)
	}

	oldWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(oldWd) }()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	viper.Reset()
	defer viper.Reset()

	var buf strings.Builder
	generateCmd.SetOut(&buf)
	generateCmd.SetErr(&buf)
	_ = generateCmd.Flags().Set("service", "orders")
	_ = generateCmd.Flags().Set("workspace", wsPath)

	if err := generateCmd.RunE(generateCmd, []string{"service", "Order"}); err != nil {
		t.Fatalf("generate --service with hook: %v\n%s", err, buf.String())
	}

	// The hook must have run inside the service dir (marker there).
	marker := filepath.Join(svcDir, "hook-marker.txt")
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("hook marker not found at %s (err: %v) — hook ran in wrong CWD", marker, err)
	}
}

// TestServiceConfigIsolation verifies the service's .go-arch.yaml is used and
// the prior config is restored afterwards (spec config-isolation).
func TestServiceConfigIsolation(t *testing.T) {
	root, wsPath, svcDir := setupServiceGenerateEnv(t)

	// Different architecture in the service vs the monorepo root config.
	svcCfg := `project_name: orders
module_name: example.com/orders
architecture: Minimalist
`
	if err := os.WriteFile(filepath.Join(svcDir, ".go-arch.yaml"), []byte(svcCfg), 0o644); err != nil {
		t.Fatal(err)
	}
	rootCfg := `project_name: monorepo-root
architecture: Hexagonal
`
	if err := os.WriteFile(filepath.Join(root, ".go-arch.yaml"), []byte(rootCfg), 0o644); err != nil {
		t.Fatal(err)
	}

	oldWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(oldWd) }()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	viper.Reset()
	defer viper.Reset()
	// Simulate the startup config read (root config).
	viper.SetConfigName(".go-arch")
	viper.AddConfigPath(".")
	if err := viper.ReadInConfig(); err != nil {
		t.Fatal(err)
	}

	// Run a workspace operation that reloads the service config.
	ws, err := resolveWorkspace(wsPath)
	if err != nil {
		t.Fatal(err)
	}
	svc, _ := ws.Find("orders")
	if err := withService(ws, svc.Name, func() error {
		// Inside the service: the service's config must win.
		if got := viper.GetString("architecture"); got != "Minimalist" {
			t.Errorf("inside service: architecture = %q, want Minimalist", got)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// After restore: the previous (root) config must be visible again.
	if got := viper.GetString("architecture"); got != "Hexagonal" {
		t.Errorf("after restore: architecture = %q, want Hexagonal (config not restored)", got)
	}
}
