package integration_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

type deviceConfig struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Token       string `json:"token"`
}

type testConfig struct {
	Role      string         `json:"role"`
	Device    deviceConfig   `json:"device"`
	Hub       map[string]any `json:"hub,omitempty"`
	Agent     map[string]any `json:"agent,omitempty"`
	HubClient map[string]any `json:"hub_client,omitempty"`
	SSHTunnel map[string]any `json:"ssh_tunnel,omitempty"`
	GUI       map[string]any `json:"gui,omitempty"`
	Devices   []deviceConfig `json:"devices,omitempty"`
	Groups    []testGroup    `json:"groups,omitempty"`
}

type testGroup struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Members []string `json:"members"`
}

type processHandle struct {
	cmd    *exec.Cmd
	cancel context.CancelFunc
}

func TestLocalMultiDeviceTransfersAndOfflineReplay(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	root := repoRoot(t)
	tmp := t.TempDir()
	exe := buildQuickDrop(t, root, tmp)
	ports := allocatePorts(t, 7)
	hubPort := ports[0]
	hubBaseURL := fmt.Sprintf("http://127.0.0.1:%d", hubPort)

	paths := writeIntegrationConfigs(t, tmp, hubPort, ports[1:])
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	var procs []*processHandle
	defer func() {
		for i := len(procs) - 1; i >= 0; i-- {
			stopProcess(procs[i])
		}
	}()

	procs = append(procs, startProcess(t, ctx, exe, tmp, "hub", paths["hub"]))
	waitForHealth(t, ctx, hubBaseURL)
	procs = append(procs, startProcess(t, ctx, exe, tmp, "agent", paths["laptop"]))
	procs = append(procs, startProcess(t, ctx, exe, tmp, "agent", paths["workstation"]))
	procs = append(procs, startProcess(t, ctx, exe, tmp, "agent", paths["server"]))
	waitForOnline(t, ctx, hubBaseURL, "laptop", []string{"laptop", "workstation", "main-server"})
	waitForMonitorOnline(t, ctx, hubBaseURL, "laptop", []string{"laptop", "workstation", "main-server"})

	// Bidirectional text delivery between laptop and workstation.
	runQuickDrop(t, ctx, exe, tmp, "text", "-c", paths["laptop"], "device:workstation", "laptop to workstation")
	waitForLog(t, ctx, filepath.Join(tmp, "logs", "workstation.err.log"), "New text from laptop: laptop to workstation")
	runQuickDrop(t, ctx, exe, tmp, "text", "-c", paths["workstation"], "device:laptop", "workstation to laptop")
	waitForLog(t, ctx, filepath.Join(tmp, "logs", "laptop.err.log"), "New text from workstation: workstation to laptop")

	// Bidirectional file delivery with content verification.
	laptopPayload := filepath.Join(tmp, "payloads", "from-laptop.txt")
	workstationPayload := filepath.Join(tmp, "payloads", "from-workstation.txt")
	mustWriteFile(t, laptopPayload, []byte("file from laptop to workstation\n"))
	mustWriteFile(t, workstationPayload, []byte("file from workstation to laptop\n"))
	out := runQuickDrop(t, ctx, exe, tmp, "send", "-c", paths["laptop"], "device:workstation", laptopPayload)
	msgLaptopFile := parseMessageID(t, out)
	waitForDownloadedFile(t, ctx, filepath.Join(tmp, "data", "workstation", "downloads"), msgLaptopFile, "from-laptop.txt", sha256Hex(t, laptopPayload))
	out = runQuickDrop(t, ctx, exe, tmp, "send", "-c", paths["workstation"], "device:laptop", workstationPayload)
	msgWorkstationFile := parseMessageID(t, out)
	waitForDownloadedFile(t, ctx, filepath.Join(tmp, "data", "laptop", "downloads"), msgWorkstationFile, "from-workstation.txt", sha256Hex(t, workstationPayload))

	// Group fan-out to all three configured devices.
	out = runQuickDrop(t, ctx, exe, tmp, "text", "-c", paths["server"], "group:all", "group hello from server")
	groupMessageID := parseMessageID(t, out)
	waitForLog(t, ctx, filepath.Join(tmp, "logs", "laptop.err.log"), "New text from main-server: group hello from server")
	waitForLog(t, ctx, filepath.Join(tmp, "logs", "workstation.err.log"), "New text from main-server: group hello from server")
	waitForDeliveryCount(t, ctx, filepath.Join(tmp, "data", "hub", "quickdrop.db"), groupMessageID, 3)

	// Offline delivery stays pending and is replayed when the target reconnects.
	stopByConfig(t, &procs, "workstation")
	waitForOffline(t, ctx, hubBaseURL, "laptop", "workstation")
	out = runQuickDrop(t, ctx, exe, tmp, "text", "-c", paths["laptop"], "device:workstation", "offline text for workstation")
	offlineMessageID := parseMessageID(t, out)
	waitForDeliveryStatus(t, ctx, filepath.Join(tmp, "data", "hub", "quickdrop.db"), offlineMessageID, "workstation", "pending")
	waitForMonitorPending(t, ctx, hubBaseURL, "laptop", "workstation", 1)

	procs = append(procs, startProcessNamed(t, ctx, exe, tmp, "workstation-restart", "agent", paths["workstation"]))
	waitForLog(t, ctx, filepath.Join(tmp, "logs", "workstation-restart.err.log"), "New text from laptop: offline text for workstation")
	waitForDeliveryStatus(t, ctx, filepath.Join(tmp, "data", "hub", "quickdrop.db"), offlineMessageID, "workstation", "delivered")
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			return wd
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			t.Fatal("could not find repo root")
		}
		wd = parent
	}
}

