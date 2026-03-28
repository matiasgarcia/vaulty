package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// ForwardResult holds the destination's raw response.
type ForwardResult struct {
	StatusCode int                 `json:"status_code"`
	Headers    map[string]string   `json:"headers,omitempty"`
	Body       json.RawMessage     `json:"body"`
}

// Forwarder sends revealed payloads to destination URLs.
type Forwarder struct {
	client *http.Client
}

func NewForwarder(client *http.Client) *Forwarder {
	return &Forwarder{client: client}
}

// Forward sends the payload to the destination and returns the raw response.
func (f *Forwarder) Forward(ctx context.Context, destination, method string, headers map[string]string, payload interface{}) (*ForwardResult, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, destination, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("forward request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	// Collect response headers
	respHeaders := make(map[string]string)
	for k := range resp.Header {
		respHeaders[k] = resp.Header.Get(k)
	}

	// Wipe payload from memory
	for i := range body {
		body[i] = 0
	}

	return &ForwardResult{
		StatusCode: resp.StatusCode,
		Headers:    respHeaders,
		Body:       json.RawMessage(respBody),
	}, nil
}
