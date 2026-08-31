//go:build !linux && !darwin

package tmux

import (
	"context"
	"errors"
)

func acquireTmuxLock(context.Context, string) (func(), error) {
	return func() {}, errors.New("tmux locking is unsupported on this platform")
}
