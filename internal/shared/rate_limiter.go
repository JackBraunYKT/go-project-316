package shared

import (
	"context"
	"time"
)

type RateLimiter struct {
	ticker *time.Ticker
	done   chan struct{}
}

// NewRateLimiter создает ограничитель частоты запросов.
func NewRateLimiter(delay time.Duration, rps int) *RateLimiter {
	if rps > 0 {
		delay = time.Second / time.Duration(rps)
	}

	if delay <= 0 {
		return nil
	}

	limiter := &RateLimiter{
		ticker: time.NewTicker(delay),
		done:   make(chan struct{}),
	}

	return limiter
}

// Wait ожидает разрешения на следующий запрос или отмены контекста.
func (r *RateLimiter) Wait(ctx context.Context) error {
	if r == nil {
		return nil
	}

	select {
	case <-r.ticker.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Stop останавливает ограничитель частоты запросов.
func (r *RateLimiter) Stop() {
	if r != nil {
		r.ticker.Stop()
		close(r.done)
	}
}
