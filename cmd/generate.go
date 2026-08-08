package cmd

import (
	"fmt"
	"go-arch/internal/pkg/scaffold"
	"go-arch/internal/ui"

	"github.com/samber/oops"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
	RootCmd.AddCommand(generateCmd)
}

var generateCmd = &cobra.Command{
	Use:     "generate [type] [name]",
	Short:   "Generate a new component",
	Long:    `Generate components like service, repository, or handler based on the project layout.`,
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
		}

		ui.Info(fmt.Sprintf("Generating %s component: %s...", compType, name))

		scaffolder := scaffold.NewScaffolder(config)
		var err error
		if compType == "crud" {
			err = scaffolder.GenerateCRUD(name)
		} else {
			err = scaffolder.GenerateComponent(compType, name)
		}

		if err != nil {
			return oops.
				Code("generation_failed").
				With("type", compType).
				With("name", name).
				Wrapf(err, "Component generation failed")
		}

		ui.Success(fmt.Sprintf("Component '%s' (%s) generated successfully.", name, compType))
		return nil
	},
}
