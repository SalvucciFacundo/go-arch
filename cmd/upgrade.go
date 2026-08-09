package cmd

import (
	"fmt"
	"go-arch/internal/pkg/scaffold"
	"go-arch/internal/ui"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/term"
)

func init() {
	RootCmd.AddCommand(upgradeCmd)
	upgradeCmd.Flags().Bool("dry-run", true, "Print plan only, do not apply changes (default)")
	upgradeCmd.Flags().Bool("yes", false, "Apply all upgradable files without prompting")
	upgradeCmd.Flags().String("project-path", "", "Override project root directory")
}

var upgradeCmd = &cobra.Command{
	Use:           "upgrade",
	Short:         "Propagate embedded template changes via the fingerprint manifest",
	Long:          `The 'upgrade' command re-renders every file recorded in the fingerprint manifest, classifies changes (upgradable / protected / absent), and applies upgradable files when --yes is supplied. Legacy projects without a manifest fall back to a static whitelist with per-file confirmation.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		dryRunSet := cmd.Flags().Changed("dry-run")
		yesSet := cmd.Flags().Changed("yes")

		// Mutual exclusion: both explicitly supplied → usage error.
		// Using Changed() instead of value comparison because dry-run defaults
		// to true, so a value check would always see both as "true".
		if dryRunSet && yesSet {
			return oops.Code("invalid_flags").
				Hint("Use --dry-run to preview, or --yes to apply. They are mutually exclusive.").
				Errorf("--dry-run and --yes are mutually exclusive")
		}

		projectPath, _ := cmd.Flags().GetString("project-path")
		if projectPath != "" {
			if err := os.Chdir(projectPath); err != nil {
				return oops.Code("invalid_project_path").
					Hint("Check that the path exists and is a directory").
					Errorf("cannot change to %s: %v", projectPath, err)
			}

			// Re-read viper from the new working directory (initConfig
			// already read from CWD — but we just chdir'd to the target).
			viper.Reset()
			viper.AddConfigPath(".")
			viper.SetConfigName(".go-arch")
			if err := viper.ReadInConfig(); err != nil {
				return oops.Code("missing_config").
					Hint("Run 'go-arch setup' to initialize the project").
					Errorf("No valid configuration found. Are you in the root of a go-arch project?")
			}
		}

		// Validate config (missing_config pattern from check.go)
		projectName := viper.GetString("project_name")
		if projectName == "" {
			return oops.Code("missing_config").
				Hint("Run 'go-arch setup' to initialize the project").
				Errorf("No valid configuration found. Are you in the root of a go-arch project?")
		}

		cfg := configFromViper(projectName)

		plan, err := scaffold.Upgrade(cfg)
		if err != nil {
			return oops.Code("upgrade_failed").Wrapf(err, "Upgrade classification failed")
		}

		// Always display the plan to the command's output writer
		out := cmd.OutOrStdout()
		displayPlan(out, plan)

		yes, _ := cmd.Flags().GetBool("yes")
		isTTY := term.IsTerminal(int(os.Stdin.Fd()))

		// Decision: apply or plan-only (ADR-6)
		if !yes {
			// Non-TTY without --yes: plan only (spec: exit 0)
			if !isTTY {
				return nil
			}
			// TTY without --yes: for legacy, interactive per-file; for manifest, plan only
			if plan.IsLegacy {
				return applyLegacyInteractive(plan, cfg)
			}
			return nil // manifest project, default dry-run
		}

		// --yes: apply all upgradable
		if plan.CountBy(scaffold.ClassUpgradable) == 0 {
			fmt.Fprintf(out, "%s All files are up to date.\n", ansiSuccess())
			return nil
		}

		applied, err := plan.Apply()
		if err != nil {
			return oops.Code("upgrade_apply_failed").Wrapf(err, "Failed to apply upgrade")
		}

		// Surgical version write (ADR-4)
		configPath := ".go-arch.yaml"
		if err := scaffold.WriteVersionField(configPath, Version); err != nil {
			// Non-fatal: warn but don't fail
			fmt.Fprintf(out, "%s Could not update go_arch_version: %v\n", ansiWarning(), err)
		}

		fmt.Fprintf(out, "%s Applied %d update(s).\n", ansiSuccess(), applied)

		// templ generate hint (when views or style were upgraded)
		if plan.TemplHint {
			fmt.Fprintln(out, "💡 Run `templ generate` to recompile updated views.")
		}

		return nil
	},
}

// ──────────────────────────────────────────────────────────
// configFromViper maps viper config to ProjectConfig.
// ──────────────────────────────────────────────────────────

func configFromViper(projectName string) *ui.ProjectConfig {
	return &ui.ProjectConfig{
		ProjectName:          projectName,
		ModuleName:           viper.GetString("module_name"),
		Architecture:         viper.GetString("architecture"),
		DBDriver:             viper.GetString("db_driver"),
		UseDocker:            viper.GetBool("use_docker"),
		UseObservability:     viper.GetBool("use_observability"),
		ObservabilityBackend: viper.GetString("observability_backend"),
		UseGRPC:              viper.GetBool("use_grpc"),
		UseTemplHTMX:         viper.GetBool("use_templ_htmx"),
	}
}

// ──────────────────────────────────────────────────────────
// ANSI helpers (inline to avoid coupling to mgutz/ansi in tests)
// ──────────────────────────────────────────────────────────

func ansiSuccess() string { return "SUCCESS:" }
func ansiWarning() string { return "WARNING:" }

// ──────────────────────────────────────────────────────────
// displayPlan prints the upgrade plan grouped by classification.
// ──────────────────────────────────────────────────────────

func displayPlan(w io.Writer, plan *scaffold.UpgradePlan) {
	if plan.IsLegacy {
		fmt.Fprintf(w, "⚠️  Legacy project (no manifest found). Using whitelist fallback.\n")
		fmt.Fprintln(w)
	}

	upgradable := 0
	protected := 0
	absent := 0

	for _, f := range plan.Files {
		switch f.Classification {
		case scaffold.ClassUpgradable:
			upgradable++
			if f.Path == "go.mod" {
				fmt.Fprintf(w, "📦 %s: go.mod has updates (report-only — run suggested `go get` commands)\n", f.Path)
				printGoGetHints(w, f.RerenderBytes)
			} else {
				fmt.Fprintf(w, "🔄 %s: update available\n", f.Path)
			}
		case scaffold.ClassProtected:
			protected++
			fmt.Fprintf(w, "🔒 %s: user-modified (protected, skipping)\n", f.Path)
		case scaffold.ClassAbsent:
			absent++
			fmt.Fprintf(w, "❌ %s: absent on disk (not recreating)\n", f.Path)
		}
	}

	if upgradable == 0 && protected == 0 && absent == 0 {
		fmt.Fprintf(w, "%s All files are up to date.\n", ansiSuccess())
		return
	}

	fmt.Fprintf(w, "\nSummary: %d upgradable, %d protected, %d absent\n", upgradable, protected, absent)
}

// ──────────────────────────────────────────────────────────
// printGoGetHints extracts require lines from the template go.mod.
// ──────────────────────────────────────────────────────────

func printGoGetHints(w io.Writer, goModBytes []byte) {
	lines := strings.Split(string(goModBytes), "\n")
	inRequire := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "require (") {
			inRequire = true
			continue
		}
		if inRequire && line == ")" {
			inRequire = false
			continue
		}
		if inRequire && line != "" {
			parts := strings.Fields(line)
			if len(parts) >= 1 {
				fmt.Fprintf(w, "   go get %s@latest\n", parts[0])
			}
		}
		if strings.HasPrefix(line, "require ") && !strings.Contains(line, "(") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				fmt.Fprintf(w, "   go get %s@latest\n", parts[1])
			}
		}
	}
}

// ──────────────────────────────────────────────────────────
// applyLegacyInteractive prompts per file for legacy projects on TTY.
// ──────────────────────────────────────────────────────────

func applyLegacyInteractive(plan *scaffold.UpgradePlan, cfg *ui.ProjectConfig) error {
	applied := 0
	for i := range plan.Files {
		f := &plan.Files[i]
		if f.Classification != scaffold.ClassUpgradable {
			continue
		}
		if f.Path == "go.mod" {
			continue // report-only
		}
		var confirm bool
		prompt := &survey.Confirm{
			Message: fmt.Sprintf("Update %s?", f.Path),
			Default: false,
		}
		if err := survey.AskOne(prompt, &confirm); err != nil {
			return err
		}
		if !confirm {
			f.Classification = scaffold.ClassProtected // mark as skipped
			continue
		}
		fullPath := f.Path
		if plan.ProjectRoot != "." {
			fullPath = filepath.Join(plan.ProjectRoot, f.Path)
		}
		if err := os.WriteFile(fullPath, f.RerenderBytes, 0644); err != nil {
			return err
		}
		applied++
	}

	// Surgical version write
	configPath := ".go-arch.yaml"
	if plan.ProjectRoot != "." {
		configPath = filepath.Join(plan.ProjectRoot, ".go-arch.yaml")
	}
	_ = scaffold.WriteVersionField(configPath, Version)

	ui.Success(fmt.Sprintf("Applied %d update(s).", applied))

	if plan.TemplHint {
		fmt.Println("💡 Run `templ generate` to recompile updated views.")
	}
	return nil
}
