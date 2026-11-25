package db

import (
	"bytes"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/persistence"
	"gorm.io/gorm"
)

type transferState struct {
	enrollmentID string
	latest       messageCounter
	lastMessage  messageCounter
}

var transferTables = []transferTable{
	transferRows[enrollmentRow]{order: "key", maxRows: 1, validate: validateTransferEnrollment},
	transferRows[authRow]{order: "id", validate: validateTransferAuth},
	transferRows[workspaceLayoutRow]{order: "authentication_session_id", validate: validateTransferWorkspace},
	transferRows[uploadRow]{
		order: "id", validate: validateTransferUpload,
		blob: func(r *uploadRow) *[]byte { return &r.Payload }, blobLimit: transferUploadLimit,
		selectRow: func(db *gorm.DB) *gorm.DB {
			return db.Select("id, CASE WHEN length(payload) <= ? THEN payload END AS payload", transferUploadLimit)
		},
	},
	transferRows[instanceRow]{order: "id", validate: validateTransferInstance},
	transferRows[instanceSnapshotRow]{
		order: "id", validate: validateTransferSnapshot,
		blob: func(r *instanceSnapshotRow) *[]byte { return &r.Data }, blobLimit: encodedSnapshotLimit,
		selectRow: func(db *gorm.DB) *gorm.DB {
			return db.Select("id, CASE WHEN length(data) <= ? THEN data END AS data", encodedSnapshotLimit)
		},
	},
	transferRows[instanceAuditRow]{
		order: "id", validate: validateTransferAudit,
		blob: func(r *instanceAuditRow) *[]byte { return &r.Snapshot }, blobLimit: encodedSnapshotLimit,
		selectRow: func(db *gorm.DB) *gorm.DB {
			// Distinguish oversized data from a deliberately absent snapshot.
			return db.Select("id, metadata, CASE WHEN length(snapshot) > ? THEN ? ELSE snapshot END AS snapshot", encodedSnapshotLimit, []byte("oversized"))
		},
	},
	transferRows[messageStateRow]{
		order: "id", numericOrder: true, maxRows: 1, singleton: true, validate: validateTransferMessageState,
		insert: func(db *gorm.DB, r *messageStateRow) error { return saveMessageState(db, *r) },
	},
	transferRows[messageRow]{order: "sequence", maxRows: persistence.MaxStoredMessages, validate: validateTransferMessage},
	// Position is an engine-local generated key. Transfer the list in that
	// order, and allow the target to generate fresh positions on insert.
	transferRows[pushSubscriptionRow]{
		order: "position", numericOrder: true, maxRows: persistence.MaxPushSubscriptions,
		validate: validateTransferPush, selectRow: omitTransferPosition,
	},
	transferRows[notificationPreferenceRow]{
		order: "position", numericOrder: true, maxRows: persistence.MaxNotificationPreferences,
		validate: validateTransferPreference, selectRow: omitTransferPosition,
	},
	transferRows[agentEndpointRow]{order: "key", validate: validateTransferAgent},
	transferRows[connectionOptionRow]{order: "key", validate: validateTransferOption},
}

func omitTransferPosition(db *gorm.DB) *gorm.DB { return db.Omit("Position") }

func validateTransferEnrollment(row *enrollmentRow, state *transferState) bool {
	record := domain.TOTPEnrollmentRecord{FormatVersion: row.FormatVersion, EnrollmentID: row.EnrollmentID,
		Secret: row.Secret, LastAcceptedTimeStep: row.LastAcceptedTimeStep}
	if row.Key != primaryKey || validateEnrollment(record) != nil {
		return false
	}
	state.enrollmentID = row.EnrollmentID
	return true
}

