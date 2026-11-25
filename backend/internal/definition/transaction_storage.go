package definition

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ben-wangz/roaminal/backend/internal/connectionoptions"
	"github.com/ben-wangz/roaminal/backend/internal/sshfs"
)

func readDefinitionTransaction(path string) (*definitionTransaction, bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, false, errors.New("definition transaction journal is not a regular file")
	}
	if info.Size() > definitionJournalMaxBytes {
		return nil, false, errors.New("definition transaction journal exceeds size limit")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, definitionJournalMaxBytes+1))
	if err != nil {
		return nil, false, err
	}
	transaction, err := decodeDefinitionTransaction(data)
	return transaction, err == nil, err
}

func decodeDefinitionTransaction(data []byte) (*definitionTransaction, error) {
	if len(data) > definitionJournalMaxBytes {
		return nil, errors.New("definition transaction journal exceeds size limit")
	}
	var transaction definitionTransaction
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&transaction); err != nil || dec.Decode(new(any)) != io.EOF {
		return nil, errors.New("invalid definition transaction journal")
	}
	if transaction.Version != definitionTransactionVersion || transaction.ID == "" {
		return nil, errors.New("unsupported definition transaction journal")
	}
	if transaction.Phase != definitionTransactionPrepared && transaction.Phase != definitionTransactionCommitted {
		return nil, errors.New("invalid definition transaction phase")
	}
	if !validTransactionSnapshot(transaction.Config, sshfs.ConfigMaxBytes) || !validTransactionSnapshot(transaction.Options, connectionoptions.MaxBytes) {
		return nil, errors.New("invalid definition transaction snapshot")
	}
	return &transaction, nil
}

func validTransactionSnapshot(snapshot transactionSnapshot, limit int) bool {
	return len(snapshot.Data) <= limit && (snapshot.Exists || len(snapshot.Data) == 0)
}

// LegacyMutationBackups returns the pre-mutation file snapshots. The old
// journal stored no intended digest, so a prepared journal cannot safely
// forward-commit. Require the operator to reconcile the copied SSH file.
func LegacyMutationBackups(data []byte) (config, options connectionoptions.Snapshot, prepared bool, err error) {
	tx, err := decodeDefinitionTransaction(data)
	if err != nil {
		return config, options, false, err
	}
	return connectionoptions.Snapshot{Exists: tx.Config.Exists, Data: tx.Config.Data},
		connectionoptions.Snapshot{Exists: tx.Options.Exists, Data: tx.Options.Data}, tx.Phase == definitionTransactionPrepared, nil
}

func writeDefinitionTransaction(path string, transaction *definitionTransaction) error {
	data, err := json.Marshal(transaction)
	if err != nil {
		return err
	}
	if len(data) > definitionJournalMaxBytes {
		return errors.New("definition transaction journal exceeds size limit")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return errors.New("definition transaction journal must not be a symlink")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return err
	}
	temporary := filepath.Join(dir, fmt.Sprintf(".%s.%x.tmp", filepath.Base(path), raw))
	file, err := os.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	removeTemporary := true
	defer func() {
		if removeTemporary {
			_ = os.Remove(temporary)
		}
	}()
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return errors.New("definition transaction journal must not be a symlink")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		return err
	}
	removeTemporary = false
	return syncDefinitionJournalDirectory(dir)
}

func removeDefinitionTransaction(path string) error {
	if path == "" {
		return nil
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("definition transaction journal is not a regular file")
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return syncDefinitionJournalDirectory(filepath.Dir(path))
}

func syncDefinitionJournalDirectory(path string) error {
	directory, err := os.Open(filepath.Clean(path))
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
