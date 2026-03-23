package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/asleak/openclaw-gateway/internal/config"
	"github.com/asleak/openclaw-gateway/internal/providers"
	"github.com/asleak/openclaw-gateway/internal/router"
	"github.com/asleak/openclaw-gateway/internal/session"
	"github.com/asleak/openclaw-gateway/internal/telemetry"
)

type stubProvider struct {
	name       string
	api        providers.APIKind
	statusCode int
	response   []byte
	lastBody   []byte
	attempts   int
}

func (s *stubProvider) Name() string {
	return s.name
}

func (s *stubProvider) API() providers.APIKind {
	return s.api
}

func (s *stubProvider) Invoke(_ context.Context, req providers.Request) ([]byte, int, error) {
	s.attempts++
	s.lastBody = append([]byte(nil), req.Body...)
	return s.response, s.statusCode, nil
}

func TestDecisionEndpointUsesStickySession(t *testing.T) {
	cfg := config.Config{
		Version: "v1",
		Server: config.ServerConfig{
			Host:         "127.0.0.1",
			Port:         8080,
			ReadTimeout:  config.Duration(10 * time.Second),
			WriteTimeout: config.Duration(30 * time.Second),
		},
		Routing: config.RoutingConfig{
			DefaultMode:         "local",
			StickyTTL:           config.Duration(30 * time.Minute),
			CloudDwellTime:      config.Duration(15 * time.Minute),
			ComplexityThreshold: 0.75,
			ConfidenceThreshold: 0.55,
			LocalContextLimit:   8192,
			OfflineForceLocal:   true,
		},
		Providers: config.ProvidersConfig{},
		Observability: config.ObservabilityConfig{
			LogLevel:    "error",
			DecisionLog: false,
		},
	}

	decider := router.NewDecider(cfg.Routing, session.NewStore())
	httpServer := New(cfg, decider, nil, nil, telemetry.New("error"))

	firstRecorder := httptest.NewRecorder()
	firstRequest := httptest.NewRequest(http.MethodPost, "/v1/route/decision", strings.NewReader(`{
		"model": "auto",
		"messages": [{"role": "user", "content": "complex request"}],
		"metadata": {"session_id": "sticky-1", "complexity": 0.95}
	}`))
	firstRequest.Header.Set("Content-Type", "application/json")
	httpServer.Handler.ServeHTTP(firstRecorder, firstRequest)

	if firstRecorder.Code != http.StatusOK {
		t.Fatalf("expected first response status 200, got %d", firstRecorder.Code)
	}

	secondRecorder := httptest.NewRecorder()
	secondRequest := httptest.NewRequest(http.MethodPost, "/v1/route/decision", strings.NewReader(`{
		"model": "auto",
		"messages": [{"role": "user", "content": "follow up"}],
		"metadata": {"session_id": "sticky-1", "complexity": 0.10}
	}`))
	secondRequest.Header.Set("Content-Type", "application/json")
	httpServer.Handler.ServeHTTP(secondRecorder, secondRequest)

	if secondRecorder.Code != http.StatusOK {
		t.Fatalf("expected second response status 200, got %d", secondRecorder.Code)
	}

	var response DecisionResponse
	if err := json.Unmarshal(secondRecorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if !response.Sticky {
		t.Fatal("expected sticky session routing on second request")
	}

	if response.Mode != "cloud" {
		t.Fatalf("expected second request to stay on cloud, got %s", response.Mode)
	}
}

func TestResponsesEndpointUsesStickySession(t *testing.T) {
	cfg := config.Config{
		Version: "v1",
		Server: config.ServerConfig{
			Host:         "127.0.0.1",
			Port:         8080,
			ReadTimeout:  config.Duration(10 * time.Second),
			WriteTimeout: config.Duration(30 * time.Second),
		},
		Routing: config.RoutingConfig{
			DefaultMode:         "local",
			StickyTTL:           config.Duration(30 * time.Minute),
			CloudDwellTime:      config.Duration(15 * time.Minute),
			ComplexityThreshold: 0.75,
			ConfidenceThreshold: 0.55,
			LocalContextLimit:   8192,
			OfflineForceLocal:   true,
		},
		Providers: config.ProvidersConfig{},
		Observability: config.ObservabilityConfig{
			LogLevel:    "error",
			DecisionLog: false,
		},
	}

	decider := router.NewDecider(cfg.Routing, session.NewStore())
	httpServer := New(cfg, decider, nil, nil, telemetry.New("error"))

	firstRecorder := httptest.NewRecorder()
	firstRequest := httptest.NewRequest(http.MethodPost, "/v1/route/decision", strings.NewReader(`{
		"model": "auto",
		"input": [{"role": "user", "content": "complex request"}],
		"metadata": {"session_id": "sticky-resp-1", "complexity": 0.95}
	}`))
	firstRequest.Header.Set("Content-Type", "application/json")
	httpServer.Handler.ServeHTTP(firstRecorder, firstRequest)

	secondRecorder := httptest.NewRecorder()
	secondRequest := httptest.NewRequest(http.MethodPost, "/v1/route/decision", strings.NewReader(`{
		"model": "auto",
		"input": [{"role": "user", "content": "follow up"}],
		"metadata": {"session_id": "sticky-resp-1", "complexity": 0.10}
	}`))
	secondRequest.Header.Set("Content-Type", "application/json")
	httpServer.Handler.ServeHTTP(secondRecorder, secondRequest)

	if secondRecorder.Code != http.StatusOK {
		t.Fatalf("expected second response status 200, got %d", secondRecorder.Code)
	}

	var response DecisionResponse
	if err := json.Unmarshal(secondRecorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if !response.Sticky {
		t.Fatal("expected sticky session routing for responses-style request")
	}

	if response.Mode != "cloud" {
		t.Fatalf("expected second request to stay on cloud, got %s", response.Mode)
	}
}

func TestResponsesEndpointTranslatesToChatCompletionProvider(t *testing.T) {
	cfg := config.Config{
		Version: "v1",
		Server: config.ServerConfig{
			Host:         "127.0.0.1",
			Port:         8080,
			ReadTimeout:  config.Duration(10 * time.Second),
			WriteTimeout: config.Duration(30 * time.Second),
		},
		Routing: config.RoutingConfig{
			DefaultMode:         "local",
			StickyTTL:           config.Duration(30 * time.Minute),
			CloudDwellTime:      config.Duration(15 * time.Minute),
			ComplexityThreshold: 0.75,
			ConfidenceThreshold: 0.55,
			LocalContextLimit:   8192,
			OfflineForceLocal:   true,
		},
		Providers: config.ProvidersConfig{},
		Observability: config.ObservabilityConfig{
			LogLevel:    "error",
			DecisionLog: false,
		},
	}

	local := &stubProvider{
		name:       "local",
		api:        providers.ChatCompletionsAPI,
		statusCode: http.StatusOK,
		response: []byte(`{
			"id": "chatcmpl_123",
			"object": "chat.completion",
			"created": 1711111111,
			"model": "openclaw-local",
			"choices": [
				{
					"index": 0,
					"message": {"role": "assistant", "content": "local answer"},
					"finish_reason": "stop"
				}
			]
		}`),
	}

	decider := router.NewDecider(cfg.Routing, session.NewStore())
	httpServer := New(cfg, decider, local, nil, telemetry.New("error"))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{
		"model": "auto",
		"input": [{"role": "user", "content": "hello"}],
		"metadata": {"session_id": "resp-e2e-1", "complexity": 0.1}
	}`))
	request.Header.Set("Content-Type", "application/json")
	httpServer.Handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}

	var upstreamReq ChatCompletionRequest
	if err := json.Unmarshal(local.lastBody, &upstreamReq); err != nil {
		t.Fatalf("decode translated upstream request: %v", err)
	}

	if len(upstreamReq.Messages) != 1 || upstreamReq.Messages[0].Role != "user" {
		t.Fatalf("expected translated upstream request to contain one user message, got %+v", upstreamReq.Messages)
	}

	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode gateway response: %v", err)
	}

	if response["object"] != "response" {
		t.Fatalf("expected responses object, got %v", response["object"])
	}

	if response["output_text"] != "local answer" {
		t.Fatalf("expected translated output_text, got %v", response["output_text"])
	}
}

func TestResponsesEndpointNormalizesUpstreamHTMLFailure(t *testing.T) {
	cfg := config.Config{
		Version: "v1",
		Server: config.ServerConfig{
			Host:         "127.0.0.1",
			Port:         8080,
			ReadTimeout:  config.Duration(10 * time.Second),
			WriteTimeout: config.Duration(30 * time.Second),
		},
		Routing: config.RoutingConfig{
			DefaultMode:         "cloud",
			StickyTTL:           config.Duration(30 * time.Minute),
			CloudDwellTime:      config.Duration(15 * time.Minute),
			ComplexityThreshold: 0.75,
			ConfidenceThreshold: 0.55,
			LocalContextLimit:   8192,
			OfflineForceLocal:   true,
		},
		Providers: config.ProvidersConfig{},
		Observability: config.ObservabilityConfig{
			LogLevel:    "error",
			DecisionLog: false,
		},
	}

	cloud := &stubProvider{
		name:       "cloud-test",
		api:        providers.ResponsesAPI,
		statusCode: http.StatusBadGateway,
		response:   []byte(`<!DOCTYPE html><html><head><title>Bad gateway</title></head><body>bad gateway</body></html>`),
	}

	decider := router.NewDecider(cfg.Routing, session.NewStore())
	httpServer := New(cfg, decider, nil, cloud, telemetry.New("error"))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{
		"model": "auto",
		"input": [{"role": "user", "content": "hello"}]
	}`))
	request.Header.Set("Content-Type", "application/json")
	httpServer.Handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("expected status 502, got %d", recorder.Code)
	}

	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("expected JSON error response, got decode error: %v", err)
	}

	errorPayload, ok := response["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error payload, got %v", response)
	}

	if errorPayload["type"] != "upstream_error" {
		t.Fatalf("expected upstream_error type, got %v", errorPayload["type"])
	}

	if !strings.Contains(errorPayload["message"].(string), "Bad gateway") {
		t.Fatalf("expected normalized upstream message, got %v", errorPayload["message"])
	}
}
