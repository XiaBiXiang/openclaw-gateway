package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/asleak/openclaw-gateway/internal/config"
	"github.com/asleak/openclaw-gateway/internal/policy"
	"github.com/asleak/openclaw-gateway/internal/providers"
	"github.com/asleak/openclaw-gateway/internal/router"
	"github.com/asleak/openclaw-gateway/internal/telemetry"
)

type application struct {
	config        config.Config
	logger        *telemetry.Logger
	decider       *router.Decider
	localProvider providers.Provider
	cloudProvider providers.Provider
	decisionLog   bool
}

type ChatCompletionRequest struct {
	Model    string         `json:"model"`
	Messages []ChatMessage  `json:"messages"`
	Metadata map[string]any `json:"metadata,omitempty"`
	Stream   bool           `json:"stream,omitempty"`
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type DecisionResponse struct {
	Mode            string         `json:"mode"`
	Reason          string         `json:"reason"`
	Sticky          bool           `json:"sticky"`
	SessionID       string         `json:"session_id,omitempty"`
	EstimatedTokens int            `json:"estimated_tokens"`
	Signals         map[string]any `json:"signals"`
}

var errServerClosed = http.ErrServerClosed

func ErrServerClosed() error {
	return errServerClosed
}

func New(cfg config.Config, decider *router.Decider, localProvider, cloudProvider providers.Provider, logger *telemetry.Logger) *http.Server {
	app := &application{
		config:        cfg,
		logger:        logger,
		decider:       decider,
		localProvider: localProvider,
		cloudProvider: cloudProvider,
		decisionLog:   cfg.Observability.DecisionLog,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", app.handleHealth)
	mux.HandleFunc("/v1/route/decision", app.handleDecision)
	mux.HandleFunc("/v1/chat/completions", app.handleChatCompletions)
	mux.HandleFunc("/v1/responses", app.handleResponses)

	return &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      mux,
		ReadTimeout:  time.Duration(cfg.Server.ReadTimeout),
		WriteTimeout: time.Duration(cfg.Server.WriteTimeout),
	}
}

func (a *application) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, http.MethodGet)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"version": a.config.Version,
		"providers": map[string]any{
			"local": a.localProvider != nil,
			"cloud": a.cloudProvider != nil,
		},
	})
}

func (a *application) handleDecision(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, http.MethodPost)
		return
	}

	rawBody, err := readRequestBody(r)
	if err != nil {
		writeOpenAIError(w, http.StatusBadRequest, err.Error())
		return
	}

	messages, metadata, stream, err := decodeDecisionRequest(rawBody)
	if err != nil {
		writeOpenAIError(w, http.StatusBadRequest, err.Error())
		return
	}
	if stream {
		writeOpenAIError(w, http.StatusBadRequest, "streaming is not supported yet")
		return
	}

	requestContext, signals := buildRequestContext(r, messages, metadata)
	decision := a.decider.Decide(requestContext)

	writeJSON(w, http.StatusOK, DecisionResponse{
		Mode:            string(decision.Mode),
		Reason:          decision.Reason,
		Sticky:          decision.Sticky,
		SessionID:       decision.SessionID,
		EstimatedTokens: requestContext.Signals.EstimatedTokens,
		Signals:         signals,
	})
}

