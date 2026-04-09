// Package amarwave provides a server-side client for triggering real-time
// events through the AmarWave messaging platform.
//
// Usage:
//
//	client := amarwave.New("my_app_key", "my_app_secret")
//	err := client.TriggerEvent(ctx, "my-channel", "my-event", map[string]any{
//	    "message": "hello world",
//	})
package amarwave

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const (
	defaultTimeout = 10 * time.Second
	apiPath        = "/api/v1/trigger"
)

// clusterBaseURLs maps cluster names to their base API URLs.
var clusterBaseURLs = map[string]string{
	"default": "https://amarwave.com",
	"local":   "http://localhost:8000",
	"eu":      "https://amarwave.com",
	"us":      "https://amarwave.com",
	"ap1":     "https://amarwave.com",
	"ap2":     "https://amarwave.com",
}

// Client is the AmarWave server-side client for triggering events.
type Client struct {
	appKey     string
	appSecret  string
	baseURL    string
	httpClient *http.Client
}

// Option is a functional option for configuring the Client.
type Option func(*Client)

// WithCluster selects a predefined AmarWave cluster.
// Available clusters: "default", "local", "eu", "us", "ap1", "ap2".
// Defaults to "default" (https://amarwave.com).
func WithCluster(cluster string) Option {
	return func(c *Client) {
		if url, ok := clusterBaseURLs[cluster]; ok {
			c.baseURL = url
		}
	}
}

// withBaseURL is an internal option used in tests to redirect requests.
func withBaseURL(url string) Option {
	return func(c *Client) {
		c.baseURL = url
	}
}

// WithHTTPClient sets a custom *http.Client, allowing full control over
// transport, TLS configuration, proxies, etc.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) {
		c.httpClient = hc
	}
}

// WithTimeout sets the HTTP request timeout. Defaults to 10 seconds.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) {
		c.httpClient = &http.Client{Timeout: d}
	}
}

// New creates a new AmarWave client with the given app credentials.
// Defaults to the "default" cluster (https://api.amarwave.com).
//
// Example:
//
//	client := amarwave.New("app_key", "app_secret")
//
//	client := amarwave.New("app_key", "app_secret",
//	    amarwave.WithCluster("eu"),
//	    amarwave.WithTimeout(5*time.Second),
//	)
func New(appKey, appSecret string, opts ...Option) *Client {
	c := &Client{
		appKey:    appKey,
		appSecret: appSecret,
		baseURL:   clusterBaseURLs["default"],
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// triggerPayload is the JSON body sent to POST /api/v1/trigger.
type triggerPayload struct {
	AppKey    string `json:"app_key"`
	AppSecret string `json:"app_secret"`
	Channel   string `json:"channel"`
	Event     string `json:"event"`
	Data      any    `json:"data"`
}

// TriggerEvent publishes a single event to the given channel.
// The data argument is serialised as JSON and can be any JSON-compatible value
// (map, struct, slice, string, etc.). A nil data value sends a JSON null.
//
// The context is forwarded to the underlying HTTP request, so callers can
// cancel or set a deadline independently of the client-level timeout.
//
// Example:
//
//	err := client.TriggerEvent(ctx, "notifications", "new-message", map[string]any{
//	    "from": "Alice",
//	    "body": "Hey there!",
//	})
func (c *Client) TriggerEvent(ctx context.Context, channel, event string, data any) error {
	if channel == "" {
		return fmt.Errorf("amarwave: channel must not be empty")
	}
	if event == "" {
		return fmt.Errorf("amarwave: event must not be empty")
	}

	return c.doRequest(ctx, triggerPayload{
		AppKey:    c.appKey,
		AppSecret: c.appSecret,
		Channel:   channel,
		Event:     event,
		Data:      data,
	})
}

// BatchEvent represents a single event within a batch trigger call.
type BatchEvent struct {
	// Channel is the target channel name (e.g. "chat-room-1").
	Channel string
	// Event is the event name subscribers will receive (e.g. "message").
	Event string
	// Data is the arbitrary payload serialised to JSON.
	Data any
}

// TriggerBatch publishes multiple events sequentially. Each event is sent as
// an individual HTTP request. If any request fails, TriggerBatch returns the
// error immediately without sending the remaining events.
//
// Example:
//
//	err := client.TriggerBatch(ctx, []amarwave.BatchEvent{
//	    {Channel: "ch-1", Event: "update", Data: map[string]int{"count": 10}},
//	    {Channel: "ch-2", Event: "ping",   Data: nil},
//	})
func (c *Client) TriggerBatch(ctx context.Context, events []BatchEvent) error {
	for i, e := range events {
		if e.Channel == "" {
			return fmt.Errorf("amarwave: events[%d].Channel must not be empty", i)
		}
		if e.Event == "" {
			return fmt.Errorf("amarwave: events[%d].Event must not be empty", i)
		}
		if err := c.doRequest(ctx, triggerPayload{
			AppKey:    c.appKey,
			AppSecret: c.appSecret,
			Channel:   e.Channel,
			Event:     e.Event,
			Data:      e.Data,
		}); err != nil {
			return fmt.Errorf("amarwave: batch event %d (%s/%s): %w", i, e.Channel, e.Event, err)
		}
	}
	return nil
}

// doRequest encodes payload as JSON and POSTs it to the trigger endpoint.
// It returns an error for non-2xx responses, including the status code and
// as much of the response body as available.
func (c *Client) doRequest(ctx context.Context, payload triggerPayload) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("amarwave: marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+apiPath, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("amarwave: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("amarwave: http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		buf := make([]byte, 512)
		n, _ := resp.Body.Read(buf)
		return fmt.Errorf("amarwave: server returned %d: %s", resp.StatusCode, string(buf[:n]))
	}

	return nil
}
