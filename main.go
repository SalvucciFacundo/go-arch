package main

import (
	"go-arch/cmd"
	"go-arch/internal/pkg/mcp"
)

func main() {
	cmd.Version = version
	mcp.Version = version
	cmd.Execute()
}
