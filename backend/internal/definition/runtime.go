package definition

import (
	"errors"
	"fmt"
	"os"

	"github.com/ben-wangz/roaminal/backend/internal/connectionoptions"
	"github.com/ben-wangz/roaminal/backend/internal/sshconfig"
	"github.com/ben-wangz/roaminal/backend/internal/sshfs"
)

var ErrUnavailable = errors.New("connection definition temporarily unavailable")

// RuntimeSSH is the option and effective OpenSSH revision observed together.
type RuntimeSSH struct {
	Option         connectionoptions.Tmux
	SourceRevision string
}

// NativeCollection is the transport watcher's view. It does not use add-on
// options, but must not observe a file replacement awaiting its SQL commit.
func (s *Service) NativeCollection() (sshconfig.Collection, error) {
	if !s.Available() {
		return sshconfig.Collection{}, sshfs.ErrUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.nativeCollectionLocked(nil)
}

func (s *Service) nativeCollectionLocked(knownKeys map[string]bool) (sshconfig.Collection, error) {
	if err := s.recoverPendingMutation(); err != nil {
		return sshconfig.Collection{}, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	return s.configRepo.Collection(knownKeys)
}

// Load provides coordinated options for already live connection instances.
// Their historical aliases remain usable after a source edit or removal; a
// passed-in alias is not checked against the current native config collection.
func (s *Service) Load(aliases map[string]bool) (connectionoptions.Collection, error) {
	if !s.Available() || s.options == nil {
		return connectionoptions.Collection{}, connectionoptions.ErrOptionsUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.recoverPendingMutation(); err != nil {
		return connectionoptions.Collection{}, fmt.Errorf("%w: %w", connectionoptions.ErrOptionsUnavailable, err)
	}
	return s.options.Load(aliases)
}

// ResolveSSH reads the definition, options and effective revision under the
// mutation lock. The fingerprint callback may run a bounded OpenSSH probe but
// must not reenter the definition service or start a connection instance.
func (s *Service) ResolveSSH(alias string, fingerprint func(string) (string, error)) (RuntimeSSH, error) {
	if !s.Available() {
		return RuntimeSSH{}, ErrUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	collection, err := s.nativeCollectionLocked(nil)
	if err != nil {
		return RuntimeSSH{}, err
	}
	if !collection.ConfigSource.Readable && collection.ConfigSource.Status != "missing" {
		return RuntimeSSH{}, ErrUnavailable
	}
	if _, ok := findSSHDefinition(collection, alias); !ok {
		return RuntimeSSH{}, os.ErrNotExist
	}
	var option connectionoptions.Tmux
	if s.options != nil {
		loaded, err := s.options.Load(map[string]bool{alias: true})
		if err != nil {
			return RuntimeSSH{}, fmt.Errorf("%w: %w", ErrUnavailable, err)
		}
		option = loaded.Options[alias]
	}
	revision, err := fingerprint(alias)
	if err != nil {
		return RuntimeSSH{}, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	// The lock covers our own writes, not a user's external edits to the native
	// file. Reject a visible change across the effective-config probe as well.
	if err := s.checkNativeETag(collection.ETag); err != nil {
		return RuntimeSSH{}, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	return RuntimeSSH{Option: option, SourceRevision: revision}, nil
}

func (s *Service) checkNativeETag(expected string) error {
	_, etag, _, err := s.configRepo.Read(nil)
	if err == nil && etag != expected {
		return sshconfig.ErrPreconditionFailed
	}
	return err
}

func findSSHDefinition(collection sshconfig.Collection, alias string) (sshconfig.Definition, bool) {
	for _, item := range collection.Definitions {
		if item.Type == "ssh" && item.HostAlias == alias {
			return item, true
		}
	}
	return sshconfig.Definition{}, false
}
