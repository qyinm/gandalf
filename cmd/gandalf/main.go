package main

import (
	"os"

	"github.com/qyinm/gandalf/internal/cli"

	// Register full scanner plugin set (including Codex).
	_ "github.com/qyinm/gandalf/internal/gandalfcore/scan/plugins"
)

func main() {
	os.Exit(cli.Execute())
}
