// gt is the Gas Town CLI for managing multi-agent workspaces.
package main

import (
	"os"

	"github.com/colbymchenry/devpit/internal/cmd"
)

func main() {
	os.Exit(cmd.Execute())
}
