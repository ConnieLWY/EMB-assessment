package realtime

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"sync"
	"time"

	"ev-charger-assessment/backend/internal/charger"
	"ev-charger-assessment/backend/internal/httpapi"
	"github.com/coder/websocket"
)

const queueSize = 32

type client struct {
	queue chan []byte
	stop  func()
	once  sync.Once
}

func (c *client) terminate() {
	c.once.Do(func() {
		if c.stop != nil {
			c.stop()
		}
	})
}

type Hub struct {
	mu      sync.Mutex
	clients map[*client]struct{}
	origins []string
	closed  bool
	wg      sync.WaitGroup
}

func NewHub(origins ...string) *Hub {
	if len(origins) == 0 {
		origins = []string{"http://localhost:3000", "http://localhost:8080"}
	}
	return &Hub{clients: make(map[*client]struct{}), origins: slices.Clone(origins)}
}

func (h *Hub) Publish(event charger.StatusEvent) {
	event.UpdatedAt = event.UpdatedAt.UTC()
	payload, err := json.Marshal(struct {
		Event string              `json:"event"`
		Data  charger.StatusEvent `json:"data"`
	}{Event: "CHARGER_STATUS_UPDATED", Data: event})
	if err != nil {
		return
	}
	var overflowing []*client
	h.mu.Lock()
	if !h.closed {
		for c := range h.clients {
			select {
			case c.queue <- payload:
			default:
				delete(h.clients, c)
				overflowing = append(overflowing, c)
			}
		}
	}
	h.mu.Unlock()
	for _, c := range overflowing {
		c.terminate()
	}
}

// ServeHTTP upgrades an allowed public client to the device-status stream.
// @Summary Stream charger status changes
// @Description Public WebSocket stream of CHARGER_STATUS_UPDATED events. An allowed Origin is required. Clients do not send application messages. Swagger UI cannot open a WebSocket through Try it out.
// @Tags Chargers
// @ID streamChargerStatus
// @Success 101 "WebSocket connection established"
// @Failure 403 {object} httpapi.ErrorResponse "ORIGIN_NOT_ALLOWED"
// @Router /api/ws [get]
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	originValues := r.Header.Values("Origin")
	if len(originValues) != 1 || !slices.Contains(h.origins, originValues[0]) {
		httpapi.Error(w, http.StatusForbidden, "ORIGIN_NOT_ALLOWED", "Request origin is not allowed.")
		return
	}
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: h.origins})
	if err != nil {
		return
	}
	conn.SetReadLimit(1024)
	ctx, cancel := context.WithCancel(context.Background())
	c := &client{queue: make(chan []byte, queueSize)}
	c.stop = func() { cancel(); _ = conn.CloseNow() }
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		c.terminate()
		return
	}
	h.clients[c] = struct{}{}
	h.wg.Add(1)
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.clients, c)
		h.mu.Unlock()
		c.terminate()
		h.wg.Done()
	}()
	readCtx := conn.CloseRead(ctx)
	ping := time.NewTicker(30 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-readCtx.Done():
			return
		case payload := <-c.queue:
			writeCtx, done := context.WithTimeout(ctx, 5*time.Second)
			err := conn.Write(writeCtx, websocket.MessageText, payload)
			done()
			if err != nil {
				return
			}
		case <-ping.C:
			pingCtx, done := context.WithTimeout(ctx, 10*time.Second)
			err := conn.Ping(pingCtx)
			done()
			if err != nil {
				return
			}
		}
	}
}

func (h *Hub) Close() {
	h.mu.Lock()
	h.closed = true
	clients := make([]*client, 0, len(h.clients))
	for c := range h.clients {
		clients = append(clients, c)
	}
	h.mu.Unlock()
	for _, c := range clients {
		c.terminate()
	}
	h.wg.Wait()
}
