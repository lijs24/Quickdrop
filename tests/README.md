# QuickDrop Test Notes

Automated tests live next to the Go packages. The required project-wide check is:

```sh
go test ./...
```

Manual MVP acceptance should cover:

- Hub starts with `configs/dev/hub.json`.
- Laptop and workstation agents connect through SSE.
- Laptop sends text to workstation.
- Laptop sends `README.md` to workstation and the workstation agent downloads it under `data/workstation/downloads/<message_id>/`.
- Laptop sends `group:all` text and Hub creates deliveries for the group members.
- Workstation can be stopped, receive a pending delivery after restart, and ack it after processing.
- GUI lists devices and groups, sends text and files, and shows the message stream.
