package sqlite

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

func Open(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve sqlite path: %w", err)
	}
	dsn := "file:" + filepath.ToSlash(absPath) + "?_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)
	if _, err := db.Exec(`PRAGMA foreign_keys = ON; PRAGMA busy_timeout = 5000; PRAGMA journal_mode = WAL;`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("configure sqlite database: %w", err)
	}
	return db, nil
}

func ApplyHubSchema(db *sql.DB) error {
	schema := `
CREATE TABLE IF NOT EXISTS devices (
  id TEXT PRIMARY KEY,
  display_name TEXT,
  token_hash TEXT,
  created_at TEXT,
  last_seen_at TEXT,
  online INTEGER
);

CREATE TABLE IF NOT EXISTS "groups" (
  id TEXT PRIMARY KEY,
  name TEXT,
  created_at TEXT
);

CREATE TABLE IF NOT EXISTS group_members (
  group_id TEXT,
  device_id TEXT,
  PRIMARY KEY(group_id, device_id)
);

CREATE TABLE IF NOT EXISTS messages (
  id TEXT PRIMARY KEY,
  conversation_id TEXT,
  sender_device_id TEXT,
  target_type TEXT,
  target_id TEXT,
  message_type TEXT,
  text TEXT,
  created_at TEXT
);

CREATE TABLE IF NOT EXISTS attachments (
  id TEXT PRIMARY KEY,
  message_id TEXT,
  original_name TEXT,
  safe_name TEXT,
  blob_sha256 TEXT,
  size_bytes INTEGER,
  mime_type TEXT,
  created_at TEXT
);

CREATE TABLE IF NOT EXISTS deliveries (
  message_id TEXT,
  target_device_id TEXT,
  status TEXT,
  delivered_at TEXT,
  read_at TEXT,
  error TEXT,
  PRIMARY KEY(message_id, target_device_id)
);

CREATE INDEX IF NOT EXISTS idx_messages_created_at ON messages(created_at);
CREATE INDEX IF NOT EXISTS idx_messages_target ON messages(target_type, target_id);
CREATE INDEX IF NOT EXISTS idx_deliveries_target_status ON deliveries(target_device_id, status);
`
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("apply hub schema: %w", err)
	}
	return nil
}

func ApplyAgentSchema(db *sql.DB) error {
	schema := `
CREATE TABLE IF NOT EXISTS messages (
  id TEXT PRIMARY KEY,
  conversation_id TEXT,
  sender_device_id TEXT,
  target_type TEXT,
  target_id TEXT,
  message_type TEXT,
  text TEXT,
  created_at TEXT,
  raw_json TEXT
);

CREATE TABLE IF NOT EXISTS attachments (
  id TEXT PRIMARY KEY,
  message_id TEXT,
  original_name TEXT,
  safe_name TEXT,
  blob_sha256 TEXT,
  size_bytes INTEGER,
  mime_type TEXT,
  local_path TEXT,
  created_at TEXT
);
`
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("apply agent schema: %w", err)
	}
	return nil
}

func Now() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}
