package charger_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ev-charger-assessment/backend/internal/server"
	"ev-charger-assessment/backend/internal/testutil"
)

func TestListChargers(t *testing.T) {
	pool, _ := testutil.NewDatabase(t, true)
	router := server.NewRouter(server.Dependencies{Pool: pool})
	t.Run("seed chargers", func(t *testing.T) {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/chargers", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		if rec.Header().Get("Content-Type") != "application/json" {
			t.Fatalf("content type=%q", rec.Header().Get("Content-Type"))
		}
		var body struct {
			Chargers []struct {
				ID, Name, Location, Status string
				UpdatedAt                  string `json:"updated_at"`
			} `json:"chargers"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if len(body.Chargers) != 3 {
			t.Fatalf("chargers=%+v", body.Chargers)
		}
		first := body.Chargers[0]
		if first.ID != "charger-1" || first.Name != "Charger 1" || first.Location != "Level 1, Bay A" || first.Status != "AVAILABLE" {
			t.Fatalf("first charger=%+v", first)
		}
		if !strings.HasSuffix(first.UpdatedAt, "Z") {
			t.Fatalf("timestamp not UTC: %s", first.UpdatedAt)
		}
		if _, err := time.Parse(time.RFC3339Nano, first.UpdatedAt); err != nil {
			t.Fatal(err)
		}
		if body.Chargers[1].ID != "charger-2" || body.Chargers[2].ID != "charger-3" || body.Chargers[2].Status != "MAINTENANCE" {
			t.Fatalf("unexpected list: %+v", body.Chargers)
		}
	})
	t.Run("empty collection", func(t *testing.T) {
		if _, err := pool.Exec(context.Background(), "DELETE FROM chargers"); err != nil {
			t.Fatal(err)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/chargers", nil))
		if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != `{"chargers":[]}` {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
	})
	t.Run("database failure", func(t *testing.T) {
		pool.Close()
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/chargers", nil))
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status=%d", rec.Code)
		}
		var body struct {
			Error struct{ Code, Message string }
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Error.Code != "INTERNAL_ERROR" || body.Error.Message == "" || strings.Contains(rec.Body.String(), "pool") {
			t.Fatalf("error body=%s", rec.Body.String())
		}
	})
}
