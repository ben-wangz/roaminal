package agent

import (
	"context"

	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

// ConnectionService is the narrow remote capability needed by Agent
// provisioning and state projection. The agent feature does not depend on the
// concrete connection-instance manager or terminal runtime.
type ConnectionService interface {
	ConnectionInstanceViews() []ports.ConnectionInstanceView
	RemoteTransferInfo(string) (ports.RemoteTransferInfo, error)
	RunRemote(context.Context, string, ports.RemoteCommand) (ports.RemoteResult, error)
	ResolveEndpoint(context.Context, string) (ports.EffectiveEndpoint, error)
}

// ProvisioningService and ProjectionService are the two public capabilities
// exposed by the agent feature. The concrete Service composes both while
// callers depend only on the capability they need.
type ProvisioningService interface {
	Details(context.Context, ports.ConnectionInstanceView) DetailsResponse
	StartInitialization(context.Context, string) (Initialization, error)
	GetInitialization(string) (Initialization, bool)
}

type ProjectionService interface {
	Summary(ports.ConnectionInstanceView) ports.AgentSummary
}
