package main

import (
	"os"

	"openlist-cli/internal/cli"
)

func main() {
	os.Exit(cli.New().Execute(os.Args[1:]))
}
