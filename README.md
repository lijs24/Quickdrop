# QuickDrop

QuickDrop is a personal, trusted-device message and file drop MVP. It is shaped like a small chat app: one device runs the Hub, other devices run Agents, and CLI/GUI clients send text, files, and group messages through the Hub.

This repository is intentionally lightweight: one Go module, SQLite storage, REST APIs, SSE events, multipart file upload, and an embedded static Web GUI.

## What Is Implemented

- `quickdrop hub` starts the Hub HTTP server.
- `quickdrop agent` connects to the Hub SSE stream, stores local history, downloads attachments, verifies SHA-256, and acks deliveries.
- `quickdrop text` sends text to `device:<id>` or `group:<id>`.
- `quickdrop send` uploads one file to `device:<id>` or `group:<id>`.
- `quickdrop devices` and `quickdrop groups` list Hub state.
- `quickdrop gui` starts a local browser UI that proxies Hub API calls with the configured device token.
- The Web GUI includes a Settings panel for editing the current device config without manually editing JSON.
- `quickdrop init-dev` writes local dev configs and data directories without overwriting existing config files unless `--force` is used.
- SSH tunnel configuration is present and can start system `ssh -N -L ...` when enabled.

## Windows PowerShell Demo

Terminal 1:

```powershell
go run ./cmd/quickdrop init-dev
go run ./cmd/quickdrop hub -c configs/dev/hub.json
```

Terminal 2:

```powershell
go run ./cmd/quickdrop agent -c configs/dev/laptop.json
```

Terminal 3:

```powershell
go run ./cmd/quickdrop agent -c configs/dev/workstation.json
```

Terminal 4:

```powershell
go run ./cmd/quickdrop devices -c configs/dev/laptop.json
go run ./cmd/quickdrop groups -c configs/dev/laptop.json
go run ./cmd/quickdrop text -c configs/dev/laptop.json device:workstation "hello from laptop"
go run ./cmd/quickdrop send -c configs/dev/laptop.json device:workstation README.md
go run ./cmd/quickdrop text -c configs/dev/laptop.json group:all "hello all"
```

Terminal 5:

```powershell
go run ./cmd/quickdrop gui -c configs/dev/laptop.json
```

Then open the printed URL, normally:

```text
http://127.0.0.1:47900
```

Use the `Settings` button in the GUI to edit the current device identity, language, Hub URL, local directories, GUI listen address, and SSH tunnel fields. Save writes back to the config file used to start the GUI. Restart the relevant QuickDrop processes after changing connection, identity, listen, or tunnel settings.

The workstation agent should print messages like:

```text
[QuickDrop] New text from laptop: hello from laptop
[QuickDrop] New file from laptop: README.md -> data/workstation/downloads/<message_id>/README.md
```

## CLI Targets

- `device:workstation` sends to one device.
- `group:all` sends to a group.
- A bare target such as `workstation` defaults to a device.
- A bare target such as `all` is treated as a group when the Hub has a group with that id.

## Data Layout

Hub:

```text
data/hub/
  quickdrop.db
  blobs/
  tmp/
  logs/
```

Agent:

```text
data/<device_id>/
  agent.db
  downloads/
  logs/
```

## API Authentication

Clients send:

```text
X-Device-ID: <device_id>
Authorization: Bearer <token>
```

The Hub stores `SHA-256(token)` through the `internal/auth` package. This is an MVP placeholder that is deliberately centralized for later replacement.

## SSH Tunnel

Local dev configs keep SSH tunneling disabled. To enable it, set either:

```json
"hub_client": {
  "use_ssh_tunnel": true
}
```

or:

```json
"ssh_tunnel": {
  "enabled": true
}
```

When enabled, QuickDrop calls the system SSH client:

```text
ssh -N -L <local_port>:<remote_host>:<remote_port> <ssh_host>
```

QuickDrop does not implement SSH itself and does not rely on the current Codex SSH session.

## Current Limits

- No installer or packaging.
- No Windows Service or systemd deployment.
- No system notifications.
- No automatic updates.
- No end-to-end encryption.
- No directory transfer.
- No mobile client.
- SSH tunnel support is basic and disabled by default in local dev.
- Hub defaults to `127.0.0.1`; real cross-device use should go through SSH tunnel or a later explicit LAN listen configuration.

## Development Checks

Run:

```powershell
gofmt -w ./cmd ./internal ./webui
go test ./...
```

The project rule is to run formatting and tests after each implementation stage.

More detailed local and SSH-based integration coverage is documented in `tests/INTEGRATION.md`.

## Packaging

Build portable release packages:

```powershell
powershell -ExecutionPolicy Bypass -File scripts\package.ps1 -Version v0.1.0
```

Packages are written to `dist/` for Windows, Linux, and macOS, with `checksums.txt`.

GitHub release automation is included in `.github/workflows/release.yml`. Push a tag such as `v0.1.0` to run tests, build all packages, and publish release assets.

See `PACKAGING.md` for the full distribution flow.
