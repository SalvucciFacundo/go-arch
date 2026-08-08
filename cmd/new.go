package cmd

import (
	"fmt"
	"go-arch/internal/pkg/scaffold"
	"go-arch/internal/ui"

	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(newCmd)
}

var newCmd = &cobra.Command{
	Use:   "new [name]",
	Short: "Create a new project",
	Long:  `The 'new' command initializes a new Go project with the specified name and architecture.`,
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
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
		scaffolder := scaffold.NewScaffolder(config)
		if err := scaffolder.Execute(); err != nil {
			return oops.
				Code("scaffold_failed").
				With("project_name", config.ProjectName).
				Wrapf(err, "Error while scaffolding the project")
		}

		ui.Success(fmt.Sprintf("Project '%s' created successfully!", config.ProjectName))
		fmt.Printf("👉 %s cd %s and go-arch serve\n", ui.InfoMsg("Run:"), config.ProjectName)
		return nil
	},
}
