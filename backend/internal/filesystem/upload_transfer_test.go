package filesystem

import "testing"

func TestScpRemoteSpecDoesNotAddShellQuotes(t *testing.T) {
	if got := scpRemoteSpec("fixture", "/tmp/with spaces/file.txt"); got != "fixture:/tmp/with spaces/file.txt" {
		t.Fatalf("scp destination = %q", got)
	}
}

func TestRsyncRemoteSpecRetainsShellQuotes(t *testing.T) {
	if got := remoteSpec("fixture", "/tmp/with spaces/file.txt"); got != "fixture:'/tmp/with spaces/file.txt'" {
		t.Fatalf("rsync destination = %q", got)
	}
}
