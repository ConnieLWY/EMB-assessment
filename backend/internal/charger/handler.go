package charger

import (
	"context"
	"errors"
	"ev-charger-assessment/backend/internal/auth"
	"ev-charger-assessment/backend/internal/httpapi"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"time"
)

type Handler struct {
	store   *Store
	service *Service
}

func NewHandler(store *Store, service *Service) *Handler {
	return &Handler{store: store, service: service}
}

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

// Reserve creates a future reservation for the signed-in user.
// @Summary Reserve a charger
// @Description Requires the session cookie and an allowed Origin. The user_id must match the signed-in user. Intervals are half-open and timestamps require an explicit offset.
// @Tags Reservations
// @ID reserveCharger
// @Accept json
// @Produce json
// @Param id path string true "Charger ID"
// @Param reservation body ReserveInput true "Reservation interval and user ID"
// @Success 201 {object} ReservationResponse "Reservation created"
// @Failure 400 {object} httpapi.ErrorResponse "VALIDATION_ERROR"
// @Failure 401 {object} httpapi.ErrorResponse "UNAUTHENTICATED"
// @Failure 403 {object} httpapi.ErrorResponse "USER_ID_MISMATCH or ORIGIN_NOT_ALLOWED"
// @Failure 404 {object} httpapi.ErrorResponse "CHARGER_NOT_FOUND"
// @Failure 409 {object} httpapi.ErrorResponse "RESERVATION_CONFLICT, CHARGER_OCCUPIED, or CHARGER_IN_MAINTENANCE"
// @Failure 413 {object} httpapi.ErrorResponse "PAYLOAD_TOO_LARGE"
// @Failure 415 {object} httpapi.ErrorResponse "UNSUPPORTED_MEDIA_TYPE"
// @Failure 500 {object} httpapi.ErrorResponse "INTERNAL_ERROR"
// @Router /api/chargers/{id}/reserve [post]
func (h *Handler) Reserve(w http.ResponseWriter, r *http.Request) {
	var input ReserveInput
	if !httpapi.Decode(w, r, &input) {
		return
	}
	user, _ := auth.UserFromContext(r.Context())
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	reservation, err := h.service.Reserve(ctx, user.ID, r.PathValue("id"), input)
	if err != nil {
		reservationError(w, r, err)
		return
	}
	httpapi.JSON(w, 201, ReservationResponse{Reservation: reservation})
}

// ListReservations returns the signed-in user's reservations.
// @Summary List my reservations
// @Description Returns only the signed-in user's reservations. Use group=upcoming for scheduled, waiting, and active reservations; group=history for completed, expired, and cancelled reservations. Groups are paginated independently.
// @Tags Reservations
// @ID listReservations
// @Produce json
// @Param page query int false "Page number, starting at 1" default(1)
// @Param limit query int false "Page size, 1–100" default(5)
// @Param group query string false "Reservation group" Enums(upcoming,history)
// @Success 200 {object} ReservationList "Current user's reservations"
// @Failure 400 {object} httpapi.ErrorResponse "VALIDATION_ERROR"
// @Failure 401 {object} httpapi.ErrorResponse "UNAUTHENTICATED"
// @Failure 403 {object} httpapi.ErrorResponse "ORIGIN_NOT_ALLOWED"
// @Failure 500 {object} httpapi.ErrorResponse "INTERNAL_ERROR"
// @Router /api/reservations [get]
func (h *Handler) ListReservations(w http.ResponseWriter, r *http.Request) {
	page, limit := 1, 5
	group := ""
	if values, ok := r.URL.Query()["group"]; ok {
		if len(values) != 1 || (values[0] != "upcoming" && values[0] != "history") {
			httpapi.Error(w, 400, "VALIDATION_ERROR", "Invalid reservation group.")
			return
		}
		group = values[0]
	}
	for _, param := range []struct {
		name  string
		value *int
	}{{"page", &page}, {"limit", &limit}} {
		if values, ok := r.URL.Query()[param.name]; ok {
			if len(values) != 1 {
				httpapi.Error(w, 400, "VALIDATION_ERROR", "Invalid pagination parameters.")
				return
			}
			parsed, err := strconv.Atoi(values[0])
			if err != nil || parsed < 1 {
				httpapi.Error(w, 400, "VALIDATION_ERROR", "Invalid pagination parameters.")
				return
			}
			*param.value = parsed
		}
	}
	if limit > 100 || page-1 > math.MaxInt/limit {
		httpapi.Error(w, 400, "VALIDATION_ERROR", "Invalid pagination parameters.")
		return
	}
	user, _ := auth.UserFromContext(r.Context())
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	reservations, total, err := h.store.ListReservations(ctx, user.ID, group, page, limit)
	if err != nil {
		reservationError(w, r, err)
		return
	}
	httpapi.JSON(w, 200, ReservationList{Reservations: reservations, Page: page, Limit: limit, Total: total})
}

