package server

import "testing"

func TestDependenciesValidateReportsFirstMissingRequiredCapability(t *testing.T) {
	if err := (Dependencies{}).Validate(); err == nil || err.Error() != "missing server dependency: auth" {
		t.Fatalf("unexpected validation error: %v", err)
	}
}
