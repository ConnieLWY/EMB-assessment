package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"ev-charger-assessment/backend/internal/auth"
	"ev-charger-assessment/backend/internal/httpapi"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type concurrencyDemoInput struct {
	ChargerID string `json:"charger_id"`
}

type concurrencyDemoAttempt struct {
	Number      int                         `json:"number"`
	User        string                      `json:"user"`
	Status      int                         `json:"status"`
	Code        string                      `json:"code,omitempty"`
	RequestedAt time.Time                   `json:"requested_at"`
	RespondedAt time.Time                   `json:"responded_at"`
	DurationMS  float64                     `json:"duration_ms"`
	Request     concurrencyDemoHTTPRequest  `json:"request"`
	Response    concurrencyDemoHTTPResponse `json:"response"`
}

type concurrencyDemoHTTPRequest struct {
	Method  string          `json:"method"`
	URL     string          `json:"url"`
	Headers http.Header     `json:"headers"`
	Body    json.RawMessage `json:"body"`
}

type concurrencyDemoHTTPResponse struct {
	Status  int             `json:"status"`
	Headers http.Header     `json:"headers"`
	Body    json.RawMessage `json:"body"`
}

type concurrencyDemoResult struct {
	ChargerID string                   `json:"charger_id"`
	StartTime time.Time                `json:"start_time"`
	EndTime   time.Time                `json:"end_time"`
	Results   []concurrencyDemoAttempt `json:"results"`
	Created   int                      `json:"created"`
	Conflicts int                      `json:"conflicts"`
	Persisted int                      `json:"persisted"`
	Passed    bool                     `json:"passed"`
}

