package terminal

import (
	"context"
	"fmt"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

// Repositories is the terminal runtime's persistence boundary. The runtime
// only knows about connection metadata and bounded snapshots; file formats and
// audit paths stay in the persistence adapter.
type Repositories struct {
	Instances           ports.ConnectionInstanceRepository
	Audit               ports.AuditRepository
	Snapshots           ports.TerminalSnapshotRepository
	PersistenceDegraded func() bool

	// Optional transactional archive-and-delete; otherwise use Audit then Instances.
	RetireConnectionInstance func(context.Context, domain.ConnectionInstanceID) error
}

func (r Repositories) available() bool {
	return r.Instances != nil && r.Snapshots != nil
}

func (r Repositories) saveMeta(ctx context.Context, meta domain.ConnectionInstanceMeta) error {
	if r.Instances == nil {
		return nil
	}
	return r.Instances.SaveConnectionInstance(ctx, meta)
}

func (r Repositories) saveSnapshot(ctx context.Context, id domain.ConnectionInstanceID, header domain.SnapshotHeader, payload []byte) error {
	if r.Snapshots == nil {
		return nil
	}
	return r.Snapshots.SaveSnapshot(ctx, id, header, payload)
}

func (r Repositories) loadSnapshot(ctx context.Context, id domain.ConnectionInstanceID) ([]byte, error) {
	if r.Snapshots == nil {
		return nil, context.Canceled
	}
	_, payload, err := r.Snapshots.LoadSnapshot(ctx, id)
	return payload, err
}

func (r Repositories) listInstances(ctx context.Context) ([]domain.ConnectionInstanceMeta, error) {
	if r.Instances == nil {
		return nil, nil
	}
	return r.Instances.ListConnectionInstances(ctx)
}

func (r Repositories) retireInstance(ctx context.Context, id domain.ConnectionInstanceID) error {
	if r.RetireConnectionInstance != nil {
		if err := r.RetireConnectionInstance(ctx, id); err != nil {
			return fmt.Errorf("retire connection instance: %w", err)
		}
		return nil
	}
	if r.Audit != nil {
		if err := r.Audit.ArchiveConnectionInstance(ctx, id); err != nil {
			return fmt.Errorf("archive connection instance: %w", err)
		}
	}
	if r.Instances != nil {
		if err := r.Instances.DeleteConnectionInstance(ctx, id); err != nil {
			return fmt.Errorf("remove active connection instance: %w", err)
		}
	}
	return nil
}
