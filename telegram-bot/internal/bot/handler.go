package bot

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

type Service interface {
	LinkTelegram(ctx context.Context, chatID int64, chatKind, code string) (string, error)
	TelegramMessage(ctx context.Context, chatID int64, chatKind string, messageID int64, text string) (string, error)
	TelegramAudio(ctx context.Context, chatID int64, chatKind string, messageID int64, filename, contentType string, data []byte) (string, error)
	TelegramAgentLink(ctx context.Context, chatID int64, chatKind string) (string, error)
}

type Handler struct {
	service Service
}

func NewHandler(service Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Handle(ctx context.Context, chatID int64, chatKind string, messageID int64, raw string) string {
	text := strings.TrimSpace(raw)
	if text == "" {
		return help()
	}
	if strings.HasPrefix(text, "/start") || strings.HasPrefix(text, "/help") {
		return help()
	}
	if strings.HasPrefix(text, "/apikey") {
		return "Use /link <code> from Goal Stakes Settings. Do not paste raw API keys into Telegram."
	}
	if strings.HasPrefix(text, "/link") {
		return h.linkTelegram(ctx, chatID, chatKind, text)
	}
	if strings.HasPrefix(text, "/agent") {
		return h.agentLink(ctx, chatID, chatKind)
	}
	reply, err := h.service.TelegramMessage(ctx, chatID, telegramChatKind(chatKind), messageID, text)
	if err != nil {
		return "Goal Stakes backend error: " + err.Error()
	}
	return strings.TrimSpace(reply)
}

func (h *Handler) linkTelegram(ctx context.Context, chatID int64, chatKind, text string) string {
	parts := strings.Fields(text)
	if len(parts) != 2 {
		return "Usage: /link code"
	}
	kind := strings.TrimSpace(chatKind)
	if kind == "" {
		kind = "private"
	}
	reply, err := h.service.LinkTelegram(ctx, chatID, kind, parts[1])
	if err != nil {
		return "Could not link Telegram chat: " + err.Error()
	}
	return strings.TrimSpace(reply)
}

func (h *Handler) agentLink(ctx context.Context, chatID int64, chatKind string) string {
	reply, err := h.service.TelegramAgentLink(ctx, chatID, telegramChatKind(chatKind))
	if err != nil {
		return "Could not create own-agent link: " + err.Error()
	}
	return strings.TrimSpace(reply)
}

func (h *Handler) HandleAudio(ctx context.Context, chatID int64, chatKind string, messageID int64, filename, contentType string, data []byte) string {
	reply, err := h.service.TelegramAudio(ctx, chatID, telegramChatKind(chatKind), messageID, filename, contentType, data)
	if err != nil {
		return "Goal Stakes backend error: " + err.Error()
	}
	return strings.TrimSpace(reply)
}

func telegramChatKind(chatKind string) string {
	kind := strings.TrimSpace(chatKind)
	if kind == "" {
		return "private"
	}
	return kind
}

func help() string {
	return strings.Join([]string{
		"Goal Stakes Telegram commands:",
		"/link code - link this Telegram chat with a code from Goal Stakes Settings",
		"/agent - create a private Markdown skill link for your own agent",
		"After linking, send goal commands or normal messages.",
	}, "\n")
}

var decimalAmountPattern = regexp.MustCompile(`^\d+(\.\d{0,6})?$`)

func ParseBaseUnits(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" || !decimalAmountPattern.MatchString(value) {
		return "", fmt.Errorf("stake must be a non-negative decimal with up to 6 decimals")
	}
	whole, frac, ok := strings.Cut(value, ".")
	if !ok {
		frac = ""
	}
	frac = (frac + "000000")[:6]
	combined := strings.TrimLeft(whole+frac, "0")
	if combined == "" {
		return "", fmt.Errorf("stake must be positive")
	}
	return combined, nil
}
