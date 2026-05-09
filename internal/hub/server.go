package hub

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"quickdrop/internal/auth"
	"quickdrop/internal/blobstore"
	"quickdrop/internal/config"
	"quickdrop/internal/protocol"
	qsqlite "quickdrop/internal/sqlite"
)

type contextKey string

const deviceIDKey contextKey = "device_id"

type Server struct {
	cfg    *config.Config
	db     *sql.DB
	broker *broker
}

type pendingAttachment struct {
	OriginalName string
	SafeName     string
	BlobSHA256   string
	SizeBytes    int64
	MimeType     string
}

func Run(ctx context.Context, cfg *config.Config) error {
	if cfg.Role != "hub" {
		return fmt.Errorf("config role must be hub, got %q", cfg.Role)
	}
	if !strings.HasPrefix(cfg.Hub.Listen, "127.0.0.1:") {
		log.Printf("[QuickDrop] warning: hub listen address is %s; default dev configs use 127.0.0.1", cfg.Hub.Listen)
	}
	if err := os.MkdirAll(filepath.Join(cfg.Hub.DataDir, "logs"), 0o755); err != nil {
		return fmt.Errorf("create hub logs directory: %w", err)
	}

	db, err := qsqlite.Open(filepath.Join(cfg.Hub.DataDir, "quickdrop.db"))
	if err != nil {
		return err
	}
	defer db.Close()
	if err := qsqlite.ApplyHubSchema(db); err != nil {
		return err
	}

	server := &Server{cfg: cfg, db: db, broker: newBroker()}
	if err := server.initFromConfig(ctx); err != nil {
		return err
	}

	mux := http.NewServeMux()
	server.registerRoutes(mux)
	httpServer := &http.Server{
		Addr:              cfg.Hub.Listen,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("[QuickDrop] Hub listening on http://%s", cfg.Hub.Listen)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
		return nil
	case err := <-errCh:
		return err
	}
}

func (s *Server) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.Handle("/api/monitor", s.auth(http.HandlerFunc(s.handleMonitor)))
	mux.Handle("/api/devices/me", s.auth(http.HandlerFunc(s.handleDeviceProfile)))
	mux.Handle("/api/devices", s.auth(http.HandlerFunc(s.handleDevices)))
	mux.Handle("/api/groups", s.auth(http.HandlerFunc(s.handleGroups)))
	mux.Handle("/api/messages", s.auth(http.HandlerFunc(s.handleMessages)))
	mux.Handle("/api/messages/text", s.auth(http.HandlerFunc(s.handleTextMessage)))
	mux.Handle("/api/messages/file", s.auth(http.HandlerFunc(s.handleFileMessage)))
	mux.Handle("/api/attachments/", s.auth(http.HandlerFunc(s.handleAttachmentDownload)))
	mux.Handle("/api/deliveries/", s.auth(http.HandlerFunc(s.handleDeliveryAck)))
	mux.Handle("/api/events", s.auth(http.HandlerFunc(s.handleEvents)))
}

