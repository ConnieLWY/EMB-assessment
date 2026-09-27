package charger

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrValidation          = errors.New("invalid reservation fields")
	ErrUserMismatch        = errors.New("reservation user differs from session user")
	ErrChargerNotFound     = errors.New("charger not found")
	ErrReservationConflict = errors.New("reservation overlaps another reservation")
	ErrOccupied            = errors.New("charger is occupied")
	ErrMaintenance         = errors.New("charger is in maintenance")
)

type Service struct {
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
