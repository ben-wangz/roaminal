package domain

import (
	"errors"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
var sequencePattern = regexp.MustCompile(`^(0|[1-9][0-9]*)$`)

// ValidateConnectionInstanceOrder validates the legacy flat representation
// used by the application boundary while grouped layouts are being reconciled.
func ValidateConnectionInstanceOrder(order []string) error {
	if len(order) > 256 {
		return errors.New("connection instance order exceeds maximum size")
	}
	seen := make(map[string]struct{}, len(order))
	for _, value := range order {
		if !uuidPattern.MatchString(value) {
			return errors.New("invalid connection instance order id")
		}
		if _, ok := seen[value]; ok {
			return errors.New("duplicate connection instance order id")
		}
		seen[value] = struct{}{}
	}
	return nil
}

// ValidateConnectionInstanceLayout validates the user-owned sidebar
// aggregate. Ungrouped is implicit in the group collection but explicit in
// GroupOrder so order reconciliation remains deterministic.
func ValidateConnectionInstanceLayout(layout *ConnectionInstanceLayout) error {
	if layout == nil {
		return nil
	}
	if len(layout.Groups) > 256 || len(layout.GroupOrder) > 257 {
		return errors.New("connection instance layout exceeds maximum size")
	}
	groupIDs := make(map[string]struct{}, len(layout.Groups))
	groupNames := make(map[string]struct{}, len(layout.Groups))
	for _, group := range layout.Groups {
		if !uuidPattern.MatchString(group.GroupID) {
			return errors.New("invalid connection instance group id")
		}
		if _, ok := groupIDs[group.GroupID]; ok {
			return errors.New("duplicate connection instance group id")
		}
		groupIDs[group.GroupID] = struct{}{}
		if err := validateGroupName(group.Name); err != nil {
			return err
		}
		nameKey := strings.ToLower(group.Name)
		if _, ok := groupNames[nameKey]; ok {
			return errors.New("duplicate connection instance group name")
		}
		groupNames[nameKey] = struct{}{}
		if len(group.ConnectionInstanceIDs) > 10 {
			return errors.New("connection instance group exceeds maximum size")
		}
	}
	seenOrder := make(map[string]struct{}, len(layout.GroupOrder))
	ungrouped := false
	for _, groupID := range layout.GroupOrder {
		if groupID == UngroupedConnectionInstanceGroupID {
			if ungrouped {
				return errors.New("duplicate ungrouped group order entry")
			}
			ungrouped = true
		} else if _, ok := groupIDs[groupID]; !ok {
			return errors.New("group order references unknown group")
		}
		if _, ok := seenOrder[groupID]; ok {
			return errors.New("duplicate group order entry")
		}
		seenOrder[groupID] = struct{}{}
	}
	if !ungrouped || len(seenOrder) != len(groupIDs)+1 {
		return errors.New("group order must contain every group and ungrouped")
	}
	instances := make(map[string]struct{})
	for _, group := range layout.Groups {
		for _, id := range group.ConnectionInstanceIDs {
			if !uuidPattern.MatchString(id) {
				return errors.New("invalid connection instance group member id")
			}
			if _, ok := instances[id]; ok {
				return errors.New("duplicate connection instance group member id")
			}
			instances[id] = struct{}{}
		}
	}
	for _, id := range layout.UngroupedConnectionInstanceIDs {
		if !uuidPattern.MatchString(id) {
			return errors.New("invalid ungrouped connection instance id")
		}
		if _, ok := instances[id]; ok {
			return errors.New("duplicate connection instance group member id")
		}
		instances[id] = struct{}{}
	}
	if len(instances) > 256 {
		return errors.New("connection instance layout contains too many instances")
	}
	return nil
}

func validateGroupName(value string) error {
	if value == "" || value != strings.TrimSpace(value) || strings.EqualFold(value, "ungrouped") || !utf8.ValidString(value) {
		return errors.New("invalid connection instance group name")
	}
	if len([]rune(value)) > 64 {
		return errors.New("connection instance group name exceeds 64 characters")
	}
	for _, r := range value {
		if unicode.IsControl(r) || r == 0x7f || (r >= 0x202a && r <= 0x202e) || (r >= 0x2066 && r <= 0x2069) {
			return errors.New("connection instance group name contains a prohibited control character")
		}
	}
	return nil
}

func ValidateTitleOverride(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return errors.New("title must not be empty")
	}
	if !utf8.ValidString(value) || len([]byte(value)) > 512 {
		return errors.New("title exceeds size limit or contains invalid UTF-8")
	}
	if len([]rune(value)) > 128 {
		return errors.New("title exceeds 128 characters")
	}
	for _, r := range value {
		if unicode.IsControl(r) || r == 0x7f || (r >= 0x202a && r <= 0x202e) || (r >= 0x2066 && r <= 0x2069) {
			return errors.New("title contains a prohibited control character")
		}
	}
	return nil
}

