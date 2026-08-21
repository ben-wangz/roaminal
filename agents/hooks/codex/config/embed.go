package config

import _ "embed"

// HooksJSON is the canonical user-level Codex hook template shipped with the component.
//
//go:embed hooks.json
var HooksJSON []byte
