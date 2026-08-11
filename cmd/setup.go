package cmd

import (
	"fmt"
	"github.com/SalvucciFacundo/go-arch/v2/internal/ui"
	"runtime"

	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(setupCmd)
}

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Setup Go environment",
	Long:  `The 'setup' command detects your OS and installs Go and necessary tools like 'air'.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ui.Info(fmt.Sprintf("Detecting environment for %s/%s...", runtime.GOOS, runtime.GOARCH))

		switch runtime.GOOS {
		case "linux":
			setupLinux()
		case "windows":
			setupWindows()
		default:
			return oops.
				Code("os_not_supported").
				With("os", runtime.GOOS).
				Errorf("Operating system not yet supported automatically")
		}

		ui.Success("Setup process finished. Review the instructions above to complete the installation.")
		return nil
	},
}

func setupLinux() {
	ui.Info("🐧 Linux environment detected.")
	fmt.Println("1. Downloading official go.dev installer...")
	// TODO: Implement actual download with net/http
	fmt.Println("2. To install, run: sudo tar -C /usr/local -xzf go1.24.linux-amd64.tar.gz")
	fmt.Println("3. Installing Air for hot-reload...")
	fmt.Printf("👉 %s go install github.com/air-verse/air@latest\n", ui.InfoMsg("Run:"))
}

func setupWindows() {
	ui.Info("🪟 Windows environment detected.")
	fmt.Println("1. Downloading official MSI from go.dev...")
	// TODO: Implement actual download
	fmt.Println("2. Running installer...")
	fmt.Println("3. Installing Air for hot-reload...")
	fmt.Printf("👉 %s go install github.com/air-verse/air@latest\n", ui.InfoMsg("Run:"))
}
