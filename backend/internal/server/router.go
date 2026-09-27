package server

import (
	"ev-charger-assessment/backend/internal/apidocs"
	"ev-charger-assessment/backend/internal/auth"
	"ev-charger-assessment/backend/internal/charger"
	"ev-charger-assessment/backend/internal/realtime"
	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"
	"net/http"
	"time"
)

type Dependencies struct {
	Pool           *pgxpool.Pool
	Now            func() time.Time
	FrontendOrigin string
	APIOrigin      string
	CookieSecure   bool
	Hub            *realtime.Hub
	ChargerService *charger.Service
	Logger         *slog.Logger
}

func NewRouter(deps Dependencies) http.Handler {
	if deps.FrontendOrigin == "" {
		deps.FrontendOrigin = "http://localhost:3000"
	}
	if deps.APIOrigin == "" {
		deps.APIOrigin = "http://localhost:8080"
	}
	mux := http.NewServeMux()
	apidocs.RegisterRoutes(mux)
	if deps.Hub == nil {
		deps.Hub = realtime.NewHub(deps.FrontendOrigin, deps.APIOrigin)
	}
	mux.Handle("GET /api/ws", deps.Hub)
	store := charger.NewStore(deps.Pool)
	if deps.ChargerService == nil {
		deps.ChargerService = charger.NewService(store, deps.Now, nil)
	}
	handler := charger.NewHandler(store, deps.ChargerService)
	mux.HandleFunc("GET /api/chargers", handler.List)
	authHandler := auth.NewHandler(auth.NewService(deps.Pool, deps.Now), deps.CookieSecure)
	mux.Handle("POST /api/chargers/{id}/reserve", authHandler.RequireUser(http.HandlerFunc(handler.Reserve)))
	mux.Handle("GET /api/reservations", authHandler.RequireUser(http.HandlerFunc(handler.ListReservations)))
	mux.HandleFunc("POST /api/auth/login", authHandler.Login)
	mux.HandleFunc("POST /api/auth/logout", authHandler.Logout)
	mux.Handle("GET /api/auth/me", authHandler.RequireUser(http.HandlerFunc(authHandler.Me)))
	return requestLog(originPolicy(mux, deps.FrontendOrigin, deps.APIOrigin), deps.Logger)
}
