package bot_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	. "goalstakes-telegram-bot/internal/bot"
)

func TestHandlerRequiresLinkCodeBeforeCommands(t *testing.T) {
	handler := NewHandler(&fakeService{messageReply: "Link this Telegram chat first with /link <code> from Goal Stakes Settings."})

	reply := handler.Handle(context.Background(), 42, "private", 7, "/goals")

	if !strings.Contains(reply, "Link this Telegram chat first") || !strings.Contains(reply, "/link") {
		t.Fatalf("reply = %q, want link-code instruction", reply)
	}
}

func TestHandlerForwardsTextCommandsToBackend(t *testing.T) {
	service := &fakeService{messageReply: "No active goals."}
	handler := NewHandler(service)

	reply := handler.Handle(context.Background(), -100123, "channel", 99, "/goals")

	if reply != "No active goals." {
		t.Fatalf("reply = %q, want backend reply", reply)
	}
	if service.messageChatID != -100123 || service.messageChatKind != "channel" || service.messageID != 99 || service.messageText != "/goals" {
		t.Fatalf("message call = chatID %d kind %q id %d text %q", service.messageChatID, service.messageChatKind, service.messageID, service.messageText)
	}
}

func TestHandlerForwardsFreeTextToBackend(t *testing.T) {
	service := &fakeService{messageReply: "Check-in recorded."}
	handler := NewHandler(service)

	reply := handler.Handle(context.Background(), 42, "group", 8, "I did 10 push-ups")

	if reply != "Check-in recorded." {
		t.Fatalf("reply = %q, want backend reply", reply)
	}
	if service.messageChatKind != "group" || service.messageText != "I did 10 push-ups" {
		t.Fatalf("message call = kind %q text %q", service.messageChatKind, service.messageText)
	}
}

func TestHandlerForwardsAudioToBackend(t *testing.T) {
	service := &fakeService{audioReply: "Записал: 10 отжиманий"}
	handler := NewHandler(service)

	reply := handler.HandleAudio(context.Background(), -100123, "channel", 301, "voice.ogg", "audio/ogg", []byte("fake-ogg"))

	if reply != "Записал: 10 отжиманий" {
		t.Fatalf("reply = %q, want backend audio reply", reply)
	}
	if service.audioChatID != -100123 || service.audioChatKind != "channel" || service.audioMessageID != 301 || service.audioFilename != "voice.ogg" || service.audioContentType != "audio/ogg" || string(service.audioData) != "fake-ogg" {
		t.Fatalf("audio call = chatID %d kind %q id %d filename %q contentType %q data %q", service.audioChatID, service.audioChatKind, service.audioMessageID, service.audioFilename, service.audioContentType, service.audioData)
	}
}

func TestHandlerCreatesOwnAgentLink(t *testing.T) {
	service := &fakeService{agentReply: "Own-agent skill link: https://api.goalstakes.test/agent-skills/agt_private.md"}
	handler := NewHandler(service)

	reply := handler.Handle(context.Background(), -100123, "channel", 302, "/agent")

	if !strings.Contains(reply, "/agent-skills/agt_private.md") {
		t.Fatalf("reply = %q, want own-agent skill link", reply)
	}
	if service.agentChatID != -100123 || service.agentChatKind != "channel" {
		t.Fatalf("agent call = chatID %d kind %q", service.agentChatID, service.agentChatKind)
	}
	if strings.Contains(reply, "sk_") {
		t.Fatalf("reply leaked raw agent secret: %q", reply)
	}
}

func TestHandlerLinksTelegramChatWithoutRawAPIKey(t *testing.T) {
	service := &fakeService{reply: "Linked to Goal Stakes."}
	handler := NewHandler(service)

	reply := handler.Handle(context.Background(), 42, "private", 7, "/link ABCD1234")

	if reply != "Linked to Goal Stakes." {
		t.Fatalf("reply = %q, want backend linked reply", reply)
	}
	if service.chatID != 42 || service.chatKind != "private" || service.code != "ABCD1234" {
		t.Fatalf("service call = chatID %d kind %q code %q", service.chatID, service.chatKind, service.code)
	}
	if strings.Contains(reply, "sk_") {
		t.Fatalf("reply leaked API key language: %q", reply)
	}
}

