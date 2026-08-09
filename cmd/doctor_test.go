package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

// TestDoctorOutsideProject verifies the doctor command reports an issue when
// not inside a go-arch project. The environment checks (go, air, git) depend
// on external tools, but the project-config check is deterministic and always
// fails outside a project, so RunE is guaranteed to return an error.
func TestDoctorOutsideProject(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "doctor-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	err = doctorCmd.RunE(doctorCmd, nil)
	if err == nil {
		t.Fatal("expected doctor to report an issue when not in a go-arch project")
	}
}

// TestProjectConfigStatusDetected verifies the pure project-config check
// detects a valid .go-arch.yaml. This is deterministic and does not depend on
// external tools.
func TestProjectConfigStatusDetected(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "doctor-proj-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	configYAML := `project_name: "doctor-test"
module_name: github.com/test/doctor
architecture: Standard
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".go-arch.yaml"), []byte(configYAML), 0644); err != nil {
		t.Fatal(err)
	}

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	viper.Reset()
	viper.AddConfigPath(".")
	viper.SetConfigName(".go-arch")
	if err := viper.ReadInConfig(); err != nil {
		t.Fatal(err)
	}

	status, ok := projectConfigStatus()
	if !ok {
		t.Fatalf("expected project config to be detected, got ok=false status=%q", status)
	}
	if !strings.Contains(status, "doctor-test") || !strings.Contains(status, "Standard") {
		t.Errorf("status should mention project name and architecture, got: %q", status)
	}
}

// TestProjectConfigStatusMissing verifies the pure project-config check
// reports a finding when no .go-arch.yaml exists.
func TestProjectConfigStatusMissing(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "doctor-noconf-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	viper.Reset()

	status, ok := projectConfigStatus()
	if ok {
		t.Fatalf("expected project config to be missing, got ok=true status=%q", status)
	}
	if !strings.Contains(status, "not in a go-arch project") {
		t.Errorf("status should explain the missing project, got: %q", status)
	}
}
