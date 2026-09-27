package simulator

import (
	"context"
	"log/slog"
	"time"
)

func Run(ctx context.Context, interval time.Duration, step func(context.Context) error) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := step(ctx); err != nil && ctx.Err() == nil {
				slog.ErrorContext(ctx, "background step failed", "error", err)
			}
		}
	}
}
