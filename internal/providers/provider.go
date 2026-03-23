package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type APIKind string

const (
	ChatCompletionsAPI APIKind = "chat-completions"
	ResponsesAPI       APIKind = "responses"
)

type Request struct {
	Body  []byte
	Model string
}

type Provider interface {
	Name() string
	API() APIKind
	Invoke(ctx context.Context, req Request) ([]byte, int, error)
}

type HTTPProvider struct {
	name          string
	baseURL       string
	api           APIKind
	model         string
	apiKey        string
	client        *http.Client
	retryAttempts int
	retryBackoff  time.Duration
}

func NewHTTPProvider(name, baseURL string, api APIKind, model, apiKey string, timeout time.Duration, retryAttempts int, retryBackoff time.Duration) *HTTPProvider {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	if api == "" {
		api = ChatCompletionsAPI
	}
	if retryAttempts < 0 {
		retryAttempts = 0
	}
	if retryBackoff < 0 {
		retryBackoff = 0
	}

	return &HTTPProvider{
		name:          name,
		baseURL:       strings.TrimRight(baseURL, "/"),
		api:           api,
		model:         model,
		apiKey:        apiKey,
		retryAttempts: retryAttempts,
		retryBackoff:  retryBackoff,
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

func (p *HTTPProvider) Name() string {
	return p.name
}

func (p *HTTPProvider) API() APIKind {
	return p.api
}

func (p *HTTPProvider) Invoke(ctx context.Context, req Request) ([]byte, int, error) {
	payload, err := forceModel(req.Body, p.model)
	if err != nil {
		return nil, 0, fmt.Errorf("prepare upstream request: %w", err)
	}

	var lastStatus int
	var lastBody []byte

	for attempt := 0; attempt <= p.retryAttempts; attempt++ {
		if attempt > 0 {
			if err := sleepWithContext(ctx, p.retryBackoff); err != nil {
				return nil, 0, err
			}
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+pathForAPI(p.api), bytes.NewReader(payload))
		if err != nil {
			return nil, 0, fmt.Errorf("build upstream request: %w", err)
		}

		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Accept", "application/json")
		if p.apiKey != "" {
			httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
		}

		resp, err := p.client.Do(httpReq)
		if err != nil {
			if shouldRetryTransportError(err) && attempt < p.retryAttempts {
				continue
			}
			return nil, 0, fmt.Errorf("call upstream: %w", err)
		}

		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		resp.Body.Close()
		if readErr != nil {
			return nil, resp.StatusCode, fmt.Errorf("read upstream response: %w", readErr)
		}

		lastStatus = resp.StatusCode
		lastBody = body
		if shouldRetryStatus(resp.StatusCode) && attempt < p.retryAttempts {
			continue
		}

		return body, resp.StatusCode, nil
	}

	return lastBody, lastStatus, nil
}

func ParseAPIKind(raw string) APIKind {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(ResponsesAPI):
		return ResponsesAPI
	default:
		return ChatCompletionsAPI
	}
}

func pathForAPI(api APIKind) string {
	switch api {
	case ResponsesAPI:
		return "/responses"
	default:
		return "/chat/completions"
	}
}

func forceModel(body []byte, model string) ([]byte, error) {
	if strings.TrimSpace(model) == "" {
		return body, nil
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}

	payload["model"] = model
	return json.Marshal(payload)
}

func shouldRetryStatus(statusCode int) bool {
	switch statusCode {
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func shouldRetryTransportError(err error) bool {
	return !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)
}

func sleepWithContext(ctx context.Context, duration time.Duration) error {
	if duration <= 0 {
		return nil
	}

	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
