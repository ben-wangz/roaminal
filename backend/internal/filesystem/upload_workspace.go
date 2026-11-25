package filesystem

import (
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	uploadStagingDirectory = "upload-staging"
	maxStagedUploadJobs    = 8
)

// The reservation follows an upload from HTTP admission through job cleanup.
// The byte limit accounts for declared payload, not filesystem block overhead.
type uploadStagingBudget struct {
	mu        sync.Mutex
	jobLimit  int
	byteLimit int64
	jobs      int
	bytes     int64
}

type uploadReservation struct {
	budget *uploadStagingBudget
	bytes  int64
	path   string
}

func (b *uploadStagingBudget) admit() (*uploadReservation, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.jobs >= b.jobLimit {
		return nil, ErrUploadStagingCapacity
	}
	b.jobs++
	return &uploadReservation{budget: b}, nil
}

func (r *uploadReservation) reserveBytes(bytes int64) error {
	b := r.budget
	b.mu.Lock()
	defer b.mu.Unlock()
	if bytes > b.byteLimit-b.bytes {
		return ErrUploadStagingCapacity
	}
	b.bytes += bytes
	r.bytes = bytes
	return nil
}

func (r *uploadReservation) cleanup() {
	if r.path != "" {
		if err := os.RemoveAll(r.path); err != nil {
			// Do not admit more writes as if the old payload were gone.
			// The next startup retries removal under the state lock.
			log.Print("upload staging cleanup failed; capacity remains reserved until restart")
			return
		}
	}
	b := r.budget
	b.mu.Lock()
	b.jobs--
	b.bytes -= r.bytes
	b.mu.Unlock()
}

// PrepareUploadStaging runs once under the exclusive application state lock.
// The contents are temporary and never used to resume a persisted upload.
func PrepareUploadStaging(stateDir string) error {
	path := filepath.Join(stateDir, uploadStagingDirectory)
	if err := os.Mkdir(path, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return errors.New("create upload staging directory failed")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		return errors.New("upload staging directory must be private and not a symlink")
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return err
	}
	defer root.Close()
	dir, err := root.Open(".")
	if err != nil {
		return err
	}
	defer dir.Close()
	entries, err := dir.ReadDir(-1)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "upload-") {
			if err := root.RemoveAll(entry.Name()); err != nil {
				return errors.New("clean up interrupted upload staging failed")
			}
		}
	}
	return nil
}
