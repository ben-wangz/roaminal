package agent

type localInstallRequest struct {
	SchemaVersion    int    `json:"schemaVersion"`
	ComponentVersion string `json:"componentVersion"`
	ComponentSHA256  string `json:"componentSha256"`
}

type remoteProbe struct {
	Configured       bool   `json:"configured"`
	Provider         string `json:"provider"`
	ComponentVersion string `json:"componentVersion"`
	ComponentSHA256  string `json:"componentSha256"`
	HooksConfigured  bool   `json:"hooksConfigured"`
}

type remotePlatformInfo struct {
	OS    string
	Arch  string
	Tmux  bool
	Codex bool
}
