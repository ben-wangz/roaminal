package clientdiag

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
	"time"
)

var managedFilePattern = regexp.MustCompile(`^client-[0-9]{8}T[0-9]{6}\.[0-9]{9}Z-[0-9a-f]{8}\.ndjson$`)

type fileWriter struct {
	mu   sync.Mutex
	dir  string
	file *os.File
	path string
	size int64
}

func newFileWriter(dir string) (*fileWriter, error) {
	if info, err := os.Lstat(dir); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("diagnostics directory is a symlink")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, errors.New("diagnostics path is not a directory")
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, err
	}
	writer := &fileWriter{dir: dir}
	writer.mu.Lock()
	defer writer.mu.Unlock()
	if err := writer.pruneLocked(time.Now().UTC()); err != nil {
		return nil, err
	}
	if err := writer.rotateLocked(); err != nil {
		return nil, err
	}
	if err := writer.pruneLocked(time.Now().UTC()); err != nil {
		return nil, err
	}
	return writer, nil
}

func (w *fileWriter) WriteBatch(lines [][]byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	now := time.Now().UTC()
	for _, line := range lines {
		if len(line) > MaxStoredFileBytes {
			return errors.New("diagnostic record exceeds file limit")
		}
		if w.file == nil || (w.size > 0 && w.size+int64(len(line)) > MaxStoredFileBytes) {
			if err := w.rotateLocked(); err != nil {
				return err
			}
		}
		written, err := w.file.Write(line)
		if err != nil {
			return err
		}
		w.size += int64(written)
	}
	if w.file != nil {
		if err := w.file.Sync(); err != nil {
			return err
		}
	}
	return w.pruneLocked(now)
}

func (w *fileWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Sync()
	closeErr := w.file.Close()
	w.file = nil
	if err != nil {
		return err
	}
	return closeErr
}

func (w *fileWriter) rotateLocked() error {
	if w.file != nil {
		if err := w.file.Sync(); err != nil {
			return err
		}
		if err := w.file.Close(); err != nil {
			return err
		}
	}
	for attempts := 0; attempts < 10; attempts++ {
		var raw [4]byte
		if _, err := io.ReadFull(rand.Reader, raw[:]); err != nil {
			return err
		}
		path := filepath.Join(w.dir, fmt.Sprintf("client-%s-%x.ndjson", time.Now().UTC().Format("20060102T150405.000000000Z"), raw))
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return err
		}
		w.file, w.path, w.size = file, path, 0
		return nil
	}
	return errors.New("could not create a unique diagnostics file")
}

type diagnosticFile struct {
	path    string
	modTime time.Time
	size    int64
}

func (w *fileWriter) pruneLocked(now time.Time) error {
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return err
	}
	files := make([]diagnosticFile, 0, len(entries))
	for _, entry := range entries {
		if !managedFilePattern.MatchString(entry.Name()) {
			continue
		}
		path := filepath.Join(w.dir, entry.Name())
		info, err := os.Lstat(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			continue
		}
		if now.Sub(info.ModTime()) > Retention {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			continue
		}
		files = append(files, diagnosticFile{path: path, modTime: info.ModTime(), size: info.Size()})
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].modTime.Equal(files[j].modTime) {
			return files[i].path < files[j].path
		}
		return files[i].modTime.Before(files[j].modTime)
	})
	total := int64(0)
	for _, file := range files {
		total += file.size
	}
	for len(files) > MaxStoredFiles || total > MaxStoredBytes {
		removeIndex := -1
		for i, file := range files {
			if file.path != w.path {
				removeIndex = i
				break
			}
		}
		if removeIndex < 0 {
			break
		}
		file := files[removeIndex]
		if err := os.Remove(file.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		total -= file.size
		files = append(files[:removeIndex], files[removeIndex+1:]...)
	}
	return nil
}
