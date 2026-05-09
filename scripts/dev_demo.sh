#!/usr/bin/env sh
set -eu

cat <<'TEXT'
QuickDrop local dev demo

Open separate terminals:
  Terminal 1: go run ./cmd/quickdrop init-dev
  Terminal 1: go run ./cmd/quickdrop hub -c configs/dev/hub.json
  Terminal 2: go run ./cmd/quickdrop agent -c configs/dev/laptop.json
  Terminal 3: go run ./cmd/quickdrop agent -c configs/dev/workstation.json
  Terminal 4: go run ./cmd/quickdrop text -c configs/dev/laptop.json device:workstation "hello from laptop"
  Terminal 4: go run ./cmd/quickdrop send -c configs/dev/laptop.json device:workstation README.md
  Terminal 4: go run ./cmd/quickdrop text -c configs/dev/laptop.json group:all "hello all"
  Terminal 5: go run ./cmd/quickdrop gui -c configs/dev/laptop.json
TEXT
