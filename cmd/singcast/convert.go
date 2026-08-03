package main

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"
)

func convertCommand() *cli.Command {
	return &cli.Command{
		Name:  "convert",
		Usage: "Translate a Clash YAML / URI list / base64 subscription to sing-box JSON",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "config",
				Aliases:  []string{"c"},
				Usage:    "input config file path",
				Required: true,
			},
			&cli.StringFlag{
				Name:    "output",
				Aliases: []string{"o"},
				Usage:   "output file path (default: stdout)",
			},
			&cli.StringFlag{
				Name:    "rule-set-proxy",
				Aliases: []string{"p"},
				Usage:   "URL prefix for rule-set downloads (e.g. https://gh-proxy.org)",
				Sources: cli.EnvVars("SINGCAST_RULE_SET_PROXY"),
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			result, err := loadConfigJSON(cmd.String("config"), cmd.String("rule-set-proxy"))
			if err != nil {
				return err
			}

			outputPath := cmd.String("output")
			if outputPath == "" {
				if _, err := os.Stdout.Write([]byte(result)); err != nil {
					return fmt.Errorf("write output: %w", err)
				}
				_, _ = os.Stdout.Write([]byte("\n"))
			} else {
				if err := os.WriteFile(outputPath, []byte(result), 0o644); err != nil {
					return fmt.Errorf("write output: %w", err)
				}
			}
			return nil
		},
	}
}
