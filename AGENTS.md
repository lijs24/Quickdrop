# QuickDrop Engineering Rules

- Windows-first, but keep the code as cross-platform as practical.
- Always use `filepath` for filesystem paths.
- Do not require administrator privileges.
- Never automatically execute received files.
- Do not hard-code tokens, secrets, or absolute paths in code.
- By default, all network services must listen on `127.0.0.1`.
- File uploads must first be written to a temporary file, then verified with SHA-256, then atomically renamed into the final blob path.
- All APIs must validate `device_id` and token.
- Original filenames may be displayed, but paths written to disk must be sanitized to avoid path traversal.
- All commands must return clear error messages.
- After each implementation stage, run `gofmt` and `go test ./...`.
- Do not create Git commits unless the user explicitly asks for one.
