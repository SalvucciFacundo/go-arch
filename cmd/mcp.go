package cmd

import (
	"go-arch/internal/pkg/mcp"

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
