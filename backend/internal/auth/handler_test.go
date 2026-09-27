package auth_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ev-charger-assessment/backend/internal/server"
	"ev-charger-assessment/backend/internal/testutil"
)

const credentials = `{"username":"demo","password":"DemoPass123!"}`

func call(h http.Handler, method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Origin", "http://localhost:8080")
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "192.0.2.1:12345"
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func sessionCookie(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	if rec.Code != 200 {
		t.Fatalf("login status=%d body=%s", rec.Code, rec.Body.String())
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == "ev_session" {
			return c
		}
	}
	t.Fatal("missing session cookie")
	return nil
}

func assertError(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	var body struct {
		Error struct{ Code, Message string }
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("non-JSON error: %s", rec.Body.String())
	}
	if rec.Code != status || body.Error.Code != code || body.Error.Message == "" {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAuthenticationLifecycle(t *testing.T) {
	pool, _ := testutil.NewDatabase(t, true)
	now := time.Now().UTC().Truncate(time.Second)
	h := server.NewRouter(server.Dependencies{Pool: pool, Now: func() time.Time { return now }})
	assertError(t, call(h, "GET", "/api/auth/me", "", nil), 401, "UNAUTHENTICATED")
	login := call(h, "POST", "/api/auth/login", credentials, nil)
	first := sessionCookie(t, login)
	if !first.HttpOnly || first.SameSite != http.SameSiteLaxMode || first.Path != "/" || first.Domain != "" || first.Secure || first.MaxAge != 86400 || !first.Expires.Equal(now.Add(24*time.Hour)) {
		t.Fatalf("incorrect cookie attributes: %+v", first)
	}
	var body struct{ User struct{ ID, Username string } }
	if err := json.Unmarshal(login.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.User.ID != "11111111-1111-4111-8111-111111111111" || body.User.Username != "demo" {
		t.Fatalf("user=%+v", body.User)
	}
	if strings.Contains(login.Body.String(), "password") || strings.Contains(login.Body.String(), first.Value) {
		t.Fatal("login response leaks credentials")
	}
	digest := sha256.Sum256([]byte(first.Value))
	var hash string
	var expiry time.Time
	if err := pool.QueryRow(context.Background(), "SELECT token_hash, expires_at FROM auth_sessions").Scan(&hash, &expiry); err != nil {
		t.Fatal(err)
	}
	if hash != hex.EncodeToString(digest[:]) || hash == first.Value || !expiry.Equal(first.Expires) {
		t.Fatal("session must store a hash with matching expiry")
	}
	me := call(h, "GET", "/api/auth/me", "", first)
	if me.Code != 200 || me.Body.String() != login.Body.String() {
		t.Fatalf("me=%d %s", me.Code, me.Body.String())
	}
	second := sessionCookie(t, call(h, "POST", "/api/auth/login", credentials, first))
	if second.Value == first.Value {
		t.Fatal("login reused the previous token")
	}
	assertError(t, call(h, "GET", "/api/auth/me", "", first), 401, "UNAUTHENTICATED")
	logout := call(h, "POST", "/api/auth/logout", "", second)
	if logout.Code != 204 || logout.Body.Len() != 0 {
		t.Fatalf("logout=%d %s", logout.Code, logout.Body.String())
	}
	cleared := logout.Result().Cookies()
	if len(cleared) != 1 || cleared[0].Name != "ev_session" || cleared[0].MaxAge != -1 || cleared[0].Value != "" || cleared[0].Path != "/" || !cleared[0].HttpOnly {
		t.Fatalf("cookie not cleared: %+v", cleared)
	}
	assertError(t, call(h, "GET", "/api/auth/me", "", second), 401, "UNAUTHENTICATED")
	if rec := call(h, "POST", "/api/auth/logout", "", second); rec.Code != 204 {
		t.Fatalf("repeated logout=%d", rec.Code)
	}
	if rec := call(h, "POST", "/api/auth/logout", "", nil); rec.Code != 204 {
		t.Fatalf("anonymous logout=%d", rec.Code)
	}
	third := sessionCookie(t, call(h, "POST", "/api/auth/login", credentials, nil))
	now = now.Add(24 * time.Hour)
	assertError(t, call(h, "GET", "/api/auth/me", "", third), 401, "UNAUTHENTICATED")
	assertError(t, call(h, "GET", "/api/auth/me", "", &http.Cookie{Name: "ev_session", Value: "invalid"}), 401, "UNAUTHENTICATED")
}

func TestLoginValidation(t *testing.T) {
	pool, _ := testutil.NewDatabase(t, true)
	h := server.NewRouter(server.Dependencies{Pool: pool})
	for i, tc := range []struct {
		name, body, contentType string
		status                  int
		code                    string
	}{
		{"missing fields", `{}`, "application/json", 400, "VALIDATION_ERROR"},
		{"invalid JSON", `{`, "application/json", 400, "VALIDATION_ERROR"},
		{"multiple values", credentials + ` {}`, "application/json", 400, "VALIDATION_ERROR"},
		{"unknown field", `{"username":"demo","password":"DemoPass123!","admin":true}`, "application/json", 400, "VALIDATION_ERROR"},
		{"null body", `null`, "application/json", 400, "VALIDATION_ERROR"},
		{"password too long", `{"username":"demo","password":"` + strings.Repeat("x", 73) + `"}`, "application/json", 400, "VALIDATION_ERROR"},
		{"oversized body", credentials + strings.Repeat(" ", 16384), "application/json", 413, "PAYLOAD_TOO_LARGE"},
		{"wrong media type", credentials, "text/plain", 415, "UNSUPPORTED_MEDIA_TYPE"},
		{"missing media type", credentials, "", 415, "UNSUPPORTED_MEDIA_TYPE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(tc.body))
			req.RemoteAddr = "192.0.2.10:" + string(rune('0'+i))
			req.Header.Set("Origin", "http://localhost:8080")
			req.Header.Set("Content-Type", tc.contentType)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			assertError(t, rec, tc.status, tc.code)
		})
	}
}

func TestInvalidCredentialsPreserveSession(t *testing.T) {
	pool, _ := testutil.NewDatabase(t, true)
	h := server.NewRouter(server.Dependencies{Pool: pool})
	cookie := sessionCookie(t, call(h, "POST", "/api/auth/login", credentials, nil))
	wrong := call(h, "POST", "/api/auth/login", `{"username":"demo","password":"wrong"}`, cookie)
	unknown := call(h, "POST", "/api/auth/login", `{"username":"missing","password":"wrong"}`, nil)
	assertError(t, wrong, 401, "INVALID_CREDENTIALS")
	assertError(t, unknown, 401, "INVALID_CREDENTIALS")
	if wrong.Body.String() != unknown.Body.String() || len(wrong.Result().Cookies()) != 0 {
		t.Fatal("credential errors disclose account existence or replace the session")
	}
	if rec := call(h, "GET", "/api/auth/me", "", cookie); rec.Code != 200 {
		t.Fatalf("failed login invalidated existing session: %d", rec.Code)
	}
}

func TestPrivateResponseHeaders(t *testing.T) {
	pool, _ := testutil.NewDatabase(t, true)
	h := server.NewRouter(server.Dependencies{Pool: pool, CookieSecure: true})
	login := call(h, "POST", "/api/auth/login", credentials, nil)
	cookie := sessionCookie(t, login)
	if !cookie.Secure {
		t.Fatal("HTTPS configuration must set Secure cookie")
	}
	for _, rec := range []*httptest.ResponseRecorder{login, call(h, "GET", "/api/auth/me", "", cookie), call(h, "GET", "/api/auth/me", "", nil), call(h, "POST", "/api/auth/logout", "", cookie)} {
		if rec.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("private response cache policy=%q", rec.Header().Get("Cache-Control"))
		}
	}
}

func TestLoginRateLimit(t *testing.T) {
	now := time.Now()
	h := server.NewRouter(server.Dependencies{Now: func() time.Time { return now }})
	for i := 0; i < 10; i++ {
		assertError(t, call(h, "POST", "/api/auth/login", `{}`, nil), 400, "VALIDATION_ERROR")
	}
	rec := call(h, "POST", "/api/auth/login", `{}`, nil)
	assertError(t, rec, 429, "RATE_LIMITED")
	if rec.Header().Get("Retry-After") != "60" {
		t.Fatalf("retry-after=%q", rec.Header().Get("Retry-After"))
	}
	req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{}`))
	req.Header.Set("Origin", "http://localhost:8080")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", "192.0.2.99")
	req.RemoteAddr = "192.0.2.1:54321"
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assertError(t, rec, 429, "RATE_LIMITED")
	now = now.Add(time.Minute)
	assertError(t, call(h, "POST", "/api/auth/login", `{}`, nil), 400, "VALIDATION_ERROR")
}
