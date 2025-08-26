package batch

import (
	"context"
	"sync"

	"golang.org/x/sync/errgroup"
)

func New(concurrency int) *Batch {
	return &Batch{
		mu:  sync.RWMutex{},
		sem: make(chan struct{}, concurrency),
		eg:  new(errgroup.Group),
	}
}

type Batch struct {
	mu  sync.RWMutex
	sem chan struct{}
	eg  *errgroup.Group
}

func (b *Batch) Add(ctx context.Context, fn func(ctx context.Context) error) error {
	b.mu.RLock()
	defer b.mu.RUnlock()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case b.sem <- struct{}{}:
	}
	b.eg.Go(func() error {
		defer func() { <-b.sem }()
		return fn(ctx)
	})
	return nil
}

func (b *Batch) Wait() error {
	// Ensure all adds have been processed before waiting
	b.mu.Lock()
	defer b.mu.Unlock()

	// Wait for all tasks to complete
	return b.eg.Wait()
}
