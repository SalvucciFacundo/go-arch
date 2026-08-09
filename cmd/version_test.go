package cmd

import (
	"bytes"
	"strings"
	"testing"
)

// TestVersionCommand verifies the version subcommand output.
// Runs inside one function because cobra RootCmd has global state
// that persists across test functions (same convention as generate_test.go:33-34).
func TestVersionCommand(t *testing.T) {
	t.Run("dev fallback", func(t *testing.T) {
		original := Version
		Version = "dev"
		defer func() { Version = original }()

		buf := new(bytes.Buffer)
		RootCmd.SetOut(buf)
		RootCmd.SetErr(buf)
		RootCmd.SetArgs([]string{"version"})

		if err := RootCmd.Execute(); err != nil {
			t.Fatalf("version command failed: %v", err)
		}

		if !strings.Contains(buf.String(), "dev") {
			t.Errorf("expected output to contain 'dev', got: %q", buf.String())
		}
	})

	t.Run("injected version", func(t *testing.T) {
		original := Version
		Version = "1.5.0"
		defer func() { Version = original }()

		buf := new(bytes.Buffer)
		RootCmd.SetOut(buf)
		RootCmd.SetErr(buf)
		RootCmd.SetArgs([]string{"version"})

		if err := RootCmd.Execute(); err != nil {
			t.Fatalf("version command failed: %v", err)
		}

		if !strings.Contains(buf.String(), "1.5.0") {
			t.Errorf("expected output to contain '1.5.0', got: %q", buf.String())
		}
	})
}
