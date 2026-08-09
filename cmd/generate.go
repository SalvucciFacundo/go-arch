package cmd

import (
	"fmt"
	"go-arch/internal/pkg/hooks"
	"go-arch/internal/pkg/scaffold"
	"go-arch/internal/ui"

	"github.com/samber/oops"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
	generateCmd.Flags().String("route", "", `Route pattern for handler type (e.g. "GET /stats"). CRUD auto-registers in web projects.`)
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

Flags:
  --route "METHOD /path"   Register a route for the handler type in web projects
                           (e.g. --route "GET /stats"). CRUD auto-registers routes
                           in web projects by default.`,
	Args:    cobra.ExactArgs(2),
	Aliases: []string{"g"},
	RunE: func(cmd *cobra.Command, args []string) error {
		compType := args[0]
		name := args[1]

		// Validate basic config
		projectName := viper.GetString("project_name")
		if projectName == "" {
			return oops.
				Code("missing_config").
				Hint("Run 'go-arch setup' to initialize the project").
				Errorf(".go-arch.yaml file not found or empty")
		}

		// Map Viper config to struct
		config := &ui.ProjectConfig{
			ProjectName:  projectName,
			ModuleName:   viper.GetString("module_name"),
			Architecture: viper.GetString("architecture"),
			DBDriver:     viper.GetString("db_driver"),
			UseDocker:    viper.GetBool("use_docker"),
			UseTemplHTMX: viper.GetBool("use_templ_htmx"),
		}

		ui.Info(fmt.Sprintf("Generating %s component: %s...", compType, name))

		routeFlag, _ := cmd.Flags().GetString("route")

		hooksCfg, loadErr := hooks.Load(hooks.ResolveConfigPath())
		if loadErr != nil {
			return oops.
				Code("hooks_load_failed").
				Wrap(loadErr)
		}
		runner := hooks.NewRunner(hooksCfg, hooks.RealRunner{}, ui.Out)

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
	},
}

// templHint returns the post-success hint printed after page/component generation.
func templHint(genType string) string {
	return fmt.Sprintf("💡 Run `templ generate` to compile the new %s.", genType)
}