func (a *application) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, http.MethodPost)
		return
	}

	rawBody, err := readRequestBody(r)
	if err != nil {
		writeOpenAIError(w, http.StatusBadRequest, err.Error())
		return
	}

	req, err := decodeChatCompletionRequest(rawBody)
	if err != nil {
		writeOpenAIError(w, http.StatusBadRequest, err.Error())
		return
	}

	if req.Stream {
		writeOpenAIError(w, http.StatusBadRequest, "streaming is not supported yet")
		return
	}

	requestContext, signals := buildRequestContext(r, req.Messages, req.Metadata)
	decision := a.decider.Decide(requestContext)
	upstream := a.providerFor(decision.Mode)
	if upstream == nil {
		writeOpenAIError(w, http.StatusServiceUnavailable, fmt.Sprintf("%s provider is not configured", decision.Mode))
		return
	}

	ctx, cancel := a.requestContextWithTimeout(r.Context(), decision.Mode)
	defer cancel()

	upstreamBody, err := bodyForUpstream(upstream.API(), providers.ChatCompletionsAPI, rawBody)
	if err != nil {
		writeOpenAIError(w, http.StatusBadRequest, err.Error())
		return
	}

	responseBody, statusCode, err := upstream.Invoke(ctx, providers.Request{
		Body:  upstreamBody,
		Model: req.Model,
	})
	if err != nil {
		a.logger.Error("upstream request failed", map[string]any{
			"provider": upstream.Name(),
			"route":    decision.Mode,
			"reason":   decision.Reason,
			"error":    err.Error(),
		})
		writeOpenAIError(w, http.StatusBadGateway, err.Error())
		return
	}
	if statusCode < 200 || statusCode >= 300 {
		writeUpstreamError(w, statusCode, upstream.Name(), responseBody)
		return
	}

	responseBody, err = bodyForClient(providers.ChatCompletionsAPI, upstream.API(), statusCode, responseBody)
	if err != nil {
		writeOpenAIError(w, http.StatusBadGateway, err.Error())
		return
	}

	if a.decisionLog {
		a.logger.Info("request routed", map[string]any{
			"session_id":       decision.SessionID,
			"route":            decision.Mode,
			"reason":           decision.Reason,
			"sticky":           decision.Sticky,
			"provider":         upstream.Name(),
			"estimated_tokens": requestContext.Signals.EstimatedTokens,
			"signals":          signals,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-OCG-Route", string(decision.Mode))
	w.Header().Set("X-OCG-Reason", decision.Reason)
	w.WriteHeader(statusCode)
	_, _ = w.Write(responseBody)
}

func (a *application) handleResponses(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, http.MethodPost)
		return
	}

	rawBody, err := readRequestBody(r)
	if err != nil {
		writeOpenAIError(w, http.StatusBadRequest, err.Error())
		return
	}

	req, err := decodeResponsesRequest(rawBody)
	if err != nil {
		writeOpenAIError(w, http.StatusBadRequest, err.Error())
		return
	}

	if req.Stream {
		writeOpenAIError(w, http.StatusBadRequest, "streaming is not supported yet")
		return
	}

	messages := responsesToMessages(req)
	requestContext, signals := buildRequestContext(r, messages, req.Metadata)
	decision := a.decider.Decide(requestContext)
	upstream := a.providerFor(decision.Mode)
	if upstream == nil {
		writeOpenAIError(w, http.StatusServiceUnavailable, fmt.Sprintf("%s provider is not configured", decision.Mode))
		return
	}

	ctx, cancel := a.requestContextWithTimeout(r.Context(), decision.Mode)
	defer cancel()

	upstreamBody, err := bodyForUpstream(upstream.API(), providers.ResponsesAPI, rawBody)
	if err != nil {
		writeOpenAIError(w, http.StatusBadRequest, err.Error())
		return
	}

	responseBody, statusCode, err := upstream.Invoke(ctx, providers.Request{
		Body:  upstreamBody,
		Model: req.Model,
	})
	if err != nil {
		a.logger.Error("upstream request failed", map[string]any{
			"provider": upstream.Name(),
			"route":    decision.Mode,
			"reason":   decision.Reason,
			"error":    err.Error(),
		})
		writeOpenAIError(w, http.StatusBadGateway, err.Error())
		return
	}
	if statusCode < 200 || statusCode >= 300 {
		writeUpstreamError(w, statusCode, upstream.Name(), responseBody)
		return
	}

	responseBody, err = bodyForClient(providers.ResponsesAPI, upstream.API(), statusCode, responseBody)
	if err != nil {
		writeOpenAIError(w, http.StatusBadGateway, err.Error())
		return
	}

	if a.decisionLog {
		a.logger.Info("request routed", map[string]any{
			"session_id":       decision.SessionID,
			"route":            decision.Mode,
			"reason":           decision.Reason,
			"sticky":           decision.Sticky,
			"provider":         upstream.Name(),
			"estimated_tokens": requestContext.Signals.EstimatedTokens,
			"signals":          signals,
			"endpoint":         "responses",
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-OCG-Route", string(decision.Mode))
	w.Header().Set("X-OCG-Reason", decision.Reason)
	w.WriteHeader(statusCode)
	_, _ = w.Write(responseBody)
}

func (a *application) providerFor(mode policy.Mode) providers.Provider {
	if mode == policy.CloudMode {
		return a.cloudProvider
	}
	return a.localProvider
}

func (a *application) requestContextWithTimeout(parent context.Context, mode policy.Mode) (context.Context, context.CancelFunc) {
	timeout := 30 * time.Second
	if mode == policy.CloudMode && a.config.Providers.Cloud.Timeout > 0 {
		timeout = time.Duration(a.config.Providers.Cloud.Timeout)
	}
	if mode == policy.LocalMode && a.config.Providers.Local.Timeout > 0 {
		timeout = time.Duration(a.config.Providers.Local.Timeout)
	}
	return context.WithTimeout(parent, timeout)
}

func readRequestBody(r *http.Request) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("read request body: %w", err)
	}
	if len(body) == 0 {
		return nil, errors.New("request body is empty")
	}

	return body, nil
}

func buildRequestContext(r *http.Request, messages []ChatMessage, metadata map[string]any) (router.RequestContext, map[string]any) {
	if metadata == nil {
		metadata = map[string]any{}
	}

	sessionID := pickString(metadata["session_id"], r.Header.Get("X-Session-ID"))
	privacyLevel := pickString(metadata["privacy_level"], r.Header.Get("X-Privacy-Level"))
	offline := pickBool(metadata["offline"], r.Header.Get("X-Offline-Mode"))
	complexity := pickFloat(metadata["complexity"], r.Header.Get("X-Complexity-Score"))

	var confidence *float64
	if value, ok := optionalFloat(metadata["local_confidence"], r.Header.Get("X-Local-Confidence")); ok {
		confidence = &value
	}

	estimatedTokens := estimateTokens(messages)
	signals := policy.Signals{
		PrivacyLevel:    privacyLevel,
		Offline:         offline,
		Complexity:      complexity,
		LocalConfidence: confidence,
		EstimatedTokens: estimatedTokens,
	}

	decisionSignals := map[string]any{
		"privacy_level":    privacyLevel,
		"offline":          offline,
		"complexity":       complexity,
		"estimated_tokens": estimatedTokens,
	}
	if confidence != nil {
		decisionSignals["local_confidence"] = *confidence
	}

	return router.RequestContext{
		SessionID: sessionID,
		Signals:   signals,
	}, decisionSignals
}

func estimateTokens(messages []ChatMessage) int {
	totalChars := 0
	for _, message := range messages {
		totalChars += len(message.Role)
		totalChars += len(flattenContent(message.Content))
	}
	if totalChars == 0 {
		return 0
	}
	return int(math.Ceil(float64(totalChars) / 4.0))
}

func flattenContent(content any) string {
	switch value := content.(type) {
	case string:
		return value
	default:
		encoded, err := json.Marshal(value)
		if err != nil {
			return ""
		}
		return string(encoded)
	}
}

func bodyForUpstream(upstreamAPI, incomingAPI providers.APIKind, rawBody []byte) ([]byte, error) {
	if upstreamAPI == incomingAPI {
		return rawBody, nil
	}

	switch {
	case incomingAPI == providers.ResponsesAPI && upstreamAPI == providers.ChatCompletionsAPI:
		return responsesToChatCompletionRequest(rawBody)
	case incomingAPI == providers.ChatCompletionsAPI && upstreamAPI == providers.ResponsesAPI:
		return chatCompletionToResponsesRequest(rawBody)
	default:
		return nil, fmt.Errorf("unsupported protocol translation: %s -> %s", incomingAPI, upstreamAPI)
	}
}

func bodyForClient(clientAPI, upstreamAPI providers.APIKind, statusCode int, rawBody []byte) ([]byte, error) {
	if statusCode < 200 || statusCode >= 300 || clientAPI == upstreamAPI {
		return rawBody, nil
	}

	switch {
	case clientAPI == providers.ResponsesAPI && upstreamAPI == providers.ChatCompletionsAPI:
		return chatCompletionToResponsesResponse(rawBody)
	case clientAPI == providers.ChatCompletionsAPI && upstreamAPI == providers.ResponsesAPI:
		return responsesToChatCompletionResponse(rawBody)
	default:
		return nil, fmt.Errorf("unsupported protocol translation: %s -> %s", upstreamAPI, clientAPI)
	}
}

func pickString(value any, fallback string) string {
	switch typed := value.(type) {
	case string:
		if strings.TrimSpace(typed) != "" {
			return typed
		}
	}
	return strings.TrimSpace(fallback)
}

func pickBool(value any, fallback string) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(typed, "true")
	}
	return strings.EqualFold(strings.TrimSpace(fallback), "true")
}

