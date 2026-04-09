package amarwave_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	amarwave "github.com/amarwave/amarwave-go"
)

// triggerBody mirrors the JSON payload the SDK sends.
type triggerBody struct {
	AppKey    string      `json:"app_key"`
	AppSecret string      `json:"app_secret"`
	Channel   string      `json:"channel"`
	Event     string      `json:"event"`
	Data      interface{} `json:"data"`
}

// newMockServer returns a test HTTP server. The handler responds with the given
// statusCode for every request, and appends each decoded request body to the
// provided slice.
func newMockServer(t *testing.T, statusCode int, received *[]triggerBody) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/trigger" {
			t.Errorf("expected path /api/v1/trigger, got %s", r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
			t.Errorf("expected Content-Type application/json, got %s", ct)
		}

		body, _ := io.ReadAll(r.Body)
		var tb triggerBody
		if err := json.Unmarshal(body, &tb); err != nil {
			t.Errorf("unmarshal body: %v", err)
		}
		*received = append(*received, tb)

		w.WriteHeader(statusCode)
		if statusCode == http.StatusOK {
			w.Write([]byte(`{"status":"ok"}`))
		} else {
			w.Write([]byte(`{"error":"bad request"}`))
		}
	}))
}

// ─── New / options ───────────────────────────────────────────────────────────

func TestNew_Defaults(t *testing.T) {
	c := amarwave.New("key", "secret")
	if c == nil {
		t.Fatal("New() returned nil")
	}
}

