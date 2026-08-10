package cmd

import (
	"fmt"
	"go-arch/internal/pkg/generators"
	"go-arch/internal/pkg/hooks"
	"go-arch/internal/pkg/packs"
	"go-arch/internal/pkg/scaffold"
	"go-arch/internal/ui"
	"sort"
	"strings"

	"github.com/samber/oops"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
	generateCmd.Flags().String("route", "", `Route pattern for handler type (e.g. "GET /stats"). CRUD auto-registers in web projects.`)
	generateCmd.Flags().Bool("list", false, "List available generators grouped by source")
	addServiceFlag(generateCmd)
	generateCmd.Flags().String("workspace", "", "path to go-arch.workspace.yaml (default: discover upward)")
	RootCmd.AddCommand(generateCmd)
}

var generateCmd = &cobra.Command{
	Use:   "generate [type] [name]",
	Short: "Generate a new component (service, repository, handler, crud, page, component)",
	Long: `Generate components for the project.

Backend types: service, repository, handler, crud.
Web types (require use_templ_htmx: true in .go-arch.yaml):
  page      → views/pages/<lowercase_name>.templ
  component → views/components/<lowercase_name>.templ (a templ component)

Generator resolution (three-tier):
  1. Pack generators — if .go-arch.yaml declares template: <pack> and the
     installed pack has a generator matching <type>.
  2. Builtin generators — CLI-registered builtins (none in v2 by default).
  3. Component types — the existing service, repository, handler, crud,
     page, component types.

Flags:
  --list                    List available generators grouped by source.
  --route "METHOD /path"    Register a route for the handler type in web projects
                            (e.g. --route "GET /stats"). CRUD auto-registers routes
                            in web projects by default.`,
	Args: func(cmd *cobra.Command, args []string) error {
		// --list flag takes 0 positional args, otherwise exactly 2.
		listFlag, _ := cmd.Flags().GetBool("list")
		if listFlag {
			if len(args) > 0 {
				return fmt.Errorf("no arguments allowed when using --list")
			}
			return nil
		}
		if len(args) != 2 {
			return fmt.Errorf("accepts 2 arg(s), received %d", len(args))
		}
		return nil
	},
	Aliases: []string{"g"},
	RunE: func(cmd *cobra.Command, args []string) error {
		listFlag, _ := cmd.Flags().GetBool("list")
		if listFlag {
			return runListGenerators(cmd)
		}

		compType := args[0]
		name := args[1]

		// --service: run the generation inside a workspace service.
		if svcName := getServiceFlag(cmd); svcName != "" {
			wsFlag, _ := cmd.Flags().GetString("workspace")
			ws, err := requireWorkspace(wsFlag)
			if err != nil {
				return err
			}
			return withService(ws, svcName, func() error {
				return runGenerate(cmd, compType, name)
			})
		}

		return runGenerate(cmd, compType, name)
	},
}

