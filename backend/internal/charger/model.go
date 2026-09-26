package charger

import "time"

type Charger struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Location  string    `json:"location"`
	Status    string    `json:"status"`
	UpdatedAt time.Time `json:"updated_at"`
}