func buildQuickDrop(t *testing.T, root, tmp string) string {
	t.Helper()
	exe := filepath.Join(tmp, "quickdrop"+exeSuffix())
	cmd := exec.Command(goTool(t), "build", "-o", exe, "./cmd/quickdrop")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build quickdrop: %v\n%s", err, out)
	}
	return exe
}

func goTool(t *testing.T) string {
	t.Helper()
	name := "go"
	if runtime.GOOS == "windows" {
		name = "go.exe"
	}
	path := filepath.Join(runtime.GOROOT(), "bin", name)
	if _, err := os.Stat(path); err == nil {
		return path
	}
	return name
}

func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

func allocatePorts(t *testing.T, count int) []int {
	t.Helper()
	ports := make([]int, 0, count)
	for len(ports) < count {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("allocate port: %v", err)
		}
		port := listener.Addr().(*net.TCPAddr).Port
		_ = listener.Close()
		ports = append(ports, port)
	}
	return ports
}

func writeIntegrationConfigs(t *testing.T, tmp string, hubPort int, ports []int) map[string]string {
	t.Helper()
	devices := []deviceConfig{
		{ID: "laptop", DisplayName: "Laptop", Token: "dev-laptop-token"},
		{ID: "workstation", DisplayName: "Workstation", Token: "dev-workstation-token"},
		{ID: "main-server", DisplayName: "Main Server", Token: "dev-main-server-token"},
	}
	hubBaseURL := fmt.Sprintf("http://127.0.0.1:%d", hubPort)
	paths := map[string]string{}
	paths["hub"] = writeConfig(t, tmp, "hub.json", testConfig{
		Role:   "hub",
		Device: devices[2],
		Hub: map[string]any{
			"listen":           fmt.Sprintf("127.0.0.1:%d", hubPort),
			"data_dir":         filepath.Join(tmp, "data", "hub"),
			"max_upload_bytes": 104857600,
		},
		Devices: devices,
		Groups: []testGroup{
			{ID: "all", Name: "All Devices", Members: []string{"laptop", "workstation", "main-server"}},
		},
	})
	paths["laptop"] = writeAgentConfig(t, tmp, "laptop", devices[0], hubBaseURL, ports[0], ports[3])
	paths["workstation"] = writeAgentConfig(t, tmp, "workstation", devices[1], hubBaseURL, ports[1], ports[4])
	paths["server"] = writeAgentConfig(t, tmp, "server", devices[2], hubBaseURL, ports[2], ports[5])
	return paths
}

func writeAgentConfig(t *testing.T, tmp, name string, device deviceConfig, hubBaseURL string, agentPort, guiPort int) string {
	t.Helper()
	return writeConfig(t, tmp, name+".json", testConfig{
		Role:   "agent",
		Device: device,
		Agent: map[string]any{
			"listen":        fmt.Sprintf("127.0.0.1:%d", agentPort),
			"data_dir":      filepath.Join(tmp, "data", device.ID),
			"downloads_dir": filepath.Join(tmp, "data", device.ID, "downloads"),
		},
		HubClient: map[string]any{
			"base_url":       hubBaseURL,
			"sse_url":        hubBaseURL + "/api/events",
			"use_ssh_tunnel": false,
		},
		SSHTunnel: map[string]any{
			"enabled":     false,
			"ssh_host":    "unused",
			"local_port":  47891,
			"remote_host": "127.0.0.1",
			"remote_port": 47891,
		},
		GUI: map[string]any{
			"listen": fmt.Sprintf("127.0.0.1:%d", guiPort),
		},
	})
}

