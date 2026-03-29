package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
)

var tokenPattern = regexp.MustCompile(`^tok_[a-zA-Z0-9]{32,}$`)

// DetokenizeResult holds the plain string value for a revealed token.
type DetokenizeResult struct {
	Value string `json:"value"`
}

// Revealer scans JSON payloads for token patterns and reveals them
// by calling the Tokenizer's /internal/detokenize endpoint.
type Revealer struct {
	client        *http.Client
	detokenizeURL string
	authHeader    string
}

func NewRevealer(client *http.Client, tokenizerBaseURL, authHeader string) *Revealer {
	return &Revealer{
		client:        client,
		detokenizeURL: tokenizerBaseURL + "/internal/detokenize",
		authHeader:    authHeader,
	}
}

// ScanAndReveal recursively walks a JSON payload, finds string values
// matching the token pattern, calls detokenize for each, and replaces
// the token value with the plain string value (raw PAN or raw CVV).
func (rv *Revealer) ScanAndReveal(ctx context.Context, tenantID string, payload interface{}) (interface{}, error) {
	switch v := payload.(type) {
	case map[string]interface{}:
		for key, val := range v {
			revealed, err := rv.ScanAndReveal(ctx, tenantID, val)
			if err != nil {
				return nil, err
			}
			v[key] = revealed
		}
		return v, nil

	case []interface{}:
		for i, val := range v {
			revealed, err := rv.ScanAndReveal(ctx, tenantID, val)
			if err != nil {
				return nil, err
			}
			v[i] = revealed
		}
		return v, nil

	case string:
		if tokenPattern.MatchString(v) {
			result, err := rv.detokenize(ctx, tenantID, v)
			if err != nil {
				return nil, fmt.Errorf("detokenize %s: %w", v[:12]+"...", err)
			}
			return result.Value, nil
		}
		return v, nil

	default:
		return v, nil
	}
}

func (rv *Revealer) detokenize(ctx context.Context, tenantID, token string) (*DetokenizeResult, error) {
	body, _ := json.Marshal(map[string]string{"token": token})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rv.detokenizeURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if rv.authHeader != "" {
		req.Header.Set("Authorization", rv.authHeader)
	}
	if tenantID != "" {
		req.Header.Set("X-Tenant-ID", tenantID)
	}

	resp, err := rv.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("detokenize request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("detokenize returned %d: %s", resp.StatusCode, respBody)
	}

	var result DetokenizeResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode detokenize response: %w", err)
	}
	return &result, nil
}
