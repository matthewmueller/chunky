package rate

import (
	"context"

	"golang.org/x/time/rate"
)

type Limiter interface {
	Use(ctx context.Context, tokens int) error
}

// New rate limiter with a bucket size of and refill rate of n. For example, if
// n is 10, you could immediately use all 10 tokens, but if you want to use
// 10 more tokens, you'll need to wait a second for 10 tokens to be available.
func New(n int) Limiter {
	if n == 0 {
		return &unlimited{}
	}
	l := rate.NewLimiter(rate.Limit(n), n)
	return &limiter{l}
}

type limiter struct {
	l *rate.Limiter
}

var _ Limiter = (*limiter)(nil)

func (l *limiter) Use(ctx context.Context, tokens int) error {
	// bucket allows waiting for at most Burst() tokens at once
	maxWait := l.l.Burst()
	for tokens > maxWait {
		if err := l.l.WaitN(ctx, maxWait); err != nil {
			return err
		}
		tokens -= maxWait
	}
	return l.l.WaitN(ctx, tokens)
}

type unlimited struct{}

var _ Limiter = (*unlimited)(nil)

func (u *unlimited) Use(ctx context.Context, tokens int) error {
	return nil
}
