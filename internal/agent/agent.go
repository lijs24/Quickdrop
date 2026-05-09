package agent

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"quickdrop/internal/blobstore"
	"quickdrop/internal/client"
	"quickdrop/internal/config"
	"quickdrop/internal/protocol"
	qsqlite "quickdrop/internal/sqlite"
	"quickdrop/internal/transport"
)

func Run(ctx context.Context, cfg *config.Config) error {
	if cfg.Role != "agent" {
		return fmt.Errorf("config role must be agent, got %q", cfg.Role)
	}
	tunnel, baseURL, err := transport.StartTunnelIfEnabled(ctx, cfg)
	if err != nil {
		return err
	}
	defer tunnel.Close()
	return RunWithBaseURL(ctx, cfg, baseURL)
}

func RunWithBaseURL(ctx context.Context, cfg *config.Config, baseURL string) error {
	if cfg.Role != "agent" {
		return fmt.Errorf("config role must be agent, got %q", cfg.Role)
	}
	if err := os.MkdirAll(cfg.Agent.DownloadsDir, 0o755); err != nil {
		return fmt.Errorf("create downloads directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(cfg.Agent.DataDir, "logs"), 0o755); err != nil {
		return fmt.Errorf("create agent logs directory: %w", err)
	}

	db, err := qsqlite.Open(filepath.Join(cfg.Agent.DataDir, "agent.db"))
	if err != nil {
		return err
	}
	defer db.Close()
	if err := qsqlite.ApplyAgentSchema(db); err != nil {
		return err
	}

	hubClient := client.NewFromConfig(cfg, baseURL)
	backoffs := []time.Duration{time.Second, 2 * time.Second, 5 * time.Second, 10 * time.Second, 30 * time.Second}
	attempt := 0
	for {
		log.Printf("[QuickDrop] Agent %s connecting to %s", cfg.Device.ID, baseURL)
		err := hubClient.Subscribe(ctx, func(env protocol.MessageEnvelope) {
			if env.Type != "message" {
				return
			}
			if err := handleMessage(ctx, db, hubClient, cfg, env); err != nil {
				log.Printf("[QuickDrop] handle message %s: %v", env.Message.ID, err)
			}
		})
		if ctx.Err() != nil {
			return nil
		}
		delay := backoffs[min(attempt, len(backoffs)-1)]
		log.Printf("[QuickDrop] SSE disconnected: %v; reconnecting in %s", err, delay)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(delay):
		}
		if attempt < len(backoffs)-1 {
			attempt++
		}
	}
}

func handleMessage(ctx context.Context, db *sql.DB, hubClient *client.Client, cfg *config.Config, env protocol.MessageEnvelope) error {
	if err := storeEnvelope(ctx, db, env); err != nil {
		return err
	}
	shouldAck := env.Delivery.MessageID == env.Message.ID && env.Delivery.TargetDeviceID == cfg.Device.ID
	isOwnMessage := env.Message.SenderDeviceID == cfg.Device.ID
	switch env.Message.MessageType {
	case "text":
		if !isOwnMessage {
			log.Printf("[QuickDrop] New text from %s: %s", env.Message.SenderDeviceID, env.Message.Text)
		}
		if shouldAck {
			return hubClient.Ack(ctx, env.Message.ID)
		}
		return nil
	case "file":
		if !shouldAck {
			return nil
		}
		for _, att := range env.Attachments {
			localPath, err := downloadAttachment(ctx, hubClient, cfg.Agent.DownloadsDir, env.Message.ID, att)
			if err != nil {
				return err
			}
			if err := setAttachmentLocalPath(ctx, db, att.ID, localPath); err != nil {
				return err
			}
			log.Printf("[QuickDrop] New file from %s: %s -> %s", env.Message.SenderDeviceID, att.OriginalName, localPath)
		}
		return hubClient.Ack(ctx, env.Message.ID)
	default:
		return fmt.Errorf("unsupported message_type %q", env.Message.MessageType)
	}
}

func storeEnvelope(ctx context.Context, db *sql.DB, env protocol.MessageEnvelope) error {
	raw, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("marshal local message: %w", err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin local store transaction: %w", err)
	}
	defer tx.Rollback()
	msg := env.Message
	if _, err := tx.ExecContext(ctx, `
INSERT INTO messages(id, conversation_id, sender_device_id, target_type, target_id, message_type, text, created_at, raw_json)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  conversation_id=excluded.conversation_id,
  text=excluded.text,
  raw_json=excluded.raw_json
`, msg.ID, msg.ConversationID, msg.SenderDeviceID, msg.TargetType, msg.TargetID, msg.MessageType, msg.Text, msg.CreatedAt, string(raw)); err != nil {
		return fmt.Errorf("store local message: %w", err)
	}
	for _, att := range env.Attachments {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO attachments(id, message_id, original_name, safe_name, blob_sha256, size_bytes, mime_type, created_at)
VALUES(?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  original_name=excluded.original_name,
  safe_name=excluded.safe_name,
  blob_sha256=excluded.blob_sha256,
  size_bytes=excluded.size_bytes,
  mime_type=excluded.mime_type
`, att.ID, att.MessageID, att.OriginalName, att.SafeName, att.BlobSHA256, att.SizeBytes, att.MimeType, att.CreatedAt); err != nil {
			return fmt.Errorf("store local attachment: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit local store transaction: %w", err)
	}
	return nil
}

func downloadAttachment(ctx context.Context, hubClient *client.Client, downloadsDir, messageID string, att protocol.Attachment) (string, error) {
	dir := filepath.Join(downloadsDir, messageID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create message downloads directory: %w", err)
	}
	targetPath := filepath.Join(dir, blobstore.SafeName(att.SafeName))
	tmpPath := targetPath + ".tmp"
	tmp, err := os.Create(tmpPath)
	if err != nil {
		return "", fmt.Errorf("create download temp file: %w", err)
	}
	if err := hubClient.DownloadAttachment(ctx, att.ID, tmp); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return "", err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("close download temp file: %w", err)
	}
	if err := blobstore.VerifyFileSHA256(tmpPath, att.BlobSHA256); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}
	_ = os.Remove(targetPath)
	if err := os.Rename(tmpPath, targetPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("promote downloaded file: %w", err)
	}
	return targetPath, nil
}

func setAttachmentLocalPath(ctx context.Context, db *sql.DB, attachmentID, localPath string) error {
	_, err := db.ExecContext(ctx, `UPDATE attachments SET local_path = ? WHERE id = ?`, localPath, attachmentID)
	if err != nil {
		return fmt.Errorf("update local attachment path: %w", err)
	}
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
