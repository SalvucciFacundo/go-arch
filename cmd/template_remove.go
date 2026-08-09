package cmd

import (
	"fmt"
	"go-arch/internal/pkg/packs"
	"go-arch/internal/ui"

	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

func init() {
	templateCmd.AddCommand(templateRemoveCmd)
}

var templateRemoveCmd = &cobra.Command{
	Use:   "remove <name>[@<version>]",
	Short: "Remove an installed template pack",
	Long: `Removes an installed pack. Without @version, the latest installed version
is removed. With @version, only that specific version is removed.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ref := args[0]
		name, version, err := packs.ParseRef(ref)
		if err != nil {
			return oops.
				Code("invalid_pack_ref").
				Wrapf(err, "invalid pack reference %q", ref)
		}

		if version == "" {
			var latestErr error
			version, latestErr = packs.LatestInstalled(name)
			if latestErr != nil {
				return oops.
					Code("template_remove_failed").
					Wrap(latestErr)
			}
		}

		if err := packs.Remove(name, version); err != nil {
			return oops.
				Code("template_remove_failed").
				Wrap(err)
		}

		fmt.Fprintf(ui.Out, "✅ Pack %q (v%s) removed.\n", name, version)
		return nil
	},
}
