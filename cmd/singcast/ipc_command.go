package main

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"
)

func ipcCommand() *cli.Command {
	return &cli.Command{
		Name:  "ipc",
		Usage: "Start IPC service and wait for GUI connections",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "home",
				Usage:    "home directory",
				Required: true,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			homeDir := cmd.String("home")

			if err := os.MkdirAll(homeDir, 0o700); err != nil {
				return fmt.Errorf("create home dir: %w", err)
			}

			return runIpc(homeDir)
		},
	}
}
