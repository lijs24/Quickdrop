# QuickDrop Integration Tests

This file defines the integration coverage for the MVP. The default local integration test is executable with `go test ./...`.

## Local Multi-Device Matrix

Implemented in `tests/integration/local_multi_device_test.go`.

The test creates an isolated temp workspace, builds `quickdrop`, allocates random localhost ports, writes configs, and starts:

- one Hub
- `laptop` Agent
- `workstation` Agent
- `main-server` Agent

Assertions:

- all three Agents become online through Hub `/api/devices`
- a non-Hub device can read `/api/monitor`, including online state, SSE connection counts, and last-seen values
- `laptop -> workstation` text is delivered and logged
- `workstation -> laptop` text is delivered and logged
- `laptop -> workstation` file is uploaded, downloaded, and SHA-256 verified
- `workstation -> laptop` file is uploaded, downloaded, and SHA-256 verified
- `main-server -> group:all` expands to three deliveries
- group text reaches both other online Agents
- when `workstation` is stopped, Hub marks it offline
- a `laptop -> workstation` message stays `pending`
- `/api/monitor` reports the pending delivery while `workstation` is offline
- after `workstation` restarts, the pending message is replayed and becomes `delivered`

Run:

```powershell
go test ./...
```

Or only the integration package:

```powershell
go test ./tests/integration -run TestLocalMultiDeviceTransfersAndOfflineReplay -v
```

## Manual Cross-Device Content Cases

Use these when debugging a live demo or remote setup.

1. Device-to-device text:

```powershell
go run ./cmd/quickdrop text -c configs/dev/laptop.json device:workstation "laptop text 1"
go run ./cmd/quickdrop text -c configs/dev/workstation.json device:laptop "workstation text 1"
```

Expected:

- workstation Agent logs `New text from laptop`
- laptop Agent logs `New text from workstation`

2. Device-to-device files:

```powershell
go run ./cmd/quickdrop send -c configs/dev/laptop.json device:workstation README.md
go run ./cmd/quickdrop send -c configs/dev/workstation.json device:laptop README.md
```

Expected:

- workstation downloads to `data/workstation/downloads/<message_id>/README.md`
- laptop downloads to `data/laptop/downloads/<message_id>/README.md`
- downloaded SHA-256 matches the source file

3. Group fan-out:

```powershell
go run ./cmd/quickdrop text -c configs/dev/server.json group:all "server group message"
```

Expected:

- Hub creates one delivery per group member
- laptop and workstation display/log the message from `main-server`

4. Offline replay:

```powershell
# stop workstation Agent first
go run ./cmd/quickdrop text -c configs/dev/laptop.json device:workstation "pending while offline"
# restart workstation Agent
```

Expected:

- Hub shows workstation offline before the send
- delivery is `pending` while workstation is offline
- workstation receives the message after reconnect
- delivery becomes `delivered`

## SSH Remote Deployment Smoke

The recommended first remote topology is:

- Remote SSH machine runs the Hub on `127.0.0.1:<remote_port>`
- local machine opens `ssh -N -L <local_port>:127.0.0.1:<remote_port> <ssh_host>`
- local Agents and CLI use `http://127.0.0.1:<local_port>`

This proves the future deployment shape without exposing Hub to the LAN.

Concrete smoke sequence:

Run the helper:

```powershell
powershell -ExecutionPolicy Bypass -File scripts\ssh_probe.ps1
powershell -ExecutionPolicy Bypass -File scripts\remote_hub_smoke.ps1 -HostName ljs_macbookair -LocalPort 48991 -RemotePort 47891
```

What `remote_hub_smoke.ps1` does:

- detects the remote OS and CPU with SSH
- cross-builds a Hub binary for that remote
- copies the binary and Hub config with `scp`
- starts remote Hub on `127.0.0.1:<remote_port>`
- opens local `ssh -N -L <local_port>:127.0.0.1:<remote_port> <ssh_host>`
- starts local laptop/workstation Agents pointed at the tunnel
- sends laptop-to-workstation text through the remote Hub
- sends workstation-to-laptop file through the remote Hub
- waits for local Agent logs and downloaded attachment
- stops local processes, SSH tunnel, and remote Hub

The script was verified against `ljs_macbookair` using:

```powershell
powershell -ExecutionPolicy Bypass -File scripts\remote_hub_smoke.ps1 -HostName ljs_macbookair -LocalPort 48991 -RemotePort 47891
```

Observed result:

```text
Remote Hub smoke passed through ljs_macbookair via local tunnel 48991 -> 47891
```

Do not hard-code remote hostnames, absolute paths, or tokens into source. Keep them in local test configs or script parameters.