func writeConfig(t *testing.T, tmp, name string, cfg testConfig) string {
	t.Helper()
	dir := filepath.Join(tmp, "configs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func startProcess(t *testing.T, ctx context.Context, exe, tmp, role, configPath string) *processHandle {
	t.Helper()
	return startProcessNamed(t, ctx, exe, tmp, filepath.Base(strings.TrimSuffix(configPath, ".json")), role, configPath)
}

func startProcessNamed(t *testing.T, ctx context.Context, exe, tmp, name, role, configPath string) *processHandle {
	t.Helper()
	procCtx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(procCtx, exe, role, "-c", configPath)
	cmd.Dir = tmp
	logDir := filepath.Join(tmp, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}
	stdoutPath := filepath.Join(logDir, name+".out.log")
	stderrPath := filepath.Join(logDir, name+".err.log")
	stdout, err := os.Create(stdoutPath)
	if err != nil {
		t.Fatal(err)
	}
	stderr, err := os.Create(stderrPath)
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatalf("start %s %s: %v", role, configPath, err)
	}
	go func() {
		_ = cmd.Wait()
		_ = stdout.Close()
		_ = stderr.Close()
	}()
	return &processHandle{cmd: cmd, cancel: cancel}
}

func stopProcess(proc *processHandle) {
	if proc == nil {
		return
	}
	proc.cancel()
	if proc.cmd.Process != nil {
		_ = proc.cmd.Process.Kill()
	}
}

func stopByConfig(t *testing.T, procs *[]*processHandle, name string) {
	t.Helper()
	for i, proc := range *procs {
		if proc != nil && strings.Contains(strings.Join(proc.cmd.Args, " "), name+".json") {
			stopProcess(proc)
			(*procs)[i] = nil
			return
		}
	}
	t.Fatalf("process for %s not found", name)
}

func runQuickDrop(t *testing.T, ctx context.Context, exe, tmp string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Dir = tmp
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("quickdrop %s failed: %v\n%s", strings.Join(args, " "), err, out.String())
	}
	return out.String()
}

func waitForHealth(t *testing.T, ctx context.Context, baseURL string) {
	t.Helper()
	waitFor(t, ctx, func() (bool, string) {
		resp, err := http.Get(baseURL + "/api/health")
		if err != nil {
			return false, err.Error()
		}
		defer resp.Body.Close()
		return resp.StatusCode == http.StatusOK, resp.Status
	})
}

func waitForOnline(t *testing.T, ctx context.Context, baseURL, authDevice string, ids []string) {
	t.Helper()
	waitFor(t, ctx, func() (bool, string) {
		devices, err := getDevices(ctx, baseURL, authDevice)
		if err != nil {
			return false, err.Error()
		}
		for _, id := range ids {
			if !devices[id] {
				return false, fmt.Sprintf("%s is offline", id)
			}
		}
		return true, ""
	})
}

func waitForOffline(t *testing.T, ctx context.Context, baseURL, authDevice, id string) {
	t.Helper()
	waitFor(t, ctx, func() (bool, string) {
		devices, err := getDevices(ctx, baseURL, authDevice)
		if err != nil {
			return false, err.Error()
		}
		return !devices[id], fmt.Sprintf("%s is still online", id)
	})
}

func getDevices(ctx context.Context, baseURL, authDevice string) (map[string]bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/devices", nil)
	if err != nil {
		return nil, err
	}
	setAuthHeaders(req, authDevice)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("devices returned %s: %s", resp.Status, data)
	}
	var payload struct {
		Devices []struct {
			ID     string `json:"id"`
			Online bool   `json:"online"`
		} `json:"devices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	devices := map[string]bool{}
	for _, dev := range payload.Devices {
		devices[dev.ID] = dev.Online
	}
	return devices, nil
}

type monitorDevice struct {
	ID                string `json:"id"`
	Online            bool   `json:"online"`
	LastSeenAt        string `json:"last_seen_at"`
	SSEConnections    int    `json:"sse_connections"`
	PendingDeliveries int    `json:"pending_deliveries"`
}

func waitForMonitorOnline(t *testing.T, ctx context.Context, baseURL, authDevice string, ids []string) {
	t.Helper()
	waitFor(t, ctx, func() (bool, string) {
		devices, err := getMonitor(ctx, baseURL, authDevice)
		if err != nil {
			return false, err.Error()
		}
		for _, id := range ids {
			dev, ok := devices[id]
			if !ok {
				return false, fmt.Sprintf("%s missing from monitor", id)
			}
			if !dev.Online {
				return false, fmt.Sprintf("%s is offline in monitor", id)
			}
			if dev.SSEConnections < 1 {
				return false, fmt.Sprintf("%s has no SSE connection in monitor", id)
			}
			if dev.LastSeenAt == "" {
				return false, fmt.Sprintf("%s has no last_seen_at in monitor", id)
			}
		}
		return true, ""
	})
}

func waitForMonitorPending(t *testing.T, ctx context.Context, baseURL, authDevice, id string, minPending int) {
	t.Helper()
	waitFor(t, ctx, func() (bool, string) {
		devices, err := getMonitor(ctx, baseURL, authDevice)
		if err != nil {
			return false, err.Error()
		}
		dev, ok := devices[id]
		if !ok {
			return false, fmt.Sprintf("%s missing from monitor", id)
		}
		return dev.PendingDeliveries >= minPending, fmt.Sprintf("%s pending deliveries = %d", id, dev.PendingDeliveries)
	})
}

func getMonitor(ctx context.Context, baseURL, authDevice string) (map[string]monitorDevice, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/monitor", nil)
	if err != nil {
		return nil, err
	}
	setAuthHeaders(req, authDevice)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("monitor returned %s: %s", resp.Status, data)
	}
	var payload struct {
		Devices []monitorDevice `json:"devices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	devices := map[string]monitorDevice{}
	for _, dev := range payload.Devices {
		devices[dev.ID] = dev
	}
	return devices, nil
}

