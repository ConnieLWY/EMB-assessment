package realtime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ev-charger-assessment/backend/internal/charger"

	"github.com/coder/websocket"
)

func dial(t *testing.T, address, origin string) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	header := http.Header{}
	if origin != "" {
		header.Set("Origin", origin)
	}
	return websocket.Dial(ctx, "ws"+strings.TrimPrefix(address, "http"), &websocket.DialOptions{HTTPHeader: header})
}

func waitClients(t *testing.T, hub *Hub, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		hub.mu.Lock()
		got := len(hub.clients)
		hub.mu.Unlock()
		if got == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("client count never became %d", want)
}

func TestStatusBroadcast(t *testing.T) {
	hub := NewHub("http://app.test", "http://backend.test")
	server := httptest.NewServer(hub)
	t.Cleanup(func() { hub.Close(); server.Close() })
	for _, origin := range []string{"", "http://evil.test"} {
		conn, resp, err := dial(t, server.URL, origin)
		if err == nil {
			conn.CloseNow()
			t.Fatalf("origin %q was accepted", origin)
		}
		if resp == nil || resp.StatusCode != http.StatusForbidden {
			t.Fatalf("origin %q status=%v err=%v", origin, resp, err)
		}
	}
	a, _, err := dial(t, server.URL, "http://app.test")
	if err != nil {
		t.Fatal(err)
	}
	defer a.CloseNow()
	b, _, err := dial(t, server.URL, "http://backend.test")
	if err != nil {
		t.Fatal(err)
	}
	defer b.CloseNow()
	waitClients(t, hub, 2)
	at := time.Date(2026, 10, 1, 14, 0, 0, 123, time.UTC)
	hub.Publish(charger.StatusEvent{ChargerID: "charger-1", Status: "CHARGING", UpdatedAt: at})
	for _, conn := range []*websocket.Conn{a, b} {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_, payload, err := conn.Read(ctx)
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		var body struct {
			Event string              `json:"event"`
			Data  charger.StatusEvent `json:"data"`
		}
		if err := json.Unmarshal(payload, &body); err != nil {
			t.Fatal(err)
		}
		if body.Event != "CHARGER_STATUS_UPDATED" || body.Data.ChargerID != "charger-1" || body.Data.Status != "CHARGING" || !body.Data.UpdatedAt.Equal(at) {
			t.Fatalf("payload=%s", payload)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(payload, &fields); err != nil {
			t.Fatal(err)
		}
		if len(fields) != 2 {
			t.Fatalf("unexpected envelope=%s", payload)
		}
	}
	a.CloseNow()
	b.CloseNow()
	waitClients(t, hub, 0)
}

func TestSlowClientDoesNotBlockBroadcast(t *testing.T) {
	hub := NewHub()
	slowStopped := make(chan struct{})
	slow := &client{queue: make(chan []byte, 2), stop: func() { close(slowStopped) }}
	fast := &client{queue: make(chan []byte, 4), stop: func() {}}
	hub.clients[slow] = struct{}{}
	hub.clients[fast] = struct{}{}
	done := make(chan struct{})
	go func() {
		for i := 0; i < 3; i++ {
			hub.Publish(charger.StatusEvent{ChargerID: "charger-1", Status: "AVAILABLE", UpdatedAt: time.Unix(int64(i), 0)})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("slow client blocked publication")
	}
	select {
	case <-slowStopped:
	default:
		t.Fatal("overflowing client was not disconnected")
	}
	if len(hub.clients) != 1 {
		t.Fatalf("clients=%d", len(hub.clients))
	}
	if len(fast.queue) != 3 {
		t.Fatalf("fast client received %d events", len(fast.queue))
	}
}
