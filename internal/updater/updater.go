package updater

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type Options struct {
	Repo           string
	Version        string
	CurrentVersion string
	InstallDir     string
	Force          bool
	DryRun         bool
}

type Result struct {
	CurrentVersion string
	TargetVersion  string
	AssetName      string
	InstallDir     string
	StagingDir     string
	ApplyScript    string
	AlreadyCurrent bool
}

type Release struct {
	TagName string  `json:"tag_name"`
	Assets  []Asset `json:"assets"`
	HTMLURL string  `json:"html_url"`
}

type Asset struct {
	URL                string `json:"url"`
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

func Run(ctx context.Context, opts Options) (Result, error) {
	if opts.Repo == "" {
		opts.Repo = "lijs24/Quickdrop"
	}
	if opts.Version == "" {
		opts.Version = "latest"
	}
	if opts.InstallDir == "" {
		exe, err := os.Executable()
		if err != nil {
			return Result{}, fmt.Errorf("resolve executable path: %w", err)
		}
		opts.InstallDir = filepath.Dir(exe)
	}
	release, err := FetchRelease(ctx, opts.Repo, opts.Version)
	if err != nil {
		return Result{}, err
	}
	result := Result{
		CurrentVersion: opts.CurrentVersion,
		TargetVersion:  release.TagName,
		InstallDir:     opts.InstallDir,
	}
	if !opts.Force && !IsTargetNewer(opts.CurrentVersion, release.TagName) {
		result.AlreadyCurrent = true
		return result, nil
	}
	asset, err := SelectAsset(release.Assets, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return result, err
	}
	checksums, err := SelectChecksumAsset(release.Assets)
	if err != nil {
		return result, err
	}
	result.AssetName = asset.Name
	if opts.DryRun {
		return result, nil
	}

	tmpRoot, err := os.MkdirTemp("", "quickdrop-update-*")
	if err != nil {
		return result, fmt.Errorf("create update temp directory: %w", err)
	}
	result.StagingDir = tmpRoot
	assetPath := filepath.Join(tmpRoot, asset.Name)
	checksumsPath := filepath.Join(tmpRoot, checksums.Name)
	if err := DownloadAsset(ctx, asset, assetPath); err != nil {
		return result, err
	}
	if err := DownloadAsset(ctx, checksums, checksumsPath); err != nil {
		return result, err
	}
	if err := VerifyChecksum(assetPath, checksumsPath); err != nil {
		return result, err
	}

	extractDir := filepath.Join(tmpRoot, "payload")
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		return result, fmt.Errorf("create extract directory: %w", err)
	}
	if err := Extract(assetPath, extractDir); err != nil {
		return result, err
	}
	payloadDir, err := FindPayloadDir(extractDir)
	if err != nil {
		return result, err
	}
	if runtime.GOOS == "windows" {
		script, err := WriteWindowsApplyScript(opts.InstallDir, payloadDir, tmpRoot, release.TagName)
		if err != nil {
			return result, err
		}
		result.ApplyScript = script
		if err := exec.Command("cmd", "/c", "start", "", script).Start(); err != nil {
			return result, fmt.Errorf("start update apply script: %w", err)
		}
		return result, nil
	}
	if err := CopyTree(payloadDir, opts.InstallDir); err != nil {
		return result, err
	}
	_ = os.RemoveAll(tmpRoot)
	return result, nil
}

func FetchRelease(ctx context.Context, repo, version string) (Release, error) {
	path := "latest"
	if version != "" && version != "latest" {
		path = "tags/" + version
	}
	url := "https://api.github.com/repos/" + repo + "/releases/" + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "QuickDrop-Updater")
	token := GitHubToken(ctx)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return Release{}, fmt.Errorf("fetch GitHub release: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if resp.StatusCode == http.StatusNotFound {
			if token == "" {
				return Release{}, fmt.Errorf("GitHub release request returned %s. The repository may be private or no published release exists. Sign in with Git Credential Manager, set GH_TOKEN/GITHUB_TOKEN, or publish a release: %s", resp.Status, strings.TrimSpace(string(data)))
			}
			return Release{}, fmt.Errorf("GitHub release request returned %s even with local GitHub credentials. Check that %s has a published release: %s", resp.Status, repo, strings.TrimSpace(string(data)))
		}
		return Release{}, fmt.Errorf("GitHub release request returned %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return Release{}, fmt.Errorf("decode GitHub release: %w", err)
	}
	return release, nil
}

