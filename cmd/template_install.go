package cmd

import (
	"context"
	"fmt"
	"github.com/SalvucciFacundo/go-arch/v2/internal/pkg/packs"
	"github.com/SalvucciFacundo/go-arch/v2/internal/ui"

	"github.com/AlecAivazis/survey/v2"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

func init() {
	templateCmd.AddCommand(templateInstallCmd)
}

var templateInstallCmd = &cobra.Command{
	Use:   "install <module>[@<version>]",
	Short: "Install a template pack from a Go module",
	Long: `Fetches a pack module via go mod download, validates it, and installs
it under ~/.go-arch/packs/<name>@<version>/. If no @version is provided,
@latest is used.

If the pack declares hooks, a trust prompt is shown before installation.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ref := args[0]

		// Parse module and version from the ref.
		// "<module>[@<version>]" — default @latest.
		module, version, err := packs.ParseRef(ref)
		if err != nil {
			return oops.
				Code("invalid_pack_ref").
				Wrapf(err, "invalid pack reference %q", ref)
		}
		if version == "" {
			version = "latest"
		}

		dl := packs.RealDownloader{}
		ctx := context.Background()

		m, err := packs.Install(ctx, dl, module, version, trustPrompt)
		if err != nil {
			return oops.
				Code("template_install_failed").
				Wrap(err)
		}

		fmt.Fprintf(ui.Out, "✅ Pack %q (v%s) installed successfully.\n", m.Name, m.Version)
		return nil
	},
}

// trustPrompt shows a warning and prompts the user whether to enable hooks.
// It uses survey.AskOne directly since the ui package does not expose
// a Confirm helper.
func trustPrompt(packName string) (bool, error) {
	ui.Warning(fmt.Sprintf(
		"⚠ Pack %q declares hooks or generators that may run shell commands. Review before enabling.",
		packName,
	))
	var answer bool
	err := survey.AskOne(&survey.Confirm{
		Message: "Enable hooks from this pack?",
		Default: false,
	}, &answer)
	return answer, err
}