func TestHandlerRejectsRawAPIKeyCommand(t *testing.T) {
	handler := NewHandler(&fakeService{})
	rawKey := "sk_secret_value"

	reply := handler.Handle(context.Background(), 42, "private", 7, "/apikey "+rawKey)

	if !strings.Contains(reply, "Use /link") {
		t.Fatalf("reply = %q, want link-code instruction", reply)
	}
	if strings.Contains(reply, rawKey) {
		t.Fatalf("reply leaked raw API key: %q", reply)
	}
}

func TestHandlerReportsLinkFailure(t *testing.T) {
	handler := NewHandler(&fakeService{err: errors.New("invalid or expired telegram link code")})

	reply := handler.Handle(context.Background(), 42, "private", 7, "/link BADCODE")

	if !strings.Contains(reply, "Could not link Telegram chat") || !strings.Contains(reply, "invalid or expired") {
		t.Fatalf("reply = %q, want link failure", reply)
	}
}

func TestHelpMentionsLinkCodeNotAPIKey(t *testing.T) {
	reply := NewHandler(&fakeService{}).Handle(context.Background(), 42, "private", 7, "/help")

	if !strings.Contains(reply, "/link") {
		t.Fatalf("help = %q, want /link", reply)
	}
	if strings.Contains(reply, "/apikey") || strings.Contains(reply, "sk_") {
		t.Fatalf("help mentions raw API key flow: %q", reply)
	}
}

func TestParseBaseUnits(t *testing.T) {
	checks := map[string]string{
		"100":      "100000000",
		"2.5":      "2500000",
		"0.000001": "1",
	}
	for raw, want := range checks {
		got, err := ParseBaseUnits(raw)
		if err != nil {
			t.Fatalf("ParseBaseUnits(%q) error: %v", raw, err)
		}
		if got != want {
			t.Fatalf("ParseBaseUnits(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestParseBaseUnitsRejectsInvalidDecimals(t *testing.T) {
	for _, raw := range []string{"", "abc", "1.0000001", "-1", "1.2.3"} {
		if got, err := ParseBaseUnits(raw); err == nil {
			t.Fatalf("ParseBaseUnits(%q) = %q, want error", raw, got)
		}
	}
}

type fakeService struct {
	chatID           int64
	chatKind         string
	code             string
	reply            string
	err              error
	messageChatID    int64
	messageChatKind  string
	messageID        int64
	messageText      string
	messageReply     string
	messageErr       error
	audioChatID      int64
	audioChatKind    string
	audioMessageID   int64
	audioFilename    string
	audioContentType string
	audioData        []byte
	audioReply       string
	audioErr         error
	agentChatID      int64
	agentChatKind    string
	agentReply       string
	agentErr         error
}

func (f *fakeService) LinkTelegram(ctx context.Context, chatID int64, chatKind, code string) (string, error) {
	f.chatID = chatID
	f.chatKind = chatKind
	f.code = code
	if f.err != nil {
		return "", f.err
	}
	if f.reply == "" {
		return "Linked to Goal Stakes.", nil
	}
	return f.reply, nil
}

func (f *fakeService) TelegramMessage(ctx context.Context, chatID int64, chatKind string, messageID int64, text string) (string, error) {
	f.messageChatID = chatID
	f.messageChatKind = chatKind
	f.messageID = messageID
	f.messageText = text
	if f.messageErr != nil {
		return "", f.messageErr
	}
	return f.messageReply, nil
}

func (f *fakeService) TelegramAudio(ctx context.Context, chatID int64, chatKind string, messageID int64, filename, contentType string, data []byte) (string, error) {
	f.audioChatID = chatID
	f.audioChatKind = chatKind
	f.audioMessageID = messageID
	f.audioFilename = filename
	f.audioContentType = contentType
	f.audioData = append([]byte(nil), data...)
	if f.audioErr != nil {
		return "", f.audioErr
	}
	return f.audioReply, nil
}

func (f *fakeService) TelegramAgentLink(ctx context.Context, chatID int64, chatKind string) (string, error) {
	f.agentChatID = chatID
	f.agentChatKind = chatKind
	if f.agentErr != nil {
		return "", f.agentErr
	}
	return f.agentReply, nil
}
