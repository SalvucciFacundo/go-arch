package cmd

import (
	"fmt"
	"go-arch/internal/pkg/validator"
	"go-arch/internal/ui"

	"github.com/samber/oops"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
	RootCmd.AddCommand(checkCmd)
}

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Check project architecture health",
	Long:  `Validates the project structure and dependency rules (imports) based on the configured architecture.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// 1. Load configuration (already read in root, but validate critical fields)
		projectName := viper.GetString("project_name")
		if projectName == "" {
			return oops.
				Code("missing_config").
				Hint("Run 'go-arch setup' to initialize the project").
				Errorf("No valid configuration found. Are you in the root of a go-arch project?")
		}

		config := &ui.ProjectConfig{
			ProjectName:  projectName,
			ModuleName:   viper.GetString("module_name"),
			Architecture: viper.GetString("architecture"),
		}

		ui.Analyzing(config.Architecture)

		v := validator.NewValidator(config)
		violations, err := v.Validate()
		if err != nil {
			return oops.
				Code("validation_failed").
				Wrapf(err, "Critical error during architecture validation")
		}

		if len(violations) == 0 {
			ui.Success("Architecture is clean! No violations detected.")
			return nil
		}

		ui.Warning(fmt.Sprintf("Detected %d violation(s):", len(violations)))
		fmt.Println()

		for _, v := range violations {
			statusSymbol := "❌"
			if v.Severity == "WARNING" {
				statusSymbol = "⚠️ "
			}
			fmt.Printf("%s [%s] %s\n   └─ %s\n", statusSymbol, v.Severity, v.File, v.Message)
		}

		return oops.
			Code("architecture_violations").
			Errorf("The project does not comply with the architectural rules")
	},
}
