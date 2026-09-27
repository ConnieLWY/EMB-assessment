package server

import (
	"bufio"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"
)

type statusWriter struct {
	http.ResponseWriter
	status   int
	hijacked bool
	onHijack func()
}

func (w *statusWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(p)
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *statusWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	conn, rw, err := hijacker.Hijack()
	if err == nil {
		w.hijacked = true
		w.status = http.StatusSwitchingProtocols
		w.onHijack()
	}
	return conn, rw, err
}

func requestLog(next http.Handler, logger *slog.Logger) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		started := time.Now()
		logRequest := func(status int) {
			logger.InfoContext(r.Context(), "HTTP request", "method", r.Method, "path", r.URL.Path,
				"status", status, "duration_ms", float64(time.Since(started).Nanoseconds())/1e6)
		}
		tracked := &statusWriter{ResponseWriter: w, onHijack: func() { logRequest(http.StatusSwitchingProtocols) }}
		next.ServeHTTP(tracked, r)
		if !tracked.hijacked {
			if tracked.status == 0 {
				tracked.status = http.StatusOK
			}
			logRequest(tracked.status)
		}
	})
}
