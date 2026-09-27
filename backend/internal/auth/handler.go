package auth

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	"ev-charger-assessment/backend/internal/httpapi"
)

type Handler struct {
	service *Service
	secure  bool
	limiter loginLimiter
}

func NewHandler(service *Service, secure bool) *Handler {
	return &Handler{service: service, secure: secure, limiter: loginLimiter{windows: make(map[string]loginWindow), now: service.now}}
}

// Login signs in with a seeded account.
// @Summary Sign in
// @Description Use demo or demo2 with DemoPass123!. The browser stores the HttpOnly ev_session cookie for 24 hours. A successful login replaces the previous browser session. CLI clients must send an allowed Origin. Login attempts are limited to 10 per direct client IP per minute.
// @Tags Authentication
// @ID login
// @Accept json
// @Produce json
// @Param credentials body LoginRequest true "Login credentials"
// @Success 200 {object} UserResponse "Signed in"
// @Header 200 {string} Set-Cookie "HttpOnly ev_session cookie; Secure when API_ORIGIN uses HTTPS"
// @Header 200 {string} Cache-Control "no-store"
// @Failure 400 {object} httpapi.ErrorResponse "VALIDATION_ERROR: invalid JSON or login fields"
// @Failure 401 {object} httpapi.ErrorResponse "INVALID_CREDENTIALS: invalid username or password"
// @Failure 403 {object} httpapi.ErrorResponse "ORIGIN_NOT_ALLOWED: missing or untrusted Origin"
// @Failure 413 {object} httpapi.ErrorResponse "PAYLOAD_TOO_LARGE: body exceeds 16 KiB"
// @Failure 415 {object} httpapi.ErrorResponse "UNSUPPORTED_MEDIA_TYPE: use application/json"
// @Failure 429 {object} httpapi.ErrorResponse "RATE_LIMITED: login attempt limit or limiter capacity reached"
// @Header 429 {integer} Retry-After "Seconds until another attempt may be made"
// @Failure 500 {object} httpapi.ErrorResponse "INTERNAL_ERROR: unexpected server error"
// @Router /api/auth/login [post]
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	if ok, retry := h.limiter.allow(ip); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(retry))
		httpapi.Error(w, 429, "RATE_LIMITED", "Too many login attempts. Try again later.")
		return
	}
	var input LoginRequest
	if !httpapi.Decode(w, r, &input) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	user, token, err := h.service.Login(ctx, input.Username, input.Password, cookieToken(r))
	if err != nil {
		h.error(w, r, err)
		return
	}
	h.setCookie(w, token, int(SessionLifetime/time.Second), h.service.now().Add(SessionLifetime))
	httpapi.JSON(w, 200, UserResponse{User: user})
}

// Logout invalidates the current session.
// @Summary Sign out
// @Description Invalidates the current session and clears its cookie. Also succeeds when already signed out. Requires an allowed Origin.
// @Tags Authentication
// @ID logout
// @Produce json
// @Success 204 "Signed out; no response body"
// @Header 204 {string} Set-Cookie "Clears ev_session with Max-Age=0"
// @Failure 403 {object} httpapi.ErrorResponse "ORIGIN_NOT_ALLOWED: missing or untrusted Origin"
// @Failure 500 {object} httpapi.ErrorResponse "INTERNAL_ERROR: unexpected server error"
// @Router /api/auth/logout [post]
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := h.service.Logout(ctx, cookieToken(r)); err != nil {
		h.error(w, r, err)
		return
	}
	h.setCookie(w, "", -1, time.Unix(1, 0))
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) RequireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		user, err := h.service.Authenticate(ctx, cookieToken(r))
		cancel()
		if err != nil {
			h.error(w, r, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userContextKey{}, user)))
	})
}

// Me returns the authenticated user.
// @Summary Get the signed-in user
// @Description Requires the HttpOnly ev_session cookie set by POST /api/auth/login. Run login first; the browser sends its cookie automatically. Swagger 2.0 cannot model cookie authentication as a security scheme, so no manual token input is provided.
// @Tags Authentication
// @ID getCurrentUser
// @Produce json
// @Success 200 {object} UserResponse "Current signed-in user"
// @Header 200 {string} Cache-Control "no-store"
// @Failure 401 {object} httpapi.ErrorResponse "UNAUTHENTICATED: missing, invalid, or expired session"
// @Failure 403 {object} httpapi.ErrorResponse "ORIGIN_NOT_ALLOWED: untrusted Origin"
// @Failure 500 {object} httpapi.ErrorResponse "INTERNAL_ERROR: unexpected server error"
// @Router /api/auth/me [get]
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	user, _ := UserFromContext(r.Context())
	httpapi.JSON(w, 200, UserResponse{User: user})
}

func (h *Handler) setCookie(w http.ResponseWriter, value string, maxAge int, expires time.Time) {
	http.SetCookie(w, &http.Cookie{Name: "ev_session", Value: value, Path: "/", HttpOnly: true, Secure: h.secure, SameSite: http.SameSiteLaxMode, MaxAge: maxAge, Expires: expires.UTC()})
}

func cookieToken(r *http.Request) string {
	c, err := r.Cookie("ev_session")
	if err != nil {
		return ""
	}
	return c.Value
}

func (h *Handler) error(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrInvalidInput):
		httpapi.Error(w, 400, "VALIDATION_ERROR", "Provide a username of 1–128 bytes and a password of 1–72 bytes.")
	case errors.Is(err, ErrInvalidCredentials):
		httpapi.Error(w, 401, "INVALID_CREDENTIALS", "Invalid username or password.")
	case errors.Is(err, ErrUnauthenticated):
		httpapi.Error(w, 401, "UNAUTHENTICATED", "Sign in to continue.")
	default:
		slog.ErrorContext(r.Context(), "authentication request failed", "error", err)
		httpapi.Error(w, 500, "INTERNAL_ERROR", "An unexpected error occurred.")
	}
}
