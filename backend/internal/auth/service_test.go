package auth_test

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ev-charger-assessment/backend/internal/auth"
	"ev-charger-assessment/backend/internal/server"
	"ev-charger-assessment/backend/internal/testutil"
)

func TestSessionRotationRollback(t *testing.T) {
	pool, _ := testutil.NewDatabase(t, true)
	svc := auth.NewService(pool, time.Now)
	ctx := context.Background()
	user, token, err := svc.Login(ctx, "demo", "DemoPass123!", "")
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(decoded) != 32 {
		t.Fatal("session token must contain 32 random bytes")
	}
	if _, err := pool.Exec(ctx, "ALTER TABLE auth_sessions ADD CONSTRAINT reject_new_sessions CHECK (false) NOT VALID"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.Login(ctx, "demo", "DemoPass123!", token); err == nil {
		t.Fatal("expected session insert failure")
	}
	authenticated, err := svc.Authenticate(ctx, token)
	if err != nil || authenticated.ID != user.ID {
		t.Fatal("failed rotation revoked the existing session")
	}
	if _, err := svc.Authenticate(ctx, token[:42]+"!"); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatalf("tampered token accepted: %v", err)
	}
}

func TestAuthenticatedContext(t *testing.T) {
	pool, _ := testutil.NewDatabase(t, true)
	svc := auth.NewService(pool, time.Now)
	user, token, err := svc.Login(context.Background(), "demo2", "DemoPass123!", "")
	if err != nil {
		t.Fatal(err)
	}
	called := false
	protected := auth.NewHandler(svc, false).RequireUser(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		actual, ok := auth.UserFromContext(r.Context())
		if !ok || actual != user {
			t.Errorf("context user=%+v", actual)
		}
		w.WriteHeader(204)
	}))
	req := httptest.NewRequest("GET", "/private", nil)
	req.AddCookie(&http.Cookie{Name: "ev_session", Value: token})
	rec := httptest.NewRecorder()
	protected.ServeHTTP(rec, req)
	if !called || rec.Code != 204 {
		t.Fatal("authenticated request did not reach protected handler")
	}
	called = false
	rec = httptest.NewRecorder()
	protected.ServeHTTP(rec, httptest.NewRequest("GET", "/private", nil))
	if called || rec.Code != 401 {
		t.Fatal("anonymous request reached protected handler")
	}
}

func TestAuthDatabaseFailures(t *testing.T) {
	pool, _ := testutil.NewDatabase(t, true)
	h := server.NewRouter(server.Dependencies{Pool: pool})
	cookie := sessionCookie(t, call(h, "POST", "/api/auth/login", credentials, nil))
	pool.Close()
	for _, tc := range []struct{ method, path, body string }{{"POST", "/api/auth/login", credentials}, {"GET", "/api/auth/me", ""}, {"POST", "/api/auth/logout", ""}} {
		rec := call(h, tc.method, tc.path, tc.body, cookie)
		assertError(t, rec, 500, "INTERNAL_ERROR")
		if len(rec.Result().Cookies()) != 0 {
			t.Error("failed database operation must not change the cookie")
		}
	}
}
