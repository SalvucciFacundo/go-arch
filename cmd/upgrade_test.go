package cmd

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"go-arch/internal/ui"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

// ──────────────────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────────────────

// ──────────────────────────────────────────────────────────
// 3.4: Help output lists upgrade command and flags
// ──────────────────────────────────────────────────────────

func TestUpgradeHelp(t *testing.T) {
	// Root help lists upgrade
	t.Run("root help lists upgrade", func(t *testing.T) {
		buf := new(bytes.Buffer)
		RootCmd.SetOut(buf)
		RootCmd.SetErr(buf)
		RootCmd.SetArgs([]string{"--help"})

		if err := RootCmd.Execute(); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(buf.String(), "upgrade") {
			t.Error("root help should mention upgrade subcommand")
		}
	})

	// Upgrade help describes flags
	t.Run("upgrade help describes flags", func(t *testing.T) {
		buf := new(bytes.Buffer)
		RootCmd.SetOut(buf)
		RootCmd.SetErr(buf)
		RootCmd.SetArgs([]string{"upgrade", "--help"})

		if err := RootCmd.Execute(); err != nil {
			t.Fatal(err)
		}
		out := buf.String()
		for _, flag := range []string{"--dry-run", "--yes", "--project-path"} {
			if !strings.Contains(out, flag) {
				t.Errorf("upgrade --help should mention %s flag\nGot: %s", flag, out)
			}
		}
	})
}

