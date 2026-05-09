# Packaging and GitHub Releases

QuickDrop uses portable release packages. There is no installer, service registration, or auto-update step yet.

## Local Packaging

From the repository root:

```powershell
powershell -ExecutionPolicy Bypass -File scripts\package.ps1 -Version v0.1.0
```

Outputs are written to `dist/`:

- `quickdrop-v0.1.0-windows-amd64.zip`
- `quickdrop-v0.1.0-windows-arm64.zip`
- `quickdrop-v0.1.0-linux-amd64.tar.gz`
- `quickdrop-v0.1.0-linux-arm64.tar.gz`
- `quickdrop-v0.1.0-darwin-amd64.tar.gz`
- `quickdrop-v0.1.0-darwin-arm64.tar.gz`
- `checksums.txt`

Each package contains:

- `quickdrop` or `quickdrop.exe`
- `configs/dev/*.json`
- `start-hub`, `start-agent`, and `start-gui` scripts for the target OS
- `README.md`, `QUICKSTART.md`, and `INTEGRATION.md`

On Linux/macOS, run `chmod +x quickdrop start-*.sh` after extracting if your archive tool does not preserve executable bits.

## GitHub Release Flow

1. Push the repository to GitHub.
2. Create a tag:

```powershell
git tag v0.1.0
git push origin v0.1.0
```

3. GitHub Actions runs tests, builds all portable packages, uploads artifacts, and publishes the release assets.

Manual packaging without publishing is also available from the `Release` workflow through `workflow_dispatch`.

## Device Distribution

For the MVP, distribute the matching package to each device:

- Windows: use the `windows-amd64` or `windows-arm64` zip.
- macOS Apple Silicon: use `darwin-arm64`.
- macOS Intel: use `darwin-amd64`.
- Linux: use the matching Linux package.

One device runs Hub. Each other device runs Agent and GUI with its own config. The GUI Settings panel can edit language, identity, Hub URL, SSH tunnel fields, and local directories.

For real cross-device use, prefer SSH tunnel mode first:

- Hub listens on `127.0.0.1:<remote_port>`.
- Each device runs an SSH local forward.
- Agent/GUI connect to `http://127.0.0.1:<local_port>`.

The dev configs are convenient starter templates. Replace dev tokens before using QuickDrop as a real personal transfer tool.
