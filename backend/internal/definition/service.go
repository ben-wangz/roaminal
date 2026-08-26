package definition

import (
	"fmt"
	"sync"

	"github.com/ben-wangz/roaminal/backend/internal/connectionoptions"
	"github.com/ben-wangz/roaminal/backend/internal/sshconfig"
	"github.com/ben-wangz/roaminal/backend/internal/sshfs"
	"github.com/ben-wangz/roaminal/backend/internal/sshkey"
)

// Service owns connection-definition source precedence, SSH key references,
// and the persisted tmux/FileSystem options associated with a definition.
// HTTP handlers receive complete collections but do not access these adapters.
type Service struct {
	configRepo  *sshconfig.Repository
	keys        *sshkey.Inventory
	options     *connectionoptions.Store
	journalPath string
	mu          sync.Mutex
}

func New(configRepo *sshconfig.Repository, keys *sshkey.Inventory, options *connectionoptions.Store) (*Service, error) {
	service := &Service{configRepo: configRepo, keys: keys, options: options, journalPath: definitionJournalPath(options)}
	if err := service.recoverPendingTransaction(); err != nil {
		return nil, fmt.Errorf("recover connection-definition mutation: %w", err)
	}
	return service, nil
}

func (s *Service) Available() bool { return s != nil && s.configRepo != nil }

func (s *Service) Collection() (sshconfig.Collection, error) {
	if !s.Available() {
		return sshconfig.Collection{}, sshfs.ErrUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	collection, err := s.configRepo.Collection(s.knownKeys())
	if err != nil {
		return sshconfig.Collection{}, err
	}
	s.enrich(&collection)
	return collection, nil
}

func (s *Service) Create(ifMatch string, edit sshconfig.Edit, tmux *sshconfig.TmuxOptions, filesystem *sshconfig.FileSystemOptions) (sshconfig.Collection, error) {
	if !s.Available() {
		return sshconfig.Collection{}, sshfs.ErrUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	configSnapshot, optionsSnapshot, err := s.snapshots(s.options != nil)
	if err != nil {
		return sshconfig.Collection{}, err
	}
	transaction, err := s.beginTransaction(configSnapshot, optionsSnapshot)
	if err != nil {
		return sshconfig.Collection{}, err
	}
	collection, err := s.configRepo.Create(ifMatch, s.knownKeys(), edit)
	if err != nil {
		return sshconfig.Collection{}, s.abortTransaction(transaction, err)
	}
	if tmux != nil || filesystem != nil {
		if err := s.SaveOptions(edit.HostAlias, tmux, filesystem); err != nil {
			return sshconfig.Collection{}, s.abortTransaction(transaction, err)
		}
	}
	if err := s.completeTransaction(transaction); err != nil {
		return sshconfig.Collection{}, err
	}
	s.enrich(&collection)
	return collection, nil
}

func (s *Service) Update(ifMatch, alias string, edit sshconfig.Edit, tmux *sshconfig.TmuxOptions, filesystem *sshconfig.FileSystemOptions) (sshconfig.Collection, error) {
	if !s.Available() {
		return sshconfig.Collection{}, sshfs.ErrUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	newAlias := edit.HostAlias
	if newAlias == "" {
		newAlias = alias
	}
	configSnapshot, optionsSnapshot, err := s.snapshots(s.options != nil)
	if err != nil {
		return sshconfig.Collection{}, err
	}
	transaction, err := s.beginTransaction(configSnapshot, optionsSnapshot)
	if err != nil {
		return sshconfig.Collection{}, err
	}
	collection, err := s.configRepo.Update(ifMatch, s.knownKeys(), alias, edit)
	if err != nil {
		return sshconfig.Collection{}, s.abortTransaction(transaction, err)
	}
	if tmux != nil || filesystem != nil {
		if err := s.SaveOptions(newAlias, tmux, filesystem); err != nil {
			return sshconfig.Collection{}, s.abortTransaction(transaction, err)
		}
		if s.options != nil && newAlias != alias {
			if err := s.options.RemoveAlias(alias); err != nil {
				return sshconfig.Collection{}, s.abortTransaction(transaction, err)
			}
		}
	} else if s.options != nil && newAlias != alias {
		if err := s.options.MoveAlias(alias, newAlias); err != nil {
			return sshconfig.Collection{}, s.abortTransaction(transaction, err)
		}
	}
	if err := s.completeTransaction(transaction); err != nil {
		return sshconfig.Collection{}, err
	}
	s.enrich(&collection)
	return collection, nil
}

func (s *Service) Duplicate(ifMatch, alias, newAlias string) (sshconfig.Collection, error) {
	if !s.Available() {
		return sshconfig.Collection{}, sshfs.ErrUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	configSnapshot, optionsSnapshot, err := s.snapshots(s.options != nil)
	if err != nil {
		return sshconfig.Collection{}, err
	}
	transaction, err := s.beginTransaction(configSnapshot, optionsSnapshot)
	if err != nil {
		return sshconfig.Collection{}, err
	}
	collection, err := s.configRepo.Duplicate(ifMatch, s.knownKeys(), alias, newAlias)
	if err != nil {
		return sshconfig.Collection{}, s.abortTransaction(transaction, err)
	}
	if s.options != nil {
		if err := s.options.CopyAlias(alias, newAlias); err != nil {
			return sshconfig.Collection{}, s.abortTransaction(transaction, err)
		}
	}
	if err := s.completeTransaction(transaction); err != nil {
		return sshconfig.Collection{}, err
	}
	s.enrich(&collection)
	return collection, nil
}

func (s *Service) Delete(ifMatch, alias string) (sshconfig.Collection, error) {
	if !s.Available() {
		return sshconfig.Collection{}, sshfs.ErrUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	configSnapshot, optionsSnapshot, err := s.snapshots(s.options != nil)
	if err != nil {
		return sshconfig.Collection{}, err
	}
	transaction, err := s.beginTransaction(configSnapshot, optionsSnapshot)
	if err != nil {
		return sshconfig.Collection{}, err
	}
	collection, err := s.configRepo.Delete(ifMatch, s.knownKeys(), alias)
	if err != nil {
		return sshconfig.Collection{}, s.abortTransaction(transaction, err)
	}
	if s.options != nil {
		if err := s.options.RemoveAlias(alias); err != nil {
			return sshconfig.Collection{}, s.abortTransaction(transaction, err)
		}
	}
	if err := s.completeTransaction(transaction); err != nil {
		return sshconfig.Collection{}, err
	}
	s.enrich(&collection)
	return collection, nil
}

func (s *Service) snapshots(withOptions bool) (sshconfig.Snapshot, connectionoptions.Snapshot, error) {
	configSnapshot, err := s.configRepo.Snapshot(s.knownKeys())
	if err != nil {
		return sshconfig.Snapshot{}, connectionoptions.Snapshot{}, err
	}
	if !withOptions || s.options == nil {
		return configSnapshot, connectionoptions.Snapshot{}, nil
	}
	optionsSnapshot, err := s.options.Snapshot()
	if err != nil {
		return sshconfig.Snapshot{}, connectionoptions.Snapshot{}, err
	}
	return configSnapshot, optionsSnapshot, nil
}
