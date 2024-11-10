package main

import (
	"os"

	"github.com/matthewmueller/chunky/internal/cli"
)

func main() {
	os.Exit(cli.Run())
}
