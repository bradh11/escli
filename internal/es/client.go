// Package es is a small HTTP client for the Elasticsearch REST API that
// authenticates with an API key.
package es

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/bradh11/escli/internal/version"
)

// Client sends requests to one Elasticsearch cluster.
type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

// New returns a Client for the cluster at baseURL. apiKey must be in the
// encoded form; see NormalizeAPIKey.
func New(baseURL, apiKey string, timeout time.Duration) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		http:    &http.Client{Timeout: timeout},
	}
}

// BaseURL returns the cluster address.
func (c *Client) BaseURL() string {
	return c.baseURL
}

// Do sends a request and returns the response body. path starts with "/".
// A response status of 300 or higher is returned as an *Error.
func (c *Client) Do(ctx context.Context, method, path string, query url.Values, body []byte) ([]byte, error) {
	target := c.baseURL + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "ApiKey "+c.apiKey)
	req.Header.Set("User-Agent", "escli/"+version.Version)
	// A "format" parameter asks for CSV or text, which an Accept header would override.
	if query.Get("format") == "" {
		req.Header.Set("Accept", "application/json")
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request to %s failed: %w", c.baseURL, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response from %s: %w", c.baseURL, err)
	}
	if resp.StatusCode >= 300 {
		return nil, newError(resp.StatusCode, data)
	}
	return data, nil
}

// Error is an error response from Elasticsearch.
type Error struct {
	Status int
	Type   string
	Reason string
	Body   string
}

func (e *Error) Error() string {
	msg := fmt.Sprintf("elasticsearch returned HTTP %d", e.Status)
	if e.Type != "" {
		msg += ": " + e.Type
	}
	switch {
	case e.Reason != "":
		msg += ": " + e.Reason
	case e.Body != "":
		msg += ": " + truncate(e.Body, 500)
	}
	switch e.Status {
	case http.StatusUnauthorized:
		msg += " (the API key is missing, wrong, expired or revoked; run `escli init` to store a new one)"
	case http.StatusForbidden:
		msg += " (the API key has no privilege for this request)"
	}
	return msg
}

// IsStatus reports whether err is an *Error with the given HTTP status.
func IsStatus(err error, status int) bool {
	var e *Error
	return errors.As(err, &e) && e.Status == status
}

func newError(status int, body []byte) *Error {
	e := &Error{Status: status}
	var envelope struct {
		Error json.RawMessage `json:"error"`
	}
	if json.Unmarshal(body, &envelope) == nil && len(envelope.Error) > 0 {
		var detail struct {
			Type   string `json:"type"`
			Reason string `json:"reason"`
		}
		var text string
		switch {
		case json.Unmarshal(envelope.Error, &detail) == nil:
			e.Type, e.Reason = detail.Type, detail.Reason
		case json.Unmarshal(envelope.Error, &text) == nil:
			e.Reason = text
		}
	}
	if e.Type == "" && e.Reason == "" {
		e.Body = strings.TrimSpace(string(body))
	}
	return e
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// NormalizeAPIKey accepts an API key in any form that Kibana or the create API
// key API shows (the encoded key, the "id:api_key" pair or the JSON response)
// and returns the encoded form that the Authorization header needs.
func NormalizeAPIKey(s string) (string, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimSpace(strings.TrimPrefix(s, "ApiKey "))
	if s == "" {
		return "", errors.New("the API key is empty")
	}
	if strings.HasPrefix(s, "{") {
		var k struct {
			ID      string `json:"id"`
			APIKey  string `json:"api_key"`
			Encoded string `json:"encoded"`
		}
		if err := json.Unmarshal([]byte(s), &k); err != nil {
			return "", fmt.Errorf("the API key looks like JSON but cannot be read: %w", err)
		}
		switch {
		case k.Encoded != "":
			return k.Encoded, nil
		case k.ID != "" && k.APIKey != "":
			return base64.StdEncoding.EncodeToString([]byte(k.ID + ":" + k.APIKey)), nil
		default:
			return "", errors.New(`the API key JSON has neither "encoded" nor "id" and "api_key"`)
		}
	}
	if strings.ContainsAny(s, " \t\r\n") {
		return "", errors.New("the API key contains spaces; paste only the key")
	}
	// The encoded form is base64 and never contains ':', so this is the id:api_key pair.
	if strings.Contains(s, ":") {
		return base64.StdEncoding.EncodeToString([]byte(s)), nil
	}
	return s, nil
}
