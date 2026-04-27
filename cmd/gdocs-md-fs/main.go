package main

import (
	"os"

	"github.com/brittcrawford/gdocs-md-fs/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
