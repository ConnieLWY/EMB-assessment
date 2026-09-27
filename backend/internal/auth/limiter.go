package auth

import (
	"sync"
	"time"
)

type loginWindow struct {
	count int
	until time.Time
}
type loginLimiter struct {
	mu      sync.Mutex
	windows map[string]loginWindow
	now     func() time.Time
}

func (l *loginLimiter) allow(ip string) (bool, int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	window, exists := l.windows[ip]
	if exists && !now.Before(window.until) {
		delete(l.windows, ip)
		exists = false
	}
	if !exists {
		// Bound memory without trusting forwarded IPs. Saturation fails closed until windows expire.
		if len(l.windows) >= 1024 {
			for key, w := range l.windows {
				if !now.Before(w.until) {
					delete(l.windows, key)
				}
			}
			if len(l.windows) >= 1024 {
				return false, 60
			}
		}
		window = loginWindow{until: now.Add(time.Minute)}
	}
	if window.count >= 10 {
		return false, int((window.until.Sub(now) + time.Second - 1) / time.Second)
	}
	window.count++
	l.windows[ip] = window
	return true, 0
}
