package cmd

import (
	"fmt"
	"github.com/SalvucciFacundo/go-arch/v2/internal/ui"
	"os"
	"os/exec"

	"github.com/samber/oops"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
	RootCmd.AddCommand(serveCmd)
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run the project with hot-reload",
	Long:  `Run the project using 'air' for hot-reload if available, otherwise fallback to 'go run'.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		layout := viper.GetString("architecture")
		if layout == "" {
			return oops.
				Code("missing_config").
				Hint("Make sure you are in the root of a go-arch project").
				Errorf("No valid architecture configuration found")
		}

		mainPath := "cmd/api/main.go"
		if layout == "Minimalist" {
			mainPath = "main.go"
		}

		ui.Info(fmt.Sprintf("Starting server for the project (Layout: %s)...", layout))

		// Check if Air is installed
		_, err := exec.LookPath("air")
		if err == nil {
			ui.Info("🔥 Using Air for hot-reload...")
			if err := runWithAir(); err != nil {
				return oops.
					Code("server_error").
					Wrapf(err, "'air' execution failed")
			}
			return nil
		}

		ui.Warning("Air not detected in PATH. Using 'go run' (no hot-reload)...")
		if err := runWithGo(mainPath); err != nil {
			return oops.
				Code("server_error").
				With("path", mainPath).
				Wrapf(err, "'go run' execution failed")
		}

		return nil
	},
}

func runWithAir() error {
	cmd := exec.Command("air")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runWithGo(path string) error {
	cmd := exec.Command("go", "run", path)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
