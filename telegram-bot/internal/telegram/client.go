package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	token   string
	baseURL string
	http    *http.Client
}

type Update struct {
	ID          int64   `json:"update_id"`
	Message     Message `json:"message"`
	ChannelPost Message `json:"channel_post"`
}

type Message struct {
	ID    int64      `json:"message_id"`
	Chat  Chat       `json:"chat"`
	Text  string     `json:"text"`
	Voice *MediaFile `json:"voice,omitempty"`
	Audio *MediaFile `json:"audio,omitempty"`
}

type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

type MediaFile struct {
	FileID   string `json:"file_id"`
	FileName string `json:"file_name,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
	Duration int    `json:"duration,omitempty"`
}

type AudioAttachment struct {
	Kind     string
	FileID   string
	FileName string
	MimeType string
}

type File struct {
	ID   string `json:"file_id"`
	Path string `json:"file_path"`
}

func NewClient(token, rawBaseURL string) *Client {
	base := strings.TrimRight(strings.TrimSpace(rawBaseURL), "/")
	if base == "" {
		base = "https://api.telegram.org"
	}
	return &Client{
		token:   strings.TrimSpace(token),
		baseURL: base,
		http:    &http.Client{Timeout: 65 * time.Second},
	}
}

func (c *Client) GetUpdates(ctx context.Context, offset int64, timeoutSeconds int) ([]Update, error) {
	query := url.Values{}
	query.Set("offset", fmt.Sprintf("%d", offset))
	query.Set("timeout", fmt.Sprintf("%d", timeoutSeconds))
	var resp telegramResponse[[]Update]
	if err := c.request(ctx, http.MethodGet, "getUpdates?"+query.Encode(), nil, &resp); err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, fmt.Errorf("telegram api: %s", resp.Description)
	}
	return resp.Result, nil
}

func (u Update) EffectiveMessage() (Message, bool) {
	if u.Message.Chat.ID != 0 {
		return u.Message, true
	}
	if u.ChannelPost.Chat.ID != 0 {
		return u.ChannelPost, true
	}
	return Message{}, false
}

func (m Message) AudioAttachment() (AudioAttachment, bool) {
	if m.Voice != nil && strings.TrimSpace(m.Voice.FileID) != "" {
		return audioAttachment("voice", *m.Voice), true
	}
	if m.Audio != nil && strings.TrimSpace(m.Audio.FileID) != "" {
		return audioAttachment("audio", *m.Audio), true
	}
	return AudioAttachment{}, false
}

func audioAttachment(kind string, media MediaFile) AudioAttachment {
	mimeType := strings.TrimSpace(media.MimeType)
	if mimeType == "" && kind == "voice" {
		mimeType = "audio/ogg"
	}
	fileName := strings.TrimSpace(media.FileName)
	if fileName == "" {
		extension := ".bin"
		if mimeType == "audio/ogg" {
			extension = ".ogg"
		}
		fileName = strings.TrimSpace(media.FileID) + extension
	}
	return AudioAttachment{
		Kind:     kind,
		FileID:   strings.TrimSpace(media.FileID),
		FileName: fileName,
		MimeType: mimeType,
	}
}

func (c *Client) GetFile(ctx context.Context, fileID string) (File, error) {
	var resp telegramResponse[File]
	body := map[string]any{"file_id": strings.TrimSpace(fileID)}
	if err := c.request(ctx, http.MethodPost, "getFile", body, &resp); err != nil {
		return File{}, err
	}
	if !resp.OK {
		return File{}, fmt.Errorf("telegram api: %s", resp.Description)
	}
	return resp.Result, nil
}

func (c *Client) DownloadFile(ctx context.Context, filePath string) ([]byte, error) {
	path := strings.TrimLeft(strings.TrimSpace(filePath), "/")
	if path == "" {
		return nil, fmt.Errorf("telegram file path is required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/file/bot"+url.PathEscape(c.token)+"/"+escapeTelegramFilePath(path), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("telegram file download failed: %s", strings.TrimSpace(string(raw)))
	}
	return raw, nil
}

func escapeTelegramFilePath(path string) string {
	parts := strings.Split(path, "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return strings.Join(parts, "/")
}

func (c *Client) SendMessage(ctx context.Context, chatID int64, text string) error {
	var resp telegramResponse[map[string]any]
	body := map[string]any{
		"chat_id": chatID,
		"text":    text,
	}
	if err := c.request(ctx, http.MethodPost, "sendMessage", body, &resp); err != nil {
		return err
	}
	if !resp.OK {
		return fmt.Errorf("telegram api: %s", resp.Description)
	}
	return nil
}

type telegramResponse[T any] struct {
	OK          bool   `json:"ok"`
	Description string `json:"description"`
	Result      T      `json:"result"`
}

func (c *Client) request(ctx context.Context, method, apiMethod string, body any, out any) error {
	var requestBody io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode telegram request: %w", err)
		}
		requestBody = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+"/bot"+url.PathEscape(c.token)+"/"+apiMethod, requestBody)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("telegram api: %s", strings.TrimSpace(string(raw)))
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decode telegram response: %w", err)
	}
	return nil
}
