package db

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/ben-wangz/roaminal/backend/internal/connectionoptions"
	"github.com/ben-wangz/roaminal/backend/internal/definition"
	"github.com/ben-wangz/roaminal/backend/internal/sshconfig"
)

const mutationIntentVersion = 1

type mutationRevisionRow struct {
	Exists bool   `gorm:"not null"`
	Mode   uint32 `gorm:"not null"`
	Digest []byte `gorm:"not null"`
}

type definitionMutationRow struct {
	Key           string              `gorm:"primaryKey;size:32"`
	ID            string              `gorm:"not null;size:32"`
	FormatVersion int                 `gorm:"not null"`
	ETag          string              `gorm:"not null;size:66"`
	Before        mutationRevisionRow `gorm:"embedded;embeddedPrefix:before_"`
	After         mutationRevisionRow `gorm:"embedded;embeddedPrefix:after_"`
	Updates       []byte              `gorm:"not null"`
}

func (definitionMutationRow) TableName() string { return "definition_mutation_intent" }

func encodeDefinitionMutation(intent definition.MutationIntent) (definitionMutationRow, error) {
	if !validDefinitionMutation(intent) {
		return definitionMutationRow{}, errInvalidMutationIntent
	}
	data, err := json.Marshal(intent.Updates)
	if err != nil || len(data) > connectionoptions.MaxBytes {
		return definitionMutationRow{}, errInvalidMutationIntent
	}
	return definitionMutationRow{Key: mutationKey, ID: intent.ID, FormatVersion: mutationIntentVersion,
		ETag: intent.ETag, Before: encodeMutationRevision(intent.Before), After: encodeMutationRevision(intent.After), Updates: data}, nil
}

func (row definitionMutationRow) intent() (definition.MutationIntent, error) {
	if row.FormatVersion != mutationIntentVersion || len(row.Updates) > connectionoptions.MaxBytes ||
		len(row.Before.Digest) != sha256.Size || len(row.After.Digest) != sha256.Size {
		return definition.MutationIntent{}, errInvalidMutationIntent
	}
	intent := definition.MutationIntent{ID: row.ID, ETag: row.ETag, Before: row.Before.revision(), After: row.After.revision()}
	decoder := json.NewDecoder(bytes.NewReader(row.Updates))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&intent.Updates); err != nil || !validDefinitionMutation(intent) {
		return definition.MutationIntent{}, errInvalidMutationIntent
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return definition.MutationIntent{}, errInvalidMutationIntent
	}
	return intent, nil
}

func validDefinitionMutation(intent definition.MutationIntent) bool {
	if !validMutationID(intent.ID) || len(intent.ETag) != 66 || intent.ETag[0] != '"' || intent.ETag[65] != '"' ||
		!intent.Before.Valid() || !intent.After.Valid() || !intent.After.Exists || intent.After.Mode != 0o600 || len(intent.Updates) > 2 {
		return false
	}
	if _, err := hex.DecodeString(intent.ETag[1:65]); err != nil {
		return false
	}
	seen := make(map[string]bool, len(intent.Updates))
	for _, update := range intent.Updates {
		if update.Alias == "" || !utf8.ValidString(update.Alias) || seen[update.Alias] {
			return false
		}
		seen[update.Alias] = true
		if update.Value != nil {
			value, err := connectionoptions.NormalizeAliasOption(update.Alias, *update.Value)
			if err != nil || !utf8.ValidString(value.Pwd) || !utf8.ValidString(value.SessionName) || value != *update.Value {
				return false
			}
		}
	}
	return true
}

func validMutationID(id string) bool {
	if len(id) != 32 || strings.ToLower(id) != id {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

func encodeMutationRevision(revision sshconfig.FileRevision) mutationRevisionRow {
	return mutationRevisionRow{Exists: revision.Exists, Mode: uint32(revision.Mode), Digest: revision.Digest[:]}
}

func (row mutationRevisionRow) revision() sshconfig.FileRevision {
	var digest [sha256.Size]byte
	copy(digest[:], row.Digest)
	return sshconfig.FileRevision{Exists: row.Exists, Mode: os.FileMode(row.Mode), Digest: digest}
}
