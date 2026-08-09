package packs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseRef(t *testing.T) {
	tests := []struct {
		name      string
		ref       string
		wantName  string
		wantVer   string
		wantErr   bool
		errSubstr string
	}{
		{
			name:     "name and version",
			ref:      "express@1.0.0",
			wantName: "express",
			wantVer:  "1.0.0",
		},
		{
			name:     "name only",
			ref:      "express",
			wantName: "express",
			wantVer:  "",
		},
		{
			name:      "empty",
			ref:       "",
			wantErr:   true,
			errSubstr: "empty",
		},
		{
			name:     "name with multiple @",
			ref:      "express@1.0.0@extra",
			wantName: "express",
			wantVer:  "1.0.0@extra",
		},
		{
			name:     "scoped name with version",
			ref:      "scoped-pack@2.1.3",
			wantName: "scoped-pack",
			wantVer:  "2.1.3",
		},
		{
			name:      "only version no name",
			ref:       "@1.0.0",
			wantErr:   true,
			errSubstr: "empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, gotVer, err := ParseRef(tt.ref)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errSubstr != "" && !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.errSubstr)) {
					t.Errorf("error = %v, want substring %q", err, tt.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotName != tt.wantName {
				t.Errorf("name = %q, want %q", gotName, tt.wantName)
			}
			if gotVer != tt.wantVer {
				t.Errorf("version = %q, want %q", gotVer, tt.wantVer)
			}
		})
	}
}

func TestLatestInstalled(t *testing.T) {
	t.Run("latest version wins", func(t *testing.T) {
		base := t.TempDir()
		t.Setenv("GO_ARCH_PACKS_DIR", base)

		// Create express@1.0.0 and express@1.1.0
		createPackDir(t, base, "express", "1.0.0")
		createPackDir(t, base, "express", "1.1.0")
		createPackDir(t, base, "echo", "0.5.0")

		ver, err := LatestInstalled("express")
		if err != nil {
			t.Fatalf("LatestInstalled: %v", err)
		}
		if ver != "1.1.0" {
			t.Errorf("version = %q, want %q", ver, "1.1.0")
		}
	})

	t.Run("single version", func(t *testing.T) {
		base := t.TempDir()
		t.Setenv("GO_ARCH_PACKS_DIR", base)

		createPackDir(t, base, "echo", "0.5.0")

		ver, err := LatestInstalled("echo")
		if err != nil {
			t.Fatalf("LatestInstalled: %v", err)
		}
		if ver != "0.5.0" {
			t.Errorf("version = %q, want %q", ver, "0.5.0")
		}
	})

	t.Run("pack not installed", func(t *testing.T) {
		base := t.TempDir()
		t.Setenv("GO_ARCH_PACKS_DIR", base)

		_, err := LatestInstalled("express")
		if err == nil {
			t.Fatal("expected error for uninstalled pack")
		}
		code := oopsCode(err)
		if code != CodePackNotInstalled {
			t.Errorf("oops code = %q, want %q", code, CodePackNotInstalled)
		}
	})

	t.Run("empty packs dir", func(t *testing.T) {
		base := t.TempDir()
		t.Setenv("GO_ARCH_PACKS_DIR", base)

		_, err := LatestInstalled("express")
		if err == nil {
			t.Fatal("expected error for empty packs dir")
		}
		code := oopsCode(err)
		if code != CodePackNotInstalled {
			t.Errorf("oops code = %q, want %q", code, CodePackNotInstalled)
		}
	})
}

func TestValidateSlug(t *testing.T) {
	tests := []struct {
		name    string
		slug    string
		wantErr bool
	}{
		{
			name:    "valid lowercase",
			slug:    "express",
			wantErr: false,
		},
		{
			name:    "valid with dashes",
			slug:    "my-pack",
			wantErr: false,
		},
		{
			name:    "valid single word",
			slug:    "echo",
			wantErr: false,
		},
		{
			name:    "invalid uppercase",
			slug:    "BadName",
			wantErr: true,
		},
		{
			name:    "invalid with space",
			slug:    "Bad Name!",
			wantErr: true,
		},
		{
			name:    "invalid with underscore",
			slug:    "my_pack",
			wantErr: true,
		},
		{
			name:    "invalid empty",
			slug:    "",
			wantErr: true,
		},
		{
			name:    "invalid numbers only",
			slug:    "123",
			wantErr: false,
		},
		{
			name:    "valid dash-separated numbers",
			slug:    "go-arch-express",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSlug(tt.slug)
			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestPath(t *testing.T) {
	base := "/home/user/.go-arch/packs"
	t.Setenv("GO_ARCH_PACKS_DIR", base)

	got := Path("express", "1.0.0")
	want := filepath.Join(base, "express@1.0.0")
	if got != want {
		t.Errorf("Path = %q, want %q", got, want)
	}
}

func TestBaseDir(t *testing.T) {
	custom := "/custom/packs/dir"
	t.Setenv("GO_ARCH_PACKS_DIR", custom)

	got := BaseDir()
	if got != custom {
		t.Errorf("BaseDir with env = %q, want %q", got, custom)
	}
}

func TestBaseDir_Default(t *testing.T) {
	// Unset the env so it falls back to default.
	t.Setenv("GO_ARCH_PACKS_DIR", "")

	got := BaseDir()
	if !strings.Contains(got, ".go-arch") || !strings.Contains(got, "packs") {
		t.Errorf("BaseDir default should contain .go-arch and packs, got: %s", got)
	}
}

// createPackDir creates a minimal pack directory to simulate installed state.
func createPackDir(t *testing.T, base, name, version string) {
	t.Helper()
	dir := filepath.Join(base, name+"@"+version)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
}
