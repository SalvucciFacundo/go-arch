package cmd

import (
	"fmt"
	"go-arch/internal/ui"
	"os/exec"
	"runtime"

	"github.com/samber/oops"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
	RootCmd.AddCommand(doctorCmd)
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Diagnose the Go development environment",
	Long:  `Checks the Go toolchain, air (hot-reload), git, and the current project configuration to surface any issues.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ui.Info("Running environment diagnostics...")
		fmt.Println()

		issues := 0
		checks := 0

		check := func(name, status string, ok bool) {
			checks++
			symbol := "✅"
			if !ok {
				symbol = "❌"
				issues++
			}
			fmt.Printf("%s %s: %s\n", symbol, name, status)
		}

		// 1. Go toolchain
		goVersion := "not found"
		if out, err := exec.Command("go", "version").Output(); err == nil {
			goVersion = string(out)
		}
		check("Go toolchain", goVersion, goVersion != "not found")

		// 2. Air (hot-reload)
		airInstalled := false
		if _, err := exec.LookPath("air"); err == nil {
			airInstalled = true
		}
		check("air (hot-reload)", map[bool]string{true: "installed", false: "not found — 'go-arch serve' falls back to 'go run'"}[airInstalled], airInstalled)

		// 3. Git
		gitInstalled := false
		if _, err := exec.LookPath("git"); err == nil {
			gitInstalled = true
		}
		check("git", map[bool]string{true: "installed", false: "not found"}[gitInstalled], gitInstalled)

		// 4. OS / arch
		check("Platform", fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH), true)

		// 5. Project configuration (only when in a go-arch project)
		fmt.Println()
		projectName := viper.GetString("project_name")
		if projectName == "" {
			check("Project config", "not in a go-arch project (no .go-arch.yaml found)", false)
		} else {
			architecture := viper.GetString("architecture")
			moduleName := viper.GetString("module_name")
			check("Project config", fmt.Sprintf("%s (%s) — %s", projectName, moduleName, architecture), true)
		}

		fmt.Println()
		if issues > 0 {
			return oops.
				Code("doctor_issues_found").
				Errorf("%d issue(s) found out of %d checks", issues, checks)
		}

		ui.Success(fmt.Sprintf("All %d checks passed.", checks))
		return nil
	},
}
