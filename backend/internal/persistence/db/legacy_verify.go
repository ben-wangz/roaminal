package db

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"sort"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/persistence"
)

func (s *Store) verifyLegacyRows(ctx context.Context, data persistence.LegacyData) error {
	if !s.matchLegacyAuth(ctx, data) || !s.matchLegacyNotifications(ctx, data) || !s.matchLegacyOther(ctx, data) || !s.matchLegacyInstances(ctx, data.Instances) {
		return errLegacyVerify
	}
	return nil
}

func (s *Store) countMatches(model any, want int) bool {
	var count int64
	return s.DB.Model(model).Count(&count).Error == nil && count == int64(want)
}

func (s *Store) matchLegacyAuth(ctx context.Context, data persistence.LegacyData) bool {
	enrollment, exists, err := s.LoadEnrollment(ctx)
	count := 0
	if data.Enrollment != nil {
		count++
	}
	if err != nil || exists != (data.Enrollment != nil) || (exists && enrollment != *data.Enrollment) || !s.countMatches(&enrollmentRow{}, count) {
		return false
	}
	auth, err := s.LoadAuth(ctx)
	if err != nil || len(auth) != len(data.Auth) || !s.countMatches(&workspaceLayoutRow{}, len(data.Layouts)) {
		return false
	}
	want := append([]domain.AuthSessionRecord(nil), data.Auth...)
	sort.Slice(want, func(i, j int) bool { return want[i].ID < want[j].ID })
	for i, r := range want {
		r.CreatedAt, r.LastSeenAt = r.CreatedAt.UTC(), r.LastSeenAt.UTC()
		r.RefreshExpiresAt, r.RotatedAt = r.RefreshExpiresAt.UTC(), r.RotatedAt.UTC()
		if auth[i] != r {
			return false
		}
	}
	for id, layout := range data.Layouts {
		row, err := newWorkspaceLayoutRow(domain.AuthenticationSessionID(id), layout)
		var stored workspaceLayoutRow
		if err != nil || s.DB.Take(&stored, &workspaceLayoutRow{AuthenticationSessionID: id}).Error != nil || stored.Revision != row.Revision || !bytes.Equal(stored.Payload, row.Payload) {
			return false
		}
	}
	return true
}

func (s *Store) matchLegacyNotifications(ctx context.Context, data persistence.LegacyData) bool {
	state, err := lockMessageState(s.DB)
	if err != nil || !s.countMatches(&messageStateRow{}, 1) {
		return false
	}
	actual, err := readMessageState(s.DB, state)
	if err != nil || actual != data.MessageState {
		return false
	}
	var messages []messageRow
	if s.DB.Order("sequence").Find(&messages).Error != nil || len(messages) != len(data.Messages) {
		return false
	}
	for i, row := range messages {
		want := newMessageRow(data.Messages[i])
		if row.Sequence != want.Sequence || row.ReceivedSecond != want.ReceivedSecond || row.ReceivedNano != want.ReceivedNano ||
			!bytes.Equal(row.IDDigest, want.IDDigest) || !bytes.Equal(row.IdempotencyDigest, want.IdempotencyDigest) || !sameJSON(row.Draft, want.Draft) {
			return false
		}
	}
	push, err := s.ListPushSubscriptions(ctx)
	if err != nil || len(push) != len(data.Subscriptions) || !s.countMatches(&notificationPreferenceRow{}, len(data.Preferences)) {
		return false
	}
	for i := range push {
		if !sameJSON(push[i], data.Subscriptions[i]) {
			return false
		}
	}
	for _, value := range data.Preferences {
		actual, ok, err := s.GetNotificationPreference(ctx, value.UserKey, value.ConnectionDefinitionID, value.TmuxSessionName)
		if err != nil || !ok || !sameJSON(actual, value) {
			return false
		}
	}
	return true
}

func (s *Store) matchLegacyOther(ctx context.Context, data persistence.LegacyData) bool {
	if !s.countMatches(&agentEndpointRow{}, len(data.Endpoints)) || !s.countMatches(&uploadRow{}, len(data.Uploads)) || !s.countMatches(&definitionMutationRow{}, 0) {
		return false
	}
	for id, endpoint := range data.Endpoints {
		var row agentEndpointRow
		if s.DB.Take(&row, &agentEndpointRow{Key: id}).Error != nil {
			return false
		}
		actual, err := row.record()
		if err != nil || !sameJSON(actual, endpoint) {
			return false
		}
	}
	options, err := s.ConnectionOptions().Load(nil)
	if err != nil || len(options.Options) != len(data.Options) {
		return false
	}
	for alias, option := range data.Options {
		row, err := newConnectionOptionRow(alias, option)
		actual, ok := options.Options[alias]
		if err != nil || !ok || actual.Enabled != row.Enabled || actual.Pwd != row.Pwd || actual.SessionName != string(row.SessionName) {
			return false
		}
	}
	for _, upload := range data.Uploads {
		actual, err := s.LoadUpload(ctx, upload.ID)
		if err != nil || !sameJSON(actual, upload) {
			return false
		}
	}
	return true
}

func (s *Store) matchLegacyInstances(ctx context.Context, instances []persistence.LegacyInstance) bool {
	var active, archived, snapshots int
	for _, want := range instances {
		id := domain.ConnectionInstanceID(want.Metadata.ID)
		if want.Archived {
			archived++
			if !s.matchLegacyArchive(want) {
				return false
			}
			continue
		}
		active++
		meta, err := s.LoadConnectionInstance(ctx, id)
		if err != nil || !sameJSON(meta, want.Metadata) {
			return false
		}
		header, _, err := s.LoadSnapshot(ctx, id)
		if want.Header == nil {
			if !errors.Is(err, os.ErrNotExist) {
				return false
			}
		} else if err != nil || header != *want.Header {
			return false
		} else {
			snapshots++
		}
	}
	return s.countMatches(&instanceRow{}, active) && s.countMatches(&instanceSnapshotRow{}, snapshots) && s.countMatches(&instanceAuditRow{}, archived)
}

func (s *Store) matchLegacyArchive(want persistence.LegacyInstance) bool {
	var row instanceAuditRow
	if s.DB.Select("id, metadata").Take(&row, &instanceAuditRow{ID: want.Metadata.ID}).Error != nil {
		return false
	}
	meta, err := decodeInstanceMetadata(instanceRow{ID: row.ID, Metadata: row.Metadata})
	if err != nil || !sameJSON(meta, want.Metadata) {
		return false
	}
	// Select one bounded blob at a time; keep NULL distinct from oversized data.
	var snapshot struct {
		HasData bool
		Data    []byte
	}
	if s.DB.Model(&instanceAuditRow{}).Select("snapshot IS NOT NULL AS has_data, CASE WHEN octet_length(snapshot) <= ? THEN snapshot END AS data", encodedSnapshotLimit).
		Where("id = ?", row.ID).Take(&snapshot).Error != nil {
		return false
	}
	if want.Header == nil {
		return !snapshot.HasData
	}
	header, _, err := persistence.DecodeSnapshot(snapshot.Data)
	return err == nil && header == *want.Header
}

func sameJSON(a, b any) bool {
	x, errA := json.Marshal(a)
	y, errB := json.Marshal(b)
	return errA == nil && errB == nil && bytes.Equal(x, y)
}
