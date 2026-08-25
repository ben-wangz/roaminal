package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ben-wangz/roaminal/backend/internal/api"
)

func TestWriteApplicationErrorUsesStableTypedContract(t *testing.T) {
	recorder := httptest.NewRecorder()
	value := api.NewApplicationError(api.ErrorConflict, http.StatusConflict, "layout revision conflict")
	value.Field = "revision"
	value.Details = map[string]string{"current": "4"}
	writeApplicationError(recorder, value)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d", recorder.Code)
	}
	var body api.ErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != string(api.ErrorConflict) || body.Field != "revision" || body.Details == nil {
		t.Fatalf("error body = %#v", body)
	}
}
