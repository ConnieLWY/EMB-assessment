package charger

import "time"

type Charger struct {
	ID        string    `json:"id" example:"charger-1" binding:"required"`
	Name      string    `json:"name" example:"Charger 1" binding:"required"`
	Location  string    `json:"location" example:"Level 1, Bay A" binding:"required"`
	Status    string    `json:"status" enums:"AVAILABLE,CHARGING,MAINTENANCE" example:"AVAILABLE" binding:"required"`
	UpdatedAt time.Time `json:"updated_at" format:"date-time" example:"2026-09-27T10:00:00Z" binding:"required"`
}

type ChargerList struct {
	Chargers []Charger `json:"chargers" binding:"required"`
}

type Reservation struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	ChargerID string    `json:"charger_id"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ReserveInput struct {
	UserID    string    `json:"user_id"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
}

type ReservationResponse struct {
	Reservation Reservation `json:"reservation"`
}
type ReservationList struct {
	Reservations []Reservation `json:"reservations"`
}

type StatusEvent struct {
	ChargerID string    `json:"charger_id"`
	Status    string    `json:"status"`
	UpdatedAt time.Time `json:"updated_at"`
}
