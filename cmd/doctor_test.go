package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

// TestDoctorCommand verifies the doctor command executes and reports the
// expected checks. Running outside a go-arch project surfaces the project
// config check as a finding and returns a non-zero exit (via RunE error).
func TestDoctorCommand(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "doctor-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	// No .go-arch.yaml here: doctor must fail with doctor_issues_found.
	err = doctorCmd.RunE(doctorCmd, nil)
	if err == nil {
		t.Fatal("expected doctor to report an issue when not in a go-arch project")
	}
}

// TestDoctorCommandInProject verifies doctor passes all checks when run
// inside a valid go-arch project.
func TestDoctorCommandInProject(t *testing.T) {
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

	// viper needs the config path reset for the temp dir.
	viper.Reset()
	viper.AddConfigPath(".")
	viper.SetConfigName(".go-arch")
	if err := viper.ReadInConfig(); err != nil {
		t.Fatal(err)
	}

	err = doctorCmd.RunE(doctorCmd, nil)
	if err != nil {
		t.Fatalf("expected doctor to pass in a valid project, got: %v", err)
	}
}
