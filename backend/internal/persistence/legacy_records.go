package persistence

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/ben-wangz/roaminal/backend/internal/agent"
	"github.com/ben-wangz/roaminal/backend/internal/domain"
)

func (r legacyReader) readRecords(root string, out *LegacyData) error {
	var messages messageFile
	if err := r.readJSON(root, "messages.json", &messages, legacyJSONMax, true, func() error { return validateMessageFile(messages) }); err != nil {
		return err
	}
	out.Messages = messages.Messages
	out.MessageState = domain.MessageState{LatestSequence: messages.LatestSequence, Revision: messages.Revision, ReadThroughSequence: messages.ReadThroughSequence}
	for _, message := range messages.Messages {
		if message.Sequence > messages.ReadThroughSequence {
			out.MessageState.UnreadCount++
		}
	}
	var push pushSubscriptionFile
	if err := r.readJSON(root, "push-subscriptions.json", &push, legacyJSONMax, true, func() error { return validatePushSubscriptionFile(push) }); err != nil {
		return err
	}
	out.Subscriptions = push.Subscriptions
	var preferences notificationPreferenceFile
	if err := r.readJSON(root, "notification-preferences.json", &preferences, legacyJSONMax, true, func() error { return validateNotificationPreferenceFile(preferences) }); err != nil {
		return err
	}
	out.Preferences = preferences.Preferences
	data, err := r.read(filepath.Join(root, "agent-endpoints.json"), legacyJSONMax, true)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return errors.New("unreadable legacy agent-endpoints.json")
	}
	out.Endpoints, err = agent.DecodeLegacyEndpoints(data)
	return err
}

func (r legacyReader) readUploads(root string, out *LegacyData) error {
	dir := filepath.Join(root, "uploads")
	entries, err := listLegacyDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		if isUploadStaging(entry) {
			continue
		}
		id, ok := strings.CutSuffix(name, ".json")
		if !ok || !uploadIDPattern.MatchString(id) {
			return errors.New("unexpected legacy upload artifact")
		}
		data, err := r.read(filepath.Join(dir, name), legacyJSONMax, true)
		var file uploadRecordFile
		if err != nil || decodeStrict(data, &file) != nil || file.FormatVersion != StorageSchemaVersion || file.Record.ID != id || validateUploadRecord(file.Record) != nil {
			return errors.New("invalid legacy upload record")
		}
		record := file.Record
		domain.InterruptUpload(&record)
		record.StagingPath = ""
		for i := range record.Files {
			record.Files[i].StagedPath = ""
		}
		out.Uploads = append(out.Uploads, record)
	}
	return nil
}

func isUploadStaging(entry os.DirEntry) bool {
	return entry.IsDir() && strings.HasPrefix(entry.Name(), "upload-")
}
