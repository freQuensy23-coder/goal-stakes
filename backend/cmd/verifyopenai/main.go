package main

import (
	"context"
	"errors"
	"log"
	"os"
	"strings"
	"time"

	"goalstakes/internal/config"

	openai "github.com/sashabaranov/go-openai"
)

func main() {
	if err := run(); err != nil {
		log.Printf("verifyopenai: %v", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if strings.TrimSpace(cfg.OpenAIAPIKey) == "" {
		return errors.New("OPENAI_API_KEY is required for live OpenAI verification")
	}
	if strings.TrimSpace(cfg.OpenAIModel) == "" {
		return errors.New("OPENAI_MODEL is required for live OpenAI verification")
	}
	if strings.TrimSpace(cfg.OpenAITranscriptionModel) == "" {
		return errors.New("OPENAI_TRANSCRIPTION_MODEL is required for live OpenAI verification")
	}
	timeout := 30 * time.Second
	if raw := strings.TrimSpace(os.Getenv("OPENAI_VERIFY_TIMEOUT")); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil || parsed <= 0 {
			return errors.New("OPENAI_VERIFY_TIMEOUT must be a positive Go duration")
		}
		timeout = parsed
	}

	openaiCfg := openai.DefaultConfig(cfg.OpenAIAPIKey)
	if strings.TrimSpace(cfg.OpenAIBaseURL) != "" {
		openaiCfg.BaseURL = strings.TrimRight(cfg.OpenAIBaseURL, "/")
	}
	client := openai.NewClientWithConfig(openaiCfg)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	resp, err := client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: cfg.OpenAIModel,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: "Reply with exactly: ok"},
			{Role: openai.ChatMessageRoleUser, Content: "health check"},
		},
		MaxTokens:   8,
		Temperature: 0,
	})
	if err != nil {
		return err
	}
	if len(resp.Choices) == 0 || strings.TrimSpace(resp.Choices[0].Message.Content) == "" {
		return errors.New("OpenAI verification returned no assistant content")
	}
	log.Printf("verifyopenai: model %s responded", cfg.OpenAIModel)
	return nil
}
