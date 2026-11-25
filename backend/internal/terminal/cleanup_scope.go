package terminal

import (
	"context"
	"sync"
)

// cleanupMutex also supports bounded acquisition by retirement and shutdown.
// A session's ordinary operations may hold a lock across PTY or worker I/O.
type cleanupMutex struct {
	once sync.Once
	held chan struct{}
}

func (m *cleanupMutex) Lock() { _ = m.lock(context.Background()) }

func (m *cleanupMutex) lock(ctx context.Context) error {
	m.once.Do(func() { m.held = make(chan struct{}, 1) })
	select {
	case <-ctx.Done():
		return ctx.Err()
	case m.held <- struct{}{}:
		if err := ctx.Err(); err != nil {
			m.Unlock()
			return err
		}
		return nil
	}
}

func (m *cleanupMutex) Unlock() { <-m.held }

// repositoryScope owns terminal repository calls, including timer saves and
// process-exit cleanup begun before shutdown. Cancelling the scope closes
// admission and interrupts ongoing calls. Drain before releasing the store.
type repositoryScope struct {
	ctx      context.Context
	cancel   context.CancelFunc
	mu       sync.Mutex
	inFlight sync.WaitGroup
}

func newRepositoryScope() *repositoryScope {
	ctx, cancel := context.WithCancel(context.Background())
	return &repositoryScope{ctx: ctx, cancel: cancel}
}

func (s *repositoryScope) withContext(parent context.Context) (context.Context, func()) {
	return cancelWith(parent, s.ctx)
}

func cancelWith(parent, lifetime context.Context) (context.Context, func()) {
	ctx, cancel := context.WithCancel(parent)
	stop := context.AfterFunc(lifetime, cancel)
	if lifetime.Err() != nil {
		cancel()
	}
	return ctx, func() {
		stop()
		cancel()
	}
}

func (s *repositoryScope) run(parent context.Context, op func(context.Context) error) error {
	s.mu.Lock()
	if err := s.ctx.Err(); err != nil {
		s.mu.Unlock()
		return err
	}
	s.inFlight.Add(1)
	s.mu.Unlock()
	defer s.inFlight.Done()
	ctx, cancel := s.withContext(parent)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	return op(ctx)
}

func (s *repositoryScope) stop() {
	s.mu.Lock()
	s.cancel()
	s.mu.Unlock()
}