// runGenerate runs the standard generate dispatch for the current directory.
func runGenerate(cmd *cobra.Command, compType, name string) error {
	projectName := viper.GetString("project_name")
	if projectName == "" {
		return oops.
			Code("missing_config").
			Hint("Run 'go-arch setup' to initialize the project").
			Errorf(".go-arch.yaml file not found or empty")
	}

	config := &ui.ProjectConfig{
		ProjectName:  projectName,
		ModuleName:   viper.GetString("module_name"),
		Architecture: viper.GetString("architecture"),
		DBDriver:     viper.GetString("db_driver"),
		UseDocker:    viper.GetBool("use_docker"),
		UseTemplHTMX: viper.GetBool("use_templ_htmx"),
	}

	routeFlag, _ := cmd.Flags().GetString("route")

	hooksCfg, loadErr := hooks.Load(hooks.ResolveConfigPath())
	if loadErr != nil {
		return oops.
			Code("hooks_load_failed").
			Wrap(loadErr)
	}
	runner := hooks.NewRunner(hooksCfg, hooks.RealRunner{}, ui.Out)

	// --- Three-tier generator dispatch ---
	// Check builtins first (Tier 2) since they don't need pack.
	if _, ok := generators.BuiltinRegistry[compType]; ok {
		return runBuiltinDispatch(compType, name, config, runner)
	}

	// Tier 1: pack generators (if project has a template).
	templateName := viper.GetString("template")
	packResolved := false
	if templateName != "" {
		packName, packVersion, parseErr := packs.ParseRef(templateName)
		if parseErr == nil {
			if packVersion == "" {
				latest, lErr := packs.LatestInstalled(packName)
				if lErr == nil {
					packVersion = latest
				}
			}
			if packVersion != "" {
				packDir := packs.Path(packName, packVersion)
				packManifest, mErr := packs.Load(packDir)
				if mErr == nil {
					packResolved = true
					if _, ok := packManifest.Generators[compType]; ok {
						config.Template = packName
						pi := packs.PackInfo{Dir: packDir, Manifest: packManifest}
						scaffolder := scaffold.NewScaffolder(config,
							scaffold.WithRunner(runner),
							scaffold.WithPackInfo(pi),
						)
						if err := scaffolder.GeneratePackGenerator(compType, nil); err != nil {
							return oops.
								Code("generation_failed").
								With("type", compType).
								With("name", name).
								Wrapf(err, "Pack generator failed")
						}
						ui.Success(fmt.Sprintf("Generator '%s' (%s) from pack '%s' completed.", name, compType, packName))
						return nil
					}
				}
				// Pack installed but generator not found — fall through.
			}
			// Pack not installed or could not resolve version.
			// Try component type directly.
		}
	}

	// Tier 3: component types. If a template was declared but the pack
	// was NOT successfully resolved, and the type is not a known
	// component type, emit pack_not_installed.
	if templateName != "" && !packResolved && !isKnownComponentType(compType) {
		return oops.
			Code(generators.CodePackNotInstalled).
			With("pack", templateName).
			Hint("Run 'go-arch template install' to install the required template pack.").
			Errorf("pack %q is not installed (declared in .go-arch.yaml template field)", templateName)
	}

	return runComponentDispatch(compType, name, config, runner, routeFlag, cmd)
}

// runBuiltinDispatch handles a registered builtin generator.
func runBuiltinDispatch(compType, name string, config *ui.ProjectConfig, runner *hooks.Runner) error {
	ui.Info(fmt.Sprintf("Executing builtin generator: %s for %s...", compType, name))

	fn, err := generators.Lookup(compType)
	if err != nil {
		return oops.
			Code(generators.CodeUnknownBuiltin).
			Wrapf(err, "builtin %q lookup failed", compType)
	}

	records, fnErr := fn(generators.Generator{}, map[string]any{"name": name})
	if fnErr != nil {
		return oops.
			Code("generation_failed").
			With("type", compType).
			With("name", name).
			Wrapf(fnErr, "Builtin generator failed")
	}

	_ = records
	ui.Success(fmt.Sprintf("Builtin generator '%s' (%s) completed.", name, compType))
	return nil
}

// isKnownComponentType returns true if t is a built-in component type.
func isKnownComponentType(t string) bool {
	switch t {
	case "service", "repository", "handler", "crud", "page", "component":
		return true
	}
	return false
}

// runComponentDispatch handles component type generation (tier 3).
func runComponentDispatch(compType, name string, config *ui.ProjectConfig, runner *hooks.Runner, routeFlag string, cmd *cobra.Command) error {
	// Check if this is a known component type.
	if !isKnownComponentType(compType) {
		// Not a known component type — produce unknown_generator error
		// with available names grouped by source.
		return buildUnknownGeneratorError(compType)
	}
	ui.Info(fmt.Sprintf("Generating %s component: %s...", compType, name))

	scaffolder := scaffold.NewScaffolder(config, scaffold.WithRunner(runner))
	var err error
	if compType == "crud" {
		err = scaffolder.GenerateCRUD(name)
	} else {
		var opts []scaffold.GenerateOption
		if routeFlag != "" {
			opts = append(opts, scaffold.WithRoute(routeFlag))
		}
		err = scaffolder.GenerateComponent(compType, name, opts...)
	}

	if err != nil {
		return oops.
			Code("generation_failed").
			With("type", compType).
			With("name", name).
			Wrapf(err, "Component generation failed")
	}

	ui.Success(fmt.Sprintf("Component '%s' (%s) generated successfully.", name, compType))

	if compType == "page" || compType == "component" {
		fmt.Fprint(cmd.OutOrStdout(), templHint(compType)+"\n")
	}
	return nil
}

