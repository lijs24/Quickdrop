package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type DeviceConfig struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Token       string `json:"token"`
}

type GroupConfig struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Members []string `json:"members"`
}

type HubConfig struct {
	Listen         string `json:"listen"`
	DataDir        string `json:"data_dir"`
	MaxUploadBytes int64  `json:"max_upload_bytes"`
}

type AgentConfig struct {
	Listen       string `json:"listen"`
	DataDir      string `json:"data_dir"`
	DownloadsDir string `json:"downloads_dir"`
}

type HubClientConfig struct {
	BaseURL      string `json:"base_url"`
	SSEURL       string `json:"sse_url"`
	UseSSHTunnel bool   `json:"use_ssh_tunnel"`
}

type SSHTunnelConfig struct {
	Enabled    bool   `json:"enabled"`
	SSHHost    string `json:"ssh_host"`
	LocalPort  int    `json:"local_port"`
	RemoteHost string `json:"remote_host"`
	RemotePort int    `json:"remote_port"`
}

type GUIConfig struct {
	Listen   string `json:"listen"`
	Language string `json:"language"`
}

type Config struct {
	Path      string          `json:"-"`
	Role      string          `json:"role"`
	Device    DeviceConfig    `json:"device"`
	Hub       HubConfig       `json:"hub,omitempty"`
	Agent     AgentConfig     `json:"agent,omitempty"`
	HubClient HubClientConfig `json:"hub_client,omitempty"`
	SSHTunnel SSHTunnelConfig `json:"ssh_tunnel,omitempty"`
	GUI       GUIConfig       `json:"gui,omitempty"`
	Devices   []DeviceConfig  `json:"devices,omitempty"`
	Groups    []GroupConfig   `json:"groups,omitempty"`
}

func Load(path string) (*Config, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve config %s: %w", path, err)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open config %s: %w", path, err)
	}
	defer file.Close()

	var cfg Config
	dec := json.NewDecoder(file)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("decode config %s: %w", path, err)
	}
	if cfg.Device.ID == "" {
		return nil, errors.New("config device.id is required")
	}
	if cfg.Device.Token == "" {
		return nil, errors.New("config device.token is required")
	}
	applyDefaults(&cfg)
	cfg.Path = absPath
	return &cfg, nil
}

func Save(path string, cfg *Config) error {
	if path == "" {
		return errors.New("config path is required")
	}
	if cfg.Device.ID == "" {
		return errors.New("config device.id is required")
	}
	if cfg.Device.Token == "" {
		return errors.New("config device.token is required")
	}
	applyDefaults(cfg)
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config %s: %w", path, err)
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(path), "quickdrop-config-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp config file: %w", err)
	}
	tmpPath := tmp.Name()
	removeTmp := true
	defer func() {
		if removeTmp {
			_ = os.Remove(tmpPath)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp config file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp config file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		if _, statErr := os.Stat(path); statErr == nil {
			if removeErr := os.Remove(path); removeErr != nil {
				return fmt.Errorf("remove existing config %s after rename failed: %w", path, removeErr)
			}
			if renameErr := os.Rename(tmpPath, path); renameErr == nil {
				removeTmp = false
				cfg.Path = path
				return nil
			}
		}
		return fmt.Errorf("replace config %s: %w", path, err)
	}
	removeTmp = false
	cfg.Path = path
	return nil
}

func applyDefaults(cfg *Config) {
	if cfg.Hub.Listen == "" {
		cfg.Hub.Listen = "127.0.0.1:47891"
	}
	if cfg.Hub.DataDir == "" {
		cfg.Hub.DataDir = "./data/hub"
	}
	if cfg.Hub.MaxUploadBytes == 0 {
		cfg.Hub.MaxUploadBytes = 1 << 30
	}
	if cfg.Agent.Listen == "" {
		cfg.Agent.Listen = "127.0.0.1:47892"
	}
	if cfg.Agent.DataDir == "" {
		cfg.Agent.DataDir = filepath.Join(".", "data", cfg.Device.ID)
	}
	if cfg.Agent.DownloadsDir == "" {
		cfg.Agent.DownloadsDir = filepath.Join(cfg.Agent.DataDir, "downloads")
	}
	if cfg.HubClient.BaseURL == "" {
		cfg.HubClient.BaseURL = "http://127.0.0.1:47891"
	}
	if cfg.HubClient.SSEURL == "" {
		cfg.HubClient.SSEURL = cfg.HubClient.BaseURL + "/api/events"
	}
	if cfg.GUI.Listen == "" {
		cfg.GUI.Listen = "127.0.0.1:47900"
	}
	if cfg.GUI.Language == "" {
		cfg.GUI.Language = "zh-CN"
	}
}

