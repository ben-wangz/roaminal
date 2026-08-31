package persistence

import (
	"crypto/elliptic"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
)

const maxPushSubscriptions = 1024

type pushSubscriptionFile struct {
	FormatVersion int                             `json:"formatVersion"`
	Subscriptions []domain.PushSubscriptionRecord `json:"subscriptions"`
}

func (s *Store) pushSubscriptionsPath() string {
	return filepath.Join(s.Root, "push-subscriptions.json")
}

func emptyPushSubscriptionFile() pushSubscriptionFile {
	return pushSubscriptionFile{FormatVersion: StorageSchemaVersion, Subscriptions: []domain.PushSubscriptionRecord{}}
}

func (s *Store) loadPushSubscriptions() (pushSubscriptionFile, error) {
	data, err := os.ReadFile(s.pushSubscriptionsPath())
	if errors.Is(err, os.ErrNotExist) {
		return emptyPushSubscriptionFile(), nil
	}
	if err != nil {
		return pushSubscriptionFile{}, fmt.Errorf("read push subscription repository: %w", err)
	}
	var file pushSubscriptionFile
	if err := decodeStrict(data, &file); err != nil {
		return pushSubscriptionFile{}, fmt.Errorf("decode push subscription repository: %w", err)
	}
	if err := validatePushSubscriptionFile(file); err != nil {
		return pushSubscriptionFile{}, fmt.Errorf("validate push subscription repository: %w", err)
	}
	return file, nil
}

func (s *Store) savePushSubscriptions(file pushSubscriptionFile) error {
	file.FormatVersion = StorageSchemaVersion
	if err := validatePushSubscriptionFile(file); err != nil {
		return s.markError(err)
	}
	data, err := encodeJSON(file)
	if err != nil {
		return s.markError(err)
	}
	if err := s.atomicWrite(s.pushSubscriptionsPath(), append(data, '\n')); err != nil {
		return s.markError(err)
	}
	return nil
}

func (s *Store) initializePushSubscriptions() error {
	s.pushMu.Lock()
	defer s.pushMu.Unlock()
	if _, err := s.loadPushSubscriptions(); err != nil {
		return s.markError(err)
	}
	return nil
}

func validatePushSubscriptionFile(file pushSubscriptionFile) error {
	if file.FormatVersion != StorageSchemaVersion {
		return fmt.Errorf("unsupported push subscription schema version %d", file.FormatVersion)
	}
	if len(file.Subscriptions) > maxPushSubscriptions {
		return errors.New("push subscription retention limit exceeded")
	}
	seenIDs := make(map[string]struct{}, len(file.Subscriptions))
	seenEndpoints := make(map[string]struct{}, len(file.Subscriptions))
	for _, record := range file.Subscriptions {
		if !uuidPattern.MatchString(record.ID) {
			return errors.New("push subscription id is invalid")
		}
		if !uuidPattern.MatchString(record.AuthenticationSessionID) {
			return errors.New("push subscription login session id is invalid")
		}
		if _, exists := seenIDs[record.ID]; exists {
			return errors.New("duplicate push subscription id")
		}
		seenIDs[record.ID] = struct{}{}
		if _, exists := seenEndpoints[record.Endpoint]; exists {
			return errors.New("duplicate push subscription endpoint")
		}
		seenEndpoints[record.Endpoint] = struct{}{}
		if err := validatePushSubscriptionValues(record.Endpoint, record.AuthKey, record.P256dhKey); err != nil {
			return err
		}
		if record.CreatedAt.IsZero() || record.UpdatedAt.IsZero() || record.UpdatedAt.Before(record.CreatedAt) {
			return errors.New("push subscription timestamps are invalid")
		}
	}
	return nil
}

func validatePushSubscriptionValues(endpoint, authKey, p256dhKey string) error {
	endpoint = strings.TrimSpace(endpoint)
	if len(endpoint) == 0 || len([]byte(endpoint)) > 4096 {
		return errors.New("push subscription endpoint is invalid")
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" {
		return errors.New("push subscription endpoint must be an HTTPS URL")
	}
	if len(authKey) == 0 || len(p256dhKey) == 0 {
		return errors.New("push subscription keys are incomplete")
	}
	authBytes, err := base64.RawURLEncoding.DecodeString(authKey)
	if err != nil || len(authBytes) != 16 {
		return errors.New("push subscription auth key is invalid")
	}
	p256dhBytes, err := base64.RawURLEncoding.DecodeString(p256dhKey)
	if err != nil || len(p256dhBytes) != 65 || p256dhBytes[0] != 4 {
		return errors.New("push subscription p256dh key is invalid")
	}
	if x, y := elliptic.Unmarshal(elliptic.P256(), p256dhBytes); x == nil || y == nil {
		return errors.New("push subscription p256dh key is not a valid P-256 point")
	}
	return nil
}

func clonePushSubscription(record domain.PushSubscriptionRecord) domain.PushSubscriptionRecord {
	return record
}
