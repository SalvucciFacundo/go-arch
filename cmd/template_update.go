package cmd

import (
	"context"
	"fmt"
	"github.com/SalvucciFacundo/go-arch/v2/internal/pkg/packs"
	"github.com/SalvucciFacundo/go-arch/v2/internal/ui"

	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

func init() {
	templateCmd.AddCommand(templateUpdateCmd)
}

var templateUpdateCmd = &cobra.Command{
	Use:   "update <name>",
	Short: "Update a template pack to @latest",
	Long: `Re-fetches the @latest version of a pack, replacing the previously installed
latest. Previously pinned older versions are preserved.

If the new version declares hooks, the trust warning is re-prompted.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		dl := packs.RealDownloader{}
		ctx := context.Background()

		m, err := packs.Update(ctx, dl, name, trustPrompt)
		if err != nil {
			return oops.
				Code("template_update_failed").
				Wrap(err)
		}

		fmt.Fprintf(ui.Out, "✅ Pack %q updated to v%s.\n", m.Name, m.Version)
		return nil
	},
}
