package main

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/mapleafgo/singcast/core"
)

func main() {
	cmd := &cli.Command{
		Name:    "singcast",
		Version: core.Version,
		Usage:   "A lightweight proxy core powered by sing-box",
		Commands: []*cli.Command{
			runCommand(),
			ipcCommand(),
			convertCommand(),
			checkCommand(),
			versionCommand(),
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
