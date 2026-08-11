package main

import (
	"github.com/SalvucciFacundo/go-arch/v2/cmd"
	"github.com/SalvucciFacundo/go-arch/v2/internal/pkg/mcp"
)

func main() {
	cmd.Version = version
	mcp.Version = version
	cmd.Execute()
}
