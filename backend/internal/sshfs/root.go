package sshfs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

const (
	ConfigMaxBytes     = 1 << 20
	PublicKeyMaxBytes  = 64 << 10
	PrivateKeyMaxBytes = 1 << 20
)

var ErrUnavailable = errors.New("ssh directory unavailable")
var ErrUnsafePath = errors.New("unsafe ssh path")
var ErrNotWritable = errors.New("ssh path is not writable")

type Capability struct {
	Readable bool
	Writable bool
	Status   string
	Reason   string
}

type Root struct {
	name string
	root *os.Root
}

func Open() (*Root, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home directory: %w", err)
	}
	return OpenAt(filepath.Join(home, ".ssh"))
}

func OpenAt(name string) (*Root, error) {
	root, err := os.OpenRoot(name)
	if err != nil {
		return &Root{name: name}, fmt.Errorf("open ssh directory: %w", err)
	}
	return &Root{name: name, root: root}, nil
}

func (r *Root) Name() string {
	if r == nil {
		return ""
	}
	return r.name
}
func (r *Root) Available() bool { return r != nil && r.root != nil }
func (r *Root) Close() error {
	if r == nil || r.root == nil {
		return nil
	}
	return r.root.Close()
}

func validateName(name string) error {
	if name == "" || filepath.IsAbs(name) || strings.Contains(name, "\\") || strings.HasSuffix(name, "/") {
		return ErrUnsafePath
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" || part == "." || part == ".." {
			return ErrUnsafePath
		}
	}
	return nil
}

func (r *Root) ReadFile(name string, max int) ([]byte, os.FileInfo, error) {
	if err := validateName(name); err != nil {
		return nil, nil, err
	}
	if !r.Available() {
		return nil, nil, ErrUnavailable
	}
	info, err := r.root.Stat(name)
	if err != nil {
		return nil, nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, info, errors.New("ssh object is not a regular file")
	}
	if info.Size() > int64(max) {
		return nil, info, errors.New("ssh file exceeds size limit")
	}
	data, err := r.root.ReadFile(name)
	return data, info, err
}

func (r *Root) Lstat(name string) (os.FileInfo, error) {
	if err := validateName(name); err != nil {
		return nil, err
	}
	if !r.Available() {
		return nil, ErrUnavailable
	}
	return r.root.Lstat(name)
}

func (r *Root) Open(name string) (*os.File, error) {
	if err := validateName(name); err != nil {
		return nil, err
	}
	if !r.Available() {
		return nil, ErrUnavailable
	}
	return r.root.Open(name)
}

func (r *Root) Chmod(name string, mode os.FileMode) error {
	if err := validateName(name); err != nil {
		return err
	}
	if !r.Available() {
		return ErrUnavailable
	}
	return r.root.Chmod(name, mode)
}

func (r *Root) MkdirAll(name string, mode os.FileMode) error {
	if err := validateName(name); err != nil {
		return err
	}
	if !r.Available() {
		return ErrUnavailable
	}
	return r.root.MkdirAll(name, mode)
}

func (r *Root) Remove(name string) error {
	if err := validateName(name); err != nil {
		return err
	}
	if !r.Available() {
		return ErrUnavailable
	}
	return r.root.Remove(name)
}

func (r *Root) RemoveAll(name string) error {
	if err := validateName(name); err != nil {
		return err
	}
	if !r.Available() {
		return ErrUnavailable
	}
	return r.root.RemoveAll(name)
}

func (r *Root) ReadDir() ([]os.DirEntry, error) {
	if !r.Available() {
		return nil, ErrUnavailable
	}
	return os.ReadDir(r.name)
}

func (r *Root) CanWrite(name string) (bool, string) {
	if err := validateName(name); err != nil {
		return false, "unsafe path"
	}
	if !r.Available() {
		return false, "ssh directory unavailable"
	}
	info, err := r.root.Lstat(name)
	if err == nil && info.Mode()&os.ModeSymlink != 0 {
		return false, "target is a symlink"
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, "target cannot be inspected"
	}
	if err == nil && !info.Mode().IsRegular() {
		return false, "target is not a regular file"
	}
	dirInfo, err := os.Stat(r.name)
	if err != nil || !dirInfo.IsDir() {
		return false, "ssh directory unavailable"
	}
	if dirInfo.Mode().Perm()&0o002 != 0 {
		return false, "ssh directory is writable by other users"
	}
	if _, err := os.OpenFile(filepath.Join(r.name, ".roaminal-write-probe"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600); err != nil {
		return false, "ssh directory is not writable"
	} else {
		_ = os.Remove(filepath.Join(r.name, ".roaminal-write-probe"))
	}
	if err == nil && info != nil && info.Mode().Perm()&0o022 != 0 {
		return false, "target has unsafe permissions"
	}
	return true, ""
}

func OwnerUID(info os.FileInfo) int {
	if info == nil {
		return -1
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return int(stat.Uid)
	}
	return os.Getuid()
}
