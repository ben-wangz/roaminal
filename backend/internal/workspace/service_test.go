package workspace

import (
	"reflect"
	"testing"

	"github.com/ben-wangz/roaminal/backend/internal/persistence"
)

func TestServiceOwnsLayoutAndFlatOrderProjection(t *testing.T) {
	store, err := persistence.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repositories := persistence.NewRepositories(store)
	service := New(repositories.Workspace)
	layout := persistence.ConnectionInstanceLayout{
		Revision:   1,
		GroupOrder: []string{"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", persistence.UngroupedConnectionInstanceGroupID},
		Groups: []persistence.ConnectionInstanceGroup{{
			GroupID:               "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
			Name:                  "Production",
			ConnectionInstanceIDs: []string{"11111111-1111-4111-8111-111111111111"},
		}},
		UngroupedConnectionInstanceIDs: []string{"22222222-2222-4222-8222-222222222222"},
	}
	if err := service.SetConnectionInstanceLayout("auth-session", layout, 0); err != nil {
		t.Fatal(err)
	}
	got, ok := service.ConnectionInstanceLayout("auth-session")
	if !ok || !reflect.DeepEqual(got, layout) {
		t.Fatalf("layout = %#v, exists = %v", got, ok)
	}
	if got := service.ConnectionInstanceOrder("auth-session"); !reflect.DeepEqual(got, []string{"11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"}) {
		t.Fatalf("order = %v", got)
	}
	if err := service.SetConnectionInstanceOrder("auth-session", []string{"22222222-2222-4222-8222-222222222222", "11111111-1111-4111-8111-111111111111"}); err != nil {
		t.Fatal(err)
	}
	if got := service.ConnectionInstanceOrder("auth-session"); !reflect.DeepEqual(got, []string{"22222222-2222-4222-8222-222222222222", "11111111-1111-4111-8111-111111111111"}) {
		t.Fatalf("updated order = %v", got)
	}
}
