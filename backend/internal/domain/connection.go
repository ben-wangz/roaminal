package domain

import "time"

const UngroupedConnectionInstanceGroupID = "ungrouped"

type ConnectionInstanceGroup struct {
	GroupID               string   `json:"groupId"`
	Name                  string   `json:"name"`
	ConnectionInstanceIDs []string `json:"connectionInstanceIds"`
}

type ConnectionInstanceLayout struct {
	Revision                       uint64                    `json:"revision"`
	GroupOrder                     []string                  `json:"groupOrder"`
	Groups                         []ConnectionInstanceGroup `json:"groups"`
	UngroupedConnectionInstanceIDs []string                  `json:"ungroupedConnectionInstanceIds"`
}

type ConnectionInstanceMeta struct {
	// FormatVersion is retained only while the storage adapter decodes and
	// migrates legacy records. Application services never set it.
	FormatVersion                 int       `json:"-"`
	ID                            string    `json:"id"`
	Title                         string    `json:"-"`
	AutomaticTitle                string    `json:"automaticTitle"`
	TitleOverride                 *string   `json:"titleOverride"`
	InitialCwd                    string    `json:"initialCwd"`
	Cwd                           string    `json:"cwd"`
	Cols                          int       `json:"cols"`
	Rows                          int       `json:"rows"`
	CreatedAt                     time.Time `json:"createdAt"`
	UpdatedAt                     time.Time `json:"updatedAt"`
	BackendRuntimeID              string    `json:"backendRuntimeId,omitempty"`
	ConnectionDefinitionID        string    `json:"connectionDefinitionId,omitempty"`
	Type                          string    `json:"type,omitempty"`
	Purpose                       string    `json:"purpose,omitempty"`
	SourceHostAlias               *string   `json:"sourceHostAlias,omitempty"`
	Lifecycle                     string    `json:"lifecycle,omitempty"`
	SourceState                   string    `json:"sourceState,omitempty"`
	ExitCode                      *int      `json:"exitCode,omitempty"`
	ExitSignal                    *string   `json:"exitSignal,omitempty"`
	ReuseFromConnectionInstanceID *string   `json:"reuseFromConnectionInstanceId,omitempty"`
	GenerationStatus              string    `json:"generationStatus,omitempty"`
	GenerationError               string    `json:"generationError,omitempty"`
	TmuxEnabled                   bool      `json:"tmuxEnabled,omitempty"`
	TmuxSessionName               string    `json:"tmuxSessionName,omitempty"`
	TmuxPrefixKey                 string    `json:"tmuxPrefixKey,omitempty"`
	TmuxPrefixSource              string    `json:"tmuxPrefixSource,omitempty"`
}

func (meta ConnectionInstanceMeta) EffectiveTitle() string {
	if meta.TitleOverride != nil {
		return *meta.TitleOverride
	}
	if meta.AutomaticTitle != "" {
		return meta.AutomaticTitle
	}
	return meta.Title
}

func (meta *ConnectionInstanceMeta) SyncEffectiveTitle() {
	meta.Title = meta.EffectiveTitle()
}

type SnapshotHeader struct {
	Cols            int    `json:"cols"`
	Rows            int    `json:"rows"`
	ScrollbackLines int    `json:"scrollbackLines"`
	ThroughSequence string `json:"throughSequence"`
	ByteLength      int    `json:"byteLength"`
	SHA256          string `json:"sha256"`
}
