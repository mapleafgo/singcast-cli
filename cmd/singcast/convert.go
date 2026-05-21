package main

import (
	"bytes"
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/mapleafgo/singcast/translator"
)

func convertCommand() *cli.Command {
	return &cli.Command{
		Name:  "convert",
		Usage: "Translate a mihomo YAML config to sing-box JSON",
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
		Action: func(ctx context.Context, cmd *cli.Command) error {
			data, err := os.ReadFile(cmd.String("config"))
			if err != nil {
				return fmt.Errorf("read config: %w", err)
			}
			if len(bytes.TrimSpace(data)) == 0 {
				return fmt.Errorf("config file is empty")
			}

			var result string
			if translator.DetectFormat(data) == translator.FormatJSON {
				result = string(data)
			} else {
				opts := &translator.Options{RuleSetURLPrefix: cmd.String("rule-set-proxy")}
				translated, warnings, err := translator.TranslateWithOptions(data, opts)
				if err != nil {
					return fmt.Errorf("translate: %w", err)
				}
				for _, w := range warnings {
					fmt.Fprintln(os.Stderr, "warning:", w)
				}
				result = translated
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
