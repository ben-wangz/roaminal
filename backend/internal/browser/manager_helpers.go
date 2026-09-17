package browser

import (
	"encoding/json"
	"strings"
)

func normalizeViewport(size viewportSize) viewportSize {
	return viewportSize{
		Width:  maxInt(minViewportWidth, minInt(maxViewportWidth, size.Width)),
		Height: maxInt(minViewportHeight, minInt(maxViewportHeight, size.Height)),
	}
}

func rawString(value json.RawMessage) string {
	var result string
	if json.Unmarshal(value, &result) != nil {
		return ""
	}
	return strings.TrimSpace(result)
}

func integerRaw(value json.RawMessage) (int, bool) {
	var result int
	if len(value) == 0 || json.Unmarshal(value, &result) != nil {
		return 0, false
	}
	return result, true
}

func integerField(value any) (int, bool) {
	number, ok := value.(float64)
	if !ok || number != float64(int(number)) {
		return 0, false
	}
	return int(number), true
}

func optionalBool(value json.RawMessage) (bool, bool) {
	if len(value) == 0 {
		return false, false
	}
	var result bool
	if json.Unmarshal(value, &result) != nil {
		return false, false
	}
	return result, true
}

func cloneMap(value map[string]any) map[string]any {
	copyValue := make(map[string]any, len(value))
	for key, item := range value {
		copyValue[key] = item
	}
	return copyValue
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}