func (s *Server) initFromConfig(ctx context.Context) error {
	now := qsqlite.Now()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin hub init transaction: %w", err)
	}
	defer tx.Rollback()

	for _, dev := range s.cfg.Devices {
		if dev.ID == "" || dev.Token == "" {
			return errors.New("hub config devices require id and token")
		}
		color := normalizeDeviceColor(dev.Color)
		if _, err := tx.ExecContext(ctx, `
INSERT INTO devices(id, display_name, token_hash, color, created_at, online)
VALUES(?, ?, ?, ?, ?, 0)
ON CONFLICT(id) DO UPDATE SET
  display_name=excluded.display_name,
  token_hash=excluded.token_hash,
  color=CASE WHEN excluded.color != '' THEN excluded.color ELSE devices.color END
`, dev.ID, dev.DisplayName, auth.HashToken(dev.Token), color, now); err != nil {
			return fmt.Errorf("upsert device %s: %w", dev.ID, err)
		}
	}
	for _, group := range s.cfg.Groups {
		if group.ID == "" {
			return errors.New("hub config groups require id")
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO "groups"(id, name, created_at)
VALUES(?, ?, ?)
ON CONFLICT(id) DO UPDATE SET name=excluded.name
`, group.ID, group.Name, now); err != nil {
			return fmt.Errorf("upsert group %s: %w", group.ID, err)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM group_members WHERE group_id = ?`, group.ID); err != nil {
			return fmt.Errorf("replace members for group %s: %w", group.ID, err)
		}
		for _, member := range group.Members {
			if _, err := tx.ExecContext(ctx, `
INSERT OR IGNORE INTO group_members(group_id, device_id) VALUES(?, ?)
`, group.ID, member); err != nil {
				return fmt.Errorf("insert member %s in group %s: %w", member, group.ID, err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit hub init transaction: %w", err)
	}
	return nil
}

func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deviceID := r.Header.Get("X-Device-ID")
		token, ok := auth.BearerToken(r.Header.Get("Authorization"))
		if deviceID == "" || !ok {
			writeError(w, http.StatusUnauthorized, "missing X-Device-ID or Authorization bearer token")
			return
		}
		var tokenHash string
		err := s.db.QueryRowContext(r.Context(), `SELECT token_hash FROM devices WHERE id = ?`, deviceID).Scan(&tokenHash)
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusUnauthorized, "unknown device_id")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "lookup device token: "+err.Error())
			return
		}
		if !auth.VerifyToken(token, tokenHash) {
			writeError(w, http.StatusUnauthorized, "invalid device token")
			return
		}
		_, _ = s.db.ExecContext(r.Context(), `UPDATE devices SET last_seen_at = ? WHERE id = ?`, qsqlite.Now(), deviceID)
		ctx := context.WithValue(r.Context(), deviceIDKey, deviceID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func currentDeviceID(r *http.Request) string {
	id, _ := r.Context().Value(deviceIDKey).(string)
	return id
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}
	writeJSON(w, http.StatusOK, protocol.HealthResponse{OK: true})
}

func (s *Server) handleDevices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT id, display_name, COALESCE(color, ''), COALESCE(last_seen_at, ''), online FROM devices ORDER BY id`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list devices: "+err.Error())
		return
	}
	defer rows.Close()
	var devices []protocol.Device
	for rows.Next() {
		var dev protocol.Device
		var online int
		if err := rows.Scan(&dev.ID, &dev.DisplayName, &dev.Color, &dev.LastSeenAt, &online); err != nil {
			writeError(w, http.StatusInternalServerError, "scan device: "+err.Error())
			return
		}
		dev.Online = online != 0
		devices = append(devices, dev)
	}
	writeJSON(w, http.StatusOK, protocol.DevicesResponse{Devices: devices})
}

func (s *Server) handleMonitor(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `
SELECT
  d.id,
  d.display_name,
  COALESCE(d.color, ''),
  COALESCE(d.last_seen_at, ''),
  d.online,
  COALESCE(p.pending_count, 0)
FROM devices d
LEFT JOIN (
  SELECT target_device_id, COUNT(*) AS pending_count
  FROM deliveries
  WHERE status = 'pending'
  GROUP BY target_device_id
) p ON p.target_device_id = d.id
ORDER BY d.id
`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "load monitor data: "+err.Error())
		return
	}
	defer rows.Close()
	var devices []protocol.MonitorDevice
	for rows.Next() {
		var dev protocol.MonitorDevice
		var online int
		if err := rows.Scan(&dev.ID, &dev.DisplayName, &dev.Color, &dev.LastSeenAt, &online, &dev.PendingDeliveries); err != nil {
			writeError(w, http.StatusInternalServerError, "scan monitor data: "+err.Error())
			return
		}
		dev.Online = online != 0
		dev.SSEConnections = s.broker.Count(dev.ID)
		devices = append(devices, dev)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "iterate monitor data: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, protocol.MonitorResponse{
		HubTime: qsqlite.Now(),
		Devices: devices,
	})
}

func (s *Server) handleDeviceProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req protocol.UpdateDeviceProfileRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "decode device profile: "+err.Error())
		return
	}
	displayName := strings.TrimSpace(req.DisplayName)
	if displayName == "" {
		writeError(w, http.StatusBadRequest, "display_name is required")
		return
	}
	color := normalizeDeviceColor(req.Color)
	if req.Color != "" && color == "" {
		writeError(w, http.StatusBadRequest, "color must be a hex value like #2563eb")
		return
	}
	deviceID := currentDeviceID(r)
	if _, err := s.db.ExecContext(r.Context(), `
