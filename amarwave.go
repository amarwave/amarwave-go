// Package amarwave provides a server-side client for triggering real-time
// events through the AmarWave messaging platform.
//
// Usage:
//
//	client := amarwave.New("my_app_key", "my_app_secret")
//	err := client.TriggerEvent(ctx, "my-channel", "my-event", map[string]interface{}{
//	    "message": "hello world",
//	})
package amarwave

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

const defaultTimeout = 10 * time.Second

// defaultBaseURL builds the API base URL from environment variables.
//
// Supported env vars (all optional):
//
//	AMARWAVE_HOST   API server hostname   (default: localhost)
//	AMARWAVE_PORT   API server port       (default: 8000)
//	AMARWAVE_SSL    set 'true' for HTTPS  (default: false → http)
func defaultBaseURL() string {
	host := os.Getenv("AMARWAVE_HOST")
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv("AMARWAVE_PORT")
	if port == "" {
		port = "8000"
	}
	scheme := "http"
	if os.Getenv("AMARWAVE_SSL") == "true" {
		scheme = "https"
	}
	return scheme + "://" + host + ":" + port
}

// defaultAPIPath returns the trigger endpoint path from the environment.
//
//	AMARWAVE_API_PATH   API trigger path (default: /api/v1/trigger)
func defaultAPIPath() string {
	p := os.Getenv("AMARWAVE_API_PATH")
	if p == "" {
		p = "/api/v1/trigger"
	}
	return p
}

// Client is the AmarWave server-side client for triggering events.
type Client struct {
	appKey     string
	appSecret  string
	baseURL    string
	apiPath    string
	httpClient *http.Client
}

// Option is a functional option for configuring the Client.
type Option func(*Client)

// WithBaseURL sets a custom API base URL (e.g. "https://api.example.com").
// The URL should not have a trailing slash.
func WithBaseURL(url string) Option {
	return func(c *Client) {
		c.baseURL = url
	}
}

// WithAPIPath overrides the trigger endpoint path (default: /api/v1/trigger).
// The path must start with a slash (e.g. "/api/v1/trigger").
func WithAPIPath(path string) Option {
	return func(c *Client) {
		c.apiPath = path
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
// The baseURL defaults to "http://localhost:8000" if not set via WithBaseURL.
//
// Example:
//
//	client := amarwave.New("app_key", "app_secret",
//	    amarwave.WithBaseURL("https://api.yourapp.com"),
//	    amarwave.WithTimeout(5*time.Second),
//	)
func New(appKey, appSecret string, opts ...Option) *Client {
	c := &Client{
		appKey:    appKey,
		appSecret: appSecret,
		baseURL:   defaultBaseURL(),
		apiPath:   defaultAPIPath(),
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
	AppKey    string      `json:"app_key"`
	AppSecret string      `json:"app_secret"`
	Channel   string      `json:"channel"`
	Event     string      `json:"event"`
	Data      interface{} `json:"data"`
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
//	err := client.TriggerEvent(ctx, "notifications", "new-message", map[string]interface{}{
//	    "from":    "Alice",
//	    "body":    "Hey there!",
//	    "read":    false,
//	})
func (c *Client) TriggerEvent(ctx context.Context, channel, event string, data interface{}) error {
	if channel == "" {
		return fmt.Errorf("amarwave: channel must not be empty")
	}
	if event == "" {
		return fmt.Errorf("amarwave: event must not be empty")
	}

	payload := triggerPayload{
		AppKey:    c.appKey,
		AppSecret: c.appSecret,
		Channel:   channel,
		Event:     event,
		Data:      data,
	}

	return c.doRequest(ctx, payload)
}

// BatchEvent represents a single event within a batch trigger call.
type BatchEvent struct {
	// Channel is the target channel name (e.g. "chat-room-1").
	Channel string
	// Event is the event name subscribers will receive (e.g. "message").
	Event string
	// Data is the arbitrary payload serialised to JSON.
	Data interface{}
}

// TriggerBatch publishes multiple events sequentially. Each event is sent as
// an individual HTTP request to POST /api/v1/trigger. If any request fails,
// TriggerBatch returns the error immediately without sending the remaining
// events.
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
		payload := triggerPayload{
			AppKey:    c.appKey,
			AppSecret: c.appSecret,
			Channel:   e.Channel,
			Event:     e.Event,
			Data:      e.Data,
		}
		if err := c.doRequest(ctx, payload); err != nil {
			return fmt.Errorf("amarwave: batch event %d (%s/%s): %w", i, e.Channel, e.Event, err)
		}
	}
	return nil
}

// doRequest encodes payload as JSON and POSTs it to /api/v1/trigger.
// It returns an error for non-2xx responses, including the status code and
// as much of the response body as available.
func (c *Client) doRequest(ctx context.Context, payload triggerPayload) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("amarwave: marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+c.apiPath, bytes.NewReader(body))
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
		// Read up to 512 bytes of the error body for debugging.
		buf := make([]byte, 512)
		n, _ := resp.Body.Read(buf)
		return fmt.Errorf("amarwave: server returned %d: %s", resp.StatusCode, string(buf[:n]))
	}

	return nil
}
