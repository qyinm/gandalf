package main

import (
	"os"

	"github.com/qyinm/hem/internal/cli"

	// Register full scanner plugin set (including Codex).
	_ "github.com/qyinm/hem/internal/hemcore/scan/plugins"
)

func main() {
	os.Exit(cli.Execute())
}