UPDATE devices
SET display_name = ?, color = ?, last_seen_at = ?
WHERE id = ?
`, displayName, color, qsqlite.Now(), deviceID); err != nil {
		writeError(w, http.StatusInternalServerError, "update device profile: "+err.Error())
		return
	}
	var dev protocol.Device
	var online int
	if err := s.db.QueryRowContext(r.Context(), `
SELECT id, display_name, COALESCE(color, ''), COALESCE(last_seen_at, ''), online
FROM devices WHERE id = ?
`, deviceID).Scan(&dev.ID, &dev.DisplayName, &dev.Color, &dev.LastSeenAt, &online); err != nil {
		writeError(w, http.StatusInternalServerError, "load device profile: "+err.Error())
		return
	}
	dev.Online = online != 0
	writeJSON(w, http.StatusOK, dev)
}

func (s *Server) handleGroups(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT id, name FROM "groups" ORDER BY id`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list groups: "+err.Error())
		return
	}
	defer rows.Close()
	var groups []protocol.Group
	for rows.Next() {
		var group protocol.Group
		if err := rows.Scan(&group.ID, &group.Name); err != nil {
			writeError(w, http.StatusInternalServerError, "scan group: "+err.Error())
			return
		}
		members, err := s.groupMembers(r.Context(), group.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "load group members: "+err.Error())
			return
		}
		group.Members = members
		groups = append(groups, group)
	}
	writeJSON(w, http.StatusOK, protocol.GroupsResponse{Groups: groups})
}

func (s *Server) handleMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}
	viewer := currentDeviceID(r)
	conversationID := r.URL.Query().Get("conversation_id")
	after := r.URL.Query().Get("after")

	query := `
SELECT DISTINCT m.id
FROM messages m
LEFT JOIN deliveries d ON d.message_id = m.id
WHERE (m.sender_device_id = ? OR d.target_device_id = ?)
`
	args := []any{viewer, viewer}
	if after != "" {
		query += ` AND m.created_at > ?`
		args = append(args, after)
	}
	if strings.HasPrefix(conversationID, "device:") {
		other := strings.TrimPrefix(conversationID, "device:")
		query += ` AND m.target_type = 'device' AND ((m.sender_device_id = ? AND m.target_id = ?) OR (m.sender_device_id = ? AND m.target_id = ?))`
		args = append(args, viewer, other, other, viewer)
	} else if strings.HasPrefix(conversationID, "group:") {
		groupID := strings.TrimPrefix(conversationID, "group:")
		query += ` AND m.target_type = 'group' AND m.target_id = ?`
		args = append(args, groupID)
	} else if conversationID != "" {
		writeError(w, http.StatusBadRequest, "conversation_id must start with device: or group:")
		return
	}
	query += ` ORDER BY m.created_at ASC`

	rows, err := s.db.QueryContext(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list messages: "+err.Error())
		return
	}
	defer rows.Close()
	var envelopes []protocol.MessageEnvelope
	for rows.Next() {
		var messageID string
		if err := rows.Scan(&messageID); err != nil {
			writeError(w, http.StatusInternalServerError, "scan message id: "+err.Error())
			return
		}
		env, err := s.envelopeForDevice(r.Context(), viewer, messageID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "load message: "+err.Error())
			return
		}
		envelopes = append(envelopes, env)
	}
	writeJSON(w, http.StatusOK, protocol.MessagesResponse{Messages: envelopes})
}

func (s *Server) handleTextMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req protocol.SendTextRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "decode text request: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Text) == "" {
		writeError(w, http.StatusBadRequest, "text must not be empty")
		return
	}
	env, err := s.createMessage(r.Context(), currentDeviceID(r), req.TargetType, req.TargetID, "text", req.Text, nil)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, env)
}

