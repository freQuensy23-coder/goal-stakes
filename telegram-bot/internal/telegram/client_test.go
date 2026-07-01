package telegram

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientGetUpdatesUsesOffsetAndTimeout(t *testing.T) {
	var gotPath string
	client := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.RequestURI()
		writeJSON(t, w, map[string]any{
			"ok": true,
			"result": []map[string]any{{
				"update_id": 7,
				"message": map[string]any{
					"message_id": 99,
					"chat":       map[string]any{"id": 42, "type": "private"},
					"text":       "/goals",
				},
			}},
		})
	}))

	updates, err := client.GetUpdates(t.Context(), 5, 20)
	if err != nil {
		t.Fatalf("GetUpdates error: %v", err)
	}
	if gotPath != "/bottoken/getUpdates?offset=5&timeout=20" {
		t.Fatalf("path = %q", gotPath)
	}
	if len(updates) != 1 || updates[0].ID != 7 || updates[0].Message.ID != 99 || updates[0].Message.Chat.ID != 42 || updates[0].Message.Chat.Type != "private" || updates[0].Message.Text != "/goals" {
		t.Fatalf("updates = %+v", updates)
	}
}

func TestUpdateEffectiveMessageSupportsChannelPost(t *testing.T) {
	client := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, map[string]any{
			"ok": true,
			"result": []map[string]any{{
				"update_id": 8,
				"channel_post": map[string]any{
					"message_id": 77,
					"chat":       map[string]any{"id": -100123, "type": "channel"},
					"text":       "/goals",
				},
			}},
		})
	}))

	updates, err := client.GetUpdates(t.Context(), 0, 20)
	if err != nil {
		t.Fatalf("GetUpdates error: %v", err)
	}
	message, ok := updates[0].EffectiveMessage()
	if !ok {
		t.Fatalf("EffectiveMessage ok = false")
	}
	if message.ID != 77 || message.Chat.ID != -100123 || message.Chat.Type != "channel" || message.Text != "/goals" {
		t.Fatalf("effective message = %+v", message)
	}
}

func TestUpdateEffectiveMessageSupportsVoiceAttachment(t *testing.T) {
	client := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, map[string]any{
			"ok": true,
			"result": []map[string]any{{
				"update_id": 9,
				"channel_post": map[string]any{
					"message_id": 88,
					"chat":       map[string]any{"id": -100123, "type": "channel"},
					"voice": map[string]any{
						"file_id":   "voice-file-id",
						"duration":  2,
						"mime_type": "audio/ogg",
					},
				},
			}},
		})
	}))

	updates, err := client.GetUpdates(t.Context(), 0, 20)
	if err != nil {
		t.Fatalf("GetUpdates error: %v", err)
	}
	message, ok := updates[0].EffectiveMessage()
	if !ok {
		t.Fatalf("EffectiveMessage ok = false")
	}
	attachment, ok := message.AudioAttachment()
	if !ok {
		t.Fatalf("AudioAttachment ok = false")
	}
	if message.ID != 88 || attachment.FileID != "voice-file-id" || attachment.MimeType != "audio/ogg" || attachment.Kind != "voice" {
		t.Fatalf("message = %+v attachment = %+v", message, attachment)
	}
}

func TestClientGetFileAndDownloadFile(t *testing.T) {
	var gotGetFileBody map[string]any
	client := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/bottoken/getFile":
			if err := json.NewDecoder(r.Body).Decode(&gotGetFileBody); err != nil {
				t.Fatalf("decode getFile body: %v", err)
			}
			writeJSON(t, w, map[string]any{"ok": true, "result": map[string]any{"file_id": "voice-file-id", "file_path": "voice/file.oga"}})
		case "/file/bottoken/voice/file.oga":
			w.Header().Set("Content-Type", "audio/ogg")
			_, _ = w.Write([]byte("fake-ogg"))
		default:
			t.Fatalf("unexpected path = %s", r.URL.Path)
		}
	}))

	file, err := client.GetFile(t.Context(), "voice-file-id")
	if err != nil {
		t.Fatalf("GetFile error: %v", err)
	}
	if gotGetFileBody["file_id"] != "voice-file-id" || file.Path != "voice/file.oga" {
		t.Fatalf("getFile body = %+v file = %+v", gotGetFileBody, file)
	}
	raw, err := client.DownloadFile(t.Context(), file.Path)
	if err != nil {
		t.Fatalf("DownloadFile error: %v", err)
	}
	if string(raw) != "fake-ogg" {
		t.Fatalf("downloaded = %q", raw)
	}
}

func TestClientSendMessagePostsChatAndText(t *testing.T) {
	var body map[string]any
	client := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bottoken/sendMessage" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		writeJSON(t, w, map[string]any{"ok": true, "result": map[string]any{}})
	}))

	if err := client.SendMessage(t.Context(), 42, "Hello"); err != nil {
		t.Fatalf("SendMessage error: %v", err)
	}
	if body["chat_id"] != float64(42) || body["text"] != "Hello" {
		t.Fatalf("body = %+v", body)
	}
}

func TestClientReturnsTelegramAPIError(t *testing.T) {
	client := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, map[string]any{"ok": false, "description": "bad token"})
	}))

	err := client.SendMessage(t.Context(), 42, "Hello")
	if err == nil {
		t.Fatalf("SendMessage error = nil, want error")
	}
}

func newTestClient(handler http.Handler) *Client {
	client := NewClient("token", "http://telegram.test")
	client.http = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Scheme != "http" || req.URL.Host != "telegram.test" {
			return nil, fmt.Errorf("unexpected test request URL %s", req.URL.String())
		}
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		return rr.Result(), nil
	})}
	return client
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("write json: %v", err)
	}
}
