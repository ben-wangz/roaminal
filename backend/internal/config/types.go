package config

import "time"

const (
	DefaultHost                             = "127.0.0.1"
	DefaultPort                             = 9846
	DefaultWebsocketPing                    = 10 * time.Second
	DefaultScrollback                       = 1000
	DefaultMaxConnectionInstances           = 32
	DefaultMaxClientsPerConnectionInstance  = 8
	DefaultInitialCwd                       = "/workspace"
	DefaultAuthAccessTTL                    = 15 * time.Minute
	DefaultAuthRefreshTTL                   = 2160 * time.Hour
	DefaultAuthMaxAttempts                  = 30
	DefaultAgentHooksDir                    = "/opt/roaminal/agents/hooks"
	DefaultFilesystemImagePreviewCacheDir   = "/var/cache/roaminal/filesystem-image-previews"
	DefaultFilesystemImagePreviewCacheMiB   = 128
	DefaultFilesystemImagePreviewMaxAge     = 24 * time.Hour
	DefaultFilesystemImagePreviewCleanup    = 10 * time.Minute
	DefaultFilesystemImagePreviewConvert    = 1
	DefaultFilesystemImagePreviewSourceMiB  = 32
	DefaultFilesystemImagePreviewOutputMiB  = 16
	DefaultFilesystemImagePreviewPixels     = uint64(100000000)
	DefaultFilesystemImagePreviewFrames     = 200
	DefaultFilesystemImagePreviewAnimPixels = uint64(200000000)
	DefaultFilesystemImagePreviewTimeout    = 30 * time.Second
	DefaultShutdownDeadline                 = 10 * time.Second
	DefaultWorkerHandshake                  = 5 * time.Second
	DefaultWorkerControl                    = 30 * time.Second
	DefaultWorkerStall                      = 10 * time.Second
)

type Config struct {
	Host                                       string
	Port                                       int
	Password                                   string
	WebsocketPingInterval                      time.Duration
	ScrollbackLines                            int
	MaxConnectionInstances                     int
	MaxClientsPerConnectionInstance            int
	Debug                                      bool
	AcceptTerms                                bool
	InitialCwd                                 string
	AuthAccessTTL                              time.Duration
	AuthRefreshTTL                             time.Duration
	AuthMaxAttempts                            int
	ClientDiagnosticsEnabled                   bool
	StateDir                                   string
	WorkerPath                                 string
	FrontendDir                                string
	Version                                    string
	PasswordGenerated                          bool
	AgentHooksDir                              string
	WebPushVAPIDPublicKey                      string
	WebPushVAPIDPrivateKey                     string
	WebPushSubject                             string
	FilesystemImagePreviewCacheDir             string
	FilesystemImagePreviewCacheTargetMiB       int
	FilesystemImagePreviewCacheMaxAge          time.Duration
	FilesystemImagePreviewCacheCleanupInterval time.Duration
	FilesystemImagePreviewMaxConversions       int
	FilesystemImagePreviewMaxSourceMiB         int
	FilesystemImagePreviewMaxOutputMiB         int
	FilesystemImagePreviewMaxStaticPixels      uint64
	FilesystemImagePreviewMaxFrames            int
	FilesystemImagePreviewMaxAnimatedPixels    uint64
	FilesystemImagePreviewConversionTimeout    time.Duration
}

type fileConfig struct {
	Host                                       *string `json:"host"`
	Port                                       *int    `json:"port"`
	Password                                   *string `json:"password"`
	WebsocketPingInterval                      *string `json:"websocketPingInterval"`
	ScrollbackLines                            *int    `json:"scrollbackLines"`
	MaxConnectionInstances                     *int    `json:"maxConnectionInstances"`
	MaxClientsPerConnectionInstance            *int    `json:"maxClientsPerConnectionInstance"`
	Debug                                      *bool   `json:"debug"`
	AcceptTerms                                *bool   `json:"acceptTerms"`
	InitialCwd                                 *string `json:"initialCwd"`
	AuthAccessTTL                              *string `json:"authAccessTTL"`
	AuthRefreshTTL                             *string `json:"authRefreshTTL"`
	AuthMaxAttempts                            *int    `json:"authMaxAttempts"`
	ClientDiagnosticsEnabled                   *bool   `json:"clientDiagnosticsEnabled"`
	FrontendDir                                *string `json:"frontendDir"`
	AgentHooksDir                              *string `json:"agentHooksDir"`
	WebPushVAPIDPublicKey                      *string `json:"webPushVapidPublicKey"`
	WebPushVAPIDPrivateKey                     *string `json:"webPushVapidPrivateKey"`
	WebPushSubject                             *string `json:"webPushSubject"`
	FilesystemImagePreviewCacheDir             *string `json:"filesystemImagePreviewCacheDir"`
	FilesystemImagePreviewCacheTargetMiB       *int    `json:"filesystemImagePreviewCacheTargetMiB"`
	FilesystemImagePreviewCacheMaxAge          *string `json:"filesystemImagePreviewCacheMaxAge"`
	FilesystemImagePreviewCacheCleanupInterval *string `json:"filesystemImagePreviewCacheCleanupInterval"`
	FilesystemImagePreviewMaxConversions       *int    `json:"filesystemImagePreviewMaxConversions"`
	FilesystemImagePreviewMaxSourceMiB         *int    `json:"filesystemImagePreviewMaxSourceMiB"`
	FilesystemImagePreviewMaxOutputMiB         *int    `json:"filesystemImagePreviewMaxOutputMiB"`
	FilesystemImagePreviewMaxStaticPixels      *uint64 `json:"filesystemImagePreviewMaxStaticPixels"`
	FilesystemImagePreviewMaxFrames            *int    `json:"filesystemImagePreviewMaxFrames"`
	FilesystemImagePreviewMaxAnimatedPixels    *uint64 `json:"filesystemImagePreviewMaxAnimatedPixels"`
	FilesystemImagePreviewConversionTimeout    *string `json:"filesystemImagePreviewConversionTimeout"`
}

var allowedFileKeys = map[string]bool{
	"host": true, "port": true, "password": true, "websocketPingInterval": true,
	"scrollbackLines": true, "maxConnectionInstances": true, "maxClientsPerConnectionInstance": true,
	"debug": true, "acceptTerms": true, "initialCwd": true, "authAccessTTL": true,
	"authRefreshTTL": true, "authMaxAttempts": true, "clientDiagnosticsEnabled": true, "frontendDir": true,
	"agentHooksDir":         true,
	"webPushVapidPublicKey": true, "webPushVapidPrivateKey": true, "webPushSubject": true,
	"filesystemImagePreviewCacheDir": true, "filesystemImagePreviewCacheTargetMiB": true,
	"filesystemImagePreviewCacheMaxAge": true, "filesystemImagePreviewCacheCleanupInterval": true,
	"filesystemImagePreviewMaxConversions": true, "filesystemImagePreviewMaxSourceMiB": true,
	"filesystemImagePreviewMaxOutputMiB": true, "filesystemImagePreviewMaxStaticPixels": true,
	"filesystemImagePreviewMaxFrames": true, "filesystemImagePreviewMaxAnimatedPixels": true,
	"filesystemImagePreviewConversionTimeout": true,
}
