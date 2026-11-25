package db

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"

	"github.com/ben-wangz/roaminal/backend/internal/agent"
	"github.com/ben-wangz/roaminal/backend/internal/domain"
)

const agentEndpointVersion = 1

var errAgentRecord = errors.New("invalid Agent endpoint repository record")

type agentEndpointRow struct {
	Key           string `gorm:"primaryKey;size:43"`
	FormatVersion int    `gorm:"not null"`
	Payload       []byte `gorm:"not null"`
}

func (agentEndpointRow) TableName() string { return "agent_endpoints" }

func encodeAgentEndpoint(key string, record domain.AgentEndpointRecord) (agentEndpointRow, error) {
	if !agent.ValidateEndpointRecord(key, record) {
		return agentEndpointRow{}, errAgentRecord
	}
	data, err := json.Marshal(record)
	if err != nil {
		return agentEndpointRow{}, errAgentRecord
	}
	return agentEndpointRow{Key: key, FormatVersion: agentEndpointVersion, Payload: data}, nil
}

func (row agentEndpointRow) record() (domain.AgentEndpointRecord, error) {
	if row.FormatVersion != agentEndpointVersion {
		return domain.AgentEndpointRecord{}, errAgentRecord
	}
	decoder := json.NewDecoder(bytes.NewReader(row.Payload))
	decoder.DisallowUnknownFields()
	var value *domain.AgentEndpointRecord
	if err := decoder.Decode(&value); err != nil || value == nil || !agent.ValidateEndpointRecord(row.Key, *value) {
		return domain.AgentEndpointRecord{}, errAgentRecord
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return domain.AgentEndpointRecord{}, errAgentRecord
	}
	return *value, nil
}
