package main

import "strings"

func installErrorCode(err error) string {
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	switch message {
	case "invalid install request", "install input too large":
		return "invalid_install_request"
	case "invalid component checksum":
		return "invalid_component_checksum"
	case "invalid replacement token":
		return "invalid_replacement_token"
	case "binding_changed":
		return "binding_changed"
	case "endpoint_conflict":
		return "endpoint_conflict"
	case "component downgrade":
		return "component_downgrade"
	case "private directory permissions are unsafe":
		return "private_directory_unsafe"
	case "installed binary permissions are unsafe":
		return "installed_binary_unsafe"
	case "installation lock timeout":
		return "installation_lock_timeout"
	case "hooks file permissions are unsafe":
		return "hooks_file_unsafe"
	case "hooks must be an object", "hooks root must be an object":
		return "hooks_config_invalid"
	case "component checksum mismatch":
		return "component_checksum_mismatch"
	case "replacement token is required":
		return "replacement_token_missing"
	}
	if strings.Contains(message, "permission denied") || strings.Contains(message, "operation not permitted") {
		return "filesystem_permission_denied"
	}
	return "install_failed"
}
