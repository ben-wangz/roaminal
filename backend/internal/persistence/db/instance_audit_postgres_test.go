package db

import (
	"context"
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func TestPostgresInstanceRetirementDeadlinePreservesActiveData(t *testing.T) {
	s, _ := postgresStore(t)
	meta := instanceFixture(firstInstance)
	saveInstanceFixture(t, s, meta)
	payload := "last checkpoint"
	if err := s.SaveSnapshot(t.Context(), firstInstance, snapshotFixture(), []byte(payload)); err != nil {
		t.Fatal(err)
	}

	// The archive copy can read the active row, but the final delete must wait
	// for this lock. Cancelling retirement must roll the whole archive back.
	blocker := s.DB.Begin()
	if blocker.Error != nil {
		t.Fatal(blocker.Error)
	}
	defer blocker.Rollback()
	if err := blocker.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", meta.ID).Take(&instanceRow{}).Error; err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 250*time.Millisecond)
	defer cancel()
	if err := s.RetireConnectionInstance(ctx, firstInstance); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("blocked retirement did not observe its deadline: %v", err)
	}
	if err := s.DB.Where("id = ?", meta.ID).Take(&instanceAuditRow{}).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("cancelled retirement left an audit row: %v", err)
	}
	if _, err := s.LoadConnectionInstance(t.Context(), firstInstance); err != nil {
		t.Fatalf("cancelled retirement lost active metadata: %v", err)
	}
	if _, got, err := s.LoadSnapshot(t.Context(), firstInstance); err != nil || string(got) != payload {
		t.Fatalf("cancelled retirement changed the active checkpoint: %q, %v", got, err)
	}
	if err := blocker.Rollback().Error; err != nil {
		t.Fatal(err)
	}
	if err := s.RetireConnectionInstance(t.Context(), firstInstance); err != nil {
		t.Fatalf("retirement could not be retried: %v", err)
	}
	checkAudit(t, findAudit(t, s, firstInstance), meta.Title, &payload)
}
