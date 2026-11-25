package definition

import (
	"github.com/ben-wangz/roaminal/backend/internal/connectionoptions"
	"github.com/ben-wangz/roaminal/backend/internal/sshconfig"
)

type optionsChange struct {
	alias, destination string
	tmux               *sshconfig.TmuxOptions
	filesystem         *sshconfig.FileSystemOptions
	copy               bool
}

func (c optionsChange) patches() bool { return c.tmux != nil || c.filesystem != nil }

func (c optionsChange) apply(options connectionoptions.Repository) error {
	switch {
	case c.patches():
		if err := saveOptions(options, c.destination, c.tmux, c.filesystem); err != nil {
			return err
		}
		if c.destination != c.alias {
			return options.RemoveAlias(c.alias)
		}
	case c.destination == "":
		return options.RemoveAlias(c.alias)
	case c.copy:
		return options.CopyAlias(c.alias, c.destination)
	case c.destination != c.alias:
		return options.MoveAlias(c.alias, c.destination)
	}
	return nil
}

func (c optionsChange) prepare(current map[string]connectionoptions.Tmux) ([]OptionUpdate, error) {
	if c.destination == "" {
		return []OptionUpdate{{Alias: c.alias}}, nil
	}
	if !c.patches() && c.destination == c.alias {
		return nil, nil
	}
	value, present := current[c.alias]
	if c.patches() {
		value, present = patchOptions(value, c.tmux, c.filesystem)
	} else if !present {
		return nil, nil
	}
	update := OptionUpdate{Alias: c.destination}
	if present {
		value, err := connectionoptions.NormalizeAliasOption(c.destination, value)
		if err != nil {
			return nil, err
		}
		update.Value = &value
	}
	updates := []OptionUpdate{update}
	if !c.copy && c.destination != c.alias {
		updates = append(updates, OptionUpdate{Alias: c.alias})
	}
	return updates, nil
}
