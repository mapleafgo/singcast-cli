package main

import (
	"context"
	"fmt"
	"runtime"

	"github.com/urfave/cli/v3"

	"github.com/mapleafgo/singcast/core"
)

func versionCommand() *cli.Command {
	return &cli.Command{
		Name:  "version",
		Usage: "Print version info",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			fmt.Printf("singcast %s\n", core.Version)
			fmt.Printf("  core:    sing-box\n")
			fmt.Printf("  go:      %s\n", runtime.Version())
			fmt.Printf("  os/arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
			return nil
		},
	}
}
