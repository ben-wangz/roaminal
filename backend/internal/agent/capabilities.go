package agent

import (
	"context"

	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

type provisioningCapability struct{ service *Service }
type telemetryCapability struct{ service *Service }

func (s *Service) Provisioning() ProvisioningService { return provisioningCapability{service: s} }
func (s *Service) Telemetry() TelemetryService       { return telemetryCapability{service: s} }

func (c provisioningCapability) Details(ctx context.Context, view ports.ConnectionInstanceView, origin string) DetailsResponse {
	return c.service.Details(ctx, view, origin)
}

func (c provisioningCapability) StartInitialization(ctx context.Context, id, origin string) (Initialization, error) {
	return c.service.StartInitialization(ctx, id, origin)
}

func (c provisioningCapability) GetInitialization(id string) (Initialization, bool) {
	return c.service.GetInitialization(id)
}

func (c telemetryCapability) Summary(view ports.ConnectionInstanceView) ports.AgentSummary {
	return c.service.Summary(view)
}

func (c telemetryCapability) AcceptEvent(token string, body []byte) (bool, error) {
	return c.service.AcceptEvent(token, body)
}

var _ ProvisioningService = provisioningCapability{}
var _ TelemetryService = telemetryCapability{}
