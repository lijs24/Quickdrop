package transport

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"time"

	"quickdrop/internal/config"
)

type Tunnel struct {
	cmd    *exec.Cmd
	done   chan error
	cancel context.CancelFunc
}

func StartTunnelIfEnabled(ctx context.Context, cfg *config.Config) (*Tunnel, string, error) {
	if !cfg.HubClient.UseSSHTunnel && !cfg.SSHTunnel.Enabled {
		return nil, cfg.HubClient.BaseURL, nil
	}
	tunnelCfg := cfg.SSHTunnel
	if tunnelCfg.SSHHost == "" {
		return nil, "", fmt.Errorf("ssh tunnel enabled but ssh_tunnel.ssh_host is empty")
	}
	if tunnelCfg.LocalPort == 0 || tunnelCfg.RemotePort == 0 {
		return nil, "", fmt.Errorf("ssh tunnel enabled but local_port or remote_port is empty")
	}
	if tunnelCfg.RemoteHost == "" {
		tunnelCfg.RemoteHost = "127.0.0.1"
	}

	tunnelCtx, cancel := context.WithCancel(ctx)
	forward := fmt.Sprintf("%d:%s:%d", tunnelCfg.LocalPort, tunnelCfg.RemoteHost, tunnelCfg.RemotePort)
	cmd := exec.CommandContext(tunnelCtx, "ssh", "-N", "-L", forward, tunnelCfg.SSHHost)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, "", fmt.Errorf("start ssh tunnel with `ssh -N -L %s %s`: %w", forward, tunnelCfg.SSHHost, err)
	}
	tunnel := &Tunnel{
		cmd:    cmd,
		done:   make(chan error, 1),
		cancel: cancel,
	}
	go func() {
		tunnel.done <- cmd.Wait()
	}()

	select {
	case err := <-tunnel.done:
		cancel()
		if err != nil {
			return nil, "", fmt.Errorf("ssh tunnel exited immediately: %w", err)
		}
		return nil, "", fmt.Errorf("ssh tunnel exited immediately")
	case <-time.After(700 * time.Millisecond):
	}

	baseURL := url.URL{Scheme: "http", Host: "127.0.0.1:" + strconv.Itoa(tunnelCfg.LocalPort)}
	return tunnel, baseURL.String(), nil
}

func (t *Tunnel) Close() error {
	if t == nil {
		return nil
	}
	t.cancel()
	select {
	case err := <-t.done:
		return err
	case <-time.After(3 * time.Second):
		if t.cmd.Process != nil {
			return t.cmd.Process.Kill()
		}
		return nil
	}
}
