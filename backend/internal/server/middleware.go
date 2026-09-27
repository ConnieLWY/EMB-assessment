package server

import (
	"net/http"
	"strings"

	"ev-charger-assessment/backend/internal/httpapi"
)

func originPolicy(next http.Handler, frontendOrigin, apiOrigin string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/auth/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		w.Header().Add("Vary", "Origin")
		origin := r.Header.Get("Origin")
		allowed := origin != "" && (origin == frontendOrigin || origin == apiOrigin)
		mutation := r.Method != "GET" && r.Method != "HEAD" && r.Method != "OPTIONS"
		if (origin != "" && !allowed) || (mutation && !allowed) {
			httpapi.Error(w, 403, "ORIGIN_NOT_ALLOWED", "Request origin is not allowed.")
			return
		}
		if allowed {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}
		if r.Method == "OPTIONS" {
			w.Header().Add("Vary", "Access-Control-Request-Method")
			w.Header().Add("Vary", "Access-Control-Request-Headers")
			method := r.Header.Get("Access-Control-Request-Method")
			valid := allowed && (method == "GET" || method == "POST")
			for _, header := range strings.Split(r.Header.Get("Access-Control-Request-Headers"), ",") {
				if header = strings.TrimSpace(header); header != "" && !strings.EqualFold(header, "Content-Type") {
					valid = false
				}
			}
			if !valid {
				httpapi.Error(w, 403, "ORIGIN_NOT_ALLOWED", "Preflight request is not allowed.")
				return
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.WriteHeader(204)
			return
		}
		next.ServeHTTP(w, r)
	})
}
