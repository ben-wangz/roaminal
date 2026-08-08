package sshconfig

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/ben-wangz/roaminal/backend/internal/sshfs"
)

var ErrPreconditionRequired = errors.New("config etag is required")
var ErrPreconditionFailed = errors.New("config etag does not match")
var ErrFieldNotEditable = errors.New("field is not structurally editable")

type ConfigSource struct {
	Status   string    `json:"status"`
	Readable bool      `json:"readable"`
	Writable bool      `json:"writable"`
	Warnings []Warning `json:"warnings"`
	Blockers []string  `json:"blockers"`
	Reason   string    `json:"reason,omitempty"`
}

type Collection struct {
	ConfigSource ConfigSource `json:"configSource"`
	Definitions  []Definition `json:"definitions"`
	ETag         string       `json:"-"`
}

type Repository struct {
	root *sshfs.Root
	mu   sync.Mutex
	key  [32]byte
}

func New(root *sshfs.Root) *Repository {
	r := &Repository{root: root}
	if _, err := rand.Read(r.key[:]); err != nil {
		copy(r.key[:], []byte("roaminal-runtime-config-etag-key"))
	}
	return r
}

func (r *Repository) Read(knownKeys map[string]bool) (Document, string, ConfigSource, error) {
	var data []byte
	var info os.FileInfo
	var err error
	if r.root == nil || !r.root.Available() {
		return Parse(nil, sshfs.Capability{Status: "unavailable", Reason: "ssh directory unavailable"}), r.etag(nil, "unavailable"), ConfigSource{Status: "unavailable", Blockers: []string{"ssh_directory"}, Reason: "ssh directory unavailable"}, nil
	} else {
		data, info, err = r.root.ReadFile("config", sshfs.ConfigMaxBytes)
	}
	capability := sshfs.Capability{Status: "available", Readable: true}
	if errors.Is(err, os.ErrNotExist) {
		capability.Status = "missing"
		capability.Readable = false
		err = nil
	} else if err != nil {
		capability.Status = "unreadable"
		capability.Readable = false
		capability.Reason = "config cannot be read"
		data = nil
	}
	if info != nil {
		capability.Readable = true
	}
	if writable, reason := r.root.CanWrite("config"); writable {
		capability.Writable = true
	} else if capability.Reason == "" {
		capability.Reason = reason
	}
	doc := Parse(data, capability)
	etag := r.etag(append(data, byte(0)), capabilityFingerprint(info, capability))
	warnings := append([]Warning{}, doc.Warnings...)
	source := ConfigSource{Status: capability.Status, Readable: capability.Readable, Writable: capability.Writable, Warnings: warnings, Blockers: []string{}}
	if !capability.Writable && capability.Reason != "" {
		source.Blockers = []string{capability.Reason}
		source.Reason = capability.Reason
	}
	return doc, etag, source, nil
}

func (r *Repository) Collection(knownKeys map[string]bool) (Collection, error) {
	doc, etag, source, err := r.Read(knownKeys)
	if err != nil {
		return Collection{}, err
	}
	defs := []Definition{{ConnectionDefinitionID: "local", Type: "local", HostName: nil, User: nil, Port: nil, IdentityFileNames: []string{}, Warnings: []Warning{}, Capabilities: map[string]bool{"edit": false, "delete": false}, HostVerificationAssessment: "default"}}
	defs = append(defs, doc.Definitions(knownKeys)...)
	return Collection{ConfigSource: source, Definitions: defs, ETag: etag}, nil
}

func (r *Repository) Mutate(ifMatch string, knownKeys map[string]bool, mutation func(Document) ([]byte, error)) (Collection, error) {
	if strings.TrimSpace(ifMatch) == "" {
		return Collection{}, ErrPreconditionRequired
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	doc, etag, source, err := r.Read(knownKeys)
	if err != nil {
		return Collection{}, err
	}
	if etag != ifMatch {
		return Collection{}, ErrPreconditionFailed
	}
	if !source.Writable {
		if r.root != nil {
			if ensureErr := r.root.EnsureDirectory(); ensureErr == nil {
				doc, etag, source, _ = r.Read(knownKeys)
			}
		}
	}
	if !source.Writable {
		return Collection{}, fmt.Errorf("%w: %s", sshfs.ErrNotWritable, source.Reason)
	}
	data, err := mutation(doc)
	if err != nil {
		return Collection{}, err
	}
	if err := r.root.AtomicReplace("config", data, sshfs.ConfigMaxBytes); err != nil {
		return Collection{}, err
	}
	return r.Collection(knownKeys)
}

func (r *Repository) etag(data []byte, capability string) string {
	h := hmac.New(sha256.New, r.key[:])
	h.Write(data)
	h.Write([]byte(capability))
	return `"` + hex.EncodeToString(h.Sum(nil)) + `"`
}
func capabilityFingerprint(info os.FileInfo, capability sshfs.Capability) string {
	if info == nil {
		return capability.Status + ":" + capability.Reason
	}
	return fmt.Sprintf("%s:%d:%o:%t", capability.Status, info.Size(), info.Mode().Perm(), capability.Writable)
}

type Edit struct {
	HostAlias             string
	HostName              *string
	User                  *string
	Port                  *uint16
	IdentityFileNames     []string
	IdentitiesOnly        *string
	StrictHostKeyChecking *string
	UserKnownHostsFile    *string
	ServerAliveInterval   *uint32
}

func (r *Repository) Update(ifMatch string, knownKeys map[string]bool, alias string, edit Edit) (Collection, error) {
	return r.Mutate(ifMatch, knownKeys, func(doc Document) ([]byte, error) { return patchBlock(doc, alias, edit) })
}
func (r *Repository) Create(ifMatch string, knownKeys map[string]bool, edit Edit) (Collection, error) {
	return r.Mutate(ifMatch, knownKeys, func(doc Document) ([]byte, error) {
		if !validAlias(edit.HostAlias) {
			return nil, errors.New("invalid host alias")
		}
		for _, definition := range doc.Definitions(knownKeys) {
			if definition.HostAlias == edit.HostAlias {
				return nil, errors.New("host alias already exists")
			}
		}
		return appendBlock(doc, edit), nil
	})
}
func (r *Repository) Delete(ifMatch string, knownKeys map[string]bool, alias string) (Collection, error) {
	return r.Mutate(ifMatch, knownKeys, func(doc Document) ([]byte, error) {
		block := findBlock(doc, alias)
		if block == nil {
			return nil, os.ErrNotExist
		}
		return append(append([]byte{}, doc.Bytes[:block.Start]...), doc.Bytes[block.End:]...), nil
	})
}
func (r *Repository) Duplicate(ifMatch string, knownKeys map[string]bool, alias, newAlias string) (Collection, error) {
	return r.Mutate(ifMatch, knownKeys, func(doc Document) ([]byte, error) {
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
		definition.HostAlias = newAlias
		return appendBlock(doc, Edit{HostAlias: newAlias, HostName: definition.HostName, User: definition.User, Port: definition.Port, IdentityFileNames: definition.IdentityFileNames, IdentitiesOnly: definition.IdentitiesOnly, StrictHostKeyChecking: definition.StrictHostKeyChecking, UserKnownHostsFile: definition.UserKnownHostsFile, ServerAliveInterval: definition.ServerAliveInterval}), nil
	})
}
