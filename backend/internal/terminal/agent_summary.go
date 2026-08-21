package terminal

type AgentSummary struct {
	AgentType        string `json:"agentType"`
	Support          string `json:"support"`
	SupportReason    string `json:"supportReason"`
	Component        string `json:"component"`
	ComponentVersion string `json:"componentVersion"`
	Activity         string `json:"activity"`
	ActivityLabel    string `json:"activityLabel"`
	LastEventName    string `json:"lastEventName"`
	LastEventAt      string `json:"lastEventAt"`
	InitializationID string `json:"initializationId"`
	ErrorCode        string `json:"errorCode"`
	ErrorMessage     string `json:"errorMessage"`
}