func TestNew_WithBaseURL(t *testing.T) {
	var received []triggerBody
	srv := newMockServer(t, http.StatusOK, &received)
	defer srv.Close()

	c := amarwave.New("k", "s", amarwave.WithBaseURL(srv.URL))
	err := c.TriggerEvent(context.Background(), "ch", "ev", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNew_WithCluster(t *testing.T) {
	var received []triggerBody
	srv := newMockServer(t, http.StatusOK, &received)
	defer srv.Close()

	// WithBaseURL overrides cluster — just verify WithCluster("local") doesn't panic
	c := amarwave.New("k", "s",
		amarwave.WithCluster("local"),
		amarwave.WithBaseURL(srv.URL), // redirect to test server
	)
	err := c.TriggerEvent(context.Background(), "ch", "ev", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNew_WithHTTPClient(t *testing.T) {
	var received []triggerBody
	srv := newMockServer(t, http.StatusOK, &received)
	defer srv.Close()

	custom := &http.Client{Timeout: 5 * time.Second}
	c := amarwave.New("k", "s",
		amarwave.WithBaseURL(srv.URL),
		amarwave.WithHTTPClient(custom),
	)
	err := c.TriggerEvent(context.Background(), "ch", "ev", "data")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNew_WithTimeout(t *testing.T) {
	var received []triggerBody
	srv := newMockServer(t, http.StatusOK, &received)
	defer srv.Close()

	c := amarwave.New("k", "s",
		amarwave.WithBaseURL(srv.URL),
		amarwave.WithTimeout(3*time.Second),
	)
	err := c.TriggerEvent(context.Background(), "ch", "ev", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ─── TriggerEvent ─────────────────────────────────────────────────────────────

func TestTriggerEvent(t *testing.T) {
	tests := []struct {
		name        string
		channel     string
		event       string
		data        interface{}
		serverCode  int
		wantErr     bool
		errContains string
	}{
		{
			name:       "success with map data",
			channel:    "notifications",
			event:      "new-message",
			data:       map[string]interface{}{"text": "hello", "count": 42},
			serverCode: http.StatusOK,
			wantErr:    false,
		},
		{
			name:       "success with nil data",
			channel:    "ping-channel",
			event:      "ping",
			data:       nil,
			serverCode: http.StatusOK,
			wantErr:    false,
		},
		{
			name:       "success with string data",
			channel:    "ch",
			event:      "ev",
			data:       "simple string payload",
			serverCode: http.StatusOK,
			wantErr:    false,
		},
		{
			name:       "success with struct data",
			channel:    "orders",
			event:      "placed",
			data:       struct{ ID int }{ID: 99},
			serverCode: http.StatusOK,
			wantErr:    false,
		},
		{
			name:        "server returns 400",
			channel:     "ch",
			event:       "ev",
			data:        nil,
			serverCode:  http.StatusBadRequest,
			wantErr:     true,
			errContains: "400",
		},
		{
			name:        "server returns 401",
			channel:     "ch",
			event:       "ev",
			data:        nil,
			serverCode:  http.StatusUnauthorized,
			wantErr:     true,
			errContains: "401",
		},
		{
			name:        "server returns 500",
			channel:     "ch",
			event:       "ev",
			data:        nil,
			serverCode:  http.StatusInternalServerError,
			wantErr:     true,
			errContains: "500",
		},
		{
			name:        "empty channel",
			channel:     "",
			event:       "ev",
			data:        nil,
			serverCode:  http.StatusOK,
			wantErr:     true,
			errContains: "channel",
		},
		{
			name:        "empty event",
			channel:     "ch",
			event:       "",
			data:        nil,
			serverCode:  http.StatusOK,
			wantErr:     true,
			errContains: "event",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var received []triggerBody
			srv := newMockServer(t, tt.serverCode, &received)
			defer srv.Close()

			c := amarwave.New("test-key", "test-secret", amarwave.WithBaseURL(srv.URL))
			err := c.TriggerEvent(context.Background(), tt.channel, tt.event, tt.data)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(received) != 1 {
				t.Fatalf("expected 1 request, got %d", len(received))
			}
			rb := received[0]
			if rb.AppKey != "test-key" {
				t.Errorf("app_key: got %q, want %q", rb.AppKey, "test-key")
			}
			if rb.AppSecret != "test-secret" {
				t.Errorf("app_secret: got %q, want %q", rb.AppSecret, "test-secret")
			}
			if rb.Channel != tt.channel {
				t.Errorf("channel: got %q, want %q", rb.Channel, tt.channel)
			}
			if rb.Event != tt.event {
				t.Errorf("event: got %q, want %q", rb.Event, tt.event)
			}
		})
	}
}

// TestTriggerEvent_ContextCancelled verifies the context is forwarded to the
// underlying HTTP request.
func TestTriggerEvent_ContextCancelled(t *testing.T) {
	// serverDone is closed when we want the handler goroutine to unblock so
	// that httptest.Server.Close() can drain in-flight connections quickly.
	serverDone := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Block until either the client cancels or the test server is closing.
		select {
		case <-r.Context().Done():
		case <-serverDone:
		}
	}))
	// Signal the handler to unblock before we wait for the server to close.
	defer func() {
		close(serverDone)
		srv.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	c := amarwave.New("k", "s", amarwave.WithBaseURL(srv.URL), amarwave.WithTimeout(5*time.Second))
	err := c.TriggerEvent(ctx, "ch", "ev", nil)
	if err == nil {
		t.Fatal("expected context deadline exceeded error, got nil")
	}
}

// ─── TriggerBatch ─────────────────────────────────────────────────────────────

func TestTriggerBatch(t *testing.T) {
	tests := []struct {
		name        string
		events      []amarwave.BatchEvent
		serverCode  int
		wantErr     bool
		wantCount   int
		errContains string
	}{
		{
			name: "two events success",
			events: []amarwave.BatchEvent{
				{Channel: "ch-1", Event: "ev-1", Data: map[string]int{"x": 1}},
				{Channel: "ch-2", Event: "ev-2", Data: nil},
			},
			serverCode: http.StatusOK,
			wantCount:  2,
		},
		{
			name:       "empty batch is a no-op",
			events:     []amarwave.BatchEvent{},
			serverCode: http.StatusOK,
			wantCount:  0,
		},
		{
			name: "server error stops batch early",
			events: []amarwave.BatchEvent{
				{Channel: "ch-1", Event: "ev-1", Data: nil},
				{Channel: "ch-2", Event: "ev-2", Data: nil},
			},
			serverCode:  http.StatusBadRequest,
			wantErr:     true,
			wantCount:   1, // only the first request reaches the server
			errContains: "400",
		},
		{
			name: "empty channel in batch",
			events: []amarwave.BatchEvent{
				{Channel: "", Event: "ev", Data: nil},
			},
			serverCode:  http.StatusOK,
			wantErr:     true,
			wantCount:   0,
			errContains: "Channel",
		},
		{
			name: "empty event in batch",
			events: []amarwave.BatchEvent{
				{Channel: "ch", Event: "", Data: nil},
			},
			serverCode:  http.StatusOK,
			wantErr:     true,
			wantCount:   0,
			errContains: "Event",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var received []triggerBody
			srv := newMockServer(t, tt.serverCode, &received)
			defer srv.Close()

			c := amarwave.New("k", "s", amarwave.WithBaseURL(srv.URL))
			err := c.TriggerBatch(context.Background(), tt.events)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errContains)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}

			if len(received) != tt.wantCount {
				t.Errorf("server received %d requests, want %d", len(received), tt.wantCount)
			}
		})
	}
}

// TestTriggerBatch_PayloadsCorrect verifies that every batch event is encoded
// with the correct credentials, channel, and event name.
func TestTriggerBatch_PayloadsCorrect(t *testing.T) {
	var received []triggerBody
	srv := newMockServer(t, http.StatusOK, &received)
	defer srv.Close()

	events := []amarwave.BatchEvent{
		{Channel: "alpha", Event: "start", Data: "first"},
		{Channel: "beta", Event: "stop", Data: 42},
		{Channel: "gamma", Event: "update", Data: nil},
	}

	c := amarwave.New("batch-key", "batch-secret", amarwave.WithBaseURL(srv.URL))
	if err := c.TriggerBatch(context.Background(), events); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(received) != len(events) {
		t.Fatalf("received %d, want %d", len(received), len(events))
	}

	for i, e := range events {
		rb := received[i]
		if rb.AppKey != "batch-key" {
			t.Errorf("[%d] app_key: got %q", i, rb.AppKey)
		}
		if rb.AppSecret != "batch-secret" {
			t.Errorf("[%d] app_secret: got %q", i, rb.AppSecret)
		}
		if rb.Channel != e.Channel {
			t.Errorf("[%d] channel: got %q, want %q", i, rb.Channel, e.Channel)
		}
		if rb.Event != e.Event {
			t.Errorf("[%d] event: got %q, want %q", i, rb.Event, e.Event)
		}
	}
}
