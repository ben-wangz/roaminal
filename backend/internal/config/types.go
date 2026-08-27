package config

import "time"

const (
	DefaultHost                            = "127.0.0.1"
	DefaultPort                            = 9846
	DefaultWebsocketPing                   = 10 * time.Second
	DefaultScrollback                      = 1000
	DefaultMaxConnectionInstances          = 32
	DefaultMaxClientsPerConnectionInstance = 8
	DefaultInitialCwd                      = "/workspace"
	DefaultAuthAccessTTL                   = 15 * time.Minute
	DefaultAuthRefreshTTL                  = 2160 * time.Hour
	DefaultAuthMaxAttempts                 = 30
	DefaultAgentHooksDir                   = "/opt/roaminal/agents/hooks"
	DefaultShutdownDeadline                = 10 * time.Second
	DefaultWorkerHandshake                 = 5 * time.Second
	DefaultWorkerControl                   = 30 * time.Second
	DefaultWorkerStall                     = 10 * time.Second
)

type Config struct {
	Host                            string
	Port                            int
	Password                        string
	WebsocketPingInterval           time.Duration
	ScrollbackLines                 int
	MaxConnectionInstances          int
	MaxClientsPerConnectionInstance int
	Debug                           bool
	AcceptTerms                     bool
	InitialCwd                      string
	AuthAccessTTL                   time.Duration
	AuthRefreshTTL                  time.Duration
	AuthMaxAttempts                 int
	ClientDiagnosticsEnabled        bool
	StateDir                        string
	WorkerPath                      string
	FrontendDir                     string
	Version                         string
	PasswordGenerated               bool
	AgentWebhookBaseURL             string
	AgentAllowInsecureWebhook       bool
	AgentHooksDir                   string
	WebPushVAPIDPublicKey           string
	WebPushVAPIDPrivateKey          string
	WebPushSubject                  string
}

type fileConfig struct {
	Host                            *string `json:"host"`
	Port                            *int    `json:"port"`
	Password                        *string `json:"password"`
	WebsocketPingInterval           *string `json:"websocketPingInterval"`
	ScrollbackLines                 *int    `json:"scrollbackLines"`
	MaxConnectionInstances          *int    `json:"maxConnectionInstances"`
	MaxClientsPerConnectionInstance *int    `json:"maxClientsPerConnectionInstance"`
	Debug                           *bool   `json:"debug"`
	AcceptTerms                     *bool   `json:"acceptTerms"`
	InitialCwd                      *string `json:"initialCwd"`
	AuthAccessTTL                   *string `json:"authAccessTTL"`
	AuthRefreshTTL                  *string `json:"authRefreshTTL"`
	AuthMaxAttempts                 *int    `json:"authMaxAttempts"`
	ClientDiagnosticsEnabled        *bool   `json:"clientDiagnosticsEnabled"`
	FrontendDir                     *string `json:"frontendDir"`
	AgentWebhookBaseURL             *string `json:"agentWebhookBaseUrl"`
	AgentAllowInsecureWebhook       *bool   `json:"agentAllowInsecureWebhook"`
	AgentHooksDir                   *string `json:"agentHooksDir"`
	WebPushVAPIDPublicKey           *string `json:"webPushVapidPublicKey"`
	WebPushVAPIDPrivateKey          *string `json:"webPushVapidPrivateKey"`
	WebPushSubject                  *string `json:"webPushSubject"`
}

var allowedFileKeys = map[string]bool{
	"host": true, "port": true, "password": true, "websocketPingInterval": true,
	"scrollbackLines": true, "maxConnectionInstances": true, "maxClientsPerConnectionInstance": true,
	"debug": true, "acceptTerms": true, "initialCwd": true, "authAccessTTL": true,
	"authRefreshTTL": true, "authMaxAttempts": true, "clientDiagnosticsEnabled": true, "frontendDir": true,
	"agentWebhookBaseUrl": true, "agentAllowInsecureWebhook": true, "agentHooksDir": true,
	"webPushVapidPublicKey": true, "webPushVapidPrivateKey": true, "webPushSubject": true,
}
