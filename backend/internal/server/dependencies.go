package server

import "errors"

// Validate checks the mandatory composition-root capabilities before the HTTP
// listener is opened. SSH sources and client diagnostics remain optional and
// report their own unavailable state at the feature boundary.
func (d Dependencies) Validate() error {
	missing := ""
	switch {
	case d.Auth == nil:
		missing = "auth"
	case d.Workspace == nil:
		missing = "workspace"
	case d.Connections == nil:
		missing = "connections"
	case d.Monitor == nil:
		missing = "monitor"
	case d.Worker == nil:
		missing = "worker"
	case d.Static == nil:
		missing = "static"
	case d.Version == "":
		missing = "version"
	case d.BootID == "":
		missing = "boot id"
	case d.IDs == nil:
		missing = "id generator"
	}
	if missing != "" {
		return errors.New("missing server dependency: " + missing)
	}
	return nil
}
