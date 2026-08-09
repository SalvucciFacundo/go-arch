package main

import "go-arch/cmd"

func main() {
	cmd.Version = version
	cmd.Execute()
}