func (s *Server) handleFileMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.Hub.MaxUploadBytes)
	reader, err := r.MultipartReader()
	if err != nil {
		writeError(w, http.StatusBadRequest, "read multipart request: "+err.Error())
		return
	}

	var meta protocol.FileMetadata
	metaSeen := false
	var files []pendingAttachment
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			writeError(w, http.StatusBadRequest, "read multipart part: "+err.Error())
			return
		}
		switch part.FormName() {
		case "metadata":
			data, err := io.ReadAll(io.LimitReader(part, 1<<20))
			if err != nil {
				writeError(w, http.StatusBadRequest, "read metadata: "+err.Error())
				return
			}
			if err := json.Unmarshal(data, &meta); err != nil {
				writeError(w, http.StatusBadRequest, "decode metadata: "+err.Error())
				return
			}
			metaSeen = true
		case "files":
			if part.FileName() == "" {
				continue
			}
			stored, err := blobstore.StoreReader(s.cfg.Hub.DataDir, part.FileName(), part)
			if err != nil {
				writeError(w, http.StatusBadRequest, "store upload: "+err.Error())
				return
			}
			mimeType := part.Header.Get("Content-Type")
			if mimeType == "" {
				mimeType = "application/octet-stream"
			}
			files = append(files, pendingAttachment{
				OriginalName: stored.OriginalName,
				SafeName:     stored.SafeName,
				BlobSHA256:   stored.SHA256,
				SizeBytes:    stored.SizeBytes,
				MimeType:     mimeType,
			})
		}
	}
	if !metaSeen {
		writeError(w, http.StatusBadRequest, "multipart field metadata is required")
		return
	}
	if len(files) == 0 {
		writeError(w, http.StatusBadRequest, "at least one files field is required")
		return
	}
	env, err := s.createMessage(r.Context(), currentDeviceID(r), meta.TargetType, meta.TargetID, "file", meta.Text, files)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, env)
}