func SelectAsset(assets []Asset, goos, goarch string) (Asset, error) {
	suffix := "-" + goos + "-" + goarch
	archiveSuffix := ".tar.gz"
	if goos == "windows" {
		archiveSuffix = ".zip"
	}
	for _, asset := range assets {
		if strings.Contains(asset.Name, suffix) && strings.HasSuffix(asset.Name, archiveSuffix) {
			return asset, nil
		}
	}
	return Asset{}, fmt.Errorf("no release asset found for %s/%s", goos, goarch)
}

func SelectChecksumAsset(assets []Asset) (Asset, error) {
	for _, asset := range assets {
		if asset.Name == "checksums.txt" {
			return asset, nil
		}
	}
	return Asset{}, errors.New("release does not include checksums.txt")
}

func IsTargetNewer(current, target string) bool {
	if current == "" || current == "dev" {
		return target != ""
	}
	if current == target {
		return false
	}
	currentVersion, okCurrent := parseVersion(current)
	targetVersion, okTarget := parseVersion(target)
	if !okCurrent || !okTarget {
		return current != target
	}
	for i := range targetVersion {
		if targetVersion[i] != currentVersion[i] {
			return targetVersion[i] > currentVersion[i]
		}
	}
	return false
}

func parseVersion(value string) ([3]int, bool) {
	var out [3]int
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "QuickDrop ")
	value = strings.TrimPrefix(value, "v")
	if i := strings.IndexAny(value, "-+"); i >= 0 {
		value = value[:i]
	}
	parts := strings.Split(value, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return out, false
	}
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

func GitHubToken(ctx context.Context) string {
	if token := firstNonEmpty(os.Getenv("GH_TOKEN"), os.Getenv("GITHUB_TOKEN")); token != "" {
		return token
	}
	return gitCredentialToken(ctx)
}

func gitCredentialToken(ctx context.Context) string {
	credCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(credCtx, "git", "credential", "fill")
	cmd.Stdin = strings.NewReader("protocol=https\nhost=github.com\n\n")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	for _, line := range bytes.Split(out, []byte{'\n'}) {
		key, value, ok := bytes.Cut(line, []byte{'='})
		if ok && string(key) == "password" {
			return strings.TrimSpace(string(value))
		}
	}
	return ""
}

func Download(ctx context.Context, url, dest string) error {
	return download(ctx, url, dest, false)
}

func DownloadAsset(ctx context.Context, asset Asset, dest string) error {
	if asset.URL != "" {
		return download(ctx, asset.URL, dest, true)
	}
	return download(ctx, asset.BrowserDownloadURL, dest, false)
}

func download(ctx context.Context, url, dest string, githubAssetAPI bool) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "QuickDrop-Updater")
	if githubAssetAPI {
		req.Header.Set("Accept", "application/octet-stream")
	}
	if token := GitHubToken(ctx); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := (&http.Client{Timeout: 10 * time.Minute}).Do(req)
	if err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download %s returned %s", url, resp.Status)
	}
	tmp := dest + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("create %s: %w", tmp, err)
	}
	if _, err := io.Copy(out, resp.Body); err != nil {
		_ = out.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("close %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("promote %s: %w", dest, err)
	}
	return nil
}

