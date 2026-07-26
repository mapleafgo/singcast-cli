package main

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/mapleafgo/singcast/ipc"
)

func serviceCommand() *cli.Command {
	return &cli.Command{
		Name:  "service",
		Usage: "Manage system service",
		Commands: []*cli.Command{
			{
				Name:  "install",
				Usage: "Install the system service",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					if err := ipc.InstallService(); err != nil {
						return fmt.Errorf("install service: %w", err)
					}
					fmt.Println("service installed")
					return nil
				},
			},
			{
				Name:  "uninstall",
				Usage: "Uninstall the system service",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					if err := ipc.UninstallService(); err != nil {
						return fmt.Errorf("uninstall service: %w", err)
					}
					fmt.Println("service uninstalled")
					return nil
				},
			},
		},
	}
}
