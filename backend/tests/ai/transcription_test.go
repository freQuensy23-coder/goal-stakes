package ai_test

import (
	"context"
	"io"
	"testing"

	"goalstakes/internal/ai"

	openai "github.com/sashabaranov/go-openai"
)

func TestOpenAITranscriberUsesAudioRequest(t *testing.T) {
	client := &fakeAudioClient{response: openai.AudioResponse{Text: " I did 10 push-ups "}}
	transcriber := ai.NewOpenAITranscriber(client, "whisper-test")

	transcript, err := transcriber.Transcribe(context.Background(), ai.AudioInput{
		Filename:    "voice.ogg",
		ContentType: "audio/ogg",
		Data:        []byte("fake-ogg-audio"),
	})
	if err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	if transcript != "I did 10 push-ups" {
		t.Fatalf("transcript = %q", transcript)
	}
	if client.request.Model != "whisper-test" || client.request.FilePath != "voice.ogg" {
		t.Fatalf("audio request = %+v", client.request)
	}
	raw, err := io.ReadAll(client.request.Reader)
	if err != nil {
		t.Fatalf("read audio request reader: %v", err)
	}
	if string(raw) != "fake-ogg-audio" {
		t.Fatalf("audio bytes = %q", raw)
	}
}

type fakeAudioClient struct {
	request  openai.AudioRequest
	response openai.AudioResponse
}

func (f *fakeAudioClient) CreateTranscription(_ context.Context, req openai.AudioRequest) (openai.AudioResponse, error) {
	f.request = req
	return f.response, nil
}
