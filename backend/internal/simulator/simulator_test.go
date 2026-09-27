package simulator

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunStopsWithContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var calls atomic.Int32
	done := make(chan struct{})
	go func() {
		Run(ctx, 5*time.Millisecond, func(context.Context) error {
			if calls.Add(1) == 2 {
				cancel()
			}
			return nil
		})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker did not stop")
	}
	if calls.Load() != 2 {
		t.Fatalf("calls=%d", calls.Load())
	}
}
