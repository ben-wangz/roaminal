package db

import (
	"context"
	"errors"
	"os"
	"sync"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"gorm.io/gorm"
)

var errInstanceDatabase = errors.New("connection instance database repository unavailable")

type instanceHealth struct {
	mu         sync.RWMutex
	ids        map[string]struct{}
	listFailed bool
}

// PersistenceDegraded reports failed instance persistence for the terminal
// runtime. A successful write clears only its own instance's failure.
func (s *Store) PersistenceDegraded() bool {
	h := &s.instanceHealth
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.listFailed || len(h.ids) != 0
}

func (s *Store) MarkConnectionInstanceDegraded(ctx context.Context, id domain.ConnectionInstanceID) error {
	if err := instanceContextError(ctx); err != nil {
		return err
	}
	s.instanceHealth.set(id.String(), true)
	return nil
}

func (h *instanceHealth) set(id string, failed bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !failed {
		delete(h.ids, id)
		return
	}
	if h.ids == nil {
		h.ids = make(map[string]struct{})
	}
	h.ids[id] = struct{}{}
}

func (h *instanceHealth) setListFailed(failed bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.listFailed = failed
}

func (s *Store) instanceFailure(id string, err error) error {
	s.instanceHealth.set(id, true)
	return err
}

func (s *Store) instanceReadError(ctx context.Context, id string, err error) error {
	err = instanceDatabaseError(ctx, err)
	if errors.Is(err, errInstanceDatabase) || errors.Is(err, errSnapshotSize) {
		return s.instanceFailure(id, err)
	}
	return err
}

func instanceContextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

func checkInstanceID(ctx context.Context, id string) error {
	if err := instanceContextError(ctx); err != nil {
		return err
	}
	if !instanceIDPattern.MatchString(id) {
		return errInstanceID
	}
	return nil
}

func instanceDatabaseError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctxErr := instanceContextError(ctx); ctxErr != nil {
		return ctxErr
	}
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	case errors.Is(err, gorm.ErrRecordNotFound):
		return os.ErrNotExist
	case errors.Is(err, errSnapshotSize):
		return errSnapshotSize
	default:
		// Driver errors can include metadata or terminal contents.
		return errInstanceDatabase
	}
}
