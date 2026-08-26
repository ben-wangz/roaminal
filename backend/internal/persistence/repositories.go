package persistence

import (
	"context"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

// Repositories exposes storage adapters through application-owned ports.
type Repositories struct {
	Auth              ports.AuthRepository
	Connection        ports.ConnectionInstanceRepository
	Audit             ports.AuditRepository
	TerminalSnapshots ports.TerminalSnapshotRepository
	Workspace         ports.WorkspaceLayoutRepository
	Upload            ports.UploadRepository
	Messages          ports.MessageRepository
}

func NewRepositories(store *Store) Repositories {
	adapter := &repositoryAdapter{store: store}
	return Repositories{Auth: adapter, Connection: adapter, Audit: adapter, TerminalSnapshots: adapter, Workspace: adapter, Upload: adapter, Messages: adapter}
}

type repositoryAdapter struct{ store *Store }

func checkContext(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func (a *repositoryAdapter) LoadAuth(ctx context.Context) ([]domain.AuthSessionRecord, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}
	file, err := a.store.LoadAuth()
	if err != nil {
		return nil, err
	}
	result := make([]domain.AuthSessionRecord, 0, len(file.Sessions))
	for _, value := range file.Sessions {
		result = append(result, domain.AuthSessionRecord{ID: value.ID, PasswordFingerprint: value.PasswordFingerprint, RefreshTokenHash: value.RefreshTokenHash, CreatedAt: value.CreatedAt, LastSeenAt: value.LastSeenAt, RefreshExpiresAt: value.RefreshExpiresAt, RotatedAt: value.RotatedAt, UserAgent: value.UserAgent})
	}
	return result, nil
}

func (a *repositoryAdapter) SaveAuth(ctx context.Context, records []domain.AuthSessionRecord) error {
	if err := checkContext(ctx); err != nil {
		return err
	}
	converted := make([]AuthSession, 0, len(records))
	for _, value := range records {
		converted = append(converted, AuthSession{ID: value.ID, PasswordFingerprint: value.PasswordFingerprint, RefreshTokenHash: value.RefreshTokenHash, CreatedAt: value.CreatedAt, LastSeenAt: value.LastSeenAt, RefreshExpiresAt: value.RefreshExpiresAt, RotatedAt: value.RotatedAt, UserAgent: value.UserAgent})
	}
	return a.store.SaveAuth(AuthFile{Sessions: converted})
}

func (a *repositoryAdapter) ListConnectionInstances(ctx context.Context) ([]domain.ConnectionInstanceMeta, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}
	return a.store.ListConnectionInstances()
}

func (a *repositoryAdapter) LoadConnectionInstance(ctx context.Context, id domain.ConnectionInstanceID) (domain.ConnectionInstanceMeta, error) {
	if err := checkContext(ctx); err != nil {
		return domain.ConnectionInstanceMeta{}, err
	}
	return a.store.LoadConnectionInstance(id.String())
}

func (a *repositoryAdapter) SaveConnectionInstance(ctx context.Context, meta domain.ConnectionInstanceMeta) error {
	if err := checkContext(ctx); err != nil {
		return err
	}
	return a.store.SaveConnectionInstance(meta)
}

func (a *repositoryAdapter) DeleteConnectionInstance(ctx context.Context, id domain.ConnectionInstanceID) error {
	if err := checkContext(ctx); err != nil {
		return err
	}
	return a.store.DeleteConnectionInstance(id.String())
}

func (a *repositoryAdapter) ArchiveConnectionInstance(ctx context.Context, id domain.ConnectionInstanceID) error {
	if err := checkContext(ctx); err != nil {
		return err
	}
	return a.store.ArchiveConnectionInstance(id.String())
}

func (a *repositoryAdapter) MarkConnectionInstanceDegraded(ctx context.Context, id domain.ConnectionInstanceID) error {
	if err := checkContext(ctx); err != nil {
		return err
	}
	a.store.MarkConnectionInstanceDegraded(id.String())
	return nil
}

func (a *repositoryAdapter) SaveSnapshot(ctx context.Context, id domain.ConnectionInstanceID, header domain.SnapshotHeader, payload []byte) error {
	if err := checkContext(ctx); err != nil {
		return err
	}
	return a.store.SaveSnapshot(id.String(), header, payload)
}

