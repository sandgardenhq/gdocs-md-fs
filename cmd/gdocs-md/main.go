package main

import (
	"os"

	"github.com/brittcrawford/gdocs-md/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