func VerifyChecksum(assetPath, checksumsPath string) error {
	data, err := os.ReadFile(checksumsPath)
	if err != nil {
		return fmt.Errorf("read checksums: %w", err)
	}
	name := filepath.Base(assetPath)
	var want string
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == name {
			want = strings.ToLower(fields[0])
			break
		}
	}
	if want == "" {
		return fmt.Errorf("checksums.txt does not contain %s", name)
	}
	file, err := os.Open(assetPath)
	if err != nil {
		return fmt.Errorf("open asset for checksum: %w", err)
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return fmt.Errorf("hash asset: %w", err)
	}
	got := hex.EncodeToString(hash.Sum(nil))
	if got != want {
		return fmt.Errorf("checksum mismatch for %s: got %s want %s", name, got, want)
	}
	return nil
}

func Extract(archivePath, dest string) error {
	if strings.HasSuffix(archivePath, ".zip") {
		return extractZip(archivePath, dest)
	}
	if strings.HasSuffix(archivePath, ".tar.gz") {
		return extractTarGz(archivePath, dest)
	}
	return fmt.Errorf("unsupported archive format: %s", archivePath)
}

func extractZip(archivePath, dest string) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer reader.Close()
	for _, file := range reader.File {
		target, err := safeJoin(dest, file.Name)
		if err != nil {
			return err
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		src, err := file.Open()
		if err != nil {
			return err
		}
		if err := writeFile(target, src, file.FileInfo().Mode()); err != nil {
			_ = src.Close()
			return err
		}
		_ = src.Close()
	}
	return nil
}

func extractTarGz(archivePath, dest string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open tar.gz: %w", err)
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("open gzip: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read tar: %w", err)
		}
		target, err := safeJoin(dest, header.Name)
		if err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			if err := writeFile(target, tr, os.FileMode(header.Mode)); err != nil {
				return err
			}
		}
	}
}

func safeJoin(root, name string) (string, error) {
	clean := filepath.Clean(name)
	if filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." {
		return "", fmt.Errorf("archive contains unsafe path: %s", name)
	}
	target := filepath.Join(root, clean)
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	if targetAbs != rootAbs && !strings.HasPrefix(targetAbs, rootAbs+string(filepath.Separator)) {
		return "", fmt.Errorf("archive path escapes destination: %s", name)
	}
	return target, nil
}

func writeFile(path string, src io.Reader, mode os.FileMode) error {
	out, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, src); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func FindPayloadDir(root string) (string, error) {
	binary := "quickdrop"
	if runtime.GOOS == "windows" {
		binary = "quickdrop.exe"
	}
	if _, err := os.Stat(filepath.Join(root, binary)); err == nil {
		return root, nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		candidate := filepath.Join(root, entry.Name())
		if _, err := os.Stat(filepath.Join(candidate, binary)); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("extracted package does not contain %s", binary)
}

func WriteWindowsApplyScript(installDir, payloadDir, stagingRoot, version string) (string, error) {
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		return "", fmt.Errorf("create install directory: %w", err)
	}
	pid := os.Getpid()
	script := filepath.Join(installDir, "quickdrop-apply-update.cmd")
	body := fmt.Sprintf(`@echo off
setlocal
echo Waiting for QuickDrop to exit...
:wait
tasklist /FI "PID eq %d" | find "%d" >nul
if not errorlevel 1 (
  timeout /t 1 /nobreak >nul
  goto wait
)
echo Applying QuickDrop %s...
robocopy "%s" "%s" /E /COPY:DAT /R:2 /W:1
set RC=%%ERRORLEVEL%%
if %%RC%% GEQ 8 (
  echo QuickDrop update failed with code %%RC%%.
  pause
  exit /b %%RC%%
)
rmdir /S /Q "%s" >nul 2>nul
echo QuickDrop updated to %s.
pause
`, pid, pid, version, payloadDir, installDir, stagingRoot, version)
	if err := os.WriteFile(script, []byte(body), 0o600); err != nil {
		return "", fmt.Errorf("write apply script: %w", err)
	}
	return script, nil
}

func CopyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		return writeFile(target, in, info.Mode())
	})
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
