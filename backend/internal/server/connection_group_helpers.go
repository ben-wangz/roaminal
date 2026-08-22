package server

import (
	"net/http"

	"github.com/ben-wangz/roaminal/backend/internal/persistence"
)

func writeLayoutConflict(w http.ResponseWriter, layout persistence.ConnectionInstanceLayout) {
	writeConnectionInstanceGroupError(w, http.StatusConflict, "connection instance layout changed", "layout", map[string]any{"layout": layout})
}

func writeConnectionInstanceGroupError(w http.ResponseWriter, status int, message string, fields ...any) {
	body := map[string]any{"error": message, "code": "connection_instance_group_invalid"}
	if status == http.StatusNotFound {
		body["code"] = "connection_instance_group_not_found"
	} else if status == http.StatusConflict {
		body["code"] = "connection_instance_group_not_empty"
		if message == "connection instance group limit reached (10)" {
			body["code"] = "connection_instance_group_full"
		} else if message == "connection instance layout changed" {
			body["code"] = "connection_instance_layout_conflict"
		}
	}
	for _, value := range fields {
		switch item := value.(type) {
		case string:
			if item != "" {
				body["field"] = item
			}
		case map[string]any:
			for key, nested := range item {
				body[key] = nested
			}
		}
	}
	writeJSON(w, status, body)
}

func removeGroupID(order []string, groupID string) []string {
	result := make([]string, 0, len(order)-1)
	for _, id := range order {
		if id != groupID {
			result = append(result, id)
		}
	}
	return result
}
