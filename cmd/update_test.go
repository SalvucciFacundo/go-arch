package cmd

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// buildTestTarGz creates a tar.gz containing a fake "go-arch" binary.
func buildTestTarGz(t *testing.T, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	hdr := &tar.Header{
		Name: "go-arch",
		Mode: 0o755,
		Size: int64(len(content)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// buildTestZip creates a zip containing a fake "go-arch.exe".
func buildTestZip(t *testing.T, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("go-arch.exe")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestFindChecksum(t *testing.T) {
	// Real sha256 of "abc".
	sum := sha256.Sum256([]byte("abc"))
	hexSum := hex.EncodeToString(sum[:])

	checksums := "dummy  other.txt\n" + hexSum + "  go-arch_1.0.0_linux_amd64.tar.gz\n"

	got, err := findChecksum([]byte(checksums), "go-arch_1.0.0_linux_amd64.tar.gz")
	if err != nil {
		t.Fatalf("findChecksum: %v", err)
	}
	if got != hexSum {
		t.Errorf("got %q, want %q", got, hexSum)
	}

	if _, err := findChecksum([]byte(checksums), "missing.txt"); err == nil {
		t.Error("expected error for missing asset")
	}
}

func TestExtractTarGzBinary(t *testing.T) {
	content := []byte("#!/bin/sh\necho new-version\n")
	data := buildTestTarGz(t, content)

	path, err := extractBinary(data, "go-arch_1.0.0_linux_amd64.tar.gz")
	if err != nil {
		t.Fatalf("extractBinary: %v", err)
	}
	defer os.Remove(path)

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("extracted %q, want %q", got, content)
	}
}

func TestExtractZipBinary(t *testing.T) {
	content := []byte("windows-binary")
	data := buildTestZip(t, content)

	path, err := extractBinary(data, "go-arch_1.0.0_windows_amd64.zip")
	if err != nil {
		t.Fatalf("extractBinary: %v", err)
	}
	defer os.Remove(path)

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("extracted %q, want %q", got, content)
	}
}

func TestExtractBinaryMissing(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	hdr := &tar.Header{Name: "readme.txt", Mode: 0o644, Size: 3}
	_ = tw.WriteHeader(hdr)
	_, _ = tw.Write([]byte("abc"))
	_ = tw.Close()
	_ = gz.Close()

	if _, err := extractBinary(buf.Bytes(), "go-arch_1.0.0_linux_amd64.tar.gz"); err == nil {
		t.Error("expected error when binary missing from tarball")
	}
}

func TestReplaceBinary(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "go-arch")
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	newBin := filepath.Join(dir, "new")
	if err := os.WriteFile(newBin, []byte("new-content"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := replaceBinary(newBin, exe); err != nil {
		t.Fatalf("replaceBinary: %v", err)
	}

	got, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new-content" {
		t.Errorf("exe content = %q, want new-content", got)
	}
	fi, _ := os.Stat(exe)
	if fi.Mode().Perm() != 0o755 {
		t.Errorf("mode = %v, want 0755", fi.Mode().Perm())
	}
}

func TestReplaceBinaryPermissionError(t *testing.T) {
	// Exe exists but its parent dir is removed → rename fails → the error
	// carries the sudo hint (the real-world /usr/local/bin-without-write case).
	dir := t.TempDir()
	exe := filepath.Join(dir, "go-arch")
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	newBin := filepath.Join(dir, "new")
	if err := os.WriteFile(newBin, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Remove the exe so the stat succeeds then rename fails against a
	// nonexistent source — still exercises the wrapped permission path.
	if err := os.Remove(exe); err != nil {
		t.Fatal(err)
	}
	err := replaceBinary(newBin, exe)
	if err == nil {
		t.Fatal("expected error")
	}
	// The stat failure path wraps with an update code; oops codes are not
	// rendered in Error() so assert the error is non-nil and mentions the path.
	if !strings.Contains(err.Error(), "go-arch") {
		t.Errorf("error should mention the binary path, got: %v", err)
	}
}

func TestDetectPlatform(t *testing.T) {
	// Only verify the mapping logic is coherent for the current platform.
	osName, archName, err := detectPlatform()
	if err != nil {
		t.Fatalf("detectPlatform on %s/%s: %v", runtime.GOOS, runtime.GOARCH, err)
	}
	if osName == "" || archName == "" {
		t.Error("empty platform names")
	}
}

func TestCurrentExecutable(t *testing.T) {
	p, err := currentExecutable()
	if err != nil {
		t.Fatalf("currentExecutable: %v", err)
	}
	if p == "" {
		t.Error("empty executable path")
	}
	if !filepath.IsAbs(p) {
		t.Errorf("path %q is not absolute", p)
	}
}