func (s *Server) createMessage(ctx context.Context, sender, targetType, targetID, messageType, text string, attachments []pendingAttachment) (protocol.MessageEnvelope, error) {
	targetType = strings.TrimSpace(targetType)
	targetID = strings.TrimSpace(targetID)
	if targetType != "device" && targetType != "group" {
		return protocol.MessageEnvelope{}, errors.New("target_type must be device or group")
	}
	if targetID == "" {
		return protocol.MessageEnvelope{}, errors.New("target_id is required")
	}
	if messageType != "text" && messageType != "file" {
		return protocol.MessageEnvelope{}, errors.New("message_type must be text or file")
	}
	if messageType == "file" && len(attachments) == 0 {
		return protocol.MessageEnvelope{}, errors.New("file message requires at least one attachment")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return protocol.MessageEnvelope{}, fmt.Errorf("begin create message transaction: %w", err)
	}
	defer tx.Rollback()

	targets, err := s.resolveTargets(ctx, tx, targetType, targetID)
	if err != nil {
		return protocol.MessageEnvelope{}, err
	}
	now := qsqlite.Now()
	messageID := newID("msg")
	conversationID := conversationForStoredMessage(targetType, targetID)
	if _, err := tx.ExecContext(ctx, `
INSERT INTO messages(id, conversation_id, sender_device_id, target_type, target_id, message_type, text, created_at)
VALUES(?, ?, ?, ?, ?, ?, ?, ?)
`, messageID, conversationID, sender, targetType, targetID, messageType, text, now); err != nil {
		return protocol.MessageEnvelope{}, fmt.Errorf("insert message: %w", err)
	}
	for _, att := range attachments {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO attachments(id, message_id, original_name, safe_name, blob_sha256, size_bytes, mime_type, created_at)
VALUES(?, ?, ?, ?, ?, ?, ?, ?)
`, newID("att"), messageID, att.OriginalName, att.SafeName, att.BlobSHA256, att.SizeBytes, att.MimeType, now); err != nil {
			return protocol.MessageEnvelope{}, fmt.Errorf("insert attachment: %w", err)
		}
	}
	for _, target := range targets {
		if _, err := tx.ExecContext(ctx, `
INSERT OR IGNORE INTO deliveries(message_id, target_device_id, status, delivered_at, read_at, error)
VALUES(?, ?, 'pending', '', '', '')
`, messageID, target); err != nil {
			return protocol.MessageEnvelope{}, fmt.Errorf("insert delivery for %s: %w", target, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return protocol.MessageEnvelope{}, fmt.Errorf("commit create message transaction: %w", err)
	}

	env, err := s.envelopeForDevice(ctx, sender, messageID)
	if err != nil {
		return protocol.MessageEnvelope{}, err
	}
	s.broadcastMessage(ctx, sender, messageID, targets)
	return env, nil
}

func (s *Server) resolveTargets(ctx context.Context, tx *sql.Tx, targetType, targetID string) ([]string, error) {
	if targetType == "device" {
		var id string
		err := tx.QueryRowContext(ctx, `SELECT id FROM devices WHERE id = ?`, targetID).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("target device %q does not exist", targetID)
		}
		if err != nil {
			return nil, fmt.Errorf("lookup target device: %w", err)
		}
		return []string{id}, nil
	}

	var groupID string
	if err := tx.QueryRowContext(ctx, `SELECT id FROM "groups" WHERE id = ?`, targetID).Scan(&groupID); errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("target group %q does not exist", targetID)
	} else if err != nil {
		return nil, fmt.Errorf("lookup target group: %w", err)
	}
	rows, err := tx.QueryContext(ctx, `SELECT device_id FROM group_members WHERE group_id = ? ORDER BY device_id`, targetID)
	if err != nil {
		return nil, fmt.Errorf("list target group members: %w", err)
	}
	defer rows.Close()
	seen := map[string]bool{}
	var targets []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan group member: %w", err)
		}
		if !seen[id] {
			seen[id] = true
			targets = append(targets, id)
		}
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("target group %q has no members", targetID)
	}
	return targets, nil
}

func (s *Server) broadcastMessage(ctx context.Context, sender, messageID string, targets []string) {
	sent := map[string]bool{}
	for _, target := range targets {
		env, err := s.envelopeForDevice(ctx, target, messageID)
		if err != nil {
			log.Printf("[QuickDrop] build SSE envelope for %s: %v", target, err)
			continue
		}
		s.broker.Send(target, env)
		sent[target] = true
	}
	if !sent[sender] {
		env, err := s.envelopeForDevice(ctx, sender, messageID)
		if err == nil {
			s.broker.Send(sender, env)
		}
	}
}

func (s *Server) handleAttachmentDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}
	attachmentID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/attachments/"), "/download")
	attachmentID = strings.Trim(attachmentID, "/")
	if attachmentID == "" || strings.Contains(attachmentID, "/") {
		writeError(w, http.StatusBadRequest, "attachment id is required")
		return
	}

	var att protocol.Attachment
	err := s.db.QueryRowContext(r.Context(), `
SELECT id, message_id, original_name, safe_name, blob_sha256, size_bytes, mime_type, created_at
FROM attachments WHERE id = ?
`, attachmentID).Scan(&att.ID, &att.MessageID, &att.OriginalName, &att.SafeName, &att.BlobSHA256, &att.SizeBytes, &att.MimeType, &att.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "attachment not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "load attachment: "+err.Error())
		return
	}
	ok, err := s.canSeeMessage(r.Context(), currentDeviceID(r), att.MessageID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "check message visibility: "+err.Error())
		return
	}
	if !ok {
		writeError(w, http.StatusForbidden, "device cannot access this attachment")
		return
	}

	path := blobstore.PathForSHA(s.cfg.Hub.DataDir, att.BlobSHA256)
	file, err := os.Open(path)
	if err != nil {
		writeError(w, http.StatusNotFound, "blob file not found")
		return
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "stat blob file: "+err.Error())
		return
	}
	if att.MimeType != "" {
		w.Header().Set("Content-Type", att.MimeType)
	}
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": att.OriginalName}))
	http.ServeContent(w, r, att.SafeName, stat.ModTime(), file)
}

func (s *Server) handleDeliveryAck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/deliveries/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) != 2 || parts[1] != "ack" || parts[0] == "" {
		writeError(w, http.StatusBadRequest, "expected /api/deliveries/{message_id}/ack")
		return
	}
	messageID := parts[0]
	deviceID := currentDeviceID(r)
	res, err := s.db.ExecContext(r.Context(), `
UPDATE deliveries
SET status = 'delivered', delivered_at = ?, error = ''
WHERE message_id = ? AND target_device_id = ?
`, qsqlite.Now(), messageID, deviceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ack delivery: "+err.Error())
		return
	}
	changed, _ := res.RowsAffected()
	if changed == 0 {
		writeError(w, http.StatusNotFound, "delivery not found for this device")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}
	deviceID := currentDeviceID(r)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "SSE streaming not supported")
		return
	}

	ch := s.broker.Add(deviceID)
	defer func() {
		s.broker.Remove(deviceID, ch)
		if s.broker.Count(deviceID) == 0 {
			s.setOnline(context.Background(), deviceID, false)
		}
	}()
	s.setOnline(r.Context(), deviceID, true)
	s.enqueuePending(r.Context(), deviceID, ch)

	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			_, _ = io.WriteString(w, "event: ping\ndata: {}\n\n")
			flusher.Flush()
		case env := <-ch:
			data, err := json.Marshal(env)
			if err != nil {
				log.Printf("[QuickDrop] marshal SSE event: %v", err)
				continue
			}
			if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", env.Type, data); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (s *Server) enqueuePending(ctx context.Context, deviceID string, ch chan protocol.MessageEnvelope) {
	rows, err := s.db.QueryContext(ctx, `
SELECT d.message_id
FROM deliveries d
JOIN messages m ON m.id = d.message_id
WHERE d.target_device_id = ? AND d.status = 'pending'
ORDER BY m.created_at ASC
`, deviceID)
	if err != nil {
		log.Printf("[QuickDrop] query pending deliveries for %s: %v", deviceID, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var messageID string
		if err := rows.Scan(&messageID); err != nil {
			log.Printf("[QuickDrop] scan pending delivery for %s: %v", deviceID, err)
			return
		}
		env, err := s.envelopeForDevice(ctx, deviceID, messageID)
		if err != nil {
			log.Printf("[QuickDrop] build pending event for %s: %v", deviceID, err)
			continue
		}
		ch <- env
	}
}

func (s *Server) envelopeForDevice(ctx context.Context, viewer, messageID string) (protocol.MessageEnvelope, error) {
	var msg protocol.Message
	var text sql.NullString
	err := s.db.QueryRowContext(ctx, `
SELECT id, conversation_id, sender_device_id, target_type, target_id, message_type, text, created_at
FROM messages WHERE id = ?
`, messageID).Scan(&msg.ID, &msg.ConversationID, &msg.SenderDeviceID, &msg.TargetType, &msg.TargetID, &msg.MessageType, &text, &msg.CreatedAt)
	if err != nil {
		return protocol.MessageEnvelope{}, fmt.Errorf("query message %s: %w", messageID, err)
	}
	if text.Valid {
		msg.Text = text.String
	}
	msg.ConversationID = conversationForViewer(viewer, msg)

	attachments, err := s.attachmentsForMessage(ctx, msg.ID)
	if err != nil {
		return protocol.MessageEnvelope{}, err
	}
	delivery, err := s.deliveryForDevice(ctx, msg.ID, viewer)
	if err != nil {
		return protocol.MessageEnvelope{}, err
	}
	return protocol.MessageEnvelope{
		Type:        "message",
		Message:     msg,
		Attachments: attachments,
		Delivery:    delivery,
	}, nil
}

func (s *Server) attachmentsForMessage(ctx context.Context, messageID string) ([]protocol.Attachment, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, message_id, original_name, safe_name, blob_sha256, size_bytes, mime_type, created_at
FROM attachments WHERE message_id = ? ORDER BY id
`, messageID)
	if err != nil {
		return nil, fmt.Errorf("query attachments: %w", err)
	}
	defer rows.Close()
	var attachments []protocol.Attachment
	for rows.Next() {
		var att protocol.Attachment
		if err := rows.Scan(&att.ID, &att.MessageID, &att.OriginalName, &att.SafeName, &att.BlobSHA256, &att.SizeBytes, &att.MimeType, &att.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan attachment: %w", err)
		}
		attachments = append(attachments, att)
	}
	return attachments, nil
}

