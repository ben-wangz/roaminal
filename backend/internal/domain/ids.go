package domain

import "strings"

// Product identities are intentionally distinct even though they are encoded
// as strings at the HTTP and storage boundaries.
type ConnectionDefinitionID string
type ConnectionInstanceID string
type AuthenticationSessionID string
type TerminalRuntimeID string
type TransportID string
type TmuxSessionID string

func (id ConnectionDefinitionID) String() string  { return string(id) }
func (id ConnectionInstanceID) String() string    { return string(id) }
func (id AuthenticationSessionID) String() string { return string(id) }
func (id TerminalRuntimeID) String() string       { return string(id) }
func (id TransportID) String() string             { return string(id) }
func (id TmuxSessionID) String() string           { return string(id) }

func validIdentity(value string) bool {
	return strings.TrimSpace(value) != "" && !strings.ContainsAny(value, "\x00\r\n")
}

func (id ConnectionDefinitionID) Valid() bool  { return validIdentity(string(id)) }
func (id ConnectionInstanceID) Valid() bool    { return validIdentity(string(id)) }
func (id AuthenticationSessionID) Valid() bool { return validIdentity(string(id)) }
func (id TerminalRuntimeID) Valid() bool       { return validIdentity(string(id)) }
func (id TransportID) Valid() bool             { return validIdentity(string(id)) }
func (id TmuxSessionID) Valid() bool           { return validIdentity(string(id)) }
