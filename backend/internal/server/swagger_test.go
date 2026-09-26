package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ev-charger-assessment/backend/internal/server"
)

func TestSwaggerRoutes(t *testing.T) {
	router := server.NewRouter(server.Dependencies{})
	for _, tc := range []struct{ path, contentType string }{
		{"/swagger/", "text/html"},
		{"/swagger/swagger-ui.css", "text/css"},
		{"/swagger/swagger-ui-bundle.js", "javascript"},
		{"/swagger/openapi.json", "application/json"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Header().Get("Content-Type"), tc.contentType) {
				t.Fatalf("content type=%q", rec.Header().Get("Content-Type"))
			}
			if rec.Body.Len() == 0 {
				t.Fatal("empty documentation asset")
			}
		})
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/swagger", nil))
	if (rec.Code != http.StatusMovedPermanently && rec.Code != http.StatusTemporaryRedirect) || rec.Header().Get("Location") != "/swagger/" {
		t.Fatalf("redirect=%d location=%q", rec.Code, rec.Header().Get("Location"))
	}
}

func TestOpenAPIContract(t *testing.T) {
	rec := httptest.NewRecorder()
	server.NewRouter(server.Dependencies{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/swagger/openapi.json", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	var doc struct {
		OpenAPI string `json:"openapi"`
		Servers []struct {
			URL string `json:"url"`
		} `json:"servers"`
		Paths map[string]map[string]struct {
			Responses map[string]json.RawMessage `json:"responses"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.OpenAPI != "3.0.3" {
		t.Fatalf("OpenAPI version=%q", doc.OpenAPI)
	}
	if len(doc.Servers) != 1 || doc.Servers[0].URL != "/" {
		t.Fatalf("servers=%+v", doc.Servers)
	}
	operation, ok := doc.Paths["/api/chargers"]["get"]
	if !ok || len(doc.Paths) != 1 {
		t.Fatalf("documented paths=%v", doc.Paths)
	}
	for _, status := range []string{"200", "500"} {
		if _, ok := operation.Responses[status]; !ok {
			t.Errorf("missing response %s", status)
		}
	}
}
