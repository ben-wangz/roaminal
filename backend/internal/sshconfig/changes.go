package sshconfig

import (
	"errors"
	"os"
)

// Change renders a structural edit without publishing it.
type Change func(Document, map[string]bool) ([]byte, error)

func (r *Repository) MutateChange(ifMatch string, knownKeys map[string]bool, change Change) (Collection, error) {
	return r.Mutate(ifMatch, knownKeys, func(doc Document) ([]byte, error) { return change(doc, knownKeys) })
}

func (r *Repository) Update(ifMatch string, knownKeys map[string]bool, alias string, edit Edit) (Collection, error) {
	return r.MutateChange(ifMatch, knownKeys, UpdateChange(alias, edit))
}

func (r *Repository) Create(ifMatch string, knownKeys map[string]bool, edit Edit) (Collection, error) {
	return r.MutateChange(ifMatch, knownKeys, CreateChange(edit))
}

func (r *Repository) Delete(ifMatch string, knownKeys map[string]bool, alias string) (Collection, error) {
	return r.MutateChange(ifMatch, knownKeys, DeleteChange(alias))
}

func (r *Repository) Duplicate(ifMatch string, knownKeys map[string]bool, alias, newAlias string) (Collection, error) {
	return r.MutateChange(ifMatch, knownKeys, DuplicateChange(alias, newAlias))
}

func UpdateChange(alias string, edit Edit) Change {
	return func(doc Document, _ map[string]bool) ([]byte, error) { return patchBlock(doc, alias, edit) }
}

func CreateChange(edit Edit) Change {
	return func(doc Document, knownKeys map[string]bool) ([]byte, error) {
		if !validAlias(edit.HostAlias) {
			return nil, errors.New("invalid host alias")
		}
		for _, definition := range doc.Definitions(knownKeys) {
			if definition.HostAlias == edit.HostAlias {
				return nil, errors.New("host alias already exists")
			}
		}
		return appendBlock(doc, edit), nil
	}
}

func DeleteChange(alias string) Change {
	return func(doc Document, _ map[string]bool) ([]byte, error) {
		block := findBlock(doc, alias)
		if block == nil {
			return nil, os.ErrNotExist
		}
		return append(append([]byte{}, doc.Bytes[:block.Start]...), doc.Bytes[block.End:]...), nil
	}
}

func DuplicateChange(alias, newAlias string) Change {
	return func(doc Document, knownKeys map[string]bool) ([]byte, error) {
		if !validAlias(newAlias) {
			return nil, errors.New("invalid host alias")
		}
		if findBlock(doc, newAlias) != nil {
			return nil, errors.New("host alias already exists")
		}
		block := findBlock(doc, alias)
		if block == nil {
			return nil, os.ErrNotExist
		}
		definition := definitionFromBlock(*block, knownKeys, nil)
		return appendBlock(doc, Edit{HostAlias: newAlias, HostName: definition.HostName, User: definition.User, Port: definition.Port, IdentityFileNames: definition.IdentityFileNames, IdentitiesOnly: definition.IdentitiesOnly, StrictHostKeyChecking: definition.StrictHostKeyChecking, UserKnownHostsFile: definition.UserKnownHostsFile, ServerAliveInterval: definition.ServerAliveInterval}), nil
	}
}
