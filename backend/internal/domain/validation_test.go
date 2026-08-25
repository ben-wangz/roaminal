package domain

import "testing"

const (
	firstInstanceID  = "11111111-1111-4111-8111-111111111111"
	secondInstanceID = "22222222-2222-4222-8222-222222222222"
	groupID          = "33333333-3333-4333-8333-333333333333"
)

func TestValidateConnectionInstanceLayoutRequiresCompleteUniqueOrder(t *testing.T) {
	layout := ConnectionInstanceLayout{
		Revision:                       1,
		GroupOrder:                     []string{UngroupedConnectionInstanceGroupID, groupID},
		Groups:                         []ConnectionInstanceGroup{{GroupID: groupID, Name: "Project", ConnectionInstanceIDs: []string{firstInstanceID}}},
		UngroupedConnectionInstanceIDs: []string{secondInstanceID},
	}
	if err := ValidateConnectionInstanceLayout(&layout); err != nil {
		t.Fatal(err)
	}
	layout.GroupOrder = []string{groupID, groupID, UngroupedConnectionInstanceGroupID}
	if err := ValidateConnectionInstanceLayout(&layout); err == nil {
		t.Fatal("expected duplicate group order to be rejected")
	}
}

func TestValidateConnectionInstanceLayoutEnforcesGroupLimit(t *testing.T) {
	ids := make([]string, 0, 11)
	for index := 0; index < 11; index++ {
		ids = append(ids, "11111111-1111-4111-8111-"+string([]byte{'1' + byte(index/10), '1' + byte(index%10), '1', '1', '1', '1', '1', '1', '1', '1', '1', '1'}))
	}
	layout := ConnectionInstanceLayout{
		Revision:   1,
		GroupOrder: []string{UngroupedConnectionInstanceGroupID, groupID},
		Groups:     []ConnectionInstanceGroup{{GroupID: groupID, Name: "Project", ConnectionInstanceIDs: ids}},
	}
	if err := ValidateConnectionInstanceLayout(&layout); err == nil {
		t.Fatal("expected group limit to be rejected")
	}
}

func TestValidateSnapshotHeaderRequiresMonotonicSequence(t *testing.T) {
	header := SnapshotHeader{Cols: 80, Rows: 24, ScrollbackLines: 100, ThroughSequence: "01"}
	if err := ValidateSnapshotHeader(header); err == nil {
		t.Fatal("expected non-canonical sequence to be rejected")
	}
	header.ThroughSequence = "12"
	if err := ValidateSnapshotHeader(header); err != nil {
		t.Fatal(err)
	}
}
