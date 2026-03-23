package providers

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestHTTPProviderRetriesRetryableStatus(t *testing.T) {
	attempts := 0

	provider := NewHTTPProvider("cloud", "https://example.test", ResponsesAPI, "gpt-test", "", 5*time.Second, 2, 5*time.Millisecond)
	provider.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			attempts++
			if req.URL.Path != "/responses" {
				t.Fatalf("expected /responses path, got %s", req.URL.Path)
			}

			if attempts < 3 {
				return &http.Response{
					StatusCode: http.StatusBadGateway,
					Body:       io.NopCloser(bytes.NewBufferString(`<html><head><title>Bad gateway</title></head><body>bad gateway</body></html>`)),
					Header:     make(http.Header),
				}, nil
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(`{"id":"resp_123","object":"response","output_text":"OK"}`)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	body, statusCode, err := provider.Invoke(context.Background(), Request{
		Body:  []byte(`{"model":"auto","input":[{"role":"user","content":"hello"}]}`),
		Model: "auto",
	})
	if err != nil {
		t.Fatalf("expected no invoke error, got %v", err)
	}

	if statusCode != http.StatusOK {
		t.Fatalf("expected 200 after retries, got %d", statusCode)
	}

	if string(body) != `{"id":"resp_123","object":"response","output_text":"OK"}` {
		t.Fatalf("unexpected response body: %s", string(body))
	}

	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
}
