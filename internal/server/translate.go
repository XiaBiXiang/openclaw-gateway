package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type ResponsesRequest struct {
	Model        string         `json:"model"`
	Instructions string         `json:"instructions,omitempty"`
	Input        any            `json:"input"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	Stream       bool           `json:"stream,omitempty"`
}

func decodeChatCompletionRequest(body []byte) (ChatCompletionRequest, error) {
	var req ChatCompletionRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return ChatCompletionRequest{}, fmt.Errorf("decode chat request: %w", err)
	}
	if len(req.Messages) == 0 {
		return ChatCompletionRequest{}, errors.New("messages must not be empty")
	}
	return req, nil
}

func decodeResponsesRequest(body []byte) (ResponsesRequest, error) {
	var req ResponsesRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return ResponsesRequest{}, fmt.Errorf("decode responses request: %w", err)
	}
	if req.Input == nil && req.Instructions == "" {
		return ResponsesRequest{}, errors.New("input or instructions must not be empty")
	}
	return req, nil
}

func decodeDecisionRequest(body []byte) ([]ChatMessage, map[string]any, bool, error) {
	if req, err := decodeChatCompletionRequest(body); err == nil {
		return req.Messages, req.Metadata, req.Stream, nil
	}

	req, err := decodeResponsesRequest(body)
	if err != nil {
		return nil, nil, false, err
	}

	return responsesToMessages(req), req.Metadata, req.Stream, nil
}

func responsesToChatCompletionRequest(body []byte) ([]byte, error) {
	req, err := decodeResponsesRequest(body)
	if err != nil {
		return nil, err
	}

	out := ChatCompletionRequest{
		Model:    req.Model,
		Messages: responsesToMessages(req),
		Metadata: req.Metadata,
		Stream:   req.Stream,
	}

	if len(out.Messages) == 0 {
		return nil, errors.New("responses request does not contain any messages")
	}

	return json.Marshal(out)
}

func chatCompletionToResponsesRequest(body []byte) ([]byte, error) {
	req, err := decodeChatCompletionRequest(body)
	if err != nil {
		return nil, err
	}

	input := make([]map[string]any, 0, len(req.Messages))
	for _, message := range req.Messages {
		input = append(input, map[string]any{
			"role":    message.Role,
			"content": message.Content,
		})
	}

	out := map[string]any{
		"model": req.Model,
		"input": input,
	}
	if req.Metadata != nil {
		out["metadata"] = req.Metadata
	}
	if req.Stream {
		out["stream"] = req.Stream
	}

	return json.Marshal(out)
}

func chatCompletionToResponsesResponse(body []byte) ([]byte, error) {
	var upstream map[string]any
	if err := json.Unmarshal(body, &upstream); err != nil {
		return nil, fmt.Errorf("decode chat response: %w", err)
	}

	id := stringOr(upstream["id"], "resp_proxy")
	model := stringOr(upstream["model"], "")
	created := int64Or(upstream["created"], time.Now().Unix())
	text := extractChatCompletionText(upstream)
	if text == "" {
		return nil, errors.New("chat response does not contain assistant text")
	}

	out := map[string]any{
		"id":         id,
		"object":     "response",
		"created_at": created,
		"status":     "completed",
		"model":      model,
		"output": []map[string]any{
			{
				"type":   "message",
				"id":     "msg_" + id,
				"status": "completed",
				"role":   "assistant",
				"content": []map[string]any{
					{
						"type": "output_text",
						"text": text,
					},
				},
			},
		},
		"output_text": text,
	}

	if usage, ok := upstream["usage"]; ok {
		out["usage"] = usage
	}

	return json.Marshal(out)
}

func responsesToChatCompletionResponse(body []byte) ([]byte, error) {
	var upstream map[string]any
	if err := json.Unmarshal(body, &upstream); err != nil {
		return nil, fmt.Errorf("decode responses response: %w", err)
	}

	id := stringOr(upstream["id"], "chatcmpl_proxy")
	model := stringOr(upstream["model"], "")
	text := extractResponsesText(upstream)
	if text == "" {
		return nil, errors.New("responses response does not contain assistant text")
	}

	out := map[string]any{
		"id":      id,
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]any{
			{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": text,
				},
				"finish_reason": "stop",
			},
		},
	}

	if usage, ok := upstream["usage"]; ok {
		out["usage"] = usage
	}

	return json.Marshal(out)
}

func responsesToMessages(req ResponsesRequest) []ChatMessage {
	messages := make([]ChatMessage, 0, 4)
	if req.Instructions != "" {
		messages = append(messages, ChatMessage{
			Role:    "system",
			Content: req.Instructions,
		})
	}

	switch input := req.Input.(type) {
	case string:
		messages = append(messages, ChatMessage{Role: "user", Content: input})
	case []any:
		for _, item := range input {
			if message, ok := responseInputItemToMessage(item); ok {
				messages = append(messages, message)
			}
		}
	case map[string]any:
		if message, ok := responseInputItemToMessage(input); ok {
			messages = append(messages, message)
		}
	}

	return messages
}

func responseInputItemToMessage(item any) (ChatMessage, bool) {
	switch value := item.(type) {
	case string:
		return ChatMessage{Role: "user", Content: value}, true
	case map[string]any:
		role := stringOr(value["role"], "user")
		content, ok := value["content"]
		if !ok {
			return ChatMessage{}, false
		}
		return ChatMessage{Role: role, Content: content}, true
	default:
		return ChatMessage{}, false
	}
}

func extractChatCompletionText(payload map[string]any) string {
	choices, ok := payload["choices"].([]any)
	if !ok || len(choices) == 0 {
		return ""
	}

	first, ok := choices[0].(map[string]any)
	if !ok {
		return ""
	}

	message, ok := first["message"].(map[string]any)
	if !ok {
		return ""
	}

	return flattenContent(message["content"])
}

func extractResponsesText(payload map[string]any) string {
	if text := stringOr(payload["output_text"], ""); text != "" {
		return text
	}

	output, ok := payload["output"].([]any)
	if !ok {
		return ""
	}

	for _, item := range output {
		message, ok := item.(map[string]any)
		if !ok {
			continue
		}

		content, ok := message["content"].([]any)
		if !ok {
			continue
		}

		for _, part := range content {
			segment, ok := part.(map[string]any)
			if !ok {
				continue
			}

			if text := stringOr(segment["text"], ""); text != "" {
				return text
			}
		}
	}

	return ""
}

func stringOr(value any, fallback string) string {
	if typed, ok := value.(string); ok && typed != "" {
		return typed
	}
	return fallback
}

func int64Or(value any, fallback int64) int64 {
	switch typed := value.(type) {
	case int64:
		return typed
	case float64:
		return int64(typed)
	default:
		return fallback
	}
}
