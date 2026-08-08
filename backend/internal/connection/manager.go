// Package connection is the application-facing boundary for connection
// instances. The PTY/shadow implementation remains in terminal; this alias
// keeps HTTP code independent from the engine package while SSH support is
// added in later phases.
package connection

import (
	"github.com/ben-wangz/roaminal/backend/internal/config"
	"github.com/ben-wangz/roaminal/backend/internal/persistence"
	"github.com/ben-wangz/roaminal/backend/internal/terminal"
	"github.com/ben-wangz/roaminal/backend/internal/worker"
)

type Manager = terminal.Manager
type Client = terminal.Client
type Summary = terminal.Summary
type ExitStatus = terminal.ExitStatus

var ErrClientCapacity = terminal.ErrClientCapacity
var ErrControlNotOwner = terminal.ErrControlNotOwner

func NewManager(cfg config.Config, store *persistence.Store, terminalWorker *worker.Client) *Manager {
	return terminal.NewManager(cfg, store, terminalWorker)
}
