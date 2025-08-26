package batch_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/matryer/is"
	"github.com/matthewmueller/chunky/internal/batch"
)

func TestRunsAllTasks(t *testing.T) {
	is := is.New(t)
	b := batch.New(4)

	var ran atomic.Int32
	const n = 50

	for range n {
		err := b.Add(context.Background(), func(ctx context.Context) error {
			time.Sleep(2 * time.Millisecond)
			ran.Add(1)
			return nil
		})
		is.NoErr(err)
	}

	is.NoErr(b.Wait())
	is.Equal(ran.Load(), int32(n))
}

func TestRespectsConcurrency(t *testing.T) {
	is := is.New(t)
	const c = 3
	b := batch.New(c)

	var cur, max atomic.Int32
	const n = 50

	for range n {
		err := b.Add(context.Background(), func(ctx context.Context) error {
			// enter
			cv := cur.Add(1)
			// record the peak
			for {
				mv := max.Load()
				if cv <= mv || max.CompareAndSwap(mv, cv) {
					break
				}
			}
			time.Sleep(15 * time.Millisecond) // hold the slot briefly
			cur.Add(-1)                       // exit
			return nil
		})
		is.NoErr(err)
	}

	is.NoErr(b.Wait())
	is.True(max.Load() <= c)
	// With enough work and tasks, we should hit the cap.
	is.True(max.Load() >= int32(c))
}

func TestErrorPropagation(t *testing.T) {
	is := is.New(t)
	b := batch.New(4)
	want := errors.New("boom")

	const n = 20
	for i := range n {
		err := b.Add(context.Background(), func(ctx context.Context) error {
			if i == 7 {
				return want
			}
			return nil
		})
		is.NoErr(err)
	}

	err := b.Wait()
	is.True(err != nil) // errgroup returns some non-nil error
}
