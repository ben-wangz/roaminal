package connectionoptions

// RemoveAlias is the only options-file operation used for intentional
// connection-definition deletion. It is separate from Load so a partial SSH
// config read can never erase unrelated settings as a side effect.
func (s *Store) RemoveAlias(alias string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	decoded, _, source, err := s.decodeLocked()
	if err != nil {
		return err
	}
	if source.Status == "missing" {
		return nil
	}
	delete(decoded.Connections, alias)
	return s.saveFileLocked(decoded)
}

// MoveAlias preserves settings across an intentional Host alias rename.
func (s *Store) MoveAlias(oldAlias, newAlias string) error {
	if oldAlias == newAlias {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	decoded, _, source, err := s.decodeLocked()
	if err != nil {
		return err
	}
	if source.Status == "missing" {
		return nil
	}
	if value, ok := decoded.Connections[oldAlias]; ok {
		decoded.Connections[newAlias] = value
		delete(decoded.Connections, oldAlias)
	}
	return s.saveFileLocked(decoded)
}

// CopyAlias explicitly duplicates settings for a duplicated connection
// definition. It never relies on alias filtering or implicit reconciliation.
func (s *Store) CopyAlias(oldAlias, newAlias string) error {
	if oldAlias == newAlias {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	decoded, _, source, err := s.decodeLocked()
	if err != nil {
		return err
	}
	if source.Status == "missing" {
		return nil
	}
	if value, ok := decoded.Connections[oldAlias]; ok {
		decoded.Connections[newAlias] = value
	}
	return s.saveFileLocked(decoded)
}