func validateTransferAuth(row *authRow, state *transferState) bool {
	record := domain.AuthSessionRecord{ID: row.ID, PasswordFingerprint: row.PasswordFingerprint,
		EnrollmentID: row.EnrollmentID, RefreshTokenHash: row.RefreshTokenHash, UserAgent: row.UserAgent,
		CreatedAt: row.CreatedAt, LastSeenAt: row.LastSeenAt, RefreshExpiresAt: row.RefreshExpiresAt, RotatedAt: row.RotatedAt}
	if state.enrollmentID == "" || row.EnrollmentID != state.enrollmentID || !validTransferText(row.UserAgent) ||
		!restoreAuthPrecision(row.Submicroseconds, &record) || !validAuth(record) {
		return false
	}
	// Canonicalize v1 SQLite timestamps too, so the destination never rounds
	// away nanoseconds when sending them to PostgreSQL.
	*row = encodeAuthRow(record)
	return true
}

func validateTransferWorkspace(row *workspaceLayoutRow, _ *transferState) bool {
	var record domain.ConnectionInstanceLayout
	return validTransferText(row.AuthenticationSessionID) && decodeTransferJSON(row.Payload, &record) &&
		row.Revision == strconv.FormatUint(record.Revision, 10) && domain.ValidateConnectionInstanceLayout(&record) == nil
}

func validateTransferUpload(row *uploadRow, _ *transferState) bool {
	var record domain.UploadJobRecord
	if len(row.Payload) > transferUploadLimit || !decodeTransferJSON(row.Payload, &record) ||
		row.ID != record.ID || validateUploadRecord(record) != nil || record.StagingPath != "" {
		return false
	}
	for _, file := range record.Files {
		if file.StagedPath != "" {
			return false
		}
	}
	return true
}

func validateTransferInstance(row *instanceRow, _ *transferState) bool {
	_, err := decodeInstanceMetadata(*row)
	return err == nil
}

func validTransferSnapshot(data []byte) bool {
	_, _, err := persistence.DecodeSnapshot(data)
	return err == nil
}

func validateTransferSnapshot(row *instanceSnapshotRow, _ *transferState) bool {
	return instanceIDPattern.MatchString(row.ID) && validTransferSnapshot(row.Data)
}

func validateTransferAudit(row *instanceAuditRow, _ *transferState) bool {
	return validateTransferInstance(&instanceRow{ID: row.ID, Metadata: row.Metadata}, nil) &&
		(row.Snapshot == nil || validTransferSnapshot(row.Snapshot))
}

func validateTransferMessageState(row *messageStateRow, state *transferState) bool {
	state.latest = row.LatestSequence
	return row.ID == messageStateID && row.ReadThroughSequence <= row.LatestSequence
}

func validateTransferMessage(row *messageRow, state *transferState) bool {
	if row.Sequence <= state.lastMessage || row.Sequence > state.latest || persistence.ValidateMessageDraft(row.Draft) != nil ||
		!bytes.Equal(row.IDDigest, stringDigest(row.Draft.MessageID)) || !bytes.Equal(row.IdempotencyDigest, stringDigest(row.Draft.IdempotencyKey)) ||
		row.ReceivedSecond != row.Draft.ReceivedAt.Unix() || row.ReceivedNano != row.Draft.ReceivedAt.Nanosecond() {
		return false
	}
	state.lastMessage = row.Sequence
	return true
}

func validateTransferPush(row *pushSubscriptionRow, _ *transferState) bool {
	return row.Position == 0 && bytes.Equal(row.EndpointDigest, stringDigest(string(row.Endpoint))) &&
		persistence.ValidatePushSubscription(row.record()) == nil
}

func validateTransferPreference(row *notificationPreferenceRow, _ *transferState) bool {
	return row.Position == 0 && persistence.ValidateNotificationPreference(row.record()) == nil
}

func validateTransferAgent(row *agentEndpointRow, _ *transferState) bool {
	_, err := row.record()
	return err == nil
}

func validateTransferOption(row *connectionOptionRow, _ *transferState) bool {
	_, err := row.option()
	return err == nil
}

func validTransferText(value string) bool {
	return utf8.ValidString(value) && !strings.ContainsRune(value, '\x00')
}
