package charger

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

type Handler struct{ store *Store }

func NewHandler(store *Store) *Handler { return &Handler{store: store} }

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	chargers, err := h.store.ListChargers(ctx)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err != nil {
		slog.ErrorContext(ctx, "list chargers failed", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("{\"error\":{\"code\":\"INTERNAL_ERROR\",\"message\":\"An unexpected error occurred.\"}}\n"))
		return
	}
	if err := json.NewEncoder(w).Encode(struct {
		Chargers []Charger `json:"chargers"`
	}{chargers}); err != nil {
		slog.WarnContext(ctx, "write charger response failed", "error", err)
	}
}
