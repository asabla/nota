package main

import "github.com/emilsoderling/nota/cmd"

// Version is set at build time via -ldflags
var version = "dev"

func main() {
	cmd.SetVersion(version)
	cmd.Execute()
}
