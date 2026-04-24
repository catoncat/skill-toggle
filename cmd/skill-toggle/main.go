package main

import (
	"os"

	"github.com/catoncat/skill-toggle/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
