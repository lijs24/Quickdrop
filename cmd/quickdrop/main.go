package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"quickdrop/internal/agent"
	"quickdrop/internal/client"
	"quickdrop/internal/config"
	"quickdrop/internal/hub"
	"quickdrop/internal/transport"
	"quickdrop/internal/webui"
)

var (
	version = "dev"
	commit  = "unknown"
	builtAt = "unknown"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "quickdrop:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		printUsage()
		return nil
	}
	switch args[0] {
	case "init-dev":
		return runInitDev(args[1:])
	case "hub":
		return runHub(ctx, args[1:])
	case "agent":
		return runAgent(ctx, args[1:])
	case "gui":
		return runGUI(ctx, args[1:])
	case "text":
		return runText(ctx, args[1:])
	case "send":
		return runSend(ctx, args[1:])
	case "devices":
		return runDevices(ctx, args[1:])
	case "groups":
		return runGroups(ctx, args[1:])
	case "version":
		return runVersion()
	case "-h", "--help", "help":
		printUsage()
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runVersion() error {
	fmt.Printf("QuickDrop %s\ncommit: %s\nbuilt_at: %s\n", version, commit, builtAt)
	return nil
}

func runInitDev(args []string) error {
	fs := flag.NewFlagSet("init-dev", flag.ExitOnError)
	force := fs.Bool("force", false, "overwrite existing dev config files")
	if err := fs.Parse(args); err != nil {
		return err
	}
	root, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	if err := config.WriteDevConfigs(root, *force); err != nil {
		return err
	}
	fmt.Println("[QuickDrop] Dev configs and data directories are ready.")
	return nil
}

func runHub(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("hub", flag.ExitOnError)
	configPath := fs.String("c", "configs/dev/hub.json", "config file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	return hub.Run(ctx, cfg)
}

func runAgent(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("agent", flag.ExitOnError)
	configPath := fs.String("c", "configs/dev/laptop.json", "config file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	return agent.Run(ctx, cfg)
}

func runGUI(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("gui", flag.ExitOnError)
	configPath := fs.String("c", "configs/dev/laptop.json", "config file")
	listen := fs.String("listen", "", "local GUI listen address")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	if *listen != "" {
		cfg.GUI.Listen = *listen
	}
	return webui.Run(ctx, cfg)
}

func runText(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("text", flag.ExitOnError)
	configPath := fs.String("c", "configs/dev/laptop.json", "config file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) < 2 {
		return fmt.Errorf("usage: quickdrop text -c <config> <device:id|group:id|name> <message>")
	}
	cfg, hubClient, closeFn, err := clientFromConfig(ctx, *configPath)
	if err != nil {
		return err
	}
	defer closeFn()
	targetType, targetID, err := parseTarget(ctx, hubClient, rest[0])
	if err != nil {
		return err
	}
	text := strings.Join(rest[1:], " ")
	env, err := hubClient.SendText(ctx, targetType, targetID, text)
	if err != nil {
		return err
	}
	fmt.Printf("[QuickDrop] Sent text from %s to %s:%s as %s\n", cfg.Device.ID, targetType, targetID, env.Message.ID)
	return nil
}

func runSend(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("send", flag.ExitOnError)
	configPath := fs.String("c", "configs/dev/laptop.json", "config file")
	text := fs.String("text", "", "optional text to include with file message")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) != 2 {
		return fmt.Errorf("usage: quickdrop send -c <config> <device:id|group:id|name> <file>")
	}
	cfg, hubClient, closeFn, err := clientFromConfig(ctx, *configPath)
	if err != nil {
		return err
	}
	defer closeFn()
	targetType, targetID, err := parseTarget(ctx, hubClient, rest[0])
	if err != nil {
		return err
	}
	path, err := filepath.Abs(rest[1])
	if err != nil {
		return fmt.Errorf("resolve upload path: %w", err)
	}
	env, err := hubClient.SendFile(ctx, targetType, targetID, *text, path)
	if err != nil {
		return err
	}
	fmt.Printf("[QuickDrop] Sent file from %s to %s:%s as %s\n", cfg.Device.ID, targetType, targetID, env.Message.ID)
	return nil
}

func runDevices(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("devices", flag.ExitOnError)
	configPath := fs.String("c", "configs/dev/laptop.json", "config file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	_, hubClient, closeFn, err := clientFromConfig(ctx, *configPath)
	if err != nil {
		return err
	}
	defer closeFn()
	devices, err := hubClient.Devices(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("%-18s %-24s %-8s %s\n", "DEVICE", "DISPLAY", "ONLINE", "LAST SEEN")
	for _, dev := range devices {
		fmt.Printf("%-18s %-24s %-8t %s\n", dev.ID, dev.DisplayName, dev.Online, dev.LastSeenAt)
	}
	return nil
}

func runGroups(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("groups", flag.ExitOnError)
	configPath := fs.String("c", "configs/dev/laptop.json", "config file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	_, hubClient, closeFn, err := clientFromConfig(ctx, *configPath)
	if err != nil {
		return err
	}
	defer closeFn()
	groups, err := hubClient.Groups(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("%-18s %-24s %s\n", "GROUP", "NAME", "MEMBERS")
	for _, group := range groups {
		fmt.Printf("%-18s %-24s %s\n", group.ID, group.Name, strings.Join(group.Members, ", "))
	}
	return nil
}

func clientFromConfig(ctx context.Context, configPath string) (*config.Config, *client.Client, func(), error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, nil, nil, err
	}
	tunnel, baseURL, err := transport.StartTunnelIfEnabled(ctx, cfg)
	if err != nil {
		return nil, nil, nil, err
	}
	closeFn := func() {
		_ = tunnel.Close()
	}
	return cfg, client.NewFromConfig(cfg, baseURL), closeFn, nil
}

func parseTarget(ctx context.Context, hubClient *client.Client, raw string) (string, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", fmt.Errorf("target is required")
	}
	if strings.Contains(raw, ":") {
		parts := strings.SplitN(raw, ":", 2)
		if parts[0] != "device" && parts[0] != "group" {
			return "", "", fmt.Errorf("target prefix must be device: or group:")
		}
		if parts[1] == "" {
			return "", "", fmt.Errorf("target id is required")
		}
		return parts[0], parts[1], nil
	}
	groups, err := hubClient.Groups(ctx)
	if err == nil {
		for _, group := range groups {
			if group.ID == raw {
				return "group", raw, nil
			}
		}
	}
	return "device", raw, nil
}

func printUsage() {
	fmt.Println(`QuickDrop MVP

Usage:
  quickdrop init-dev [--force]
  quickdrop hub -c configs/dev/hub.json
  quickdrop agent -c configs/dev/laptop.json
  quickdrop gui -c configs/dev/laptop.json [-listen 127.0.0.1:47900]
  quickdrop text -c configs/dev/laptop.json device:workstation "hello"
  quickdrop send -c configs/dev/laptop.json device:workstation README.md
  quickdrop devices -c configs/dev/laptop.json
  quickdrop groups -c configs/dev/laptop.json
  quickdrop version

Targets:
  device:<id> sends to one device.
  group:<id> sends to a group.
  A bare target defaults to a device, except when it matches a known group id.`)
}
