package connectionoptions

// Reader is the options view used by connection instances and FileSystem.
// Filtering a view never removes stale aliases from the repository.
type Reader interface {
	Load(aliases map[string]bool) (Collection, error)
}

// Repository holds the options attached to connection definitions.
type Repository interface {
	Reader
	UpdateAlias(alias string, value Tmux, present bool) error
	RemoveAlias(alias string) error
	MoveAlias(oldAlias, newAlias string) error
	CopyAlias(oldAlias, newAlias string) error
}

var _ Repository = (*Store)(nil)

// NormalizeAliasOption validates a present alias and supplies its default pwd.
func NormalizeAliasOption(alias string, value Tmux) (Tmux, error) {
	if alias == "" || (value.Enabled && !ValidSessionName(value.SessionName)) {
		return Tmux{}, ErrInvalidSessionName
	}
	if value.Pwd == "" {
		value.Pwd = DefaultPwd
	}
	if !ValidPwd(value.Pwd) {
		return Tmux{}, ErrInvalidPwd
	}
	return value, nil
}
