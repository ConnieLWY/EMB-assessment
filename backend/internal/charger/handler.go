package charger

import (
	"context"
	"ev-charger-assessment/backend/internal/httpapi"
	"log/slog"
	"net/http"
	"time"
)

type Handler struct{ store *Store }

func NewHandler(store *Store) *Handler { return &Handler{store: store} }

// List returns the current charger snapshot.
// @Summary List chargers
// @Description Returns all chargers ordered by ID, or an empty array. Current device status does not guarantee availability for future time slots. No authentication is required.
// @Tags Chargers
// @ID listChargers
// @Produce json
// @Success 200 {object} ChargerList "Current charger list"
// @Failure 403 {object} httpapi.ErrorResponse "ORIGIN_NOT_ALLOWED: untrusted Origin"
// @Failure 500 {object} httpapi.ErrorResponse "INTERNAL_ERROR: the charger list could not be retrieved"
// @Router /api/chargers [get]
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	chargers, err := h.store.ListChargers(ctx)
	w.Header().Set("Cache-Control", "no-store")
	if err != nil {
		slog.ErrorContext(ctx, "list chargers failed", "error", err)
		httpapi.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "An unexpected error occurred.")
		return
	}
	httpapi.JSON(w, http.StatusOK, ChargerList{Chargers: chargers})
}
