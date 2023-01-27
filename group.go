package workgroup

import (
	"context"
	"sync"

	"golang.org/x/sync/errgroup"
)

// Group is a collection of goroutines working on subtasks that are part of
// the same overall task, simplifying the synchronization of such subtasks as
// well as the aggregation of their results.
type Group[T, Q any] struct {
	wg  *errgroup.Group
	ctx context.Context
	sem chan Q

	ch  chan T
	res []T

	mux  sync.RWMutex
	once sync.Once
}

// WithContext returns a new Group with no concurrency limit.
func WithContext[T any](ctx context.Context) (*Group[T, struct{}], context.Context) {
	return WithSemaphore[T, struct{}](ctx, nil)
}

// WithConcurrency returns a new Group with a semaphore channel of the given
// size. If concurrency is 0, no semaphore is used.
func WithConcurrency[T any](ctx context.Context, concurrency int) (*Group[T, struct{}], context.Context) {
	var ch chan struct{}
	if concurrency > 0 {
		ch = make(chan struct{}, concurrency)
	}

	return WithSemaphore[T](ctx, ch)
}

// WithSemaphore returns a new Group with the given semaphore channel. If the
// channel is nil, no semaphore is used. The semaphore channel is used to
// limit the number of goroutines that can be started at once by multiple
// groups using the same semaphore channel.
func WithSemaphore[T, Q any](ctx context.Context, sem chan Q) (*Group[T, Q], context.Context) {
	wg, ctx := errgroup.WithContext(ctx)

	g := Group[T, Q]{
		wg:  wg,
		ctx: ctx,
		sem: sem,

		ch: make(chan T),
	}

	// The results lock ensures results are fully written before they are returned.
	g.mux.Lock()
	go func() {
		defer g.mux.Unlock()

		for v := range g.ch {
			g.res = append(g.res, v)
		}
	}()

	return &g, ctx
}

// Go attempts to start a new goroutine. If the concurrency limit is reached,
// it will block until another goroutine finishes.
func (g *Group[T, Q]) Go(f func(ch chan<- T) error) {
	if g.sem != nil {
		var q Q
		g.sem <- q
	}

	g.wg.Go(func() error {
		if g.sem != nil {
			defer func() { <-g.sem }()
		}
		return f(g.ch)
	})
}

// Wait blocks until all goroutines have finished. It returns the results of
// all goroutines and the first error enountered, if any for either.
func (g *Group[T, Q]) Wait() ([]T, error) {
	err := g.wg.Wait()

	// Once all workers finish, close the channel to finish accepting results.
	g.once.Do(func() {
		close(g.ch)
	})

	// Wait for the results to be fully written, as the write lock is held
	// until the channel is closed.
	g.mux.RLock()
	defer g.mux.RUnlock()

	return g.res, err
}