// runListGenerators prints available generators grouped by source.
func runListGenerators(cmd *cobra.Command) error {
	out := cmd.OutOrStdout()

	// Component types (always available).
	componentTypes := []string{"service", "repository", "handler", "crud", "page", "component"}
	sort.Strings(componentTypes)

	fmt.Fprintln(out, "Component types:")
	for _, t := range componentTypes {
		fmt.Fprintf(out, "  %s\n", t)
	}
	fmt.Fprintln(out)

	// Pack generators (if template is set and pack is installed).
	templateName := viper.GetString("template")
	if templateName != "" {
		packName, packVersion, parseErr := packs.ParseRef(templateName)
		if parseErr == nil {
			if packVersion == "" {
				latest, lErr := packs.LatestInstalled(packName)
				if lErr == nil {
					packVersion = latest
				} else {
					fmt.Fprintf(out, "Pack %q (not installed)\n\n", packName)
					goto builtins
				}
			}
			packDir := packs.Path(packName, packVersion)
			packManifest, mErr := packs.Load(packDir)
			if mErr == nil && len(packManifest.Generators) > 0 {
				fmt.Fprintf(out, "Pack generators (%s):\n", fmt.Sprintf("pack:%s@%s", packName, packVersion))
				names := make([]string, 0, len(packManifest.Generators))
				for n := range packManifest.Generators {
					names = append(names, n)
				}
				sort.Strings(names)
				for _, n := range names {
					desc := packManifest.Generators[n].Description
					if desc != "" {
						fmt.Fprintf(out, "  %s — %s\n", n, desc)
					} else {
						fmt.Fprintf(out, "  %s\n", n)
					}
				}
				fmt.Fprintln(out)
			}
		}
	}

builtins:
	// Builtin generators.
	if len(generators.BuiltinRegistry) > 0 {
		fmt.Fprintln(out, "Builtin generators:")
		names := make([]string, 0, len(generators.BuiltinRegistry))
		for n := range generators.BuiltinRegistry {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			fmt.Fprintf(out, "  %s\n", n)
		}
	} else {
		fmt.Fprintln(out, "Builtin generators: no builtin generators registered")
	}
	fmt.Fprintln(out)

	return nil
}

// templHint returns the post-success hint printed after page/component generation.
func templHint(genType string) string {
	return fmt.Sprintf("💡 Run `templ generate` to compile the new %s.", genType)
}

// buildUnknownGeneratorError constructs an unknown_generator error with
// available generators grouped by source (pack, builtin, component types).
func buildUnknownGeneratorError(compType string) error {
	var groups []string

	// Always list component types.
	groups = append(groups, "component types: service, repository, handler, crud, page, component")

	// Builtin generators.
	if len(generators.BuiltinRegistry) > 0 {
		names := make([]string, 0, len(generators.BuiltinRegistry))
		for n := range generators.BuiltinRegistry {
			names = append(names, n)
		}
		sort.Strings(names)
		groups = append(groups, fmt.Sprintf("builtin: %s", strings.Join(names, ", ")))
	}

	// Pack generators (if project has a template and the pack is installed).
	templateName := viper.GetString("template")
	if templateName != "" {
		packName, packVersion, parseErr := packs.ParseRef(templateName)
		if parseErr == nil {
			if packVersion == "" {
				latest, lErr := packs.LatestInstalled(packName)
				if lErr == nil {
					packVersion = latest
				}
			}
			if packVersion != "" {
				packDir := packs.Path(packName, packVersion)
				packManifest, mErr := packs.Load(packDir)
				if mErr == nil && len(packManifest.Generators) > 0 {
					names := make([]string, 0, len(packManifest.Generators))
					for n := range packManifest.Generators {
						names = append(names, n)
					}
					sort.Strings(names)
					groups = append(groups, fmt.Sprintf("pack (%s): %s", packName, strings.Join(names, ", ")))
				}
			}
		}
	}

	return oops.
		Code(generators.CodeUnknownGenerator).
		With("type", compType).
		Hint("Use --list to see available generators").
		Errorf("unknown generator %q: %s", compType, strings.Join(groups, "; "))
}
