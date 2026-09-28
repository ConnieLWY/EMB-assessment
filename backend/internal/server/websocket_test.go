package server_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ev-charger-assessment/backend/internal/charger"
	"ev-charger-assessment/backend/internal/realtime"
	"ev-charger-assessment/backend/internal/server"

	"github.com/coder/websocket"
)

func TestWebSocketRouteSurvivesHTTPTimeouts(t *testing.T) {
	hub := realtime.NewHub("http://localhost:3000")
	s := httptest.NewUnstartedServer(server.NewRouter(server.Dependencies{Hub: hub}))
	s.Config.ReadTimeout = 200 * time.Millisecond
	s.Config.WriteTimeout = 200 * time.Millisecond
	s.Start()
	t.Cleanup(func() { hub.Close(); s.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	header := http.Header{"Origin": []string{"http://localhost:3000"}}
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(s.URL, "http")+"/api/ws", &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	time.Sleep(300 * time.Millisecond)
	hub.Publish(charger.StatusEvent{ChargerID: "charger-1", Status: "CHARGING", UpdatedAt: time.Now().UTC()})
	_, message, err := conn.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(message), `"event":"CHARGER_STATUS_UPDATED"`) {
		t.Fatalf("message=%s", message)
	}
}
