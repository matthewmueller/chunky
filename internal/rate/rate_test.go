package rate_test

import (
	"context"
	"testing"
	"time"

	"github.com/matryer/is"
	"github.com/matthewmueller/chunky/internal/rate"
)

func TestWriteLimiter(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	limiter := rate.New(10)
	now := time.Now()
	limiter.Use(ctx, 10)
	is.True(time.Since(now) < time.Second)
	now = time.Now()
	limiter.Use(ctx, 10)
	is.True(time.Since(now) > time.Second)
}
