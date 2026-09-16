package ports

import (
	"context"
	"net/http"
)

// BrowserRuntime owns the Roaminal-scoped Chromium instance. The HTTP server
// supplies authentication and origin validation before handing the WebSocket
// upgrade to this port. Implementations must keep the debugging transport
// private and must not expose it through the HTTP server.
type BrowserRuntime interface {
	Available() bool
	Handle(context.Context, http.ResponseWriter, *http.Request, string)
	Shutdown(context.Context)
}
