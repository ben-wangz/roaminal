package agent

import (
	"context"

	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

type provisioningCapability struct{ service *Service }
type projectionCapability struct{ service *Service }

func (s *Service) Provisioning() ProvisioningService { return provisioningCapability{service: s} }
func (s *Service) Projection() ProjectionService     { return projectionCapability{service: s} }

func (c provisioningCapability) Details(ctx context.Context, view ports.ConnectionInstanceView) DetailsResponse {
	return c.service.Details(ctx, view)
}

func (c provisioningCapability) StartInitialization(ctx context.Context, id string) (Initialization, error) {
	return c.service.StartInitialization(ctx, id)
}

func (c provisioningCapability) GetInitialization(id string) (Initialization, bool) {
	return c.service.GetInitialization(id)
}

func (c projectionCapability) Summary(view ports.ConnectionInstanceView) ports.AgentSummary {
	return c.service.Summary(view)
}

var _ ProvisioningService = provisioningCapability{}
var _ ProjectionService = projectionCapability{}
