package main

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/mapleafgo/singcast/core"
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
			// 与 run/convert 保持一致：缺了这个 flag，带 rule-set-proxy 的配置
			// check 通过而 run 的实际产物不同，校验就失去了意义。
			&cli.StringFlag{
				Name:    "rule-set-proxy",
				Aliases: []string{"p"},
				Usage:   "URL prefix for rule-set downloads (e.g. https://gh-proxy.org)",
				Sources: cli.EnvVars("SINGCAST_RULE_SET_PROXY"),
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			jsonContent, err := loadConfigJSON(cmd.String("config"), cmd.String("rule-set-proxy"))
			if err != nil {
				return err
			}

			if err := core.CheckConfig(ctx, jsonContent); err != nil {
				return fmt.Errorf("config validation failed: %w", err)
			}

			fmt.Println("config is valid")
			return nil
		},
	}
}
