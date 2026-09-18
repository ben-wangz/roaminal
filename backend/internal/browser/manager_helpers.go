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

func rawStringValue(value any) string {
	result, _ := value.(string)
	return strings.TrimSpace(result)
}

func integerRaw(value json.RawMessage) (int, bool) {
	var result int
	if len(value) == 0 || json.Unmarshal(value, &result) != nil {
		return 0, false
	}
	return result, true
}

func pageOperationRaw(value json.RawMessage) (int64, bool) {
	if len(value) == 0 {
		return 0, false
	}
	var result int64
	if json.Unmarshal(value, &result) != nil || result < 0 {
		return 0, false
	}
	return result, true
}

func pageOperationMatches(command map[string]json.RawMessage, expected int64) bool {
	if len(command["pageOperation"]) == 0 {
		return true
	}
	value, ok := pageOperationRaw(command["pageOperation"])
	return ok && value == expected
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