func concurrencyDemoHandler(pool *pgxpool.Pool, authService *auth.Service, apiOrigin string) http.Handler {
	var running sync.Mutex
	client := &http.Client{Timeout: 8 * time.Second}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, _ := auth.UserFromContext(r.Context())
		if user.Username != "demo" && user.Username != "demo2" {
			httpapi.Error(w, http.StatusForbidden, "DEMO_ONLY", "Only demo accounts can run this check.")
			return
		}
		if !running.TryLock() {
			httpapi.Error(w, http.StatusConflict, "DEMO_BUSY", "A concurrency check is already running.")
			return
		}
		defer running.Unlock()

		var input concurrencyDemoInput
		if !httpapi.Decode(w, r, &input) {
			return
		}
		if input.ChargerID == "" {
			httpapi.Error(w, http.StatusBadRequest, "VALIDATION_ERROR", "Choose a charger.")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		var status string
		var start time.Time
		err := pool.QueryRow(ctx, `SELECT c.status, GREATEST($2::timestamptz,
			COALESCE(MAX(res.end_time) + interval '1 hour', $2::timestamptz))
			FROM chargers c LEFT JOIN reservations res ON res.charger_id=c.id AND res.status <> 'CANCELLED'
			WHERE c.id=$1 GROUP BY c.id,c.status`, input.ChargerID, time.Now().UTC().Add(7*24*time.Hour)).Scan(&status, &start)
		if errors.Is(err, pgx.ErrNoRows) {
			httpapi.Error(w, http.StatusNotFound, "CHARGER_NOT_FOUND", "Charger not found.")
			return
		}
		if err != nil {
			slog.Error("concurrency demo setup failed", "error", err)
			httpapi.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not prepare the check.")
			return
		}
		if status == "MAINTENANCE" {
			httpapi.Error(w, http.StatusConflict, "CHARGER_IN_MAINTENANCE", "Choose a charger that is not in maintenance.")
			return
		}

		rows, err := pool.Query(ctx, "SELECT username,id::text FROM users WHERE username IN ('demo','demo2')")
		if err != nil {
			httpapi.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not load demo users.")
			return
		}
		users := map[string]string{}
		for rows.Next() {
			var name, id string
			if err := rows.Scan(&name, &id); err != nil {
				rows.Close()
				httpapi.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not load demo users.")
				return
			}
			users[name] = id
		}
		err = rows.Err()
		rows.Close()
		if err != nil || len(users) != 2 {
			httpapi.Error(w, http.StatusInternalServerError, "DEMO_USERS_UNAVAILABLE", "Both demo users are required.")
			return
		}
		listener, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr)
		if !ok {
			httpapi.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not locate the local API listener.")
			return
		}
		_, port, err := net.SplitHostPort(listener.String())
		if err != nil {
			httpapi.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not locate the local API listener.")
			return
		}
		target := "http://127.0.0.1:" + port + "/api/chargers/" + url.PathEscape(input.ChargerID) + "/reserve"
		callerCookie, err := r.Cookie("ev_session")
		if err != nil {
			httpapi.Error(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Sign in again to run the check.")
			return
		}
		otherName := "demo"
		if user.Username == "demo" {
			otherName = "demo2"
		}
		_, otherToken, err := authService.Login(ctx, otherName, "DemoPass123!", "")
		if err != nil {
			slog.Error("concurrency demo second login failed", "error", err)
			httpapi.Error(w, http.StatusInternalServerError, "DEMO_LOGIN_FAILED", "Could not sign in the second demo user.")
			return
		}
		defer func() {
			logoutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := authService.Logout(logoutCtx, otherToken); err != nil {
				slog.Error("concurrency demo logout failed", "error", err)
			}
		}()

		start = start.UTC().Truncate(time.Second)
		end := start.Add(time.Hour)
		payloads := map[string][]byte{}
		for _, name := range []string{"demo", "demo2"} {
			payload, err := json.Marshal(struct {
				UserID    string    `json:"user_id"`
				StartTime time.Time `json:"start_time"`
				EndTime   time.Time `json:"end_time"`
			}{UserID: users[name], StartTime: start, EndTime: end})
			if err != nil {
				httpapi.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not prepare reservation requests.")
				return
			}
			payloads[name] = payload
		}
		result := concurrencyDemoResult{
			ChargerID: input.ChargerID,
			StartTime: start,
			EndTime:   end,
			Results:   make([]concurrencyDemoAttempt, 10),
		}
		gate := make(chan struct{})
		var group sync.WaitGroup
		for i := range result.Results {
			group.Add(1)
			go func(i int) {
				defer group.Done()
				name := "demo"
				if i%2 == 1 {
					name = "demo2"
				}
				token := otherToken
				if name == user.Username {
					token = callerCookie.Value
				}
				<-gate
				result.Results[i] = demoReserveRequest(ctx, client, target, apiOrigin, token, payloads[name], i+1, name)
			}(i)
		}
		close(gate)
		group.Wait()
		for _, attempt := range result.Results {
			if attempt.Status == http.StatusCreated {
				result.Created++
			} else if attempt.Code == "RESERVATION_CONFLICT" {
				result.Conflicts++
			}
		}
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM reservations WHERE charger_id=$1
			AND start_time=$2 AND end_time=$3 AND status <> 'CANCELLED'`, input.ChargerID, start, end).Scan(&result.Persisted); err != nil {
			httpapi.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not verify the saved reservation.")
			return
		}
		result.Passed = result.Created == 1 && result.Conflicts == 9 && result.Persisted == 1
		httpapi.JSON(w, http.StatusOK, result)
	})
}

func demoReserveRequest(ctx context.Context, client *http.Client, target, origin, token string, payload []byte, number int, user string) concurrencyDemoAttempt {
	attempt := concurrencyDemoAttempt{Number: number, User: user}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(payload))
	if err != nil {
		attempt.Code = "REQUEST_FAILED"
		return attempt
	}
	req.Header.Set("Origin", origin)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "ev_session", Value: token})
	headers := req.Header.Clone()
	headers.Set("Cookie", "ev_session=[redacted]")
	attempt.Request = concurrencyDemoHTTPRequest{
		Method: req.Method, URL: req.URL.String(), Headers: headers, Body: json.RawMessage(payload),
	}
	attempt.RequestedAt = time.Now().UTC()
	resp, err := client.Do(req)
	if err != nil {
		attempt.RespondedAt = time.Now().UTC()
		attempt.DurationMS = float64(attempt.RespondedAt.Sub(attempt.RequestedAt)) / float64(time.Millisecond)
		slog.Error("concurrency demo HTTP request failed", "error", err)
		attempt.Code = "REQUEST_FAILED"
		return attempt
	}
	defer resp.Body.Close()
	attempt.Status = resp.StatusCode
	responseBody, readErr := io.ReadAll(resp.Body)
	attempt.RespondedAt = time.Now().UTC()
	attempt.DurationMS = float64(attempt.RespondedAt.Sub(attempt.RequestedAt)) / float64(time.Millisecond)
	responseHeaders := resp.Header.Clone()
	responseHeaders.Del("Set-Cookie")
	attempt.Response = concurrencyDemoHTTPResponse{Status: resp.StatusCode, Headers: responseHeaders}
	if json.Valid(responseBody) {
		attempt.Response.Body = json.RawMessage(responseBody)
	} else {
		attempt.Response.Body, _ = json.Marshal(string(responseBody))
	}
	if readErr != nil {
		attempt.Code = "RESPONSE_READ_FAILED"
		return attempt
	}
	if resp.StatusCode != http.StatusCreated {
		var body struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal(responseBody, &body); err == nil {
			attempt.Code = body.Error.Code
		}
		if attempt.Code == "" {
			attempt.Code = "HTTP_ERROR"
		}
	}
	return attempt
}