func WriteDevConfigs(root string, force bool) error {
	configs := map[string]Config{
		filepath.Join(root, "configs", "dev", "hub.json"):         devHubConfig(),
		filepath.Join(root, "configs", "dev", "laptop.json"):      devAgentConfig("laptop", "Laptop", "dev-laptop-token", "127.0.0.1:47892", "127.0.0.1:47900"),
		filepath.Join(root, "configs", "dev", "workstation.json"): devAgentConfig("workstation", "Workstation", "dev-workstation-token", "127.0.0.1:47893", "127.0.0.1:47901"),
		filepath.Join(root, "configs", "dev", "server.json"):      devAgentConfig("main-server", "Main Server", "dev-main-server-token", "127.0.0.1:47894", "127.0.0.1:47902"),
	}
	for path, cfg := range configs {
		if err := writeJSONFile(path, cfg, force); err != nil {
			return err
		}
	}
	for _, dir := range []string{
		filepath.Join(root, "data", "hub"),
		filepath.Join(root, "data", "hub", "blobs"),
		filepath.Join(root, "data", "hub", "tmp"),
		filepath.Join(root, "data", "hub", "logs"),
		filepath.Join(root, "data", "laptop", "downloads"),
		filepath.Join(root, "data", "laptop", "logs"),
		filepath.Join(root, "data", "workstation", "downloads"),
		filepath.Join(root, "data", "workstation", "logs"),
		filepath.Join(root, "data", "main-server", "downloads"),
		filepath.Join(root, "data", "main-server", "logs"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
	}
	return nil
}

func writeJSONFile(path string, cfg Config, force bool) error {
	if !force {
		if _, err := os.Stat(path); err == nil {
			return nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("stat %s: %w", path, err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func devHubConfig() Config {
	return Config{
		Role: "hub",
		Device: DeviceConfig{
			ID:          "main-server",
			DisplayName: "Main Server",
			Token:       "dev-main-server-token",
		},
		Hub: HubConfig{
			Listen:         "127.0.0.1:47891",
			DataDir:        "./data/hub",
			MaxUploadBytes: 1073741824,
		},
		Devices: []DeviceConfig{
			{ID: "laptop", DisplayName: "Laptop", Token: "dev-laptop-token"},
			{ID: "workstation", DisplayName: "Workstation", Token: "dev-workstation-token"},
			{ID: "main-server", DisplayName: "Main Server", Token: "dev-main-server-token"},
		},
		Groups: []GroupConfig{
			{ID: "all", Name: "All Devices", Members: []string{"laptop", "workstation", "main-server"}},
		},
	}
}

func devAgentConfig(id, name, token, listen, guiListen string) Config {
	return Config{
		Role: "agent",
		Device: DeviceConfig{
			ID:          id,
			DisplayName: name,
			Token:       token,
		},
		Agent: AgentConfig{
			Listen:       listen,
			DataDir:      "./data/" + id,
			DownloadsDir: "./data/" + id + "/downloads",
		},
		HubClient: HubClientConfig{
			BaseURL:      "http://127.0.0.1:47891",
			SSEURL:       "http://127.0.0.1:47891/api/events",
			UseSSHTunnel: false,
		},
		SSHTunnel: SSHTunnelConfig{
			Enabled:    false,
			SSHHost:    "quickdrop-server",
			LocalPort:  47891,
			RemoteHost: "127.0.0.1",
			RemotePort: 47891,
		},
		GUI: GUIConfig{
			Listen:   guiListen,
			Language: "zh-CN",
		},
	}
}