// sha256Sum computes the hex-encoded SHA-256 hash of data.
func sha256Sum(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// resetUpgradeState resets global state between tests.
func resetUpgradeState() {
	viper.Reset()
	upgradeCmd.ResetFlags()
	upgradeCmd.Flags().Bool("dry-run", true, "Print plan only, do not apply changes (default)")
	upgradeCmd.Flags().Bool("yes", false, "Apply all upgradable files without prompting")
	upgradeCmd.Flags().String("project-path", "", "Override project root directory")
}

// runUpgradeWithBuf executes the upgrade command with args, capturing all
// output to a buffer. Both cobra output and ui.Out go to buf.
func runUpgradeWithBuf(t *testing.T, args []string) (*bytes.Buffer, error) {
	t.Helper()

	buf := new(bytes.Buffer)
	RootCmd.SetOut(buf)
	RootCmd.SetErr(buf)

	// Also capture ui.Out (ui.Success, ui.Warning, etc.)
	oldUIOut := ui.Out
	ui.Out = buf
	defer func() { ui.Out = oldUIOut }()

	RootCmd.SetArgs(args)
	err := RootCmd.Execute()
	return buf, err
}

// ──────────────────────────────────────────────────────────
// 3.4: Conflict flags — --dry-run + --yes → usage error
// ──────────────────────────────────────────────────────────

func TestUpgradeConflictFlags(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	configYAML := `project_name: "conflict-test"
module_name: github.com/test/conflict
architecture: Standard
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".go-arch.yaml"), []byte(configYAML), 0644); err != nil {
		t.Fatal(err)
	}

	resetUpgradeState()

	_, err := runUpgradeWithBuf(t, []string{"upgrade", "--dry-run", "--yes"})
	if err == nil {
		t.Fatal("expected --dry-run + --yes to fail with usage error, got nil")
	}
	out := err.Error()
	if !strings.Contains(out, "mutually exclusive") && !strings.Contains(out, "dry-run") {
		t.Errorf("expected error about mutual exclusion, got: %v", out)
	}
}

// ──────────────────────────────────────────────────────────
// 3.4: Missing config → oops missing_config
// ──────────────────────────────────────────────────────────

func TestUpgradeMissingConfig(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	resetUpgradeState()

	_, err := runUpgradeWithBuf(t, []string{"upgrade"})
	if err == nil {
		t.Fatal("expected missing_config error, got nil")
	}

	out := err.Error()
	if !strings.Contains(out, "missing_config") && !strings.Contains(out, "config") {
		t.Errorf("expected missing_config error, got: %v", out)
	}
	if !strings.Contains(out, "go-arch project") {
		t.Errorf("expected hint about go-arch project, got: %v", out)
	}
}

// ──────────────────────────────────────────────────────────
// 3.4: Dry-run writes nothing (default behavior)
// ──────────────────────────────────────────────────────────

func TestUpgradeDryRunWritesNothing(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	configYAML := `project_name: "dryrun-test"
module_name: github.com/test/dryrun
architecture: Standard
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".go-arch.yaml"), []byte(configYAML), 0644); err != nil {
		t.Fatal(err)
	}

	envContent := "DB_HOST=localhost\nDB_PORT=5432\n"
	if err := os.WriteFile(filepath.Join(tmpDir, ".env"), []byte(envContent), 0644); err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Join(tmpDir, ".go-arch"), 0755); err != nil {
		t.Fatal(err)
	}
	envHash := sha256Sum([]byte(envContent))
	manifestYAML := `version: 1
files:
  .env:
    path: .env
    sha256: "` + envHash + `"
    origin: scaffold
    template: common/env.tmpl
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".go-arch", "manifest.yaml"), []byte(manifestYAML), 0644); err != nil {
		t.Fatal(err)
	}

	// Record pre-upgrade file state
	preContent, err := os.ReadFile(filepath.Join(tmpDir, ".env"))
	if err != nil {
		t.Fatal(err)
	}

	resetUpgradeState()

	buf, err := runUpgradeWithBuf(t, []string{"upgrade"})
	if err != nil {
		t.Fatalf("dry-run upgrade should exit 0, got: %v\nOutput: %s", err, buf.String())
	}

	// Verify file was NOT modified
	postContent, err := os.ReadFile(filepath.Join(tmpDir, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(preContent, postContent) {
		t.Error("dry-run mode should NOT modify files")
	}

	out := buf.String()
	if !strings.Contains(out, "update available") && !strings.Contains(out, "upgradable") {
		t.Errorf("output should show plan, got: %s", out)
	}
}

// ──────────────────────────────────────────────────────────
// 3.4: --yes applies upgradable, respects PROTECTED
// ──────────────────────────────────────────────────────────

func TestUpgradeYesApplies(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	configYAML := `project_name: "yes-test"
module_name: github.com/test/yesapp
architecture: Standard
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".go-arch.yaml"), []byte(configYAML), 0644); err != nil {
		t.Fatal(err)
	}

	// File 1: .env — hash MATCHES manifest → upgradable with v2 template
	envContent := "DB_HOST=localhost\n"
	if err := os.WriteFile(filepath.Join(tmpDir, ".env"), []byte(envContent), 0644); err != nil {
		t.Fatal(err)
	}

	// File 2: protected file — hash MISMATCHES manifest → PROTECTED
	protectedContent := "USER_CONTENT_HERE\n"
	if err := os.MkdirAll(filepath.Join(tmpDir, "internal", "adapters", "grpc"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "internal", "adapters", "grpc", "server.go"), []byte(protectedContent), 0644); err != nil {
		t.Fatal(err)
	}

	envHash := sha256Sum([]byte(envContent))
	if err := os.MkdirAll(filepath.Join(tmpDir, ".go-arch"), 0755); err != nil {
		t.Fatal(err)
	}
	manifestYAML := `version: 1
files:
  .env:
    path: .env
    sha256: "` + envHash + `"
    origin: scaffold
    template: common/env.tmpl
  internal/adapters/grpc/server.go:
    path: internal/adapters/grpc/server.go
    sha256: "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
    origin: scaffold
    template: common/grpc_server.tmpl
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".go-arch", "manifest.yaml"), []byte(manifestYAML), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a v2 template override for .env
	localDir := filepath.Join(tmpDir, ".go-arch", "templates", "common")
	if err := os.MkdirAll(localDir, 0755); err != nil {
		t.Fatal(err)
	}
	v2Env := "DB_HOST=upgraded-host\nDB_PORT=9999\n"
	if err := os.WriteFile(filepath.Join(localDir, "env.tmpl"), []byte(v2Env), 0644); err != nil {
		t.Fatal(err)
	}

	// Record pre-upgrade state of the protected file
	preProtected, err := os.ReadFile(filepath.Join(tmpDir, "internal", "adapters", "grpc", "server.go"))
	if err != nil {
		t.Fatal(err)
	}

	resetUpgradeState()

	buf, err := runUpgradeWithBuf(t, []string{"upgrade", "--yes"})
	if err != nil {
		t.Fatalf("upgrade --yes should exit 0, got: %v\nOutput: %s", err, buf.String())
	}

	// Verify .env was upgraded (now matches v2 template)
	envAfter, err := os.ReadFile(filepath.Join(tmpDir, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(envAfter, []byte(v2Env)) {
		t.Errorf(".env should have been upgraded.\nExpected: %q\nGot: %q", v2Env, string(envAfter))
	}

	// Verify protected file was NOT modified
	protectedAfter, err := os.ReadFile(filepath.Join(tmpDir, "internal", "adapters", "grpc", "server.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(preProtected, protectedAfter) {
		t.Error("PROTECTED file should NOT have been modified")
	}

	out := buf.String()
	if !strings.Contains(out, "Applied") {
		t.Errorf("output should report applied count\nGot: %s", out)
	}
	if !strings.Contains(out, "protected") {
		t.Errorf("output should report protected file\nGot: %s", out)
	}
}

// ──────────────────────────────────────────────────────────
// 3.4: Non-TTY plan-only (exit 0, no writes)
// ──────────────────────────────────────────────────────────

func TestUpgradeNonTTYPlanOnly(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	configYAML := `project_name: "non-tty-test"
module_name: github.com/test/nontty
architecture: Standard
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".go-arch.yaml"), []byte(configYAML), 0644); err != nil {
		t.Fatal(err)
	}

	envContent := "DB_HOST=localhost\n"
	if err := os.WriteFile(filepath.Join(tmpDir, ".env"), []byte(envContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create v2 template override → upgradable
	localDir := filepath.Join(tmpDir, ".go-arch", "templates", "common")
	if err := os.MkdirAll(localDir, 0755); err != nil {
		t.Fatal(err)
	}
	v2Env := "DB_HOST=override\n"
	if err := os.WriteFile(filepath.Join(localDir, "env.tmpl"), []byte(v2Env), 0644); err != nil {
		t.Fatal(err)
	}

	envHash := sha256Sum([]byte(envContent))
	if err := os.MkdirAll(filepath.Join(tmpDir, ".go-arch"), 0755); err != nil {
		t.Fatal(err)
	}
	manifestYAML := `version: 1
files:
  .env:
    path: .env
    sha256: "` + envHash + `"
    origin: scaffold
    template: common/env.tmpl
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".go-arch", "manifest.yaml"), []byte(manifestYAML), 0644); err != nil {
		t.Fatal(err)
	}

	preContent, err := os.ReadFile(filepath.Join(tmpDir, ".env"))
	if err != nil {
		t.Fatal(err)
	}

	resetUpgradeState()

	buf, err := runUpgradeWithBuf(t, []string{"upgrade"}) // no --yes → plan only in non-TTY
	if err != nil {
		t.Fatalf("non-TTY upgrade without --yes should exit 0, got: %v\nOutput: %s", err, buf.String())
	}

	// Verify file was NOT modified
	postContent, err := os.ReadFile(filepath.Join(tmpDir, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(preContent, postContent) {
		t.Error("non-TTY plan-only should NOT modify files")
	}

	out := buf.String()
	if !strings.Contains(out, "update available") && !strings.Contains(out, "upgradable") {
		t.Errorf("output should show plan, got: %s", out)
	}
}

// ──────────────────────────────────────────────────────────
// 3.4: Templ hint presence
// ──────────────────────────────────────────────────────────

func TestUpgradeTemplHint(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	configYAML := `project_name: "templ-test"
module_name: github.com/test/templhint
architecture: Standard
use_templ_htmx: true
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".go-arch.yaml"), []byte(configYAML), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a views template file on disk
	if err := os.MkdirAll(filepath.Join(tmpDir, "views", "layouts"), 0755); err != nil {
		t.Fatal(err)
	}
	baseContent := "package views\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "views", "layouts", "base.templ"), []byte(baseContent), 0644); err != nil {
		t.Fatal(err)
	}

	baseHash := sha256Sum([]byte(baseContent))
	if err := os.MkdirAll(filepath.Join(tmpDir, ".go-arch"), 0755); err != nil {
		t.Fatal(err)
	}
	manifestYAML := `version: 1
files:
  views/layouts/base.templ:
    path: views/layouts/base.templ
    sha256: "` + baseHash + `"
    origin: scaffold
    template: web/base.templ.tmpl
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".go-arch", "manifest.yaml"), []byte(manifestYAML), 0644); err != nil {
		t.Fatal(err)
	}

	// Create v2 template override → views file becomes upgradable
	localDir := filepath.Join(tmpDir, ".go-arch", "templates", "web")
	if err := os.MkdirAll(localDir, 0755); err != nil {
		t.Fatal(err)
	}
	v2Base := "package v2\n"
	if err := os.WriteFile(filepath.Join(localDir, "base.templ.tmpl"), []byte(v2Base), 0644); err != nil {
		t.Fatal(err)
	}

	resetUpgradeState()

	buf, err := runUpgradeWithBuf(t, []string{"upgrade", "--yes"})
	if err != nil {
		t.Fatalf("upgrade --yes should exit 0, got: %v\nOutput: %s", err, buf.String())
	}

	out := buf.String()
	if !strings.Contains(out, "templ generate") {
		t.Errorf("output should contain 'templ generate' hint\nGot: %s", out)
	}
	if !strings.Contains(out, "views/layouts/base.templ") {
		t.Errorf("output should mention the views file\nGot: %s", out)
	}
}