func pickFloat(value any, fallback string) float64 {
	if parsed, ok := optionalFloat(value, fallback); ok {
		return parsed
	}
	return 0
}

func optionalFloat(value any, fallback string) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case string:
		parsed, err := parseFloatString(typed)
		return parsed, err == nil
	}

	if strings.TrimSpace(fallback) == "" {
		return 0, false
	}

	parsed, err := parseFloatString(fallback)
	return parsed, err == nil
}

func parseFloatString(raw string) (float64, error) {
	return strconv.ParseFloat(strings.TrimSpace(raw), 64)
}

func writeMethodNotAllowed(w http.ResponseWriter, allowed string) {
	w.Header().Set("Allow", allowed)
	writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed")
}

func writeOpenAIError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"message": message,
			"type":    "gateway_error",
		},
	})
}

func writeUpstreamError(w http.ResponseWriter, status int, provider string, rawBody []byte) {
	message := summarizeUpstreamError(status, rawBody)
	writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"message":     fmt.Sprintf("%s upstream returned %d: %s", provider, status, message),
			"type":        "upstream_error",
			"provider":    provider,
			"status_code": status,
		},
	})
}

func summarizeUpstreamError(status int, rawBody []byte) string {
	if len(rawBody) == 0 {
		return http.StatusText(status)
	}

	var payload map[string]any
	if err := json.Unmarshal(rawBody, &payload); err == nil {
		if errorPayload, ok := payload["error"].(map[string]any); ok {
			if message := stringOr(errorPayload["message"], ""); message != "" {
				return message
			}
		}
		if detail := stringOr(payload["detail"], ""); detail != "" {
			return detail
		}
	}

	body := strings.TrimSpace(string(rawBody))
	if body == "" {
		return http.StatusText(status)
	}

	if title := htmlTitle(body); title != "" {
		return title
	}

	if strings.Contains(strings.ToLower(body), "<html") {
		return http.StatusText(status)
	}

	body = compactWhitespace(body)
	if len(body) > 240 {
		body = body[:240]
	}
	return body
}

func htmlTitle(body string) string {
	lower := strings.ToLower(body)
	start := strings.Index(lower, "<title>")
	end := strings.Index(lower, "</title>")
	if start == -1 || end == -1 || end <= start+7 {
		return ""
	}
	return compactWhitespace(body[start+7 : end])
}

func compactWhitespace(raw string) string {
	return strings.Join(strings.Fields(raw), " ")
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
