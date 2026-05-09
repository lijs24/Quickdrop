package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"quickdrop/internal/agent"
	quickapp "quickdrop/internal/app"
	"quickdrop/internal/client"
	"quickdrop/internal/config"
	"quickdrop/internal/hub"
	"quickdrop/internal/transport"
	"quickdrop/internal/updater"
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
		return runDefaultApp(ctx)
	}
	switch args[0] {
	case "init-dev":
		return runInitDev(args[1:])
	case "app":
		return runApp(ctx, args[1:])
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
	case "update":
		return runUpdate(ctx, args[1:])
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

func runDefaultApp(ctx context.Context) error {
	if err := preferExecutableDirWhenNeeded(); err != nil {
		return err
	}
	root, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	if err := ensureDevConfig(root); err != nil {
		return err
	}
	configPath := defaultAppConfigPath()
	hubConfigPath := ""
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	if shouldStartLocalHub(cfg) {
		candidate := filepath.Join("configs", "dev", "hub.json")
		if _, err := os.Stat(candidate); err == nil {
			hubConfigPath = candidate
		}
	}
	args := []string{"-c", configPath}
	if hubConfigPath != "" {
		args = append(args, "-hub-config", hubConfigPath)
	}
	return runApp(ctx, args)
}

func runApp(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("app", flag.ExitOnError)
	configPath := fs.String("c", defaultAppConfigPath(), "agent config file")
	hubConfigPath := fs.String("hub-config", "", "optional hub config to run in the same app process")
	listen := fs.String("listen", "", "local GUI listen address")
	openBrowser := fs.Bool("open", true, "open the GUI URL in the default browser")
	exitOnClose := fs.Bool("exit-on-close", true, "shut down app services when the GUI page is closed")
	closeGrace := fs.Duration("close-grace", 12*time.Second, "how long to wait after GUI heartbeat stops before shutting down")
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
	var hubCfg *config.Config
	if *hubConfigPath != "" {
		hubCfg, err = config.Load(*hubConfigPath)
		if err != nil {
			return err
		}
	}
	return quickapp.Run(ctx, cfg, hubCfg, quickapp.Options{
		Version:       version,
		OpenBrowser:   *openBrowser,
		ExitOnClose:   *exitOnClose,
		CloseGrace:    *closeGrace,
		HubConfigPath: *hubConfigPath,
	})
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
	fmt.Printf("%-18s %-24s %-9s %-8s %s\n", "DEVICE", "DISPLAY", "COLOR", "ONLINE", "LAST SEEN")
	for _, dev := range devices {
		fmt.Printf("%-18s %-24s %-9s %-8t %s\n", dev.ID, dev.DisplayName, dev.Color, dev.Online, dev.LastSeenAt)
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

func runUpdate(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("update", flag.ExitOnError)
	repo := fs.String("repo", "lijs24/Quickdrop", "GitHub repository owner/name")
	targetVersion := fs.String("version", "latest", "release version or latest")
	installDir := fs.String("install-dir", "", "installation directory; defaults to the directory containing quickdrop")
	force := fs.Bool("force", false, "update even when the current version matches the target release")
	dryRun := fs.Bool("dry-run", false, "check the update without downloading or applying it")
	if err := fs.Parse(args); err != nil {
		return err
	}
	result, err := updater.Run(ctx, updater.Options{
		Repo:           *repo,
		Version:        *targetVersion,
		CurrentVersion: version,
		InstallDir:     *installDir,
		Force:          *force,
		DryRun:         *dryRun,
	})
	if err != nil {
		return err
	}
	if result.AlreadyCurrent {
		if result.CurrentVersion != "" && result.TargetVersion != "" && result.CurrentVersion != result.TargetVersion {
			fmt.Printf("[QuickDrop] No newer release. Current: %s; latest: %s\n", result.CurrentVersion, result.TargetVersion)
			return nil
		}
		fmt.Printf("[QuickDrop] Already up to date: %s\n", result.TargetVersion)
		return nil
	}
	if *dryRun {
		fmt.Printf("[QuickDrop] Update available: %s -> %s (%s)\n", result.CurrentVersion, result.TargetVersion, result.AssetName)
		return nil
	}
	if result.ApplyScript != "" {
		fmt.Printf("[QuickDrop] Downloaded %s and started updater: %s\n", result.AssetName, result.ApplyScript)
		fmt.Println("[QuickDrop] Close QuickDrop if it is still running; the updater window will apply the new files.")
		return nil
	}
	fmt.Printf("[QuickDrop] Updated %s to %s in %s\n", result.AssetName, result.TargetVersion, result.InstallDir)
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

func preferExecutableDirWhenNeeded() error {
	if _, err := os.Stat("configs"); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat configs: %w", err)
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}
	exeDir := filepath.Dir(exe)
	if _, err := os.Stat(filepath.Join(exeDir, "configs")); err == nil {
		if err := os.Chdir(exeDir); err != nil {
			return fmt.Errorf("change to executable directory %s: %w", exeDir, err)
		}
	}
	return nil
}

func ensureDevConfig(root string) error {
	for _, path := range []string{
		filepath.Join(root, "quickdrop.json"),
		filepath.Join(root, "configs", "dev", "laptop.json"),
	} {
		if _, err := os.Stat(path); err == nil {
			return nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("stat %s: %w", path, err)
		}
	}
	return config.WriteDevConfigs(root, false)
}

func defaultAppConfigPath() string {
	if _, err := os.Stat("quickdrop.json"); err == nil {
		return "quickdrop.json"
	}
	return filepath.Join("configs", "dev", "laptop.json")
}

func shouldStartLocalHub(cfg *config.Config) bool {
	if cfg.HubClient.UseSSHTunnel || cfg.SSHTunnel.Enabled {
		return false
	}
	parsed, err := url.Parse(cfg.HubClient.BaseURL)
	if err != nil {
		return false
	}
	host := parsed.Hostname()
	port := parsed.Port()
	if port == "" {
		port = "80"
	}
	return (host == "127.0.0.1" || host == "localhost" || host == "::1") && port == "47891"
}

func printUsage() {
	fmt.Println(`QuickDrop MVP

Usage:
  quickdrop
  quickdrop init-dev [--force]
  quickdrop app -c configs/dev/laptop.json [-hub-config configs/dev/hub.json]
  quickdrop hub -c configs/dev/hub.json
  quickdrop agent -c configs/dev/laptop.json
  quickdrop gui -c configs/dev/laptop.json [-listen 127.0.0.1:47900]
  quickdrop text -c configs/dev/laptop.json device:workstation "hello"
  quickdrop send -c configs/dev/laptop.json device:workstation README.md
  quickdrop devices -c configs/dev/laptop.json
  quickdrop groups -c configs/dev/laptop.json
  quickdrop version
  quickdrop update

Targets:
  device:<id> sends to one device.
  group:<id> sends to a group.
  A bare target defaults to a device, except when it matches a known group id.`)
}
