package main

import (
	"os"

	"github.com/WBT112/sidequest/internal/cli"
)

var version = "dev"

func main() {
	app := cli.App{
		Out:     os.Stdout,
		Err:     os.Stderr,
		Version: version,
	}

	os.Exit(app.Run(os.Args[1:]))
}
