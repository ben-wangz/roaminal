package server

import (
	"github.com/ben-wangz/roaminal/backend/internal/auth"
	"github.com/ben-wangz/roaminal/backend/internal/connectionoptions"
	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/filesystem"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
	"github.com/ben-wangz/roaminal/backend/internal/sshconfig"
	"github.com/ben-wangz/roaminal/backend/internal/sshkey"
)

type successResponse struct {
	Status string `json:"status"`
}

type authSessionCollectionResponse struct {
	Sessions []auth.SessionSummary `json:"sessions"`
}

type connectionInstanceCollectionResponse struct {
	ConnectionInstances      []ports.ConnectionInstanceSummary `json:"connectionInstances"`
	ConnectionInstanceLayout domain.ConnectionInstanceLayout   `json:"connectionInstanceLayout"`
}

type connectionInstanceLayoutResponse struct {
	Layout domain.ConnectionInstanceLayout `json:"layout"`
}

type connectionLaunchResponse struct {
	LaunchID               string `json:"launchId"`
	ConnectionDefinitionID string `json:"connectionDefinitionId"`
	Lifecycle              string `json:"lifecycle"`
	TmuxSessionName        string `json:"tmuxSessionName"`
}

type sshKeyCollectionResponse struct {
	Keys []sshkey.Key `json:"keys"`
}

type publicKeyResponse struct {
	PublicKey string `json:"publicKey"`
}

type filesystemRootResponse struct {
	ConnectionInstanceID string                 `json:"connectionInstanceId"`
	Root                 filesystem.RootContext `json:"root"`
}

type filesystemRootChangedDetails struct {
	Root filesystem.RootContext `json:"root"`
}

type filesystemCapabilities struct {
	Read     bool `json:"read"`
	Range    bool `json:"range"`
	Stream   bool `json:"stream"`
	Download bool `json:"download"`
}

type filesystemStatResponse struct {
	ConnectionInstanceID string                 `json:"connectionInstanceId"`
	RootRevision         string                 `json:"rootRevision"`
	Entry                filesystem.Entry       `json:"entry"`
	MimeType             string                 `json:"mimeType"`
	Encoding             string                 `json:"encoding"`
	Capabilities         filesystemCapabilities `json:"capabilities"`
	ConsistencyToken     string                 `json:"consistencyToken"`
}

type unavailableDefinitionCollectionResponse struct {
	ConfigSource      sshconfig.ConfigSource   `json:"configSource"`
	TmuxOptionsSource connectionoptions.Source `json:"tmuxOptionsSource"`
	Definitions       []sshconfig.Definition   `json:"definitions"`
}
