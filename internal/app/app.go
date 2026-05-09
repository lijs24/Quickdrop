package app

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"sync/atomic"
	"time"

	"quickdrop/internal/agent"
	"quickdrop/internal/config"
	"quickdrop/internal/hub"
	"quickdrop/internal/transport"
	"quickdrop/internal/updater"
	"quickdrop/internal/webui"
)

type Options struct {
	Version       string
	OpenBrowser   bool
	ExitOnClose   bool
	CloseGrace    time.Duration
	HubConfigPath string
}

func Run(ctx context.Context, cfg *config.Config, hubCfg *config.Config, opts Options) error {
	if opts.CloseGrace == 0 {
		opts.CloseGrace = 12 * time.Second
	}

	appCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	baseURL := cfg.HubClient.BaseURL
	var tunnel *transport.Tunnel
	if hubCfg != nil {
		errCh := make(chan error, 1)
		go func() {
			errCh <- hub.Run(appCtx, hubCfg)
		}()
		if err := waitForHub(appCtx, baseURL, 8*time.Second); err != nil {
			cancel()
			select {
			case hubErr := <-errCh:
				if hubErr != nil {
					return hubErr
				}
			default:
			}
			return err
		}
	} else {
		var err error
		tunnel, baseURL, err = transport.StartTunnelIfEnabled(appCtx, cfg)
		if err != nil {
			return err
		}
		defer tunnel.Close()
	}

	var seenHeartbeat atomic.Bool
	var lastHeartbeat atomic.Int64
	onHeartbeat := func() {
		seenHeartbeat.Store(true)
		lastHeartbeat.Store(time.Now().UnixNano())
	}
	if opts.ExitOnClose {
		go func() {
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-appCtx.Done():
					return
				case <-ticker.C:
					if !seenHeartbeat.Load() {
						continue
					}
					last := time.Unix(0, lastHeartbeat.Load())
					if time.Since(last) > opts.CloseGrace {
						fmt.Println("[QuickDrop] GUI page closed; shutting down app services.")
						cancel()
						return
					}
				}
			}
		}()
	}

	errCh := make(chan error, 3)
	go func() {
		errCh <- agent.RunWithBaseURL(appCtx, cfg, baseURL)
	}()
	go func() {
		errCh <- webui.RunWithBaseURL(appCtx, cfg, baseURL, webui.Options{
			AppMode:     true,
			Version:     opts.Version,
			OnShutdown:  cancel,
			OnHeartbeat: onHeartbeat,
			OnUpdate: func(updateCtx context.Context) (any, error) {
				result, err := updater.Run(updateCtx, updater.Options{
					Repo:           "lijs24/Quickdrop",
					Version:        "latest",
					CurrentVersion: opts.Version,
				})
				if err == nil && !result.AlreadyCurrent {
					go cancel()
				}
				return result, err
			},
		})
	}()

	localURL := webui.LocalURL(cfg.GUI.Listen)
	if err := waitForLocalPage(appCtx, localURL, 8*time.Second); err != nil {
		cancel()
		return err
	}
	fmt.Printf("[QuickDrop] App for %s is ready: %s\n", cfg.Device.ID, localURL)
	if opts.OpenBrowser {
		if err := OpenBrowser(localURL); err != nil {
			fmt.Printf("[QuickDrop] Open this URL manually: %s (%v)\n", localURL, err)
		}
	}

	select {
	case <-appCtx.Done():
		return nil
	case err := <-errCh:
		cancel()
		if err != nil && appCtx.Err() == nil {
			return err
		}
		return nil
	}
}

func waitForHub(ctx context.Context, baseURL string, timeout time.Duration) error {
	return waitForHTTP(ctx, baseURL+"/api/health", timeout)
}

func waitForLocalPage(ctx context.Context, url string, timeout time.Duration) error {
	return waitForHTTP(ctx, url, timeout)
}

func waitForHTTP(ctx context.Context, url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 800 * time.Millisecond}
	var lastErr error
	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 500 {
				return nil
			}
			lastErr = fmt.Errorf("%s returned %s", url, resp.Status)
		} else {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	if lastErr != nil {
		return fmt.Errorf("wait for %s: %w", url, lastErr)
	}
	return fmt.Errorf("wait for %s timed out", url)
}

func OpenBrowser(url string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		return exec.Command("open", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}
