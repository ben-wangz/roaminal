package processenv

import (
	"reflect"
	"testing"
)

func TestFilterPreservesUserEnvironmentWithoutServiceConfiguration(t *testing.T) {
	base := []string{
		"HOME=/home/roaminal", "PATH=/usr/bin", "LANG=C.UTF-8",
		"ROAMINAL_PASSWORD=secret", "ROAMINAL_DB_POSTGRES_DSN=postgres://secret",
		"ROAMINAL_WEB_PUSH_VAPID_PRIVATE_KEY=secret", "ROAMINAL_CUSTOM_SECRET=secret",
		"OTHER_ROAMINAL_PASSWORD=user-value", "MALFORMED",
	}
	want := []string{"HOME=/home/roaminal", "PATH=/usr/bin", "LANG=C.UTF-8", "OTHER_ROAMINAL_PASSWORD=user-value", "MALFORMED"}
	if got := Filter(base); !reflect.DeepEqual(got, want) {
		t.Fatalf("filtered environment = %v, want %v", got, want)
	}
}