func (s *Server) deliveryForDevice(ctx context.Context, messageID, deviceID string) (protocol.Delivery, error) {
	var d protocol.Delivery
	err := s.db.QueryRowContext(ctx, `
SELECT message_id, target_device_id, status, delivered_at, read_at, error
FROM deliveries WHERE message_id = ? AND target_device_id = ?
`, messageID, deviceID).Scan(&d.MessageID, &d.TargetDeviceID, &d.Status, &d.DeliveredAt, &d.ReadAt, &d.Error)
	if errors.Is(err, sql.ErrNoRows) {
		return protocol.Delivery{}, nil
	}
	if err != nil {
		return protocol.Delivery{}, fmt.Errorf("query delivery: %w", err)
	}
	return d, nil
}

func (s *Server) canSeeMessage(ctx context.Context, deviceID, messageID string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM messages m
LEFT JOIN deliveries d ON d.message_id = m.id
WHERE m.id = ? AND (m.sender_device_id = ? OR d.target_device_id = ?)
`, messageID, deviceID, deviceID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *Server) groupMembers(ctx context.Context, groupID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT device_id FROM group_members WHERE group_id = ? ORDER BY device_id`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var members []string
	for rows.Next() {
		var member string
		if err := rows.Scan(&member); err != nil {
			return nil, err
		}
		members = append(members, member)
	}
	return members, nil
}

