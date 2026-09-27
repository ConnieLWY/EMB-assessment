package server_test

import (
	"net/http/httptest"
	"strings"
	"testing"

	"ev-charger-assessment/backend/internal/server"
)

func TestOriginPolicy(t *testing.T) {
	h := server.NewRouter(server.Dependencies{})
	for _, tc := range []struct {
		origin string
		want   int
	}{
		{"http://localhost:3000", 204}, {"http://localhost:8080", 204},
		{"", 403}, {"null", 403}, {"http://localhost:3000.evil.example", 403}, {"http://127.0.0.1:3000", 403},
	} {
		req := httptest.NewRequest("POST", "/api/auth/logout", nil)
		req.Header.Set("Origin", tc.origin)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Errorf("origin=%q status=%d body=%s", tc.origin, rec.Code, rec.Body.String())
		}
		if rec.Header().Get("Cache-Control") != "no-store" {
			t.Error("origin rejection must not cache auth responses")
		}
		if tc.want == 204 && (rec.Header().Get("Access-Control-Allow-Origin") != tc.origin || rec.Header().Get("Access-Control-Allow-Credentials") != "true") {
			t.Errorf("missing credentialed CORS headers: %v", rec.Header())
		}
		if tc.want == 403 && !strings.Contains(rec.Body.String(), "ORIGIN_NOT_ALLOWED") {
			t.Errorf("wrong origin error: %s", rec.Body.String())
		}
	}
	req := httptest.NewRequest("OPTIONS", "/api/auth/login", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "content-type")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 204 || rec.Header().Get("Access-Control-Allow-Origin") != "http://localhost:3000" || !strings.Contains(rec.Header().Get("Access-Control-Allow-Headers"), "Content-Type") {
		t.Fatalf("preflight=%d headers=%v", rec.Code, rec.Header())
	}
	for _, method := range []string{"DELETE", "TRACE"} {
		req.Header.Set("Access-Control-Request-Method", method)
		rec = httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != 403 {
			t.Errorf("unsupported method preflight=%d", rec.Code)
		}
	}
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "x-untrusted")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 403 {
		t.Errorf("unsupported header preflight=%d", rec.Code)
	}
}
