package definition

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/ben-wangz/roaminal/backend/internal/connectionoptions"
	"github.com/ben-wangz/roaminal/backend/internal/sshconfig"
)

var ErrMutationConflict = errors.New("SSH config does not match either revision of the pending connection-definition mutation")

// OptionUpdate replaces one alias's options, or removes them if Value is nil.
type OptionUpdate struct {
	Alias string                  `json:"alias"`
	Value *connectionoptions.Tmux `json:"value,omitempty"`
}

// MutationIntent pairs exact SSH file revisions with the option changes that
// must be finalized if the replacement was published. ETag is the client's
// precondition; the stable file revisions are used after a process restart.
type MutationIntent struct {
	ID      string
	ETag    string
	Before  sshconfig.FileRevision
	After   sshconfig.FileRevision
	Updates []OptionUpdate
}

// MutationRepository persists an intent before the SSH write. Resolve must
// serialize resolution for that intent. It clears the intent in the same
// transaction as applying its option changes (when decide returns true).
// An error leaves the intent pending. A nil decide discards without applying.
type MutationRepository interface {
	Pending() (MutationIntent, bool, error)
	Prepare(MutationIntent) error
	Resolve(id string, decide func() (bool, error)) error
}

func (s *Service) recoverPendingMutation() error {
	if s.mutations == nil {
		return nil
	}
	intent, exists, err := s.mutations.Pending()
	if err != nil || !exists {
		return err
	}
	return s.mutations.Resolve(intent.ID, func() (bool, error) {
		live, err := s.configRepo.MutationRevision()
		if err != nil {
			return false, err
		}
		switch live {
		case intent.Before:
			// Also covers identical revisions: an options-only edit is
			// visible if and only if its database transaction committed.
			return false, nil
		case intent.After:
			return true, s.configRepo.SyncMutationRevision(intent.After)
		default:
			return false, ErrMutationConflict
		}
	})
}

func (s *Service) mutateSQLLocked(ifMatch string, change sshconfig.Change, options optionsChange, edit *sshconfig.Edit) (sshconfig.Collection, error) {
	if strings.TrimSpace(ifMatch) == "" {
		return sshconfig.Collection{}, sshconfig.ErrPreconditionRequired
	}
	if err := s.recoverPendingMutation(); err != nil {
		return sshconfig.Collection{}, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	keys := s.knownKeys()
	collection, err := s.configRepo.Collection(keys)
	if err != nil {
		return sshconfig.Collection{}, err
	}
	nativeETag := collection.ETag
	current, err := s.enrich(&collection)
	if err != nil {
		return sshconfig.Collection{}, fmt.Errorf("%w: %w", connectionoptions.ErrOptionsUnavailable, err)
	}
	if collection.ETag != ifMatch {
		return sshconfig.Collection{}, sshconfig.ErrPreconditionFailed
	}
	updates, err := options.prepare(current)
	if err != nil {
		return sshconfig.Collection{}, err
	}
	if edit != nil && options.patches() && options.alias == options.destination && collection.ConfigSource.Readable {
		item, ok := findSSHDefinition(collection, options.alias)
		if !ok {
			return sshconfig.Collection{}, os.ErrNotExist
		}
		if sameNativeFields(*edit, item) {
			if err := s.checkNativeETag(nativeETag); err != nil {
				return sshconfig.Collection{}, err
			}
			update := updates[0]
			var value connectionoptions.Tmux
			if update.Value != nil {
				value = *update.Value
			}
			if err := s.options.UpdateAlias(update.Alias, value, update.Value != nil); err != nil {
				return sshconfig.Collection{}, fmt.Errorf("%w: %w", connectionoptions.ErrOptionsUnavailable, err)
			}
			return s.configRepo.Collection(keys)
		}
	}
	plan, err := s.configRepo.PrepareMutation(nativeETag, keys, change)
	if err != nil {
		return sshconfig.Collection{}, err
	}
	if err := s.publishMutation(ifMatch, plan, updates); err != nil {
		return sshconfig.Collection{}, err
	}
	return s.configRepo.Collection(keys)
}

func (s *Service) publishMutation(ifMatch string, plan sshconfig.PreparedMutation, updates []OptionUpdate) error {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return fmt.Errorf("generate definition mutation id: %w", err)
	}
	intent := MutationIntent{ID: hex.EncodeToString(raw[:]), ETag: ifMatch, Before: plan.Before, After: plan.After, Updates: updates}
	if err := s.mutations.Prepare(intent); err != nil {
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	var unpublished bool
	err := s.mutations.Resolve(intent.ID, func() (bool, error) {
		published, err := s.configRepo.ApplyMutation(plan)
		unpublished = err != nil && !published
		return err == nil, err
	})
	if err != nil {
		if unpublished {
			// No file change was made; even a concurrent external edit is
			// not an unfinished local mutation in this case.
			return errors.Join(err, s.mutations.Resolve(intent.ID, nil))
		}
		return fmt.Errorf("%w: %w", ErrUnavailable, errors.Join(err, s.recoverPendingMutation()))
	}
	return nil
}
