package hooks

import (
	"bytes"
	"io"
	"os"
	"testing"
)

// TestIntegration_NoStdoutInMCPMode verifies that hook output is routed
// exclusively through ui.Out and never leaks to os.Stdout.
//
// In MCP mode ui.Out is redirected to os.Stderr so the JSON-RPC stream
// (stdout) stays clean. This test pipes os.Stdout and asserts zero bytes
// were written to it during a successful hook run.
func TestIntegration_NoStdoutInMCPMode(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Save original stdout, replace with a pipe for the duration of the test.
	origStdout := os.Stdout
	defer func() { os.Stdout = origStdout }()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	os.Stdout = w

	// ui.Out is a separate buffer — in MCP mode this goes to stderr.
	out := &bytes.Buffer{}

	cfg := &Config{Hooks: map[Type][]Entry{
		PreNew: {{Command: "echo", Args: []string{"hello-from-hook"}, Shell: false}},
	}}

	rnn := NewRunner(cfg, RealRunner{}, out)
	err = rnn.Fire(PreNew, EnvContext{ProjectName: "test", ProjectPath: "/tmp/test", Arch: "Standard", HookType: PreNew}, "/tmp")
	if err != nil {
		t.Fatalf("Fire failed: %v", err)
	}

	// Close the write end so ReadAll returns.
	w.Close()

	var stdoutData bytes.Buffer
	_, copyErr := io.Copy(&stdoutData, r)
	if copyErr != nil {
		t.Fatalf("failed to read piped stdout: %v", copyErr)
	}

	if len(stdoutData.Bytes()) > 0 {
		t.Errorf("hook wrote %d bytes to os.Stdout, want 0: %q", stdoutData.Len(), stdoutData.String())
	}

	// The ui.Out buffer should contain the hook output.
	if out.Len() == 0 {
		t.Error("expected hook output in ui.Out buffer, got empty")
	}
}
