package server

import (
	"ev-charger-assessment/backend/internal/apidocs"
	"ev-charger-assessment/backend/internal/auth"
	"ev-charger-assessment/backend/internal/charger"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"time"
)

type Dependencies struct {
	Pool           *pgxpool.Pool
	Now            func() time.Time
	FrontendOrigin string
	APIOrigin      string
	CookieSecure   bool
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
	handler := charger.NewHandler(charger.NewStore(deps.Pool))
	mux.HandleFunc("GET /api/chargers", handler.List)
	authHandler := auth.NewHandler(auth.NewService(deps.Pool, deps.Now), deps.CookieSecure)
	mux.HandleFunc("POST /api/auth/login", authHandler.Login)
	mux.HandleFunc("POST /api/auth/logout", authHandler.Logout)
	mux.Handle("GET /api/auth/me", authHandler.RequireUser(http.HandlerFunc(authHandler.Me)))
	return originPolicy(mux, deps.FrontendOrigin, deps.APIOrigin)
}
