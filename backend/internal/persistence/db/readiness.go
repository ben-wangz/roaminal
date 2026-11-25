package db

import "context"

// Ready checks the bound application's schema, not just whether a network
// connection can be opened. The caller supplies a short deadline.
func (s *Store) Ready(ctx context.Context) error {
	installation, err := s.Installation(ctx)
	if err != nil || !installation.Served {
		return errInstallation
	}
	return nil
}
