package definition

import (
	"slices"

	"github.com/ben-wangz/roaminal/backend/internal/connectionoptions"
	"github.com/ben-wangz/roaminal/backend/internal/sshconfig"
)

func saveOptions(options connectionoptions.Repository, alias string, tmux *sshconfig.TmuxOptions, filesystem *sshconfig.FileSystemOptions) error {
	current, err := options.Load(map[string]bool{alias: true})
	if err != nil {
		return err
	}
	value, present := patchOptions(current.Options[alias], tmux, filesystem)
	return options.UpdateAlias(alias, value, present)
}

func patchOptions(value connectionoptions.Tmux, tmux *sshconfig.TmuxOptions, filesystem *sshconfig.FileSystemOptions) (connectionoptions.Tmux, bool) {
	if tmux != nil {
		value.Enabled = tmux.Enabled
		value.SessionName = tmux.SessionName
		if !value.Enabled {
			value.SessionName = ""
		}
	}
	if filesystem != nil {
		value.Pwd = filesystem.Pwd
	}
	return value, value.Enabled || value.Pwd != ""
}

// The editor can repeat the projected native fields alongside an options
// patch. Matching only supplied fields also supports an explicit partial edit.
func sameNativeFields(edit sshconfig.Edit, item sshconfig.Definition) bool {
	return (edit.HostAlias == "" || edit.HostAlias == item.HostAlias) &&
		matches(edit.HostName, item.HostName) && matches(edit.User, item.User) && matches(edit.Port, item.Port) &&
		(edit.IdentityFileNames == nil || slices.Equal(edit.IdentityFileNames, item.IdentityFileNames)) &&
		matches(edit.IdentitiesOnly, item.IdentitiesOnly) && matches(edit.StrictHostKeyChecking, item.StrictHostKeyChecking) &&
		matches(edit.UserKnownHostsFile, item.UserKnownHostsFile) && matches(edit.ServerAliveInterval, item.ServerAliveInterval)
}

func matches[T comparable](requested, original *T) bool {
	return requested == nil || (original != nil && *requested == *original)
}
