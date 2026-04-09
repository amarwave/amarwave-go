# amarwave-go

Official Go server-side SDK for [AmarWave](https://github.com/amarwave/amarwave-go) — trigger real-time events from your Go backend.

## Installation

```bash
go get github.com/amarwave/amarwave-go
```

Requires Go 1.21 or later. No external dependencies — uses only the Go standard library.

---

## Quick Start

```go
package main

import (
    "context"
    "log"

    amarwave "github.com/amarwave/amarwave-go"
)

func main() {
    client := amarwave.New("your_app_key", "your_app_secret")

    err := client.TriggerEvent(
        context.Background(),
        "notifications",   // channel
        "new-message",     // event name
        map[string]any{
            "from": "Alice",
            "body": "Hello!",
        },
    )
    if err != nil {
        log.Fatal(err)
    }
}
```

---

## Configuration

`New` accepts optional functional options:

```go
// Hosted — default cluster (https://api.amarwave.com)
client := amarwave.New("app_key", "app_secret")

// Specific cluster
client := amarwave.New("app_key", "app_secret",
    amarwave.WithCluster("eu"),   // eu, us, ap1, ap2
)

// Self-hosted or local development
client := amarwave.New("app_key", "app_secret",
    amarwave.WithBaseURL("http://localhost:8000"),
)

// Custom timeout
client := amarwave.New("app_key", "app_secret",
    amarwave.WithTimeout(5*time.Second),
)

// Custom *http.Client (e.g. with custom transport/TLS)
client := amarwave.New("app_key", "app_secret",
    amarwave.WithHTTPClient(&http.Client{
        Transport: myCustomTransport,
        Timeout:   5 * time.Second,
    }),
)
```

### Clusters

| Cluster     | Base URL                        |
|-------------|---------------------------------|
| `default`   | `https://api.amarwave.com`      |
| `eu`        | `https://api-eu.amarwave.com`   |
| `us`        | `https://api-us.amarwave.com`   |
| `ap1`       | `https://api-ap1.amarwave.com`  |
| `ap2`       | `https://api-ap2.amarwave.com`  |

---

## Triggering a Single Event

```go
ctx := context.Background()

// Map payload
err := client.TriggerEvent(ctx, "orders", "placed", map[string]any{
    "order_id": 12345,
    "total":    99.99,
    "currency": "USD",
})

// Struct payload (any JSON-serialisable value works)
type OrderEvent struct {
    OrderID int     `json:"order_id"`
    Total   float64 `json:"total"`
}
err = client.TriggerEvent(ctx, "orders", "updated", OrderEvent{
    OrderID: 12345,
    Total:   109.99,
})

// Nil payload (sends JSON null)
err = client.TriggerEvent(ctx, "presence-room-1", "ping", nil)
```

---

## Triggering Multiple Events (Batch)

```go
err := client.TriggerBatch(ctx, []amarwave.BatchEvent{
    {
        Channel: "user-42",
        Event:   "notification",
        Data:    map[string]string{"title": "New follower"},
    },
    {
        Channel: "private-chat-99",
        Event:   "message",
        Data:    map[string]any{"text": "Hey!", "ts": 1710000000},
    },
    {
        Channel: "presence-lobby",
        Event:   "ping",
        Data:    nil,
    },
})
if err != nil {
    log.Printf("batch error: %v", err)
}
```

Events are sent sequentially. If any event fails, the error is returned immediately and subsequent events are not sent.

---

## Context and Cancellation

Every method accepts a `context.Context`, so you can propagate deadlines or cancellation signals from your request handlers:

```go
// net/http handler
func webhookHandler(w http.ResponseWriter, r *http.Request) {
    err := client.TriggerEvent(r.Context(), "webhooks", "received", map[string]string{
        "source": r.Header.Get("X-Source"),
    })
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    w.WriteHeader(http.StatusOK)
}

// Gin handler
func NotifyHandler(c *gin.Context) {
    ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
    defer cancel()

    err := client.TriggerEvent(ctx, "updates", "refresh", nil)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    c.JSON(200, gin.H{"ok": true})
}

// Fiber handler
func NotifyHandler(c *fiber.Ctx) error {
    err := client.TriggerEvent(context.Background(), "updates", "refresh", nil)
    if err != nil {
        return c.Status(500).JSON(fiber.Map{"error": err.Error()})
    }
    return c.JSON(fiber.Map{"ok": true})
}
```

---

## Channel Naming Conventions

| Prefix      | Type     | Notes                               |
|-------------|----------|-------------------------------------|
| (none)      | Public   | Anyone can subscribe                |
| `private-`  | Private  | Requires HMAC auth from your server |
| `presence-` | Presence | Tracks online users; requires auth  |

Examples: `"chat-room-1"`, `"private-user-42"`, `"presence-lobby"`

---

## Error Handling

The SDK returns an error when:
- `channel` or `event` is an empty string
- The HTTP request fails (network error, timeout, context cancelled)
- The server returns a non-2xx status code

```go
err := client.TriggerEvent(ctx, "ch", "ev", data)
if err != nil {
    // Example: "amarwave: server returned 401: {"error":"invalid credentials"}"
    log.Printf("trigger failed: %v", err)
}
```

---

## Running Tests

```bash
go test ./...
```

---

## License

MIT
