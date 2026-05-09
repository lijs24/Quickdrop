package updater

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestSelectAsset(t *testing.T) {
	assets := []Asset{
		{Name: "quickdrop-v0.2.0-windows-amd64.zip"},
		{Name: "quickdrop-v0.2.0-linux-amd64.tar.gz"},
		{Name: "checksums.txt"},
	}
	asset, err := SelectAsset(assets, "windows", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if asset.Name != "quickdrop-v0.2.0-windows-amd64.zip" {
		t.Fatalf("unexpected asset %q", asset.Name)
	}
	asset, err = SelectAsset(assets, "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if asset.Name != "quickdrop-v0.2.0-linux-amd64.tar.gz" {
		t.Fatalf("unexpected asset %q", asset.Name)
	}
}

func TestSelectChecksumAsset(t *testing.T) {
	asset, err := SelectChecksumAsset([]Asset{{Name: "checksums.txt"}})
	if err != nil {
		t.Fatal(err)
	}
	if asset.Name != "checksums.txt" {
		t.Fatalf("unexpected checksum asset %q", asset.Name)
	}
}

func TestIsTargetNewer(t *testing.T) {
	tests := []struct {
		name    string
		current string
		target  string
		want    bool
	}{
		{name: "same", current: "v0.2.0", target: "v0.2.0", want: false},
		{name: "newer patch", current: "v0.2.0", target: "v0.2.1", want: true},
		{name: "older release than local", current: "v0.2.0-local", target: "v0.1.0", want: false},
		{name: "dev can update", current: "dev", target: "v0.1.0", want: true},
		{name: "missing current can update", current: "", target: "v0.1.0", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsTargetNewer(tt.current, tt.target); got != tt.want {
				t.Fatalf("IsTargetNewer(%q, %q) = %v, want %v", tt.current, tt.target, got, tt.want)
			}
		})
	}
}

func TestDownloadAssetUsesGitHubAssetAPI(t *testing.T) {
	t.Setenv("GH_TOKEN", "test-token")
	t.Setenv("GITHUB_TOKEN", "")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/asset" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("Accept"); got != "application/octet-stream" {
			t.Fatalf("Accept = %q, want application/octet-stream", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("Authorization = %q, want bearer token", got)
		}
		_, _ = w.Write([]byte("asset payload"))
	}))
	defer server.Close()

	dest := filepath.Join(t.TempDir(), "asset.zip")
	err := DownloadAsset(context.Background(), Asset{
		URL:                server.URL + "/asset",
		BrowserDownloadURL: server.URL + "/wrong",
		Name:               "asset.zip",
	}, dest)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "asset payload" {
		t.Fatalf("downloaded %q", string(data))
	}
}
