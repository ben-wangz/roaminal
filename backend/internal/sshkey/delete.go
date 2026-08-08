package sshkey

import (
	"errors"
	"fmt"
	"os"

	"github.com/ben-wangz/roaminal/backend/internal/sshfs"
)

// Delete removes a managed private/public key pair. It never follows a
// symlink, which keeps projected Secret mounts read-only and prevents a key ID
// from deleting a path outside the SSH directory.
func (i *Inventory) Delete(keyID string) error {
	if i == nil || i.Root == nil || !i.Root.Available() {
		return sshfs.ErrUnavailable
	}
	name, err := DecodeKeyID(keyID)
	if err != nil {
		return err
	}
	if _, ok := allowedName(name); !ok {
		return errors.New("unsupported key filename")
	}
	private, err := i.Root.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return os.ErrNotExist
	}
	if err != nil {
		return err
	}
	if private.Mode()&os.ModeSymlink != 0 || !private.Mode().IsRegular() {
		return fmt.Errorf("%w: managed key is not a regular file", sshfs.ErrNotWritable)
	}
	if writable, reason := i.Root.CanWrite(name); !writable {
		return fmt.Errorf("%w: %s", sshfs.ErrNotWritable, reason)
	}
	publicName := name + ".pub"
	publicExists := false
	if public, publicErr := i.Root.Lstat(publicName); publicErr == nil {
		publicExists = true
		if public.Mode()&os.ModeSymlink != 0 || !public.Mode().IsRegular() {
			return fmt.Errorf("%w: public key is not a regular file", sshfs.ErrNotWritable)
		}
		if writable, reason := i.Root.CanWrite(publicName); !writable {
			return fmt.Errorf("%w: %s", sshfs.ErrNotWritable, reason)
		}
	} else if !errors.Is(publicErr, os.ErrNotExist) {
		return publicErr
	}
	// Remove the public half first so a failure cannot leave the private key
	// deleted while an unremovable public file remains.
	if publicExists {
		if err := i.Root.Remove(publicName); err != nil {
			return fmt.Errorf("%w: remove public key: %v", sshfs.ErrNotWritable, err)
		}
	}
	if err := i.Root.Remove(name); err != nil {
		return fmt.Errorf("%w: remove private key: %v", sshfs.ErrNotWritable, err)
	}
	return nil
}
