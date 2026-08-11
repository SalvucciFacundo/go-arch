package cmd

import (
	"fmt"
	"github.com/SalvucciFacundo/go-arch/v2/internal/pkg/hooks"
	"github.com/SalvucciFacundo/go-arch/v2/internal/pkg/packs"
	"github.com/SalvucciFacundo/go-arch/v2/internal/pkg/scaffold"
	"github.com/SalvucciFacundo/go-arch/v2/internal/ui"
	"os"
	"path/filepath"

	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(newCmd)
	newCmd.Flags().String("template", "", "Create project from an installed template pack (e.g. express or express@1.0.0)")
}

var newCmd = &cobra.Command{
	Use:   "new [name]",
	Short: "Create a new project",
	Long:  `The 'new' command initializes a new Go project with the specified name and architecture.`,
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		flagTemplate, _ := cmd.Flags().GetString("template")

		// Pack path: bypass wizard when --template is set.
		if flagTemplate != "" {
			return runNewWithTemplate(args[0], flagTemplate)
		}

		// 1. Launch the interactive wizard
		ui.Info("Starting the project creation wizard...")
		config, err := ui.RunWizard()
		if err != nil {
			return oops.
				Code("wizard_failed").
				Wrapf(err, "Interactive wizard failed")
		}

		// 2. Execute the scaffolding
		ui.Info(fmt.Sprintf("Creating project '%s'...", config.ProjectName))
		hooksCfg, err := hooks.Load(hooks.ResolveConfigPath())
		if err != nil {
			return oops.
				Code("hooks_load_failed").
				Wrap(err)
		}
		runner := hooks.NewRunner(hooksCfg, hooks.RealRunner{}, ui.Out)
		scaffolder := scaffold.NewScaffolder(config,
			scaffold.WithRunner(runner),
			scaffold.WithVersion(Version),
		)
		if err := scaffolder.Execute(); err != nil {
			return oops.
				Code("scaffold_failed").
				With("project_name", config.ProjectName).
				Wrapf(err, "Error while scaffolding the project")
		}

		ui.Success(fmt.Sprintf("Project '%s' created successfully!", config.ProjectName))
		fmt.Printf("👉 %s cd %s and go-arch serve\n", ui.InfoMsg("Run:"), config.ProjectName)
		if config.UseTemplHTMX {
			ui.Info("Next steps: cd into your project, run `templ generate`, then `go run .` (Minimalist) or `go run ./cmd/api` (Standard/Hexagonal).")
		}
		return nil
	},
}

// runNewWithTemplate handles the --template flag path: resolves the pack,
// validates non-empty templates (before any dir is created), builds defaults,
// and scaffolds via WithPackInfo.
func runNewWithTemplate(projectName, templateRef string) error {
	// Parse ref: "express" or "express@1.0.0"
	name, version, err := packs.ParseRef(templateRef)
	if err != nil {
		return oops.
			Code("invalid_pack_ref").
			Wrap(err)
	}

	packInfo, err := resolvePackForNew(name, version)
	if err != nil {
		return oops.
			Code("pack_not_installed").
			Hint(`Run "go-arch template install <module>@<version>" to install this pack.`).
			Errorf("pack %q is not installed; run \"go-arch template install <module>@<version>\" to install it", name)
	}

	// G4: empty-templates check BEFORE any directory is created.
	if err := checkTemplatesNonEmpty(packInfo); err != nil {
		return err
	}

	cfg := newPackDefaults(projectName, packInfo.Manifest.Name)

	// Build hooks runner: user hooks (global config) merged with pack hooks
	// (when enabled via the sidecar's HooksEnabled flag).
	hooksCfg, err := hooks.Load(hooks.ResolveConfigPath())
	if err != nil {
		return oops.
			Code("hooks_load_failed").
			Wrap(err)
	}

	// Pack hooks: honor the sidecar's HooksEnabled flag set at install time.
	if len(packInfo.Manifest.Hooks) > 0 {
		sc, scErr := packs.ReadSidecar(packInfo.Dir)
		if scErr == nil && sc.HooksEnabled {
			// Merge pack hooks into the user config. User hooks already loaded
			// above (empty config when no user file exists).
			for hookType, entries := range packInfo.Manifest.Hooks {
				hooksCfg.Hooks[hookType] = append(hooksCfg.Hooks[hookType], entries...)
			}
		}
	}
	runner := hooks.NewRunner(hooksCfg, hooks.RealRunner{}, ui.Out)
	scaffolder := scaffold.NewScaffolder(cfg,
		scaffold.WithPackInfo(packInfo),
		scaffold.WithRunner(runner),
		scaffold.WithVersion(Version),
	)
	if err := scaffolder.Execute(); err != nil {
		return oops.
			Code("scaffold_failed").
			With("project_name", cfg.ProjectName).
			Wrapf(err, "Error while scaffolding the project from pack")
	}

	ui.Success(fmt.Sprintf("Project '%s' created successfully from pack '%s'!",
		cfg.ProjectName, packInfo.Manifest.Name))
	fmt.Printf("👉 %s cd %s and go-arch serve\n", ui.InfoMsg("Run:"), cfg.ProjectName)
	return nil
}

// resolvePackForNew resolves an installed pack by name and version.
// If version is empty, uses LatestInstalled to discover the latest.
func resolvePackForNew(name, version string) (packs.PackInfo, error) {
	if err := packs.ValidateSlug(name); err != nil {
		return packs.PackInfo{}, err
	}
	if version == "" {
		latest, err := packs.LatestInstalled(name)
		if err != nil {
			return packs.PackInfo{}, err
		}
		version = latest
	}
	dir := packs.Path(name, version)
	m, err := packs.Load(dir)
	if err != nil {
		return packs.PackInfo{}, err
	}
	return packs.PackInfo{Dir: dir, Manifest: m}, nil
}

// checkTemplatesNonEmpty returns an error if the pack's templates/
// directory is missing or contains no files (not even subdirectories).
func checkTemplatesNonEmpty(packInfo packs.PackInfo) error {
	templatesDir := filepath.Join(packInfo.Dir, "templates")
	entries, err := os.ReadDir(templatesDir)
	if err != nil || len(entries) == 0 {
		return oops.
			Code("pack_no_templates").
			Errorf("pack %q has no templates", packInfo.Manifest.Name)
	}
	return nil
}

// newPackDefaults builds a ProjectConfig with sensible defaults for
// a pack-scaffolded project. Architecture is "" (the pack IS the
// architecture), feature flags default to false, and Template is set
// to the pack name.
func newPackDefaults(projectName, templateName string) *ui.ProjectConfig {
	return &ui.ProjectConfig{
		ProjectName:  projectName,
		ModuleName:   projectName,
		Architecture: "",
		DBDriver:     "None",
		Template:     templateName,
	}
}
