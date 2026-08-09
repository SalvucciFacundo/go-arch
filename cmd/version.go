package cmd

import "github.com/spf13/cobra"

// Version is set by main after GoReleaser injects main.version.
var Version = "dev"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Println(Version)
	},
}

func init() {
	RootCmd.AddCommand(versionCmd)
}
