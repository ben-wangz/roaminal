package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func trimSpool(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	type spoolEntry struct {
		name      string
		size      int64
		important bool
	}
	items := make([]spoolEntry, 0, len(entries))
	var total int64
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		info, statErr := entry.Info()
		if statErr != nil {
			continue
		}
		important := false
		if data, readErr := os.ReadFile(filepath.Join(dir, entry.Name())); readErr == nil {
			var header struct {
				EventName string `json:"eventName"`
			}
			if json.Unmarshal(data, &header) == nil {
				important = header.EventName == "Stop" || header.EventName == "SessionEnd"
			}
		}
		items = append(items, spoolEntry{name: entry.Name(), size: info.Size(), important: important})
		total += info.Size()
	}
	sort.Slice(items, func(left, right int) bool { return items[left].name < items[right].name })
	for len(items) > 256 || total > 2*1024*1024 {
		if len(items) == 0 {
			break
		}
		victim := 0
		for index := 0; index < len(items); index++ {
			if items[index].important {
				continue
			}
			victim = index
			break
		}
		if err := os.Remove(filepath.Join(dir, items[victim].name)); err != nil {
			return err
		}
		total -= items[victim].size
		items = append(items[:victim], items[victim+1:]...)
	}
	return nil
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
