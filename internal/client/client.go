// Package client is a thin wrapper around the mongosync HTTP control API.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client talks to a single mongosync instance, identified by its API base URL
// (for example http://localhost:27182).
type Client struct {
	BaseURL string
	http    *http.Client
}

// New returns a Client for the given mongosync API base URL.
func New(baseURL string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// Response carries the raw JSON body and HTTP status of a mongosync call.
type Response struct {
	Body   json.RawMessage
	Status int
}

func (c *Client) call(ctx context.Context, method, path string, body any) (*Response, error) {
	var reader io.Reader
	switch {
	case body != nil:
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(raw)
	case method == http.MethodPost:
		reader = bytes.NewReader([]byte("{}"))
	}

	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return &Response{Body: json.RawMessage(data), Status: resp.StatusCode}, nil
}

// Progress returns the current synchronization status.
func (c *Client) Progress(ctx context.Context) (*Response, error) {
	return c.call(ctx, http.MethodGet, "/api/v1/progress", nil)
}

// Start begins a synchronization session with the supplied options body.
func (c *Client) Start(ctx context.Context, body any) (*Response, error) {
	return c.call(ctx, http.MethodPost, "/api/v1/start", body)
}

// Pause pauses the running session.
func (c *Client) Pause(ctx context.Context) (*Response, error) {
	return c.call(ctx, http.MethodPost, "/api/v1/pause", nil)
}

// Resume resumes a paused session.
func (c *Client) Resume(ctx context.Context) (*Response, error) {
	return c.call(ctx, http.MethodPost, "/api/v1/resume", nil)
}

// Commit finalizes the synchronization session.
func (c *Client) Commit(ctx context.Context) (*Response, error) {
	return c.call(ctx, http.MethodPost, "/api/v1/commit", nil)
}

// Reverse reverses the direction of a committed session.
func (c *Client) Reverse(ctx context.Context) (*Response, error) {
	return c.call(ctx, http.MethodPost, "/api/v1/reverse", nil)
}

// Ping verifies the mongosync API is reachable.
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.Progress(ctx)
	return err
}

// WaitReady polls the mongosync API until it responds or the timeout elapses.
func (c *Client) WaitReady(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_, err := c.Progress(ctx)
		cancel()
		if err == nil {
			return nil
		}
		lastErr = err
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("mongosync API at %s did not become ready: %v", c.BaseURL, lastErr)
}
