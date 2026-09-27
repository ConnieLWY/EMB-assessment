package charger

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrValidation                = errors.New("invalid reservation fields")
	ErrUserMismatch              = errors.New("reservation user differs from session user")
	ErrChargerNotFound           = errors.New("charger not found")
	ErrReservationConflict       = errors.New("reservation overlaps another reservation")
	ErrOccupied                  = errors.New("charger is occupied")
	ErrMaintenance               = errors.New("charger is in maintenance")
	ErrReservationNotFound       = errors.New("reservation not found")
	ErrReservationNotCancellable = errors.New("reservation cannot be cancelled")
	ErrInvalidReservationID      = errors.New("invalid reservation ID")
)

type Service struct {
	mu      sync.Mutex
	store   *Store
	now     func() time.Time
	publish func(StatusEvent)
}

func NewService(store *Store, now func() time.Time, publish func(StatusEvent)) *Service {
	if now == nil {
		now = time.Now
	}
	if publish == nil {
		publish = func(StatusEvent) {}
	}
	return &Service{store: store, now: now, publish: publish}
}

func (s *Service) Reserve(ctx context.Context, actorID, chargerID string, input ReserveInput) (Reservation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var id pgtype.UUID
	now := s.now()
	if err := id.Scan(input.UserID); err != nil || !id.Valid || input.StartTime.IsZero() || input.EndTime.IsZero() ||
		!input.StartTime.Before(input.EndTime) || input.StartTime.Before(now) {
		return Reservation{}, ErrValidation
	}
	var actor pgtype.UUID
	if err := actor.Scan(actorID); err != nil || !actor.Valid {
		return Reservation{}, ErrValidation
	}
	if id.Bytes != actor.Bytes {
		return Reservation{}, ErrUserMismatch
	}
	return s.store.reserve(ctx, actorID, chargerID, input.StartTime.UTC(), input.EndTime.UTC(), now)
}

func (s *Service) CancelReservation(ctx context.Context, actorID, reservationID string) (Reservation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var actor, id pgtype.UUID
	if actor.Scan(actorID) != nil || !actor.Valid || id.Scan(reservationID) != nil || !id.Valid {
		return Reservation{}, ErrInvalidReservationID
	}
	return s.store.cancelReservation(ctx, actorID, reservationID, s.now())
}
