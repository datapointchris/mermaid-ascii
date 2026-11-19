package main

import "github.com/AlexanderGrooff/mermaid-ascii/cmd"

// version is set at release by goreleaser's default ldflags.
var version = "dev"

func main() {
	cmd.SetVersion(version)
	cmd.Execute()
}
