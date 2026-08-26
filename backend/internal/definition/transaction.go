package definition

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"path/filepath"

	"github.com/ben-wangz/roaminal/backend/internal/connectionoptions"
	"github.com/ben-wangz/roaminal/backend/internal/sshconfig"
	"github.com/ben-wangz/roaminal/backend/internal/sshfs"
)

const (
	definitionTransactionVersion   = 1
	definitionTransactionPrepared  = "prepared"
	definitionTransactionCommitted = "committed"
	definitionJournalMaxBytes      = 2*sshfs.ConfigMaxBytes + 2*connectionoptions.MaxBytes + 4096
)

type transactionSnapshot struct {
	Exists bool   `json:"exists"`
	Data   []byte `json:"data,omitempty"`
}

type definitionTransaction struct {
	Version int                 `json:"version"`
	ID      string              `json:"id"`
	Phase   string              `json:"phase"`
	Config  transactionSnapshot `json:"config"`
	Options transactionSnapshot `json:"options"`
}

func definitionJournalPath(options *connectionoptions.Store) string {
	if options == nil {
		return ""
	}
	return filepath.Join(filepath.Dir(options.Path()), "definition-mutation.json")
}

func newDefinitionTransaction(config sshconfig.Snapshot, options connectionoptions.Snapshot) (*definitionTransaction, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return nil, fmt.Errorf("generate definition transaction id: %w", err)
	}
	return &definitionTransaction{
		Version: definitionTransactionVersion,
		ID:      hex.EncodeToString(raw[:]),
		Phase:   definitionTransactionPrepared,
		Config:  transactionSnapshot{Exists: config.Exists, Data: append([]byte(nil), config.Data...)},
		Options: transactionSnapshot{Exists: options.Exists, Data: append([]byte(nil), options.Data...)},
	}, nil
}

func (s *Service) recoverPendingTransaction() error {
	if s.journalPath == "" {
		return nil
	}
	transaction, exists, err := readDefinitionTransaction(s.journalPath)
	if err != nil || !exists {
		return err
	}
	if transaction.Phase == definitionTransactionCommitted {
		return removeDefinitionTransaction(s.journalPath)
	}
	if err := s.restoreTransaction(transaction); err != nil {
		return fmt.Errorf("restore definition transaction %q: %w", transaction.ID, err)
	}
	return removeDefinitionTransaction(s.journalPath)
}

func (s *Service) beginTransaction(config sshconfig.Snapshot, options connectionoptions.Snapshot) (*definitionTransaction, error) {
	if s.journalPath == "" {
		return nil, nil
	}
	if err := s.recoverPendingTransaction(); err != nil {
		return nil, err
	}
	transaction, err := newDefinitionTransaction(config, options)
	if err != nil {
		return nil, err
	}
	if err := writeDefinitionTransaction(s.journalPath, transaction); err != nil {
		return nil, err
	}
	return transaction, nil
}

func (s *Service) commitTransaction(transaction *definitionTransaction) error {
	if transaction == nil {
		return nil
	}
	transaction.Phase = definitionTransactionCommitted
	return writeDefinitionTransaction(s.journalPath, transaction)
}

func (s *Service) completeTransaction(transaction *definitionTransaction) error {
	if transaction == nil {
		return nil
	}
	if err := s.commitTransaction(transaction); err != nil {
		return s.abortTransaction(transaction, err)
	}
	// A committed journal is safe to leave behind if its cleanup encounters a
	// transient filesystem error. The next mutation or process start removes it.
	if err := removeDefinitionTransaction(s.journalPath); err != nil {
		log.Printf("definition_transaction_cleanup_failed id=%q error_type=%T", transaction.ID, err)
	}
	return nil
}

func (s *Service) abortTransaction(transaction *definitionTransaction, cause error) error {
	if transaction == nil {
		return cause
	}
	if err := s.restoreTransaction(transaction); err != nil {
		return errors.Join(cause, err)
	}
	if err := removeDefinitionTransaction(s.journalPath); err != nil {
		return errors.Join(cause, err)
	}
	return cause
}

func (s *Service) restoreTransaction(transaction *definitionTransaction) error {
	var restoreErrors []error
	if s.configRepo != nil {
		if err := s.configRepo.Restore(sshconfig.Snapshot{Exists: transaction.Config.Exists, Data: transaction.Config.Data}); err != nil {
			restoreErrors = append(restoreErrors, err)
		}
	}
	if s.options != nil {
		if err := s.options.Restore(connectionoptions.Snapshot{Exists: transaction.Options.Exists, Data: transaction.Options.Data}); err != nil {
			restoreErrors = append(restoreErrors, err)
		}
	}
	return errors.Join(restoreErrors...)
}