func (a *repositoryAdapter) LoadSnapshot(ctx context.Context, id domain.ConnectionInstanceID) (domain.SnapshotHeader, []byte, error) {
	if err := checkContext(ctx); err != nil {
		return domain.SnapshotHeader{}, nil, err
	}
	return a.store.LoadSnapshot(id.String())
}

func (a *repositoryAdapter) LoadWorkspaceLayout(ctx context.Context, id domain.AuthenticationSessionID) (domain.ConnectionInstanceLayout, bool, error) {
	if err := checkContext(ctx); err != nil {
		return domain.ConnectionInstanceLayout{}, false, err
	}
	a.store.workspaceMu.Lock()
	defer a.store.workspaceMu.Unlock()
	file, err := a.store.loadWorkspaceLayouts()
	if err != nil {
		return domain.ConnectionInstanceLayout{}, false, err
	}
	for _, record := range file.Layouts {
		if record.AuthenticationSessionID == id.String() {
			return cloneLayout(record.Layout), true, nil
		}
	}
	return domain.ConnectionInstanceLayout{}, false, nil
}

func (a *repositoryAdapter) SaveWorkspaceLayout(ctx context.Context, id domain.AuthenticationSessionID, layout domain.ConnectionInstanceLayout, expectedRevision uint64) error {
	if err := checkContext(ctx); err != nil {
		return err
	}
	if err := ValidateConnectionInstanceLayout(&layout); err != nil {
		return err
	}
	a.store.workspaceMu.Lock()
	defer a.store.workspaceMu.Unlock()
	file, err := a.store.loadWorkspaceLayouts()
	if err != nil {
		return err
	}
	found := false
	currentRevision := uint64(0)
	for index := range file.Layouts {
		if file.Layouts[index].AuthenticationSessionID != id.String() {
			continue
		}
		currentRevision = file.Layouts[index].Layout.Revision
		if currentRevision != expectedRevision || layout.Revision != expectedRevision+1 {
			return ports.ErrRevisionConflict
		}
		file.Layouts[index].Layout = cloneLayout(layout)
		found = true
		break
	}
	if !found && (expectedRevision != 0 || layout.Revision != 1) {
		return ports.ErrRevisionConflict
	}
	if !found {
		file.Layouts = append(file.Layouts, workspaceLayoutRecord{AuthenticationSessionID: id.String(), Layout: cloneLayout(layout)})
	}
	return a.store.saveWorkspaceLayouts(file.Layouts)
}

func cloneLayout(layout domain.ConnectionInstanceLayout) domain.ConnectionInstanceLayout {
	copyLayout := layout
	copyLayout.GroupOrder = append(make([]string, 0, len(layout.GroupOrder)), layout.GroupOrder...)
	copyLayout.UngroupedConnectionInstanceIDs = append(make([]string, 0, len(layout.UngroupedConnectionInstanceIDs)), layout.UngroupedConnectionInstanceIDs...)
	copyLayout.Groups = make([]domain.ConnectionInstanceGroup, len(layout.Groups))
	for index, group := range layout.Groups {
		copyLayout.Groups[index] = group
		copyLayout.Groups[index].ConnectionInstanceIDs = append(make([]string, 0, len(group.ConnectionInstanceIDs)), group.ConnectionInstanceIDs...)
	}
	return copyLayout
}

var _ ports.AuthRepository = (*repositoryAdapter)(nil)
var _ ports.ConnectionInstanceRepository = (*repositoryAdapter)(nil)
var _ ports.AuditRepository = (*repositoryAdapter)(nil)
var _ ports.TerminalSnapshotRepository = (*repositoryAdapter)(nil)
var _ ports.WorkspaceLayoutRepository = (*repositoryAdapter)(nil)

func (a *repositoryAdapter) LoadUpload(ctx context.Context, id string) (domain.UploadJobRecord, error) {
	if err := checkContext(ctx); err != nil {
		return domain.UploadJobRecord{}, err
	}
	return a.store.LoadUpload(ctx, id)
}

func (a *repositoryAdapter) SaveUpload(ctx context.Context, record domain.UploadJobRecord) error {
	if err := checkContext(ctx); err != nil {
		return err
	}
	return a.store.SaveUpload(ctx, record)
}

func (a *repositoryAdapter) DeleteUpload(ctx context.Context, id string) error {
	if err := checkContext(ctx); err != nil {
		return err
	}
	return a.store.DeleteUpload(ctx, id)
}

var _ ports.UploadRepository = (*repositoryAdapter)(nil)
var _ ports.MessageRepository = (*repositoryAdapter)(nil)
