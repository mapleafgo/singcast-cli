package main

import (
	"bytes"
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/mapleafgo/singcast/core"
	"github.com/mapleafgo/singcast/translator"
)

func checkCommand() *cli.Command {
	return &cli.Command{
		Name:  "check",
		Usage: "Validate a config file",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "config",
				Aliases:  []string{"c"},
				Usage:    "config file path",
				Required: true,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			data, err := os.ReadFile(cmd.String("config"))
			if err != nil {
				return fmt.Errorf("read config: %w", err)
			}
			if len(bytes.TrimSpace(data)) == 0 {
				return fmt.Errorf("config file is empty")
			}

			var jsonContent string
			if translator.DetectFormat(data) == translator.FormatJSON {
				jsonContent = string(data)
			} else {
				translated, warnings, err := translator.Translate(data)
				if err != nil {
					return fmt.Errorf("translate: %w", err)
				}
				for _, w := range warnings {
					fmt.Fprintln(os.Stderr, "warning:", w)
				}
				jsonContent = translated
			}

			if err := core.CheckConfig(jsonContent); err != nil {
				return fmt.Errorf("config validation failed: %w", err)
			}

			fmt.Println("config is valid")
			return nil
		},
	}
}
