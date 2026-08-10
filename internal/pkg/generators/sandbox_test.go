package generators

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateTarget_Table(t *testing.T) {
	// Create a real directory tree that supports symlink and sibling tests.
	tmpDir := t.TempDir()

	// Project root: tmpDir/myapp
	rootDir := filepath.Join(tmpDir, "myapp")
	mustMkdir(t, filepath.Join(rootDir, "internal", "handler"))

	// Sibling directory: tmpDir/myapp-evil (created BEFORE root symlink)
	siblingPath := filepath.Join(tmpDir, "myapp-evil")
	mustMkdir(t, siblingPath)

	// Symlink inside root → external /tmp
	externalLink := filepath.Join(rootDir, "external_link")
	mustSymlink(t, os.TempDir(), externalLink)

	// Symlink inside root → sibling (myapp-evil)
	siblingLink := filepath.Join(rootDir, "sibling_link")
	mustSymlink(t, siblingPath, siblingLink)

	// Symlink inside root → absolute path outside project
	absLink := filepath.Join(rootDir, "abs_link")
	mustSymlink(t, "/etc", absLink)

	tests := []struct {
		name    string
		root    string
		relPath string
		wantErr bool
		errCode string
		errMsg  string
	}{
		{
			name:    "relative path inside project accepted",
			root:    rootDir,
			relPath: "internal/handler/handler.go",
			wantErr: false,
		},
		{
			name:    "absolute path starting with / rejected",
			root:    rootDir,
			relPath: "/etc/passwd",
			wantErr: true,
			errCode: CodeRecipePathEscape,
			errMsg:  "path escapes project root",
		},
		{
			name:    "dot-dot traversal rejected",
			root:    rootDir,
			relPath: "../../etc/shadow",
			wantErr: true,
			errCode: CodeRecipePathEscape,
		},
		{
			name:    "symlink escape to external directory",
			root:    rootDir,
			relPath: "external_link/evil",
			wantErr: true,
			errCode: CodeRecipePathEscape,
			errMsg:  "path escapes project root",
		},
		{
			name:    "sibling false-positive via symlink (separator-aware)",
			root:    rootDir,
			relPath: "sibling_link/file",
			wantErr: true,
			errCode: CodeRecipePathEscape,
			errMsg:  "path escapes project root",
		},
		{
			name:    "symlink to absolute path outside root",
			root:    rootDir,
			relPath: "abs_link/hosts",
			wantErr: true,
			errCode: CodeRecipePathEscape,
		},
		{
			name:    "root-adjacent child path accepted",
			root:    rootDir,
			relPath: "internal/handler/subdir/deep/file.go",
			wantErr: false,
		},
		{
			name:    "single component relative path accepted",
			root:    rootDir,
			relPath: "config.yaml",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTarget(tt.root, tt.relPath)

			if !tt.wantErr {
				if err != nil {
					t.Errorf("ValidateTarget(%q, %q) unexpected error: %v", tt.root, tt.relPath, err)
				}
				return
			}

			if err == nil {
				t.Errorf("ValidateTarget(%q, %q) expected error but got nil", tt.root, tt.relPath)
				return
			}

			// Check oops error code using errors.As.
			code := oopsCode(err)
			if code == "" {
				// Not an oops error — check if the message contains expected text.
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errMsg)
				}
				return
			}

			if code != tt.errCode {
				t.Errorf("error code = %q, want %q", code, tt.errCode)
			}
			if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.errMsg)
			}
		})
	}
}

func TestValidateTarget_CrossPlatform(t *testing.T) {
	// Test that filepath.VolumeName awareness works (Linux does not
	// have volume names, but the code path must be present).
	tmpDir := t.TempDir()
	rootDir := filepath.Join(tmpDir, "myapp")
	mustMkdir(t, rootDir)

	// Absolute path on any platform should be rejected.
	err := ValidateTarget(rootDir, string(os.PathSeparator)+"tmp"+string(os.PathSeparator)+"file.txt")
	if err == nil {
		t.Error("expected error for absolute path")
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatalf("os.MkdirAll(%q): %v", path, err)
	}
}

func mustSymlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("os.Symlink(%q, %q): %v", target, link, err)
	}
}
