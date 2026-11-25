package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"

	"github.com/ben-wangz/roaminal/backend/internal/persistence/db"
)

const bindingName = "database-binding.json"

var (
	errDatabaseBinding = errors.New("configured database does not match this state directory; restore the bound database or explicitly activate a verified transfer")
	errStateLocked     = errors.New("could not lock state directory; stop Roaminal and any offline operation using it")
)

type databaseBinding struct {
	Driver           string `json:"driver"`
	ID               string `json:"id"`
	Owner            string `json:"owner"`
	TransferSourceID string `json:"transferSourceId,omitempty"`
}

type stateLock struct {
	root *os.Root
	dir  *os.File
}

// The directory inode is stable across file replacement and shared by serving
// and offline commands on the same state mount, including bind/subPath mounts.
func lockStateDir(path string) (*stateLock, error) {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return nil, errors.New("create state directory failed")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0o022 != 0 {
		return nil, errors.New("state directory must be a directory and not writable by other users")
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, errors.New("open state directory failed")
	}
	dir, err := root.Open(".")
	if err != nil {
		_ = root.Close()
		return nil, errors.New("open state directory for locking failed")
	}
	state := &stateLock{root: root, dir: dir}
	current, err := dir.Stat()
	if err != nil || !os.SameFile(info, current) {
		state.close()
		return nil, errors.New("state directory changed while opening")
	}
	if err := syscall.Flock(int(dir.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		state.close()
		return nil, errStateLocked
	}
	return state, nil
}

func (s *stateLock) close() {
	_ = s.dir.Close()
	_ = s.root.Close()
}

func (s *stateLock) binding() (databaseBinding, error) {
	var binding databaseBinding
	info, err := s.root.Lstat(bindingName)
	if errors.Is(err, os.ErrNotExist) {
		return binding, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 || info.Size() > 1024 {
		return binding, errors.New("invalid local database binding")
	}
	f, err := s.root.Open(bindingName)
	if err != nil {
		return binding, errors.New("read local database binding failed")
	}
	defer f.Close()
	dec := json.NewDecoder(io.LimitReader(f, 1025))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&binding); err != nil || dec.Decode(new(any)) != io.EOF || !binding.valid() {
		return databaseBinding{}, errors.New("invalid local database binding")
	}
	return binding, nil
}

func (b databaseBinding) valid() bool {
	if b.Driver != db.SQLite && b.Driver != db.Postgres {
		return false
	}
	return validDatabaseID(b.ID) && validDatabaseID(b.Owner) &&
		(b.TransferSourceID == "" || (validDatabaseID(b.TransferSourceID) && b.TransferSourceID != b.ID))
}

func validDatabaseID(id string) bool {
	decoded, err := hex.DecodeString(id)
	return err == nil && len(decoded) == 16 && hex.EncodeToString(decoded) == id
}

func (b databaseBinding) check(driver string, installation db.Installation) error {
	if b.ID != installation.ID || b.Driver != driver ||
		(b.TransferSourceID != "" && b.TransferSourceID != installation.TransferSourceID) {
		return errDatabaseBinding
	}
	if installation.Served && b.Owner != installation.ServingOwner {
		return db.ErrServingBinding
	}
	return nil
}

func (s *stateLock) bind(binding databaseBinding) error {
	errBinding := errors.New("write local database binding failed")
	var suffix [8]byte
	if !binding.valid() {
		return errBinding
	}
	if _, err := rand.Read(suffix[:]); err != nil {
		return errBinding
	}
	name := fmt.Sprintf(".database-binding-%x", suffix[:])
	f, err := s.root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return errBinding
	}
	defer s.root.Remove(name)
	writeErr := json.NewEncoder(f).Encode(binding)
	syncErr := f.Sync()
	if closeErr := f.Close(); writeErr != nil || syncErr != nil || closeErr != nil {
		return errBinding
	}
	if err := s.root.Rename(name, bindingName); err != nil {
		return errBinding
	}
	if err := s.dir.Sync(); err != nil {
		return errBinding
	}
	return nil
}
