package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"ev-charger-assessment/backend/internal/realtime"
	"ev-charger-assessment/backend/internal/server"

	"github.com/coder/websocket"
)

type safeLogBuffer struct {
	mu   sync.Mutex
	data bytes.Buffer
}

func (b *safeLogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.data.Write(p)
}

func (b *safeLogBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.data.String()
}

func parseRequestLog(t *testing.T, line string) struct {
	Method     string  `json:"method"`
	Path       string  `json:"path"`
	Status     int     `json:"status"`
	DurationMS float64 `json:"duration_ms"`
} {
	t.Helper()
	var entry struct {
		Method     string  `json:"method"`
		Path       string  `json:"path"`
		Status     int     `json:"status"`
		DurationMS float64 `json:"duration_ms"`
	}
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		t.Fatal(err)
	}
	return entry
}

func TestAPIRequestLogging(t *testing.T) {
	logs := &safeLogBuffer{}
	logger := slog.New(slog.NewJSONHandler(logs, nil))
	router := server.NewRouter(server.Dependencies{Logger: logger})
	req := httptest.NewRequest("POST", "/api/auth/login?token=query-secret", strings.NewReader(`{"username":"demo","password":"body-secret"}`))
	req.Header.Set("Origin", "http://untrusted.example")
	req.Header.Set("Cookie", "ev_session=cookie-secret")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != 403 {
		t.Fatalf("status=%d", rec.Code)
	}
	entry := parseRequestLog(t, strings.TrimSpace(logs.String()))
	if entry.Method != "POST" || entry.Path != "/api/auth/login" || entry.Status != 403 || entry.DurationMS < 0 {
		t.Fatalf("entry=%+v", entry)
	}
	for _, secret := range []string{"query-secret", "body-secret", "cookie-secret"} {
		if strings.Contains(logs.String(), secret) {
			t.Fatalf("logged %s", secret)
		}
	}
}

func TestWebSocketHandshakeLogging(t *testing.T) {
	logs := &safeLogBuffer{}
	logger := slog.New(slog.NewJSONHandler(logs, nil))
	hub := realtime.NewHub("http://localhost:3000")
	s := httptest.NewServer(server.NewRouter(server.Dependencies{Hub: hub, Logger: logger}))
	t.Cleanup(func() { hub.Close(); s.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(s.URL, "http")+"/api/ws", &websocket.DialOptions{
		HTTPHeader: http.Header{"Origin": []string{"http://localhost:3000"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	deadline := time.Now().Add(time.Second)
	for logs.String() == "" && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	entry := parseRequestLog(t, strings.TrimSpace(logs.String()))
	if entry.Method != "GET" || entry.Path != "/api/ws" || entry.Status != 101 || entry.DurationMS < 0 {
		t.Fatalf("entry=%+v", entry)
	}
	// The handshake log is emitted while the WebSocket is still open.
	readCtx, done := context.WithTimeout(ctx, 50*time.Millisecond)
	defer done()
	_, _, err = conn.Read(readCtx)
	if err == nil {
		t.Fatal("unexpected message")
	}
}
