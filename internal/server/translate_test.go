package server

import (
	"encoding/json"
	"testing"
)

func TestResponsesToChatCompletionRequest(t *testing.T) {
	body := []byte(`{
		"model": "auto",
		"instructions": "You are a router.",
		"input": [
			{"role": "user", "content": "Explain hybrid inference."}
		],
		"metadata": {"session_id": "abc"}
	}`)

	translated, err := responsesToChatCompletionRequest(body)
	if err != nil {
		t.Fatalf("translate request: %v", err)
	}

	var req ChatCompletionRequest
	if err := json.Unmarshal(translated, &req); err != nil {
		t.Fatalf("decode translated request: %v", err)
	}

	if len(req.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(req.Messages))
	}

	if req.Messages[0].Role != "system" {
		t.Fatalf("expected first message to be system, got %s", req.Messages[0].Role)
	}
}

func TestChatCompletionToResponsesResponse(t *testing.T) {
	body := []byte(`{
		"id": "chatcmpl_123",
		"object": "chat.completion",
		"created": 1711111111,
		"model": "openclaw-local",
		"choices": [
			{
				"index": 0,
				"message": {"role": "assistant", "content": "hello"},
				"finish_reason": "stop"
			}
		]
	}`)

	translated, err := chatCompletionToResponsesResponse(body)
	if err != nil {
		t.Fatalf("translate response: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(translated, &payload); err != nil {
		t.Fatalf("decode translated response: %v", err)
	}

	if payload["object"] != "response" {
		t.Fatalf("expected response object, got %v", payload["object"])
	}

	if payload["output_text"] != "hello" {
		t.Fatalf("expected output_text to be hello, got %v", payload["output_text"])
	}
}
