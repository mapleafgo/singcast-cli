package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/urfave/cli/v3"

	"github.com/mapleafgo/singcast/core"
)

func runCommand() *cli.Command {
	return &cli.Command{
		Name:  "run",
		Usage: "Start the proxy service",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "config",
				Aliases:  []string{"c"},
				Usage:    "config file path (YAML or JSON)",
				Required: true,
			},
			&cli.BoolFlag{
				Name:    "daemon",
				Aliases: []string{"d"},
				Usage:   "run as daemon (fork to background)",
			},
			&cli.StringFlag{
				Name:  "api",
				Usage: "override external-controller address",
			},
			&cli.StringFlag{
				Name:  "home",
				Usage: "home directory",
				Value: defaultHomeDir(),
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			homeDir := cmd.String("home")
			configPath := cmd.String("config")
			daemon := cmd.Bool("daemon")
			apiAddr := cmd.String("api")

			if err := os.MkdirAll(homeDir, 0o700); err != nil {
				return fmt.Errorf("create home dir: %w", err)
			}

			data, err := os.ReadFile(configPath)
			if err != nil {
				return fmt.Errorf("read config: %w", err)
			}
			if len(bytes.TrimSpace(data)) == 0 {
				return fmt.Errorf("config file is empty")
			}

			outPath := filepath.Join(homeDir, "config.json")
			if err := os.WriteFile(outPath, data, 0o600); err != nil {
				return fmt.Errorf("write config: %w", err)
			}

			if apiAddr != "" {
				if err := overrideAPI(outPath, apiAddr); err != nil {
					return fmt.Errorf("override api: %w", err)
				}
			}

			if daemon {
				return startDaemon(homeDir, outPath)
			}
			return runForeground(homeDir, outPath)
		},
	}
}

func runForeground(homeDir, configPath string) error {
	if err := core.Init(homeDir); err != nil {
		return fmt.Errorf("init core: %w", err)
	}
	defer core.Close()

	if err := core.Start(configPath); err != nil {
		return fmt.Errorf("start service: %w", err)
	}

	fmt.Println("singcast started, press Ctrl+C to stop")

	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, shutdownSignals()...)
	for {
		sig := <-sigCh
		if isReloadSignal(sig) {
			fmt.Println("received SIGHUP, reloading config...")
			if err := core.ReloadConfig(); err != nil {
				fmt.Fprintf(os.Stderr, "reload failed: %v\n", err)
			} else {
				fmt.Println("config reloaded")
			}
			continue
		}
		fmt.Printf("\nreceived %v, shutting down...\n", sig)
		break
	}

	core.Stop()
	return nil
}

func overrideAPI(configPath, addr string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}
	var cfg map[string]json.RawMessage
	if err := json.Unmarshal(data, &cfg); err != nil {
		return err
	}

	var exp map[string]json.RawMessage
	if raw, ok := cfg["experimental"]; ok {
		if err := json.Unmarshal(raw, &exp); err != nil {
			return fmt.Errorf("invalid experimental section: %w", err)
		}
	}
	if exp == nil {
		exp = make(map[string]json.RawMessage)
	}

	var clashAPI map[string]json.RawMessage
	if raw, ok := exp["clash_api"]; ok {
		if err := json.Unmarshal(raw, &clashAPI); err != nil {
			return fmt.Errorf("invalid clash_api section: %w", err)
		}
	}
	if clashAPI == nil {
		clashAPI = make(map[string]json.RawMessage)
	}

	quoted, err := json.Marshal(addr)
	if err != nil {
		return err
	}
	clashAPI["external_controller"] = json.RawMessage(quoted)
	exp["clash_api"], _ = json.Marshal(clashAPI)
	cfg["experimental"], _ = json.Marshal(exp)

	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, out, 0o600)
}

func defaultHomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".singcast"
	}
	return filepath.Join(home, ".singcast")
}
