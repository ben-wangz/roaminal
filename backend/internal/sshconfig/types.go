package sshconfig

import "github.com/ben-wangz/roaminal/backend/internal/sshfs"

type BlockKind string

const (
	Global     BlockKind = "global"
	HostBlock  BlockKind = "host"
	MatchBlock BlockKind = "match"
)

type Warning struct {
	Directive string `json:"directive"`
	Line      int    `json:"line"`
	Class     string `json:"class"`
}

type Directive struct {
	Keyword    string
	Value      string
	Tokens     []string
	Line       int
	LineStart  int
	LineEnd    int
	ValueStart int
	ValueEnd   int
	Raw        []byte
}

type Block struct {
	Kind       BlockKind
	Alias      string
	Start      int
	End        int
	Header     Directive
	Directives []Directive
}

type Document struct {
	Bytes      []byte
	Newline    string
	Blocks     []Block
	Warnings   []Warning
	Capability sshfs.Capability
}

type Capability struct {
	Readable bool   `json:"readable"`
	Writable bool   `json:"writable"`
	Status   string `json:"status"`
	Reason   string `json:"reason,omitempty"`
}

type Definition struct {
	ConnectionDefinitionID     string          `json:"connectionDefinitionId"`
	Type                       string          `json:"type"`
	HostAlias                  string          `json:"hostAlias,omitempty"`
	HostName                   *string         `json:"hostName"`
	User                       *string         `json:"user"`
	Port                       *uint16         `json:"port"`
	IdentityFileNames          []string        `json:"identityFileNames"`
	IdentitiesOnly             *string         `json:"identitiesOnly"`
	StrictHostKeyChecking      *string         `json:"strictHostKeyChecking"`
	UserKnownHostsFile         *string         `json:"userKnownHostsFile"`
	ServerAliveInterval        *uint32         `json:"serverAliveInterval"`
	AdvancedDirectiveCount     int             `json:"advancedDirectiveCount"`
	UnmanagedIdentityCount     int             `json:"unmanagedIdentityCount"`
	Warnings                   []Warning       `json:"warnings"`
	Capabilities               map[string]bool `json:"capabilities"`
	HostVerificationAssessment string          `json:"hostVerificationAssessment"`
}
