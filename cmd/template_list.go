package cmd

import (
	"fmt"
	"go-arch/internal/pkg/packs"
	"go-arch/internal/ui"

	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

func init() {
	templateCmd.AddCommand(templateListCmd)
}

var templateListCmd = &cobra.Command{
	Use:   "list",
	Short: "List installed template packs",
	Long:  `Lists all installed template packs sorted by name.`,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		packsList, err := packs.List()
		if err != nil {
			return oops.
				Code("template_list_failed").
				Wrap(err)
		}

		if len(packsList) == 0 {
			fmt.Fprintln(ui.Out, "No packs installed.")
			return nil
		}

		for _, p := range packsList {
			fmt.Fprintf(ui.Out, "  %s@%s\n", p.Name, p.Version)
		}
		return nil
	},
}
