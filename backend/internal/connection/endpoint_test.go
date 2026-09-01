package connection

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDisplayEndpointIsSafeAndCached(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ssh-projection")
	script := "#!/bin/sh\nprintf 'user coder\\nhostname host.example\\nport 2200\\n'\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	alias := "remote"
	manager := &Manager{sshPath: path}
	endpoint := manager.displayEndpoint(&alias, "ssh")
	if endpoint == nil || endpoint.User != "coder" || endpoint.Host != "host.example" || endpoint.Port != 2200 {
		t.Fatalf("display endpoint = %+v", endpoint)
	}
	if cached := manager.displayEndpoint(&alias, "ssh"); cached == nil || cached == endpoint || *cached != *endpoint {
		t.Fatalf("cached display endpoint = %+v", cached)
	}
	if local := manager.displayEndpoint(&alias, "local"); local != nil {
		t.Fatalf("local endpoint should be omitted: %+v", local)
	}
}

func TestDisplayEndpointOmitsIncompleteProjection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ssh-projection")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nprintf 'user coder\\n'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	alias := "remote"
	manager := &Manager{sshPath: path}
	if endpoint := manager.displayEndpoint(&alias, "ssh"); endpoint != nil {
		t.Fatalf("incomplete endpoint should be omitted: %+v", endpoint)
	}
}