func ValidateConnectionInstanceMeta(meta ConnectionInstanceMeta) error {
	if !uuidPattern.MatchString(meta.ID) {
		return errors.New("invalid connection instance id")
	}
	if !utf8.ValidString(meta.EffectiveTitle()) || len([]byte(meta.EffectiveTitle())) > 512 || !utf8.ValidString(meta.AutomaticTitle) || len([]byte(meta.AutomaticTitle)) > 512 || !utf8.ValidString(meta.InitialCwd) || !utf8.ValidString(meta.Cwd) || !utf8.ValidString(meta.GenerationStatus) || len([]byte(meta.GenerationStatus)) > 64 || !utf8.ValidString(meta.GenerationError) || len([]byte(meta.GenerationError)) > 512 {
		return errors.New("invalid connection instance text")
	}
	if meta.TitleOverride != nil {
		if err := ValidateTitleOverride(*meta.TitleOverride); err != nil {
			return err
		}
	}
	if !filepath.IsAbs(meta.InitialCwd) || !filepath.IsAbs(meta.Cwd) || len([]byte(meta.InitialCwd)) > 4096 || len([]byte(meta.Cwd)) > 4096 {
		return errors.New("invalid connection instance cwd")
	}
	if meta.Cols < 2 || meta.Cols > 1000 || meta.Rows < 1 || meta.Rows > 1000 || meta.CreatedAt.IsZero() || meta.UpdatedAt.IsZero() || meta.UpdatedAt.Before(meta.CreatedAt) {
		return errors.New("invalid connection instance dimensions or timestamp")
	}
	if meta.TmuxPrefixKey != "" && (len(meta.TmuxPrefixKey) != 1 || meta.TmuxPrefixKey[0] < 'a' || meta.TmuxPrefixKey[0] > 'z') {
		return errors.New("invalid tmux prefix key")
	}
	if meta.TmuxPrefixSource != "" && meta.TmuxPrefixSource != "runtime" && meta.TmuxPrefixSource != "fallback" && meta.TmuxPrefixSource != "unsupported" {
		return errors.New("invalid tmux prefix source")
	}
	if (meta.TmuxPrefixSource == "runtime" || meta.TmuxPrefixSource == "fallback") && meta.TmuxPrefixKey == "" {
		return errors.New("tmux prefix source requires a key")
	}
	if meta.TmuxPrefixSource == "unsupported" && meta.TmuxPrefixKey != "" {
		return errors.New("unsupported tmux prefix cannot have a key")
	}
	if !meta.TmuxEnabled && (meta.TmuxPrefixKey != "" || meta.TmuxPrefixSource != "") {
		return errors.New("non-tmux connection has tmux prefix metadata")
	}
	return nil
}

func ValidateSnapshotHeader(header SnapshotHeader) error {
	if header.Cols < 2 || header.Cols > 1000 || header.Rows < 1 || header.Rows > 1000 || header.ScrollbackLines < 0 || header.ScrollbackLines > 50000 || !sequencePattern.MatchString(header.ThroughSequence) {
		return errors.New("invalid snapshot header")
	}
	return nil
}
