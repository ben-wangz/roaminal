package agent

import (
	"testing"

	"github.com/ben-wangz/roaminal/backend/internal/connection"
)

func TestNormalizeEndpointIsStable(t *testing.T) {
	first, err := NormalizeEndpoint(connection.EffectiveEndpoint{User: "Dev", Host: "[2001:DB8::1]", Port: 22})
	if err != nil {
		t.Fatal(err)
	}
	second, err := NormalizeEndpoint(connection.EffectiveEndpoint{User: "Dev", Host: "2001:db8::1", Port: 22})
	if err != nil {
		t.Fatal(err)
	}
	if first.Key != second.Key || first.Display != "Dev@[2001:db8::1]:22" {
		t.Fatalf("unexpected endpoint normalization: %+v %+v", first, second)
	}
}

func TestNormalizeEndpointKeepsUserCase(t *testing.T) {
	first, err := NormalizeEndpoint(connection.EffectiveEndpoint{User: "Dev", Host: "host", Port: 22})
	if err != nil {
		t.Fatal(err)
	}
	second, err := NormalizeEndpoint(connection.EffectiveEndpoint{User: "dev", Host: "host", Port: 22})
	if err != nil {
		t.Fatal(err)
	}
	if first.Key == second.Key {
		t.Fatal("expected user case to change endpoint key")
	}
}
