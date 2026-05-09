package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"quickdrop/internal/config"
	"quickdrop/internal/protocol"
)

type Client struct {
	BaseURL    string
	DeviceID   string
	Token      string
	HTTPClient *http.Client
}

func New(baseURL, deviceID, token string) *Client {
	return &Client{
		BaseURL:  strings.TrimRight(baseURL, "/"),
		DeviceID: deviceID,
		Token:    token,
		HTTPClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func NewFromConfig(cfg *config.Config, baseURL string) *Client {
	return New(baseURL, cfg.Device.ID, cfg.Device.Token)
}

func (c *Client) Devices(ctx context.Context) ([]protocol.Device, error) {
	var resp protocol.DevicesResponse
	if err := c.doJSON(ctx, http.MethodGet, "/api/devices", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Devices, nil
}

func (c *Client) UpdateDeviceProfile(ctx context.Context, req protocol.UpdateDeviceProfileRequest) (protocol.Device, error) {
	var dev protocol.Device
	if err := c.doJSON(ctx, http.MethodPost, "/api/devices/me", req, &dev); err != nil {
		return protocol.Device{}, err
	}
	return dev, nil
}

func (c *Client) Groups(ctx context.Context) ([]protocol.Group, error) {
	var resp protocol.GroupsResponse
	if err := c.doJSON(ctx, http.MethodGet, "/api/groups", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Groups, nil
}

func (c *Client) Messages(ctx context.Context, conversationID, after string) ([]protocol.MessageEnvelope, error) {
	values := url.Values{}
	if conversationID != "" {
		values.Set("conversation_id", conversationID)
	}
	if after != "" {
		values.Set("after", after)
	}
	path := "/api/messages"
	if encoded := values.Encode(); encoded != "" {
		path += "?" + encoded
	}
	var resp protocol.MessagesResponse
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	return resp.Messages, nil
}

func (c *Client) SendText(ctx context.Context, targetType, targetID, text string) (protocol.MessageEnvelope, error) {
	body := protocol.SendTextRequest{
		TargetType: targetType,
		TargetID:   targetID,
		Text:       text,
	}
	var env protocol.MessageEnvelope
	if err := c.doJSON(ctx, http.MethodPost, "/api/messages/text", body, &env); err != nil {
		return protocol.MessageEnvelope{}, err
	}
	return env, nil
}

func (c *Client) SendFile(ctx context.Context, targetType, targetID, text, path string) (protocol.MessageEnvelope, error) {
	stat, err := os.Stat(path)
	if err != nil {
		return protocol.MessageEnvelope{}, fmt.Errorf("stat upload path: %w", err)
	}
	if stat.IsDir() {
		return protocol.MessageEnvelope{}, fmt.Errorf("directory upload is not supported in this MVP: %s", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return protocol.MessageEnvelope{}, fmt.Errorf("open upload file: %w", err)
	}

	pipeReader, pipeWriter := io.Pipe()
	writer := multipart.NewWriter(pipeWriter)
	meta := protocol.FileMetadata{TargetType: targetType, TargetID: targetID, Text: text}
	go func() {
		err := func() error {
			defer file.Close()
			metaData, err := json.Marshal(meta)
			if err != nil {
				return fmt.Errorf("encode file metadata: %w", err)
			}
			if err := writer.WriteField("metadata", string(metaData)); err != nil {
				return fmt.Errorf("write metadata field: %w", err)
			}
			part, err := writer.CreateFormFile("files", filepath.Base(path))
			if err != nil {
				return fmt.Errorf("create file field: %w", err)
			}
			if _, err := io.Copy(part, file); err != nil {
				return fmt.Errorf("write file field: %w", err)
			}
			if err := writer.Close(); err != nil {
				return fmt.Errorf("close multipart writer: %w", err)
			}
			return nil
		}()
		if err != nil {
			_ = pipeWriter.CloseWithError(err)
			return
		}
		_ = pipeWriter.Close()
	}()

	req, err := c.newRequest(ctx, http.MethodPost, "/api/messages/file", pipeReader)
	if err != nil {
		return protocol.MessageEnvelope{}, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	var env protocol.MessageEnvelope
	if err := c.send(req, &env); err != nil {
		return protocol.MessageEnvelope{}, err
	}
	return env, nil
}

func (c *Client) Ack(ctx context.Context, messageID string) error {
	var resp map[string]bool
	return c.doJSON(ctx, http.MethodPost, "/api/deliveries/"+url.PathEscape(messageID)+"/ack", map[string]bool{"ok": true}, &resp)
}

func (c *Client) DownloadAttachment(ctx context.Context, attachmentID string, out io.Writer) error {
	req, err := c.newRequest(ctx, http.MethodGet, "/api/attachments/"+url.PathEscape(attachmentID)+"/download", nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("download attachment: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return responseError(resp)
	}
	if _, err := io.Copy(out, resp.Body); err != nil {
		return fmt.Errorf("write attachment body: %w", err)
	}
	return nil
}

func (c *Client) Subscribe(ctx context.Context, handle func(protocol.MessageEnvelope)) error {
	req, err := c.newRequest(ctx, http.MethodGet, "/api/events", nil)
	if err != nil {
		return err
	}
	httpClient := *c.HTTPClient
	httpClient.Timeout = 0
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("connect SSE: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return responseError(resp)
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	var dataLines []string
	dispatch := func() {
		if len(dataLines) == 0 {
			return
		}
		data := strings.Join(dataLines, "\n")
		dataLines = nil
		var env protocol.MessageEnvelope
		if err := json.Unmarshal([]byte(data), &env); err != nil {
			return
		}
		if env.Type != "" {
			handle(env)
		}
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			dispatch()
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	dispatch()
	if err := scanner.Err(); err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return fmt.Errorf("read SSE: %w", err)
	}
	if ctx.Err() != nil {
		return nil
	}
	return fmt.Errorf("SSE stream closed")
}

func (c *Client) doJSON(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request body: %w", err)
		}
		reader = bytes.NewReader(data)
	}
	req, err := c.newRequest(ctx, method, path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.send(req, out)
}

func (c *Client) newRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("build request %s %s: %w", method, path, err)
	}
	req.Header.Set("X-Device-ID", c.DeviceID)
	req.Header.Set("Authorization", "Bearer "+c.Token)
	return req, nil
}

func (c *Client) send(req *http.Request, out any) error {
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", req.Method, req.URL.String(), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return responseError(resp)
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func responseError(resp *http.Response) error {
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	msg := strings.TrimSpace(string(data))
	if msg == "" {
		msg = resp.Status
	}
	return fmt.Errorf("hub returned %s: %s", resp.Status, msg)
}