func (s *Server) setOnline(ctx context.Context, deviceID string, online bool) {
	value := 0
	if online {
		value = 1
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE devices SET online = ?, last_seen_at = ? WHERE id = ?`, value, qsqlite.Now(), deviceID); err != nil {
		log.Printf("[QuickDrop] update online for %s: %v", deviceID, err)
	}
}

func conversationForStoredMessage(targetType, targetID string) string {
	return targetType + ":" + targetID
}

func conversationForViewer(viewer string, msg protocol.Message) string {
	if msg.TargetType == "device" {
		if msg.SenderDeviceID == viewer {
			return "device:" + msg.TargetID
		}
		return "device:" + msg.SenderDeviceID
	}
	return "group:" + msg.TargetID
}

func normalizeDeviceColor(color string) string {
	color = strings.TrimSpace(strings.ToLower(color))
	if color == "" {
		return ""
	}
	if !strings.HasPrefix(color, "#") {
		color = "#" + color
	}
	if len(color) != 7 {
		return ""
	}
	for _, ch := range color[1:] {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			return ""
		}
	}
	return color
}

func newID(prefix string) string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return prefix + "_" + hex.EncodeToString(b[:])
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("[QuickDrop] write json response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

type broker struct {
	mu      sync.Mutex
	clients map[string]map[chan protocol.MessageEnvelope]struct{}
}

func newBroker() *broker {
	return &broker{clients: map[string]map[chan protocol.MessageEnvelope]struct{}{}}
}

func (b *broker) Add(deviceID string) chan protocol.MessageEnvelope {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := make(chan protocol.MessageEnvelope, 64)
	if b.clients[deviceID] == nil {
		b.clients[deviceID] = map[chan protocol.MessageEnvelope]struct{}{}
	}
	b.clients[deviceID][ch] = struct{}{}
	return ch
}

func (b *broker) Remove(deviceID string, ch chan protocol.MessageEnvelope) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if clients := b.clients[deviceID]; clients != nil {
		delete(clients, ch)
		if len(clients) == 0 {
			delete(b.clients, deviceID)
		}
	}
	close(ch)
}

func (b *broker) Count(deviceID string) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.clients[deviceID])
}

func (b *broker) Send(deviceID string, env protocol.MessageEnvelope) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.clients[deviceID] {
		select {
		case ch <- env:
		default:
		}
	}
}
