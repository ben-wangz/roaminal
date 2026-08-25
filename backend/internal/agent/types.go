package agent

import (
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

type Endpoint struct {
	Key     string `json:"-"`
	User    string `json:"user,omitempty"`
	Host    string `json:"host,omitempty"`
	Port    int    `json:"port,omitempty"`
	Display string `json:"display,omitempty"`
}

type DetailsResponse struct {
	Agent           ports.AgentSummary `json:"agent"`
	Endpoint        *Endpoint          `json:"endpoint,omitempty"`
	WebhookURL      string             `json:"webhookUrl,omitempty"`
	ComponentSHA256 string             `json:"componentSha256,omitempty"`
}

type Target struct {
	EndpointKey string
	SessionName string
}

type EndpointRecord = domain.AgentEndpointRecord
type TargetState = domain.AgentTargetState

type Initialization struct {
	ID                   string     `json:"initializationId"`
	ConnectionInstanceID string     `json:"connectionInstanceId,omitempty"`
	Endpoint             Endpoint   `json:"endpoint,omitempty"`
	TmuxSessionName      string     `json:"tmuxSessionName,omitempty"`
	WebhookURL           string     `json:"webhookUrl,omitempty"`
	Status               string     `json:"status"`
	Result               string     `json:"result,omitempty"`
	Component            string     `json:"component,omitempty"`
	Changed              bool       `json:"changed,omitempty"`
	Joined               bool       `json:"joined,omitempty"`
	PriorComponent       string     `json:"-"`
	Error                *SafeError `json:"error,omitempty"`
	StartedAt            time.Time  `json:"startedAt"`
	FinishedAt           *time.Time `json:"finishedAt,omitempty"`
}

type SafeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
