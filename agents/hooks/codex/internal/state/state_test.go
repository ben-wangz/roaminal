package state

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/ben-wangz/roaminal/agents/hooks/codex/internal/model"
	"github.com/ben-wangz/roaminal/agents/hooks/codex/internal/tmux"
)

func testTmuxInfo(name, sessionID string, created int64) tmux.Info {
	return tmux.Info{
		SessionName: name, SessionID: sessionID, SessionCreated: created,
		PaneID: "%0", SocketFingerprint: "0123456789abcdef",
	}
}

func TestUpdateTrimsHistoryAndAllocatesMonotonicIndexes(t *testing.T) {
	home := t.TempDir()
	info := testTmuxInfo("team", "$1", 10)
	for index := 1; index <= model.MaxStateRecords+2; index++ {
		file, err := Update(home, info, "1", model.StateRecord{
			Timestamp: time.Unix(int64(index), 0).UTC(), State: model.StateRunning,
			EventName: "turn",
		})
		if err != nil {
			t.Fatalf("Update(%d): %v", index, err)
		}
		if file.LatestIndex != uint64(index) {
			t.Fatalf("LatestIndex = %d, want %d", file.LatestIndex, index)
		}
	}

	file, err := Read(home, info)
	if err != nil {
		t.Fatal(err)
	}
	if len(file.Records) != model.MaxStateRecords {
		t.Fatalf("records = %d, want %d", len(file.Records), model.MaxStateRecords)
	}
	if file.Records[0].Index != 3 || file.Records[len(file.Records)-1].Index != 130 {
		t.Fatalf("unexpected retained indexes: first=%d last=%d", file.Records[0].Index, file.Records[len(file.Records)-1].Index)
	}
	if file.State != model.StateRunning || file.Records[len(file.Records)-1].State != file.State {
		t.Fatalf("latest state is inconsistent: %+v", file)
	}
}

func TestUpdateSerializesConcurrentEvents(t *testing.T) {
	home := t.TempDir()
	info := testTmuxInfo("team", "$1", 10)
	const eventCount = 24
	errs := make(chan error, eventCount)
	var wait sync.WaitGroup
	for index := 0; index < eventCount; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			_, err := Update(home, info, "1", model.StateRecord{
				Timestamp: time.Unix(int64(index+1), 0).UTC(), State: model.StateRunning,
			})
			errs <- err
		}(index)
	}
	wait.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	file, err := Read(home, info)
	if err != nil {
		t.Fatal(err)
	}
	if file.LatestIndex != eventCount || len(file.Records) != eventCount {
		t.Fatalf("concurrent state = latest %d records %d, want %d/%d", file.LatestIndex, len(file.Records), eventCount, eventCount)
	}
	indexes := make([]uint64, len(file.Records))
	for index, record := range file.Records {
		indexes[index] = record.Index
	}
	sort.Slice(indexes, func(left, right int) bool { return indexes[left] < indexes[right] })
	for index, value := range indexes {
		if value != uint64(index+1) {
			t.Fatalf("concurrent index %d = %d", index, value)
		}
	}
}

func TestRuntimeStateIsolatedByTmuxIdentity(t *testing.T) {
	home := t.TempDir()
	first := testTmuxInfo("team", "$1", 10)
	second := testTmuxInfo("team", "$2", 10)
	if _, err := Update(home, first, "1", model.StateRecord{State: model.StateRelax}); err != nil {
		t.Fatal(err)
	}
	if _, err := Update(home, second, "1", model.StateRecord{State: model.StateRunning}); err != nil {
		t.Fatal(err)
	}
	firstFile, err := Read(home, first)
	if err != nil {
		t.Fatal(err)
	}
	secondFile, err := Read(home, second)
	if err != nil {
		t.Fatal(err)
	}
	if firstFile.RuntimeID == secondFile.RuntimeID || firstFile.State == secondFile.State {
		t.Fatalf("runtime state was not isolated: first=%+v second=%+v", firstFile, secondFile)
	}
}

func TestUpdateIgnoresInterruptedTemporaryFile(t *testing.T) {
	home := t.TempDir()
	info := testTmuxInfo("team", "$1", 10)
	if _, err := Update(home, info, "1", model.StateRecord{State: model.StateRelax}); err != nil {
		t.Fatal(err)
	}
	statePath := FilePath(home, tmux.RuntimeID(info))
	if err := os.WriteFile(filepath.Join(filepath.Dir(statePath), ".state-interrupted"), []byte("partial"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Update(home, info, "1", model.StateRecord{State: model.StateRunning}); err != nil {
		t.Fatal(err)
	}
	file, err := Read(home, info)
	if err != nil {
		t.Fatal(err)
	}
	if file.LatestIndex != 2 || file.State != model.StateRunning {
		t.Fatalf("state was not recovered around temporary file: %+v", file)
	}
}

func TestStateFilesAndDirectoriesArePrivate(t *testing.T) {
	home := t.TempDir()
	info := testTmuxInfo("team", "$1", 10)
	if _, err := Update(home, info, "1", model.StateRecord{State: model.StateRelax}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		filepath.Join(home, ".roaminal"),
		filepath.Join(home, ".roaminal", "state"),
		filepath.Join(home, ".roaminal", "state", "agents"),
		filepath.Join(home, ".roaminal", "state", "agents", model.ProviderCodex),
		FilePath(home, tmux.RuntimeID(info)),
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		want := os.FileMode(0700)
		if !info.IsDir() {
			want = 0600
		}
		if info.Mode().Perm() != want {
			t.Fatalf("%s mode = %o, want %o", path, info.Mode().Perm(), want)
		}
	}
}

func TestReadRejectsRuntimeMismatchAndUnsafeState(t *testing.T) {
	home := t.TempDir()
	info := testTmuxInfo("team", "$1", 10)
	if _, err := Update(home, info, "1", model.StateRecord{State: model.StateRelax}); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(home, testTmuxInfo("other", "$1", 10)); !errors.Is(err, ErrRuntimeMismatch) {
		t.Fatalf("runtime mismatch error = %v", err)
	}
	path := FilePath(home, tmux.RuntimeID(info))
	if err := os.Chmod(path, 0640); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(home, info); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("unsafe state error = %v", err)
	}
}

func TestCleanupRemovesExpiredInactiveRuntime(t *testing.T) {
	home := t.TempDir()
	active := testTmuxInfo("team", "$1", 10)
	expired := testTmuxInfo("old", "$2", 11)
	if _, err := Update(home, active, "1", model.StateRecord{State: model.StateRelax}); err != nil {
		t.Fatal(err)
	}
	if _, err := Update(home, expired, "1", model.StateRecord{State: model.StateRelax}); err != nil {
		t.Fatal(err)
	}
	expiredPath := FilePath(home, tmux.RuntimeID(expired))
	old := time.Now().Add(-8 * 24 * time.Hour)
	if err := os.Chtimes(expiredPath, old, old); err != nil {
		t.Fatal(err)
	}
	if err := Cleanup(home, time.Now(), 7*24*time.Hour, tmux.RuntimeID(active)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(expiredPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expired state still exists, error = %v", err)
	}
	if _, err := os.Stat(FilePath(home, tmux.RuntimeID(active))); err != nil {
		t.Fatalf("active state was removed: %v", err)
	}
}
