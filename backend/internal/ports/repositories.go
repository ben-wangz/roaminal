package ports

import (
	"context"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
)

type AgentRepository interface {
	Available() bool
	Err() error
	Snapshot() map[string]domain.AgentEndpointRecord
	Get(string) (domain.AgentEndpointRecord, bool)
	Update(string, func(*domain.AgentEndpointRecord) error) error
	FindToken(string, time.Time) (string, domain.AgentEndpointRecord, bool)
}

// Repositories are application-facing ports. Storage details such as JSON
// layout, locking, atomic replacement, permissions, and quarantine stay in
// infrastructure adapters.
type AuthRepository interface {
	LoadAuth(context.Context) ([]domain.AuthSessionRecord, error)
	SaveAuth(context.Context, []domain.AuthSessionRecord) error
}

type ConnectionInstanceRepository interface {
	ListConnectionInstances(context.Context) ([]domain.ConnectionInstanceMeta, error)
	LoadConnectionInstance(context.Context, domain.ConnectionInstanceID) (domain.ConnectionInstanceMeta, error)
	SaveConnectionInstance(context.Context, domain.ConnectionInstanceMeta) error
	DeleteConnectionInstance(context.Context, domain.ConnectionInstanceID) error
	MarkConnectionInstanceDegraded(context.Context, domain.ConnectionInstanceID) error
}

type AuditRepository interface {
	ArchiveConnectionInstance(context.Context, domain.ConnectionInstanceID) error
}

type TerminalSnapshotRepository interface {
	SaveSnapshot(context.Context, domain.ConnectionInstanceID, domain.SnapshotHeader, []byte) error
	LoadSnapshot(context.Context, domain.ConnectionInstanceID) (domain.SnapshotHeader, []byte, error)
}

type WorkspaceLayoutRepository interface {
	LoadWorkspaceLayout(context.Context, domain.AuthenticationSessionID) (domain.ConnectionInstanceLayout, bool, error)
	SaveWorkspaceLayout(context.Context, domain.AuthenticationSessionID, domain.ConnectionInstanceLayout, uint64) error
}

type UploadRepository interface {
	LoadUpload(context.Context, string) (domain.UploadJobRecord, error)
	SaveUpload(context.Context, domain.UploadJobRecord) error
	DeleteUpload(context.Context, string) error
}

// PushSubscriptionRepository stores browser delivery endpoints and their
// encryption keys. Implementations must keep this data private and durable.
type PushSubscriptionRepository interface {
	ListPushSubscriptions(context.Context) ([]domain.PushSubscriptionRecord, error)
	UpsertPushSubscription(context.Context, domain.PushSubscriptionRecord) (domain.PushSubscriptionRecord, error)
	DeletePushSubscription(context.Context, string, string) (bool, error)
	DeletePushSubscriptionsForAuthSession(context.Context, string) (int, error)
	DeletePushSubscriptionByID(context.Context, string) (bool, error)
}
