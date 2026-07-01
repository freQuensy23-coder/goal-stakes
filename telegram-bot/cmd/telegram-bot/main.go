package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"goalstakes-telegram-bot/internal/bot"
	"goalstakes-telegram-bot/internal/goalstakes"
	"goalstakes-telegram-bot/internal/telegram"
)

func main() {
	if err := run(); err != nil {
		log.Printf("telegram-bot: %v", err)
		os.Exit(1)
	}
}

func run() error {
	token := strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN"))
	if token == "" {
		return errors.New("TELEGRAM_BOT_TOKEN is required")
	}
	apiBase := strings.TrimSpace(os.Getenv("GOALSTAKES_API_BASE"))
	if apiBase == "" {
		apiBase = "http://127.0.0.1:8080"
	}
	botSecret := strings.TrimSpace(os.Getenv("TELEGRAM_BOT_SECRET"))
	if botSecret == "" {
		return errors.New("TELEGRAM_BOT_SECRET is required")
	}
	telegramBase := strings.TrimSpace(os.Getenv("TELEGRAM_API_BASE"))
	pollTimeout := 30
	if raw := strings.TrimSpace(os.Getenv("TELEGRAM_POLL_TIMEOUT_SECONDS")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			return errors.New("TELEGRAM_POLL_TIMEOUT_SECONDS must be a positive integer")
		}
		pollTimeout = parsed
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	goalClient := goalstakes.NewClient(apiBase)
	handler := bot.NewHandler(serviceAdapter{client: goalClient, botSecret: botSecret})
	telegramClient := telegram.NewClient(token, telegramBase)
	log.Printf("telegram-bot: polling Telegram and forwarding to Goal Stakes API at %s", apiBase)
	return poll(ctx, telegramClient, handler, pollTimeout)
}

type telegramGateway interface {
	GetUpdates(ctx context.Context, offset int64, timeoutSeconds int) ([]telegram.Update, error)
	GetFile(ctx context.Context, fileID string) (telegram.File, error)
	DownloadFile(ctx context.Context, filePath string) ([]byte, error)
	SendMessage(ctx context.Context, chatID int64, text string) error
}

func poll(ctx context.Context, tg telegramGateway, handler *bot.Handler, pollTimeout int) error {
	var offset int64
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		updates, err := tg.GetUpdates(ctx, offset, pollTimeout)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			log.Printf("telegram-bot: getUpdates failed: %v", err)
			continue
		}
		for _, update := range updates {
			if update.ID >= offset {
				offset = update.ID + 1
			}
			message, ok := update.EffectiveMessage()
			if !ok {
				continue
			}
			if attachment, ok := message.AudioAttachment(); ok {
				file, err := tg.GetFile(ctx, attachment.FileID)
				if err != nil {
					log.Printf("telegram-bot: getFile failed for chat %d: %v", message.Chat.ID, err)
					continue
				}
				raw, err := tg.DownloadFile(ctx, file.Path)
				if err != nil {
					log.Printf("telegram-bot: download voice/audio failed for chat %d: %v", message.Chat.ID, err)
					continue
				}
				reply := handler.HandleAudio(ctx, message.Chat.ID, message.Chat.Type, message.ID, attachment.FileName, attachment.MimeType, raw)
				if strings.TrimSpace(reply) == "" {
					continue
				}
				if err := tg.SendMessage(ctx, message.Chat.ID, reply); err != nil {
					if ctx.Err() != nil {
						return nil
					}
					log.Printf("telegram-bot: sendMessage failed for chat %d: %v", message.Chat.ID, err)
				}
				continue
			}
			if strings.TrimSpace(message.Text) == "" {
				continue
			}
			reply := handler.Handle(ctx, message.Chat.ID, message.Chat.Type, message.ID, message.Text)
			if strings.TrimSpace(reply) == "" {
				continue
			}
			if err := tg.SendMessage(ctx, message.Chat.ID, reply); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				log.Printf("telegram-bot: sendMessage failed for chat %d: %v", message.Chat.ID, err)
			}
		}
	}
}

type serviceAdapter struct {
	client    *goalstakes.Client
	botSecret string
}

func (s serviceAdapter) LinkTelegram(ctx context.Context, chatID int64, chatKind, code string) (string, error) {
	reply, err := s.client.LinkTelegram(ctx, s.botSecret, chatID, chatKind, code)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(reply) == "" {
		return "", fmt.Errorf("empty link reply")
	}
	return reply, nil
}

func (s serviceAdapter) TelegramMessage(ctx context.Context, chatID int64, chatKind string, messageID int64, text string) (string, error) {
	reply, err := s.client.TelegramMessage(ctx, s.botSecret, chatID, chatKind, messageID, text)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(reply) == "" {
		return "", fmt.Errorf("empty telegram message reply")
	}
	return reply, nil
}

func (s serviceAdapter) TelegramAudio(ctx context.Context, chatID int64, chatKind string, messageID int64, filename, contentType string, data []byte) (string, error) {
	result, err := s.client.TelegramAudio(ctx, s.botSecret, chatID, chatKind, messageID, filename, contentType, data)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(result.Reply) == "" {
		return "", fmt.Errorf("empty telegram audio reply")
	}
	return result.Reply, nil
}

func (s serviceAdapter) TelegramAgentLink(ctx context.Context, chatID int64, chatKind string) (string, error) {
	reply, err := s.client.TelegramAgentLink(ctx, s.botSecret, chatID, chatKind)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(reply) == "" {
		return "", fmt.Errorf("empty telegram agent-link reply")
	}
	return reply, nil
}
