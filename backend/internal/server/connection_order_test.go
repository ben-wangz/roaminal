package server

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/auth"
	"github.com/ben-wangz/roaminal/backend/internal/config"
	"github.com/ben-wangz/roaminal/backend/internal/connection"
	"github.com/ben-wangz/roaminal/backend/internal/persistence"
)

func TestConnectionInstanceOrderRouteIsNotTreatedAsAnInstanceID(t *testing.T) {
	store, err := persistence.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	authManager, err := auth.New(config.Config{Password: "secret", AuthAccessTTL: time.Minute, AuthRefreshTTL: time.Hour}, store)
	if err != nil {
		t.Fatal(err)
	}
	server := NewWithStatic(config.Config{}, "0.1.0", "boot", authManager, (*connection.Manager)(nil), nil, nil, http.NotFoundHandler())
	request := httptest.NewRequest(http.MethodPut, "http://roaminal.test/api/connection-instances/order", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; route was likely treated as an instance ID", response.Code, http.StatusUnauthorized)
	}
}

func TestConnectionInstanceOrderPrefersSavedIDsAndKeepsUnseenInstances(t *testing.T) {
	instances := []connection.Summary{{ID: "first"}, {ID: "second"}, {ID: "third"}}
	ordered := orderConnectionInstances(instances, []string{"third", "first", "retired"})
	got := make([]string, 0, len(ordered))
	for _, instance := range ordered {
		got = append(got, instance.ID)
	}
	if want := []string{"third", "first", "second"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

func TestNormalizeConnectionInstanceOrderDropsRetiredIDsAndAppendsNewInstances(t *testing.T) {
	first := "11111111-1111-4111-8111-111111111111"
	second := "22222222-2222-4222-8222-222222222222"
	third := "33333333-3333-4333-8333-333333333333"
	retired := "44444444-4444-4444-8444-444444444444"
	order, err := normalizeConnectionInstanceOrder([]string{second, retired}, []connection.Summary{{ID: first}, {ID: second}, {ID: third}})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{second, first, third}; !reflect.DeepEqual(order, want) {
		t.Fatalf("normalized order = %v, want %v", order, want)
	}
}
