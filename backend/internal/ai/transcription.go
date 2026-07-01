package ai

import (
	"bytes"
	"context"
	"strings"

	openai "github.com/sashabaranov/go-openai"
)

type AudioClient interface {
	CreateTranscription(ctx context.Context, request openai.AudioRequest) (openai.AudioResponse, error)
}

type openAITranscriber struct {
	client AudioClient
	model  string
}

func NewOpenAITranscriber(client AudioClient, model string) Transcriber {
	return openAITranscriber{client: client, model: strings.TrimSpace(model)}
}

func (t openAITranscriber) Transcribe(ctx context.Context, input AudioInput) (string, error) {
	resp, err := t.client.CreateTranscription(ctx, openai.AudioRequest{
		Model:    t.model,
		FilePath: input.Filename,
		Reader:   bytes.NewReader(input.Data),
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(resp.Text), nil
}