// CancelReservation cancels an unstarted reservation owned by the signed-in user.
// @Summary Cancel my reservation
// @Description Cancels a scheduled or waiting reservation. Active charging sessions cannot be cancelled.
// @Tags Reservations
// @ID cancelReservation
// @Produce json
// @Param id path string true "Reservation ID"
// @Success 200 {object} ReservationResponse "Reservation cancelled"
// @Failure 400 {object} httpapi.ErrorResponse "VALIDATION_ERROR"
// @Failure 401 {object} httpapi.ErrorResponse "UNAUTHENTICATED"
// @Failure 403 {object} httpapi.ErrorResponse "ORIGIN_NOT_ALLOWED"
// @Failure 404 {object} httpapi.ErrorResponse "RESERVATION_NOT_FOUND"
// @Failure 409 {object} httpapi.ErrorResponse "RESERVATION_NOT_CANCELLABLE"
// @Failure 500 {object} httpapi.ErrorResponse "INTERNAL_ERROR"
// @Router /api/reservations/{id}/cancel [post]
func (h *Handler) CancelReservation(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	reservation, err := h.service.CancelReservation(ctx, user.ID, r.PathValue("id"))
	if err != nil {
		reservationError(w, r, err)
		return
	}
	httpapi.JSON(w, http.StatusOK, ReservationResponse{Reservation: reservation})
}

func reservationError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrValidation):
		httpapi.Error(w, 400, "VALIDATION_ERROR", "Provide a valid user ID and future time interval.")
	case errors.Is(err, ErrInvalidReservationID):
		httpapi.Error(w, 400, "VALIDATION_ERROR", "Provide a valid reservation ID.")
	case errors.Is(err, ErrUserMismatch):
		httpapi.Error(w, 403, "USER_ID_MISMATCH", "Reservation user ID must match the signed-in user.")
	case errors.Is(err, ErrChargerNotFound):
		httpapi.Error(w, 404, "CHARGER_NOT_FOUND", "Charger not found.")
	case errors.Is(err, ErrReservationConflict):
		httpapi.Error(w, 409, "RESERVATION_CONFLICT", "This charger is already reserved for the selected time slot.")
	case errors.Is(err, ErrOccupied):
		httpapi.Error(w, 409, "CHARGER_OCCUPIED", "This charger is occupied for the selected time slot.")
	case errors.Is(err, ErrMaintenance):
		httpapi.Error(w, 409, "CHARGER_IN_MAINTENANCE", "This charger is currently in maintenance.")
	case errors.Is(err, ErrReservationNotFound):
		httpapi.Error(w, 404, "RESERVATION_NOT_FOUND", "Reservation not found.")
	case errors.Is(err, ErrReservationNotCancellable):
		httpapi.Error(w, 409, "RESERVATION_NOT_CANCELLABLE", "Only scheduled or waiting reservations can be cancelled.")
	default:
		slog.ErrorContext(r.Context(), "reservation request failed", "error", err)
		httpapi.Error(w, 500, "INTERNAL_ERROR", "An unexpected error occurred.")
	}
}
