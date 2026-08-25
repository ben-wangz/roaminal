package server

import (
	"net/http"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
)

func writeLayoutConflict(w http.ResponseWriter, layout domain.ConnectionInstanceLayout) {
	writeConnectionInstanceGroupError(w, http.StatusConflict, "connection instance layout changed", "layout", connectionInstanceLayoutResponse{Layout: layout})
}

func writeConnectionInstanceGroupError(w http.ResponseWriter, status int, message string, fields ...any) {
	code := "connection_instance_group_invalid"
	if status == http.StatusNotFound {
		code = "connection_instance_group_not_found"
	} else if status == http.StatusConflict {
		code = "connection_instance_group_not_empty"
		if message == "connection instance group limit reached (10)" {
			code = "connection_instance_group_full"
		} else if message == "connection instance layout changed" {
			code = "connection_instance_layout_conflict"
		}
	}
	field := ""
	var details any
	for _, value := range fields {
		switch item := value.(type) {
		case string:
			if item != "" {
				field = item
			}
		default:
			details = item
		}
	}
	writeCodedError(w, status, message, code, details, field)
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
