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
