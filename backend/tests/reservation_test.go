package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"ev-charger-assessment/backend/internal/server"
	"ev-charger-assessment/backend/internal/testutil"
	"github.com/jackc/pgx/v5/pgxpool"
)

const origin = "http://localhost:3000"

type fixture struct {
	pool   *pgxpool.Pool
	router http.Handler
	cookie *http.Cookie
	userID string
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	pool, _ := testutil.NewDatabase(t, true)
	router := server.NewRouter(server.Dependencies{Pool: pool})
	rec := request(router, "POST", "/api/auth/login", `{"username":"demo","password":"DemoPass123!"}`, nil)
	if rec.Code != 200 {
		t.Fatalf("login status=%d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return fixture{pool: pool, router: router, cookie: rec.Result().Cookies()[0], userID: body.User.ID}
}

func request(router http.Handler, method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Origin", origin)
	if method == "POST" {
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func booking(userID string, start, end time.Time) string {
	return fmt.Sprintf(`{"user_id":%q,"start_time":%q,"end_time":%q}`, userID, start.Format(time.RFC3339Nano), end.Format(time.RFC3339Nano))
}

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body.Error.Code
}

func TestConcurrentReservations(t *testing.T) {
	f := newFixture(t)
	start := time.Now().UTC().Add(2 * time.Hour)
	body := booking(f.userID, start, start.Add(time.Hour))
	startGate := make(chan struct{})
	results := make(chan *httptest.ResponseRecorder, 10)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-startGate
			results <- request(f.router, "POST", "/api/chargers/charger-1/reserve", body, f.cookie)
		}()
	}
	close(startGate)
	wg.Wait()
	close(results)
	created, conflict := 0, 0
	for rec := range results {
		switch rec.Code {
		case 201:
			created++
		case 409:
			if code := errorCode(t, rec); code != "RESERVATION_CONFLICT" {
				t.Fatalf("code=%s", code)
			}
			conflict++
		default:
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
	}
	if created != 1 || conflict != 9 {
		t.Fatalf("created=%d conflicts=%d", created, conflict)
	}
	var count int
	if err := f.pool.QueryRow(context.Background(), "SELECT count(*) FROM reservations WHERE charger_id='charger-1'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("rows=%d", count)
	}
}

func TestReservationListOrderingAndPagination(t *testing.T) {
	f := newFixture(t)
	now := time.Now().UTC().Truncate(time.Second)
	for _, item := range []struct {
		status string
		start  time.Time
	}{
		{"COMPLETED", now.Add(-72 * time.Hour)},
		{"ACTIVE", now.Add(-48 * time.Hour)},
		{"CANCELLED", now.Add(24 * time.Hour)},
		{"SCHEDULED", now.Add(48 * time.Hour)},
	} {
		_, err := f.pool.Exec(context.Background(), `INSERT INTO reservations(user_id,charger_id,start_time,end_time,status)
			VALUES($1,'charger-1',$2,$3,$4)`, f.userID, item.start, item.start.Add(time.Hour), item.status)
		if err != nil {
			t.Fatal(err)
		}
	}
	var otherID string
	if err := f.pool.QueryRow(context.Background(), "SELECT id FROM users WHERE username='demo2'").Scan(&otherID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(context.Background(), `INSERT INTO reservations(user_id,charger_id,start_time,end_time)
		VALUES($1,'charger-2',$2,$3)`, otherID, now.Add(72*time.Hour), now.Add(73*time.Hour)); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		path string
		page int
		want []string
	}{
		{"/api/reservations?page=1&limit=2", 1, []string{"ACTIVE", "SCHEDULED"}},
		{"/api/reservations?page=2&limit=2", 2, []string{"CANCELLED", "COMPLETED"}},
		{"/api/reservations?page=3&limit=2", 3, []string{}},
	} {
		rec := request(f.router, "GET", tc.path, "", f.cookie)
		if rec.Code != 200 {
			t.Fatalf("%s: %d %s", tc.path, rec.Code, rec.Body.String())
		}
		var body struct {
			Reservations []struct {
				Status string `json:"status"`
			} `json:"reservations"`
			Page, Limit, Total int
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Total != 4 || body.Limit != 2 || body.Page != tc.page {
			t.Fatalf("metadata: %s", rec.Body.String())
		}
		if len(body.Reservations) != len(tc.want) {
			t.Fatalf("%s: %s", tc.path, rec.Body.String())
		}
		for i, want := range tc.want {
			if body.Reservations[i].Status != want {
				t.Fatalf("%s: %s", tc.path, rec.Body.String())
			}
		}
	}
	for _, path := range []string{"/api/reservations?page=0", "/api/reservations?limit=0", "/api/reservations?limit=101", "/api/reservations?page=nope"} {
		rec := request(f.router, "GET", path, "", f.cookie)
		if rec.Code != 400 || errorCode(t, rec) != "VALIDATION_ERROR" {
			t.Fatalf("%s: %d %s", path, rec.Code, rec.Body.String())
		}
	}
}

func TestReservationGroupsHaveIndependentPages(t *testing.T) {
	f := newFixture(t)
	now := time.Now().UTC().Truncate(time.Second)
	for _, item := range []struct {
		status string
		start  time.Time
	}{
		{"EXPIRED", now.Add(-72 * time.Hour)},
		{"COMPLETED", now.Add(-48 * time.Hour)},
		{"ACTIVE", now.Add(-24 * time.Hour)},
		{"WAITING", now.Add(-12 * time.Hour)},
		{"SCHEDULED", now.Add(48 * time.Hour)},
		{"CANCELLED", now.Add(96 * time.Hour)},
	} {
		if _, err := f.pool.Exec(context.Background(), `INSERT INTO reservations(user_id,charger_id,start_time,end_time,status)
			VALUES($1,'charger-1',$2,$3,$4)`, f.userID, item.start, item.start.Add(time.Hour), item.status); err != nil {
			t.Fatal(err)
		}
	}
	defaultPage := request(f.router, "GET", "/api/reservations", "", f.cookie)
	var defaultBody struct {
		Reservations []json.RawMessage `json:"reservations"`
		Limit        int               `json:"limit"`
		Total        int               `json:"total"`
	}
	if err := json.Unmarshal(defaultPage.Body.Bytes(), &defaultBody); err != nil { t.Fatal(err) }
	if defaultPage.Code != 200 || defaultBody.Limit != 5 || defaultBody.Total != 6 || len(defaultBody.Reservations) != 5 {
		t.Fatalf("default page: %d %s", defaultPage.Code, defaultPage.Body.String())
	}
	for _, tc := range []struct {
		path string
		want []string
	}{
		{"/api/reservations?group=upcoming&page=1&limit=2", []string{"ACTIVE", "WAITING"}},
		{"/api/reservations?group=upcoming&page=2&limit=2", []string{"SCHEDULED"}},
		{"/api/reservations?group=history&page=1&limit=2", []string{"CANCELLED", "COMPLETED"}},
		{"/api/reservations?group=history&page=2&limit=2", []string{"EXPIRED"}},
	} {
		rec := request(f.router, "GET", tc.path, "", f.cookie)
		if rec.Code != 200 {
			t.Fatalf("%s: %d %s", tc.path, rec.Code, rec.Body.String())
		}
		var body struct {
			Reservations []struct {
				Status string `json:"status"`
			} `json:"reservations"`
			Total int `json:"total"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Total != 3 || len(body.Reservations) != len(tc.want) {
			t.Fatalf("%s: %s", tc.path, rec.Body.String())
		}
		for i, want := range tc.want {
			if body.Reservations[i].Status != want {
				t.Fatalf("%s: %s", tc.path, rec.Body.String())
			}
		}
	}
	rec := request(f.router, "GET", "/api/reservations?group=unknown", "", f.cookie)
	if rec.Code != 400 || errorCode(t, rec) != "VALIDATION_ERROR" {
		t.Fatalf("unknown group: %d %s", rec.Code, rec.Body.String())
	}
}

func TestReservationRules(t *testing.T) {
	f := newFixture(t)
	start := time.Now().UTC().Add(3 * time.Hour).Truncate(time.Second)
	reserve := func(chargerID, body string) *httptest.ResponseRecorder {
		return request(f.router, "POST", "/api/chargers/"+chargerID+"/reserve", body, f.cookie)
	}
	check := func(rec *httptest.ResponseRecorder, status int, code string) {
		t.Helper()
		if rec.Code != status {
			t.Fatalf("status=%d body=%s; want %d %s", rec.Code, rec.Body.String(), status, code)
		}
		if code != "" && errorCode(t, rec) != code {
			t.Fatalf("status=%d body=%s; want %d %s", rec.Code, rec.Body.String(), status, code)
		}
	}
	check(reserve("charger-1", booking("bad-uuid", start, start.Add(time.Hour))), 400, "VALIDATION_ERROR")
	check(reserve("charger-1", fmt.Sprintf(`{"user_id":%q,"start_time":"not-a-time","end_time":%q}`, f.userID, start.Add(time.Hour).Format(time.RFC3339))), 400, "VALIDATION_ERROR")
	check(reserve("charger-1", booking("22222222-2222-4222-8222-222222222222", start, start.Add(time.Hour))), 403, "USER_ID_MISMATCH")
	check(reserve("charger-1", booking(f.userID, start, start)), 400, "VALIDATION_ERROR")
	check(reserve("charger-1", booking(f.userID, time.Now().Add(-time.Minute), start)), 400, "VALIDATION_ERROR")
	check(reserve("missing", booking(f.userID, start, start.Add(time.Hour))), 404, "CHARGER_NOT_FOUND")
	check(reserve("charger-3", booking(f.userID, start, start.Add(time.Hour))), 409, "CHARGER_IN_MAINTENANCE")
	check(reserve("charger-1", booking(f.userID, start, start.Add(time.Hour))), 201, "")
	shifted := fmt.Sprintf(`{"user_id":%q,"start_time":%q,"end_time":%q}`, f.userID, start.In(time.FixedZone("plus8", 8*3600)).Format(time.RFC3339), start.Add(time.Hour).In(time.FixedZone("plus8", 8*3600)).Format(time.RFC3339))
	check(reserve("charger-1", shifted), 409, "RESERVATION_CONFLICT")
	if rec := reserve("charger-1", booking(f.userID, start.Add(time.Hour), start.Add(2*time.Hour))); rec.Code != 201 {
		t.Fatalf("adjacent: %d %s", rec.Code, rec.Body.String())
	}
	check(reserve("charger-1", booking(f.userID, start.Add(30*time.Minute), start.Add(90*time.Minute))), 409, "RESERVATION_CONFLICT")
	var otherID string
	if err := f.pool.QueryRow(context.Background(), "SELECT id FROM users WHERE username='demo2'").Scan(&otherID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(context.Background(), "INSERT INTO reservations(user_id,charger_id,start_time,end_time) VALUES($1,'charger-2',$2,$3)", otherID, start, start.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	list := request(f.router, "GET", "/api/reservations", "", f.cookie)
	if list.Code != 200 || list.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("list: %d %s", list.Code, list.Body.String())
	}
	var listed struct {
		Reservations []struct {
			UserID    string    `json:"user_id"`
			ChargerID string    `json:"charger_id"`
			StartTime time.Time `json:"start_time"`
		} `json:"reservations"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Reservations) != 2 || listed.Reservations[0].StartTime.Before(listed.Reservations[1].StartTime) {
		t.Fatalf("list=%s", list.Body.String())
	}
	for _, r := range listed.Reservations {
		if r.UserID != f.userID || r.ChargerID != "charger-1" {
			t.Fatalf("leaked reservation: %+v", r)
		}
	}
	unauth := request(f.router, "GET", "/api/reservations", "", nil)
	check(unauth, 401, "UNAUTHENTICATED")
	check(request(f.router, "POST", "/api/chargers/charger-1/reserve", booking(f.userID, start, start.Add(time.Hour)), nil), 401, "UNAUTHENTICATED")
	if _, err := f.pool.Exec(context.Background(), "INSERT INTO charging_sessions(charger_id,started_at,planned_end_at) VALUES ('charger-2',$1,$2)", start, start.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	check(reserve("charger-2", booking(f.userID, start.Add(10*time.Minute), start.Add(20*time.Minute))), 409, "CHARGER_OCCUPIED")
	if _, err := f.pool.Exec(context.Background(), "UPDATE charging_sessions SET started_at=$1,planned_end_at=$2 WHERE charger_id='charger-2'", time.Now().Add(-2*time.Hour), time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	check(reserve("charger-2", booking(f.userID, start.Add(2*time.Hour), start.Add(3*time.Hour))), 409, "CHARGER_OCCUPIED")
}

func TestFutureReservationDuringCharging(t *testing.T) {
	f := newFixture(t)
	now := time.Now().UTC()
	if _, err := f.pool.Exec(context.Background(), "UPDATE chargers SET status='CHARGING' WHERE id='charger-2'"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(context.Background(), "INSERT INTO charging_sessions(charger_id,started_at,planned_end_at) VALUES ('charger-2',$1,$2)", now.Add(-time.Minute), now.Add(30*time.Minute)); err != nil {
		t.Fatal(err)
	}
	near := request(f.router, "POST", "/api/chargers/charger-2/reserve", booking(f.userID, now.Add(10*time.Minute), now.Add(20*time.Minute)), f.cookie)
	if near.Code != 409 || errorCode(t, near) != "CHARGER_OCCUPIED" {
		t.Fatalf("near booking: %d %s", near.Code, near.Body.String())
	}
	future := request(f.router, "POST", "/api/chargers/charger-2/reserve", booking(f.userID, now.Add(3*time.Hour), now.Add(4*time.Hour)), f.cookie)
	if future.Code != 201 {
		t.Fatalf("future booking: %d %s", future.Code, future.Body.String())
	}
}
