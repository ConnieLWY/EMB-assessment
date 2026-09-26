package server

import (
	"ev-charger-assessment/backend/internal/apidocs"
	"ev-charger-assessment/backend/internal/charger"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
)

type Dependencies struct{ Pool *pgxpool.Pool }

func NewRouter(deps Dependencies) http.Handler {
	mux := http.NewServeMux()
	apidocs.RegisterRoutes(mux)
	handler := charger.NewHandler(charger.NewStore(deps.Pool))
	mux.HandleFunc("GET /api/chargers", handler.List)
	return mux
}
