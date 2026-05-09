package webui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"quickdrop/internal/config"
	"quickdrop/internal/transport"
	assets "quickdrop/webui"
)

type Options struct {
	AppMode     bool
	Version     string
	OnShutdown  func()
	OnHeartbeat func()
	OnUpdate    func(context.Context) (any, error)
}

func Run(ctx context.Context, cfg *config.Config) error {
	tunnel, baseURL, err := transport.StartTunnelIfEnabled(ctx, cfg)
	if err != nil {
		return err
	}
	defer tunnel.Close()
	return RunWithBaseURL(ctx, cfg, baseURL, Options{})
}

func RunWithBaseURL(ctx context.Context, cfg *config.Config, baseURL string, opts Options) error {
	target, err := url.Parse(baseURL)
	if err != nil {
		return fmt.Errorf("parse hub base_url: %w", err)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = target.Host
		req.Header.Set("X-Device-ID", cfg.Device.ID)
		req.Header.Set("Authorization", "Bearer "+cfg.Device.Token)
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		http.Error(w, "hub proxy error: "+err.Error(), http.StatusBadGateway)
	}
	proxy.FlushInterval = 100 * time.Millisecond

	mux := http.NewServeMux()
	mux.HandleFunc("/config", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "GET required", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"device_id":    cfg.Device.ID,
			"display_name": cfg.Device.DisplayName,
			"hub_base_url": baseURL,
			"language":     cfg.GUI.Language,
			"app_mode":     fmt.Sprintf("%t", opts.AppMode),
			"version":      opts.Version,
		})
	})
	mux.HandleFunc("/app/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "GET required", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"app_mode": opts.AppMode,
			"version":  opts.Version,
		})
	})
	mux.HandleFunc("/app/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		if opts.OnHeartbeat != nil {
			opts.OnHeartbeat()
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})
	mux.HandleFunc("/app/shutdown", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		if !opts.AppMode || opts.OnShutdown == nil {
			http.Error(w, "app shutdown is only available in app mode", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
		go opts.OnShutdown()
	})
	mux.HandleFunc("/app/update", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		if !opts.AppMode || opts.OnUpdate == nil {
			http.Error(w, "app update is only available in app mode", http.StatusNotFound)
			return
		}
		result, err := opts.OnUpdate(r.Context())
		if err != nil {
			http.Error(w, "update: "+err.Error(), http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	})
	mux.HandleFunc("/settings", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeSettings(w, cfg, baseURL)
		case http.MethodPost:
			var next config.Config
			dec := json.NewDecoder(r.Body)
			dec.DisallowUnknownFields()
			if err := dec.Decode(&next); err != nil {
				http.Error(w, "decode settings: "+err.Error(), http.StatusBadRequest)
				return
			}
			if next.Role == "" {
				next.Role = cfg.Role
			}
			if err := config.Save(cfg.Path, &next); err != nil {
				http.Error(w, "save settings: "+err.Error(), http.StatusBadRequest)
				return
			}
			*cfg = next
			writeSettings(w, cfg, baseURL)
		default:
			http.Error(w, "GET or POST required", http.StatusMethodNotAllowed)
		}
	})
	mux.Handle("/api/", proxy)
	mux.Handle("/", cacheControl(http.FileServer(http.FS(assets.FS))))

	httpServer := &http.Server{
		Addr:              cfg.GUI.Listen,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("[QuickDrop] GUI for %s listening on http://%s", cfg.Device.ID, cfg.GUI.Listen)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
		return nil
	case err := <-errCh:
		return err
	}
}

func cacheControl(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".js") || strings.HasSuffix(r.URL.Path, ".css") {
			w.Header().Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

func writeSettings(w http.ResponseWriter, cfg *config.Config, effectiveBaseURL string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"config_path":        cfg.Path,
		"effective_base_url": effectiveBaseURL,
		"config":             cfg,
		"restart_required":   true,
	})
}

func LocalURL(listen string) string {
	host := listen
	if strings.HasPrefix(host, ":") {
		host = "127.0.0.1" + host
	}
	if strings.HasPrefix(host, "0.0.0.0:") {
		host = "127.0.0.1:" + strings.TrimPrefix(host, "0.0.0.0:")
	}
	if strings.HasPrefix(host, "[::]:") {
		host = "127.0.0.1:" + strings.TrimPrefix(host, "[::]:")
	}
	return "http://" + host + "/"
}
