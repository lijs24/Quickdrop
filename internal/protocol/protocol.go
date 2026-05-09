package protocol

type Device struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Color       string `json:"color,omitempty"`
	LastSeenAt  string `json:"last_seen_at,omitempty"`
	Online      bool   `json:"online"`
}

type UpdateDeviceProfileRequest struct {
	DisplayName string `json:"display_name"`
	Color       string `json:"color"`
}

type UpsertDeviceRequest struct {
	ID          string   `json:"id"`
	DisplayName string   `json:"display_name"`
	Color       string   `json:"color"`
	Token       string   `json:"token,omitempty"`
	GroupIDs    []string `json:"group_ids,omitempty"`
}

type Group struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Members []string `json:"members"`
}

type Message struct {
	ID             string `json:"id"`
	ConversationID string `json:"conversation_id"`
	SenderDeviceID string `json:"sender_device_id"`
	TargetType     string `json:"target_type"`
	TargetID       string `json:"target_id"`
	MessageType    string `json:"message_type"`
	Text           string `json:"text,omitempty"`
	CreatedAt      string `json:"created_at"`
}

type Attachment struct {
	ID           string `json:"id"`
	MessageID    string `json:"message_id"`
	OriginalName string `json:"original_name"`
	SafeName     string `json:"safe_name"`
	BlobSHA256   string `json:"blob_sha256"`
	SizeBytes    int64  `json:"size_bytes"`
	MimeType     string `json:"mime_type"`
	CreatedAt    string `json:"created_at"`
}

type Delivery struct {
	MessageID      string `json:"message_id"`
	TargetDeviceID string `json:"target_device_id"`
	Status         string `json:"status"`
	DeliveredAt    string `json:"delivered_at,omitempty"`
	ReadAt         string `json:"read_at,omitempty"`
	Error          string `json:"error,omitempty"`
}

type MessageEnvelope struct {
	Type        string       `json:"type"`
	Message     Message      `json:"message"`
	Attachments []Attachment `json:"attachments,omitempty"`
	Delivery    Delivery     `json:"delivery,omitempty"`
}

type DevicesResponse struct {
	Devices []Device `json:"devices"`
}

type MonitorDevice struct {
	Device
	SSEConnections    int `json:"sse_connections"`
	PendingDeliveries int `json:"pending_deliveries"`
}

type MonitorResponse struct {
	HubTime string          `json:"hub_time"`
	Devices []MonitorDevice `json:"devices"`
}

type GroupsResponse struct {
	Groups []Group `json:"groups"`
}

type MessagesResponse struct {
	Messages []MessageEnvelope `json:"messages"`
}

type SendTextRequest struct {
	TargetType string `json:"target_type"`
	TargetID   string `json:"target_id"`
	Text       string `json:"text"`
}

type FileMetadata struct {
	TargetType string `json:"target_type"`
	TargetID   string `json:"target_id"`
	Text       string `json:"text,omitempty"`
}

type HealthResponse struct {
	OK bool `json:"ok"`
}
