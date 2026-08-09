package cmd

import "github.com/spf13/cobra"

func init() {
	RootCmd.AddCommand(templateCmd)
}

var templateCmd = &cobra.Command{
	Use:   "template",
	Short: "Manage template packs",
	Long:  `Install, list, remove and update external template packs from Go module proxies.`,
}
