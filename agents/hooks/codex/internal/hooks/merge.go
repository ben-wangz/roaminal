package hooks

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	hookconfig "github.com/ben-wangz/roaminal/agents/hooks/codex/config"
)

const Command = `"$HOME/.roaminal/bin/roaminal-agent-hook" hook`

func Merge(data []byte) ([]byte, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("parse hooks config: %w", err)
	}
	if root == nil {
		return nil, errors.New("hooks root must be an object")
	}
	var events map[string]json.RawMessage
	if raw, ok := root["hooks"]; ok {
		if err := json.Unmarshal(raw, &events); err != nil || events == nil {
			return nil, errors.New("hooks must be an object")
		}
	} else {
		events = map[string]json.RawMessage{}
	}
	var template map[string]json.RawMessage
	if err := json.Unmarshal(hookconfig.HooksJSON, &template); err != nil {
		return nil, err
	}
	var wanted map[string]json.RawMessage
	if err := json.Unmarshal(template["hooks"], &wanted); err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(wanted))
	for key := range wanted {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, event := range keys {
		groups, err := readGroups(events[event])
		if err != nil {
			return nil, fmt.Errorf("%s: %w", event, err)
		}
		groups = removeCanonical(groups)
		groups = append(groups, canonicalGroup(event == "SessionEnd"))
		encoded, err := json.Marshal(groups)
		if err != nil {
			return nil, err
		}
		events[event] = encoded
	}
	encodedEvents, err := json.Marshal(events)
	if err != nil {
		return nil, err
	}
	root["hooks"] = encodedEvents
	result, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(result, '\n'), nil
}

func readGroups(raw json.RawMessage) ([]map[string]json.RawMessage, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var groups []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &groups); err != nil {
		return nil, errors.New("event must be an array")
	}
	return groups, nil
}

func canonicalGroup(sessionEnd bool) map[string]json.RawMessage {
	handler := map[string]any{"type": "command", "command": Command, "timeout": 5}
	if sessionEnd {
		handler["timeout"] = 3
	}
	encoded, _ := json.Marshal([]any{handler})
	return map[string]json.RawMessage{"hooks": encoded}
}

func removeCanonical(groups []map[string]json.RawMessage) []map[string]json.RawMessage {
	result := make([]map[string]json.RawMessage, 0, len(groups))
	for _, group := range groups {
		var handlers []map[string]json.RawMessage
		if err := json.Unmarshal(group["hooks"], &handlers); err != nil {
			result = append(result, group)
			continue
		}
		kept := handlers[:0]
		for _, handler := range handlers {
			var typ, command string
			_ = json.Unmarshal(handler["type"], &typ)
			_ = json.Unmarshal(handler["command"], &command)
			if typ == "command" && command == Command {
				continue
			}
			kept = append(kept, handler)
		}
		if len(kept) == 0 {
			continue
		}
		encoded, _ := json.Marshal(kept)
		group["hooks"] = encoded
		result = append(result, group)
	}
	return result
}

func InstallHooks(home string) error {
	path := filepath.Join(home, ".codex", "hooks.json")
	info, statErr := os.Lstat(path)
	hadFile := statErr == nil
	var data []byte
	if os.IsNotExist(statErr) {
		data = []byte(`{"hooks":{}}`)
	} else if statErr != nil {
		return statErr
	} else {
		if !info.Mode().IsRegular() {
			return errors.New("hooks file permissions are unsafe")
		}
		if info.Mode().Perm() != 0600 {
			if chmodErr := os.Chmod(path, 0600); chmodErr != nil {
				return chmodErr
			}
			info, statErr = os.Lstat(path)
			if statErr != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
				return errors.New("hooks file permissions are unsafe")
			}
		}
		data, statErr = os.ReadFile(path)
		if statErr != nil {
			return statErr
		}
	}
	merged, err := Merge(data)
	if err != nil {
		return err
	}
	if string(data) == string(merged) && (!hadFile || info.Mode().Perm() == 0600) {
		return nil
	}
	if _, statErr := os.Stat(path + ".roaminal.bak"); statErr != nil && !os.IsNotExist(statErr) {
		return statErr
	} else if os.IsNotExist(statErr) && hadFile {
		if backupErr := atomicBackup(path+".roaminal.bak", data); backupErr != nil {
			return backupErr
		}
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".hooks.json-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(merged); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}

func Configured(home string) bool {
	path := filepath.Join(home, ".codex", "hooks.json")
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&0o077 != 0 {
		return false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	merged, err := Merge(data)
	return err == nil && string(data) == string(merged)
}

func atomicBackup(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".hooks-backup-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Link(tmpName, path); err != nil {
		if os.IsExist(err) {
			return nil
		}
		return err
	}
	return syncDirectory(filepath.Dir(path))
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
