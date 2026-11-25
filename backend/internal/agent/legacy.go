package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
)

// DecodeLegacyEndpoints validates the file-era projection without opening an
// Agent service or changing the checkpoint. Duplicate object keys are invalid.
func DecodeLegacyEndpoints(data []byte) (map[string]domain.AgentEndpointRecord, error) {
	var file struct {
		FormatVersion int             `json:"formatVersion"`
		Endpoints     json.RawMessage `json:"endpoints"`
	}
	invalid := errors.New("invalid legacy Agent endpoint repository")
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&file); err != nil || file.FormatVersion != agentStoreSchemaVersion || dec.Decode(new(any)) != io.EOF {
		return nil, invalid
	}
	dec = json.NewDecoder(bytes.NewReader(file.Endpoints))
	dec.DisallowUnknownFields()
	if token, err := dec.Token(); err != nil || token != json.Delim('{') {
		return nil, invalid
	}
	out := make(map[string]domain.AgentEndpointRecord)
	for dec.More() {
		token, err := dec.Token()
		key, ok := token.(string)
		if err != nil || !ok {
			return nil, invalid
		}
		var record *domain.AgentEndpointRecord
		if _, duplicate := out[key]; duplicate || dec.Decode(&record) != nil || record == nil || !ValidateEndpointRecord(key, *record) {
			return nil, invalid
		}
		out[key] = *record
	}
	if token, err := dec.Token(); err != nil || token != json.Delim('}') || dec.Decode(new(any)) != io.EOF {
		return nil, invalid
	}
	return out, nil
}
