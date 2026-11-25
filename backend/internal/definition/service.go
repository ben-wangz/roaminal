package definition

import (
	"errors"
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
	configRepo    *sshconfig.Repository
	keys          *sshkey.Inventory
	options       connectionoptions.Repository
	legacyOptions *connectionoptions.Store
	journalPath   string
	mutations     MutationRepository
	mu            sync.Mutex
}

// New uses the legacy options file and filesystem journal.
func New(configRepo *sshconfig.Repository, keys *sshkey.Inventory, options *connectionoptions.Store) (*Service, error) {
	service := &Service{configRepo: configRepo, keys: keys, legacyOptions: options, journalPath: definitionJournalPath(options)}
	if options != nil {
		service.options = options
	}
	if err := service.recoverPendingTransaction(); err != nil {
		return nil, fmt.Errorf("recover connection-definition mutation: %w", err)
	}
	return service, nil
}

// NewWithRepository uses options and the mutation repository from the same
// database. An unresolved mutation must be reconciled before serving requests.
func NewWithRepository(configRepo *sshconfig.Repository, keys *sshkey.Inventory, options connectionoptions.Repository, mutations MutationRepository) (*Service, error) {
	if configRepo == nil || options == nil || mutations == nil {
		return nil, errors.New("connection-definition repositories are required")
	}
	service := &Service{configRepo: configRepo, keys: keys, options: options, mutations: mutations}
	if err := service.recoverPendingMutation(); err != nil {
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
	collection, err := s.nativeCollectionLocked(s.knownKeys())
	if err != nil {
		return sshconfig.Collection{}, err
	}
	s.enrich(&collection)
	return collection, nil
}

func (s *Service) Create(ifMatch string, edit sshconfig.Edit, tmux *sshconfig.TmuxOptions, filesystem *sshconfig.FileSystemOptions) (sshconfig.Collection, error) {
	return s.mutate(ifMatch, sshconfig.CreateChange(edit), optionsChange{alias: edit.HostAlias, destination: edit.HostAlias, tmux: tmux, filesystem: filesystem}, nil)
}

func (s *Service) Update(ifMatch, alias string, edit sshconfig.Edit, tmux *sshconfig.TmuxOptions, filesystem *sshconfig.FileSystemOptions) (sshconfig.Collection, error) {
	newAlias := edit.HostAlias
	if newAlias == "" {
		newAlias = alias
	}
	return s.mutate(ifMatch, sshconfig.UpdateChange(alias, edit), optionsChange{alias: alias, destination: newAlias, tmux: tmux, filesystem: filesystem}, &edit)
}

func (s *Service) Duplicate(ifMatch, alias, newAlias string) (sshconfig.Collection, error) {
	return s.mutate(ifMatch, sshconfig.DuplicateChange(alias, newAlias), optionsChange{alias: alias, destination: newAlias, copy: true}, nil)
}

func (s *Service) Delete(ifMatch, alias string) (sshconfig.Collection, error) {
	return s.mutate(ifMatch, sshconfig.DeleteChange(alias), optionsChange{alias: alias}, nil)
}

func (s *Service) mutate(ifMatch string, change sshconfig.Change, options optionsChange, edit *sshconfig.Edit) (sshconfig.Collection, error) {
	if !s.Available() {
		return sshconfig.Collection{}, sshfs.ErrUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var collection sshconfig.Collection
	var err error
	if s.mutations == nil {
		collection, err = s.mutateWithFileJournal(ifMatch, change, options)
	} else {
		collection, err = s.mutateSQLLocked(ifMatch, change, options, edit)
	}
	if err != nil {
		return sshconfig.Collection{}, err
	}
	s.enrich(&collection)
	return collection, nil
}

func (s *Service) mutateWithFileJournal(ifMatch string, change sshconfig.Change, options optionsChange) (sshconfig.Collection, error) {
	if err := s.recoverPendingTransaction(); err != nil {
		return sshconfig.Collection{}, err
	}
	configSnapshot, optionsSnapshot, err := s.snapshots(s.options != nil)
	if err != nil {
		return sshconfig.Collection{}, err
	}
	transaction, err := s.beginTransaction(configSnapshot, optionsSnapshot)
	if err != nil {
		return sshconfig.Collection{}, err
	}
	collection, err := s.configRepo.MutateChange(ifMatch, s.knownKeys(), change)
	if err != nil {
		return sshconfig.Collection{}, s.abortTransaction(transaction, err)
	}
	if s.options != nil {
		if err := options.apply(s.options); err != nil {
			return sshconfig.Collection{}, s.abortTransaction(transaction, err)
		}
	}
	if err := s.completeTransaction(transaction); err != nil {
		return sshconfig.Collection{}, err
	}
	return collection, nil
}

func (s *Service) snapshots(withOptions bool) (sshconfig.Snapshot, connectionoptions.Snapshot, error) {
	configSnapshot, err := s.configRepo.Snapshot(s.knownKeys())
	if err != nil {
		return sshconfig.Snapshot{}, connectionoptions.Snapshot{}, err
	}
	if !withOptions || s.legacyOptions == nil {
		return configSnapshot, connectionoptions.Snapshot{}, nil
	}
	optionsSnapshot, err := s.legacyOptions.Snapshot()
	return configSnapshot, optionsSnapshot, err
}