func setAuthHeaders(req *http.Request, authDevice string) {
	req.Header.Set("X-Device-ID", authDevice)
	req.Header.Set("Authorization", "Bearer dev-"+authDevice+"-token")
	if authDevice == "main-server" {
		req.Header.Set("Authorization", "Bearer dev-main-server-token")
	}
}

func waitForLog(t *testing.T, ctx context.Context, path, needle string) {
	t.Helper()
	waitFor(t, ctx, func() (bool, string) {
		data, err := os.ReadFile(path)
		if err != nil {
			return false, err.Error()
		}
		if strings.Contains(string(data), needle) {
			return true, ""
		}
		return false, fmt.Sprintf("%q not in %s", needle, path)
	})
}

func waitForDownloadedFile(t *testing.T, ctx context.Context, downloadsDir, messageID, fileName, expectedSHA string) {
	t.Helper()
	path := filepath.Join(downloadsDir, messageID, fileName)
	waitFor(t, ctx, func() (bool, string) {
		if _, err := os.Stat(path); err != nil {
			return false, err.Error()
		}
		actual := sha256Hex(t, path)
		if actual != expectedSHA {
			return false, fmt.Sprintf("sha mismatch: got %s want %s", actual, expectedSHA)
		}
		return true, ""
	})
}

func waitForDeliveryCount(t *testing.T, ctx context.Context, dbPath, messageID string, want int) {
	t.Helper()
	waitFor(t, ctx, func() (bool, string) {
		db, err := sql.Open("sqlite", dbDSN(dbPath))
		if err != nil {
			return false, err.Error()
		}
		defer db.Close()
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM deliveries WHERE message_id = ?`, messageID).Scan(&count); err != nil {
			return false, err.Error()
		}
		return count == want, fmt.Sprintf("got %d deliveries, want %d", count, want)
	})
}

func waitForDeliveryStatus(t *testing.T, ctx context.Context, dbPath, messageID, deviceID, want string) {
	t.Helper()
	waitFor(t, ctx, func() (bool, string) {
		db, err := sql.Open("sqlite", dbDSN(dbPath))
		if err != nil {
			return false, err.Error()
		}
		defer db.Close()
		var status string
		if err := db.QueryRow(`SELECT status FROM deliveries WHERE message_id = ? AND target_device_id = ?`, messageID, deviceID).Scan(&status); err != nil {
			return false, err.Error()
		}
		return status == want, fmt.Sprintf("got status %s, want %s", status, want)
	})
}

func dbDSN(path string) string {
	abs, _ := filepath.Abs(path)
	return "file:" + filepath.ToSlash(abs) + "?_pragma=busy_timeout(5000)"
}

func waitFor(t *testing.T, ctx context.Context, check func() (bool, string)) {
	t.Helper()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	var last string
	for {
		ok, msg := check()
		if ok {
			return
		}
		last = msg
		select {
		case <-ctx.Done():
			t.Fatalf("timed out waiting: %s", last)
		case <-ticker.C:
		}
	}
}

func parseMessageID(t *testing.T, output string) string {
	t.Helper()
	fields := strings.Fields(output)
	for i, field := range fields {
		if field == "as" && i+1 < len(fields) {
			return strings.TrimSpace(fields[i+1])
		}
	}
	t.Fatalf("could not parse message id from output: %s", output)
	return ""
}

func mustWriteFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func sha256Hex(t *testing.T, path string) string {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(hash.Sum(nil))
}
