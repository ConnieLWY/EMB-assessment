package charger

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func (s *Store) ListChargers(ctx context.Context) ([]Charger, error) {
	rows, err := s.pool.Query(ctx, "SELECT id, name, location, status, updated_at FROM chargers ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	chargers := make([]Charger, 0)
	for rows.Next() {
		var c Charger
		if err := rows.Scan(&c.ID, &c.Name, &c.Location, &c.Status, &c.UpdatedAt); err != nil {
			return nil, err
		}
		c.UpdatedAt = c.UpdatedAt.UTC()
		chargers = append(chargers, c)
	}
	return chargers, rows.Err()
}
