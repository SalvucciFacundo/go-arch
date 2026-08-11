package cmd

import (
	"github.com/SalvucciFacundo/go-arch/v2/internal/pkg/mcp"

	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(mcpCmd)
}

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start MCP server over stdio",
	Long:  `Launches a Model Context Protocol (MCP) server communicating over standard input and output (stdio).`,
	Run: func(cmd *cobra.Command, args []string) {
		mcp.StartServer()
	},
}
