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
	"github.com/mapleafgo/singcast/translator"
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
			&cli.StringFlag{
				Name:    "rule-set-proxy",
				Aliases: []string{"p"},
				Usage:   "URL prefix for rule-set downloads (e.g. https://gh-proxy.org)",
				Sources: cli.EnvVars("SINGCAST_RULE_SET_PROXY"),
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			homeDir := cmd.String("home")
			configPath := cmd.String("config")
			daemon := cmd.Bool("daemon")
			apiAddr := cmd.String("api")
			proxyPrefix := cmd.String("rule-set-proxy")

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

			var jsonContent string
			if translator.DetectFormat(data) == translator.FormatYAML {
				opts := &translator.Options{RuleSetURLPrefix: proxyPrefix}
				result, warns, err := translator.TranslateWithOptions(data, opts)
				if err != nil {
					return fmt.Errorf("translate config: %w", err)
				}
				for _, w := range warns {
					fmt.Fprintf(os.Stderr, "WARN: %s\n", w)
				}
				jsonContent = result
			} else {
				jsonContent = string(data)
			}

			if err := os.WriteFile(outPath, []byte(jsonContent), 0o600); err != nil {
				return fmt.Errorf("write config: %w", err)
			}

			if apiAddr != "" {
				if err := overrideAPI(outPath, apiAddr); err != nil {
					return fmt.Errorf("override api: %w", err)
				}
			}

			if daemon {
				return startDaemon(homeDir, outPath, proxyPrefix, apiAddr)
			}
			return runForeground(homeDir, outPath, jsonContent)
		},
	}
}

func runForeground(homeDir, configPath, jsonContent string) error {
	svc := core.NewService()
	if err := svc.Init(`{"home_dir":"` + homeDir + `"}`); err != nil {
		return fmt.Errorf("init core: %w", err)
	}
	defer svc.Destroy()

	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}
	if err := svc.StartWithContent(string(data), ""); err != nil {
		return fmt.Errorf("start service: %w", err)
	}

	printListeningPorts(jsonContent)
	fmt.Println("singcast started, press Ctrl+C to stop")

	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, shutdownSignals()...)
	for {
		sig := <-sigCh
		if isReloadSignal(sig) {
			fmt.Println("received SIGHUP, reloading config...")
			rd, rerr := os.ReadFile(configPath)
			if rerr != nil {
				fmt.Fprintf(os.Stderr, "reload: read config failed: %v\n", rerr)
			} else if err := svc.StartWithContent(string(rd), ""); err != nil {
				fmt.Fprintf(os.Stderr, "reload failed: %v\n", err)
			} else {
				fmt.Println("config reloaded")
			}
			continue
		}
		fmt.Printf("\nreceived %v, shutting down...\n", sig)
		break
	}

	svc.Stop()
	return nil
}

func printListeningPorts(jsonContent string) {
	var cfg struct {
		Inbounds []struct {
			Type       string `json:"type"`
			Tag        string `json:"tag"`
			Listen     string `json:"listen"`
			ListenPort uint16 `json:"listen_port"`
		} `json:"inbounds"`
	}
	if json.Unmarshal([]byte(jsonContent), &cfg) != nil {
		return
	}
	for _, in := range cfg.Inbounds {
		addr := in.Listen
		if addr == "" {
			addr = "0.0.0.0"
		}
		if in.ListenPort > 0 {
			fmt.Printf("%s(%s) listening on %s:%d\n", in.Tag, in.Type, addr, in.ListenPort)
		}
	}
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
