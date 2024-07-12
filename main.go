package main

import (
	"github.com/mtibben/orgit/cmd"
)

// Version is set at build time
var Version = "dev"

func main() {
	cmd.Execute(Version)
}
