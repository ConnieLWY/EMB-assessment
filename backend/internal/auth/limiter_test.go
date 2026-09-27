package auth

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLimiterConcurrentAttempts(t *testing.T) {
	l := loginLimiter{windows: make(map[string]loginWindow), now: time.Now}
	var accepted atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ok, _ := l.allow("192.0.2.1"); ok {
				accepted.Add(1)
			}
		}()
	}
	wg.Wait()
	if accepted.Load() != 10 {
		t.Fatalf("accepted=%d", accepted.Load())
	}
}

func TestLimiterCapacityAndExpiry(t *testing.T) {
	now := time.Now()
	l := loginLimiter{windows: make(map[string]loginWindow), now: func() time.Time { return now }}
	for i := 0; i < 1024; i++ {
		if ok, _ := l.allow(fmt.Sprint(i)); !ok {
			t.Fatal("limit reached too early")
		}
	}
	if ok, _ := l.allow("new-client"); ok {
		t.Fatal("unbounded rate limit storage")
	}
	now = now.Add(time.Minute)
	if ok, _ := l.allow("new-client"); !ok {
		t.Fatal("expired windows were not reclaimed")
	}
}
