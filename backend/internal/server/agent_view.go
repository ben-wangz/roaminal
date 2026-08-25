package server

import (
	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

func agentConnectionInstanceView(summary ports.ConnectionInstanceSummary) ports.ConnectionInstanceView {
	return ports.ConnectionInstanceView{
		ID: summary.ID, ConnectionInstanceID: summary.ConnectionInstanceID,
		ConnectionDefinitionID: summary.ConnectionDefinitionID, Purpose: summary.Purpose,
		Type: summary.Type, Lifecycle: summary.Lifecycle, SourceState: summary.SourceState,
		SourceHostAlias: summary.SourceHostAlias, TmuxEnabled: summary.TmuxEnabled,
		TmuxSessionName: summary.TmuxSessionName,
	}
}
