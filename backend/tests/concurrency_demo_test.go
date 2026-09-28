package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ev-charger-assessment/backend/internal/server"
)

func TestConcurrencyDemoShowsOneWinnerAcrossTwoUsers(t *testing.T) {
	f := newFixture(t)
	router := server.NewRouter(server.Dependencies{Pool: f.pool, ConcurrencyDemoEnabled: true})
	var reserveRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/chargers/charger-1/reserve" {
			reserveRequests.Add(1)
		}
		router.ServeHTTP(w, r)
	}))
	defer server.Close()
	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/demo/concurrency", strings.NewReader(`{"charger_id":"charger-1"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Origin", origin)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(f.cookie)
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.StatusCode, payload)
	}
	var body struct {
		ChargerID string `json:"charger_id"`
		Created   int    `json:"created"`
		Conflicts int    `json:"conflicts"`
		Persisted int    `json:"persisted"`
		Passed    bool   `json:"passed"`
		Results   []struct {
			User        string    `json:"user"`
			Status      int       `json:"status"`
			Code        string    `json:"code"`
			RequestedAt time.Time `json:"requested_at"`
			RespondedAt time.Time `json:"responded_at"`
			DurationMS  float64   `json:"duration_ms"`
			Request     struct {
				Method  string              `json:"method"`
				URL     string              `json:"url"`
				Headers map[string][]string `json:"headers"`
				Body    struct {
					UserID string `json:"user_id"`
				} `json:"body"`
			} `json:"request"`
			Response struct {
				Status  int                 `json:"status"`
				Headers map[string][]string `json:"headers"`
				Body    json.RawMessage     `json:"body"`
			} `json:"response"`
		} `json:"results"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		t.Fatal(err)
	}
	if body.ChargerID != "charger-1" || body.Created != 1 || body.Conflicts != 9 || body.Persisted != 1 || !body.Passed || len(body.Results) != 10 {
		t.Fatalf("result=%s", payload)
	}
	if got := reserveRequests.Load(); got != 10 {
		t.Fatalf("reserve HTTP requests=%d, want 10", got)
	}
	users := map[string]int{}
	for _, result := range body.Results {
		users[result.User]++
		if result.Status != 201 && (result.Status != 409 || result.Code != "RESERVATION_CONFLICT") {
			t.Fatalf("unexpected attempt=%+v", result)
		}
		if result.RequestedAt.IsZero() || result.RespondedAt.Before(result.RequestedAt) || result.DurationMS < 0 {
			t.Fatalf("missing request timing: %+v", result)
		}
		if result.Request.Method != http.MethodPost || !strings.HasSuffix(result.Request.URL, "/api/chargers/charger-1/reserve") || len(result.Request.Headers["Cookie"]) != 1 || result.Request.Headers["Cookie"][0] != "ev_session=[redacted]" || result.Request.Body.UserID == "" {
			t.Fatalf("missing request details: %+v", result.Request)
		}
		if result.Response.Status != result.Status || len(result.Response.Body) == 0 || len(result.Response.Headers) == 0 {
			t.Fatalf("missing response details: %+v", result.Response)
		}
	}
	if bytes.Contains(payload, []byte(f.cookie.Value)) {
		t.Fatal("session cookie leaked in result")
	}
	if users["demo"] != 5 || users["demo2"] != 5 {
		t.Fatalf("users=%v", users)
	}
	var count int
	if err := f.pool.QueryRow(context.Background(), "SELECT count(*) FROM reservations WHERE charger_id='charger-1'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("rows=%d", count)
	}
}

func TestConcurrencyDemoIsOffByDefault(t *testing.T) {
	f := newFixture(t)
	rec := request(f.router, http.MethodPost, "/api/demo/concurrency", `{"charger_id":"charger-1"}`, f.cookie)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}
