package gamecoin

import (
	"context"
	"time"
)

// Test-only options: no real waiting and a deterministic jitter. They are not part of the public API.

// WithSleep replaces the function that waits between two attempts.
func WithSleep(fn func(ctx context.Context, d time.Duration) error) Option {
	return func(c *config) { c.sleep = fn }
}

// WithJitter replaces the source of jitter, a number in [0, 1): 0.5 is neutral.
func WithJitter(fn func() float64) Option {
	return func(c *config) { c.jitter = fn }
}
