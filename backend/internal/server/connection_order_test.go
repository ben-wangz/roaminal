package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/auth"
	"github.com/ben-wangz/roaminal/backend/internal/config"
	"github.com/ben-wangz/roaminal/backend/internal/connection"
	"github.com/ben-wangz/roaminal/backend/internal/identity"
	"github.com/ben-wangz/roaminal/backend/internal/persistence"
	"github.com/ben-wangz/roaminal/backend/internal/workspace"
)

func TestConnectionInstanceOrderRouteIsNotTreatedAsAnInstanceID(t *testing.T) {
	store, err := persistence.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	authManager, err := newServerTestAuth(config.Config{Password: "secret", AuthAccessTTL: time.Minute, AuthRefreshTTL: time.Hour}, store)
	if err != nil {
		t.Fatal(err)
	}
	server := New(Dependencies{Config: config.Config{}, Version: "0.1.0", BootID: "boot", Auth: authManager, Workspace: workspace.New(persistence.NewRepositories(store).Workspace)})
	request := httptest.NewRequest(http.MethodPut, "http://roaminal.test/api/v2/connection-instances/order", nil)
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

func TestNormalizeConnectionInstanceLayoutKeepsGroupsAndAppendsNewInstancesToUngrouped(t *testing.T) {
	saved := persistence.ConnectionInstanceLayout{
		Revision:   2,
		GroupOrder: []string{"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", persistence.UngroupedConnectionInstanceGroupID},
		Groups: []persistence.ConnectionInstanceGroup{{
			GroupID:               "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
			Name:                  "Production",
			ConnectionInstanceIDs: []string{"22222222-2222-4222-8222-222222222222", "44444444-4444-4444-8444-444444444444"},
		}},
		UngroupedConnectionInstanceIDs: []string{"11111111-1111-4111-8111-111111111111"},
	}
	instances := []connection.Summary{
		{ID: "11111111-1111-4111-8111-111111111111"},
		{ID: "22222222-2222-4222-8222-222222222222"},
		{ID: "33333333-3333-4333-8333-333333333333"},
	}
	got, changed := normalizeConnectionInstanceLayout(saved, true, nil, instances)
	if !changed {
		t.Fatal("layout was not normalized")
	}
	if want := []string{"22222222-2222-4222-8222-222222222222"}; !reflect.DeepEqual(got.Groups[0].ConnectionInstanceIDs, want) {
		t.Fatalf("group members = %v, want %v", got.Groups[0].ConnectionInstanceIDs, want)
	}
	if want := []string{"11111111-1111-4111-8111-111111111111", "33333333-3333-4333-8333-333333333333"}; !reflect.DeepEqual(got.UngroupedConnectionInstanceIDs, want) {
		t.Fatalf("ungrouped = %v, want %v", got.UngroupedConnectionInstanceIDs, want)
	}
}

func TestConnectionInstanceGroupRoutesPersistAndProtectRevision(t *testing.T) {
	store, err := persistence.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Password: "secret", AuthAccessTTL: time.Minute, AuthRefreshTTL: time.Hour}
	authManager, err := newServerTestAuth(cfg, store)
	if err != nil {
		t.Fatal(err)
	}
	challenge, err := authManager.Challenge()
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := authManager.Login(challenge.ChallengeID, auth.Proof(cfg.Password, challenge), "test")
	if err != nil {
		t.Fatal(err)
	}
	server := New(Dependencies{Config: cfg, Version: "0.1.0", BootID: "boot", Auth: authManager, Workspace: workspace.New(persistence.NewRepositories(store).Workspace), IDs: identity.UUIDGenerator{}})
	request := httptest.NewRequest(http.MethodGet, "http://roaminal.test/api/v2/connection-instance-groups", nil)
	request.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", response.Code, response.Body.String())
	}
	var listed struct {
		Layout persistence.ConnectionInstanceLayout `json:"layout"`
	}
	if err := json.NewDecoder(response.Body).Decode(&listed); err != nil {
		t.Fatal(err)
	}
	if listed.Layout.Revision != 1 || !reflect.DeepEqual(listed.Layout.GroupOrder, []string{persistence.UngroupedConnectionInstanceGroupID}) {
		t.Fatalf("initial layout = %#v", listed.Layout)
	}
	request = httptest.NewRequest(http.MethodPost, "http://roaminal.test/api/v2/connection-instance-groups", bytes.NewReader([]byte(`{"name":"Production","revision":1}`)))
	request.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", response.Code, response.Body.String())
	}
	var created struct {
		Layout persistence.ConnectionInstanceLayout `json:"layout"`
	}
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if len(created.Layout.Groups) != 1 || created.Layout.Revision != 2 {
		t.Fatalf("created layout = %#v", created.Layout)
	}
	request = httptest.NewRequest(http.MethodPost, "http://roaminal.test/api/v2/connection-instance-groups", bytes.NewReader([]byte(`{"name":"Stale","revision":1}`)))
	request.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("stale create status = %d, body = %s", response.Code, response.Body.String())
	}
}
