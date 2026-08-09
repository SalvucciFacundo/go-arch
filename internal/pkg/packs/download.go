package packs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"

	"github.com/samber/oops"
)

// ---------------------------------------------------------------------------
// Downloader interface
// ---------------------------------------------------------------------------

// Downloader fetches a pack module and returns the path to its local source
// directory. RealDownloader delegates to go mod download -json; FakeDownloader
// returns a canned directory for testing.
type Downloader interface {
	Download(ctx context.Context, module, version string) (dir string, err error)
}

// ---------------------------------------------------------------------------
// FakeDownloader — returns a configured directory or error.
// Defined in a non-test file so external test packages can inject it
// (same pattern as hooks.FakeRunner).
// ---------------------------------------------------------------------------

// FakeDownloader implements Downloader for testing.
type FakeDownloader struct {
	// Dir is the directory path to return on success.
	Dir string
	// Err is the error to return (overrides Dir).
	Err error
}

// Download returns f.Dir when f.Err is nil, otherwise f.Err.
func (f *FakeDownloader) Download(_ context.Context, _, _ string) (string, error) {
	if f.Err != nil {
		return "", f.Err
	}
	return f.Dir, nil
}

// ---------------------------------------------------------------------------
// RealDownloader — delegates to go mod download -json
// ---------------------------------------------------------------------------

// RealDownloader implements Downloader via the Go toolchain.
type RealDownloader struct{}

// downloadJSON is the subset of go mod download -json we care about.
type downloadJSON struct {
	Dir   string `json:"Dir"`
	Error string `json:"Error"`
}

// Download runs go mod download -json <module>@<version> and returns the
// module cache directory. It detects module-not-found from the JSON Error
// field and surfaces it as CodePackNotFound.
func (RealDownloader) Download(ctx context.Context, module, version string) (string, error) {
	var stdout, stderr bytes.Buffer
	mod := module + "@" + version

	cmd := exec.CommandContext(ctx, "go", "mod", "download", "-json", mod)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	// Parse JSON output even on error — go mod download writes structured
	// error info to stdout with an "Error" field.
	var result downloadJSON
	if decErr := json.Unmarshal(stdout.Bytes(), &result); decErr != nil && err != nil {
		// JSON parsing failed AND command failed — surface the exec error.
		return "", oops.
			Code(CodePackFetchFailed).
			Wrapf(err, "go mod download -json %s: %s", mod, stderr.String())
	}

	// Check structured error from JSON.
	if result.Error != "" {
		return "", oops.
			Code(CodePackNotFound).
			Errorf("pack %q not found in module proxy: %s", mod, result.Error)
	}

	if err != nil {
		return "", oops.
			Code(CodePackFetchFailed).
			Wrapf(err, "go mod download -json %s: %s", mod, stderr.String())
	}

	if result.Dir == "" {
		return "", fmt.Errorf("go mod download -json %s: empty Dir in output", mod)
	}

	return result.Dir, nil
}
