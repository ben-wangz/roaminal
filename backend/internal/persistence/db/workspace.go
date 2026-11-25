package db

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strconv"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var _ ports.WorkspaceLayoutRepository = (*Store)(nil)

var errWorkspaceRepositoryUnavailable = errors.New("workspace layout repository unavailable")

type workspaceLayoutRow struct {
	AuthenticationSessionID string `gorm:"primaryKey"`
	// Compare the decimal text for equality so both drivers support the full
	// uint64 range, including revisions above a signed SQL integer's limit.
	Revision string `gorm:"not null;size:20"`
	Payload  []byte `gorm:"not null"`
}

func (workspaceLayoutRow) TableName() string { return "workspace_layouts" }

func workspaceUploadModels() []any { return []any{&workspaceLayoutRow{}, &uploadRow{}} }

func (s *Store) LoadWorkspaceLayout(ctx context.Context, id domain.AuthenticationSessionID) (domain.ConnectionInstanceLayout, bool, error) {
	ctx = workspaceUploadContext(ctx)
	if err := ctx.Err(); err != nil {
		return domain.ConnectionInstanceLayout{}, false, err
	}
	var row workspaceLayoutRow
	err := s.DB.WithContext(ctx).Where("authentication_session_id = ?", id.String()).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.ConnectionInstanceLayout{}, false, nil
	}
	if err != nil {
		return domain.ConnectionInstanceLayout{}, false, workspaceUploadDBError(ctx, errWorkspaceRepositoryUnavailable)
	}
	var layout domain.ConnectionInstanceLayout
	if decodeWorkspaceUploadPayload(row.Payload, &layout) != nil || strconv.FormatUint(layout.Revision, 10) != row.Revision {
		return domain.ConnectionInstanceLayout{}, false, errors.New("invalid workspace layout repository record")
	}
	if err := domain.ValidateConnectionInstanceLayout(&layout); err != nil {
		return domain.ConnectionInstanceLayout{}, false, err
	}
	// Match the file adapter's non-nil empty layout slices without reordering.
	if layout.Groups == nil {
		layout.Groups = []domain.ConnectionInstanceGroup{}
	}
	if layout.UngroupedConnectionInstanceIDs == nil {
		layout.UngroupedConnectionInstanceIDs = []string{}
	}
	for i := range layout.Groups {
		if layout.Groups[i].ConnectionInstanceIDs == nil {
			layout.Groups[i].ConnectionInstanceIDs = []string{}
		}
	}
	return layout, true, nil
}

func (s *Store) SaveWorkspaceLayout(ctx context.Context, id domain.AuthenticationSessionID, layout domain.ConnectionInstanceLayout, expectedRevision uint64) error {
	ctx = workspaceUploadContext(ctx)
	if err := ctx.Err(); err != nil {
		return err
	}
	row, err := newWorkspaceLayoutRow(id, layout)
	if err != nil {
		return err
	}
	if layout.Revision != expectedRevision+1 {
		return ports.ErrRevisionConflict
	}
	expected := strconv.FormatUint(expectedRevision, 10)
	db := s.DB.WithContext(ctx)
	var result *gorm.DB
	if expectedRevision == 0 {
		// Only revision one may create a row. If a stored layout has revision
		// zero, it can also be advanced, just as in the file adapter.
		result = db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "authentication_session_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"revision", "payload"}),
			Where: clause.Where{Exprs: []clause.Expression{clause.Eq{
				Column: clause.Column{Table: clause.CurrentTable, Name: "revision"}, Value: expected,
			}}},
		}).Create(&row)
	} else {
		result = db.Model(&workspaceLayoutRow{}).
			Where("authentication_session_id = ? AND revision = ?", id.String(), expected).
			Updates(map[string]any{"revision": row.Revision, "payload": row.Payload})
	}
	if result.Error != nil {
		return workspaceUploadDBError(ctx, errWorkspaceRepositoryUnavailable)
	}
	if result.RowsAffected != 1 {
		return ports.ErrRevisionConflict
	}
	return nil
}

// ImportWorkspaceLayout creates a legacy layout with its stored revision and
// never replaces a row that already exists.
func (s *Store) ImportWorkspaceLayout(ctx context.Context, id domain.AuthenticationSessionID, layout domain.ConnectionInstanceLayout) error {
	ctx = workspaceUploadContext(ctx)
	if err := ctx.Err(); err != nil {
		return err
	}
	row, err := newWorkspaceLayoutRow(id, layout)
	if err != nil {
		return err
	}
	result := s.DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "authentication_session_id"}},
		DoNothing: true,
	}).Create(&row)
	if result.Error != nil {
		return workspaceUploadDBError(ctx, errWorkspaceRepositoryUnavailable)
	}
	if result.RowsAffected != 1 {
		return ports.ErrRevisionConflict
	}
	return nil
}

func newWorkspaceLayoutRow(id domain.AuthenticationSessionID, layout domain.ConnectionInstanceLayout) (workspaceLayoutRow, error) {
	if err := domain.ValidateConnectionInstanceLayout(&layout); err != nil {
		return workspaceLayoutRow{}, err
	}
	payload, err := json.Marshal(layout)
	if err != nil {
		return workspaceLayoutRow{}, errors.New("encode workspace layout repository record")
	}
	return workspaceLayoutRow{AuthenticationSessionID: id.String(), Revision: strconv.FormatUint(layout.Revision, 10), Payload: payload}, nil
}

func workspaceUploadContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func workspaceUploadDBError(ctx context.Context, unavailable error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return unavailable
}

func decodeWorkspaceUploadPayload(data []byte, dest any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dest); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("multiple JSON values")
	}
	return nil
}
